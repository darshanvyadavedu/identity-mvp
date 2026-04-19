# Schema Design: Approaches & Decision

This document compares three approaches for storing biometric check results in the identity verification backend and explains the recommended direction.

---

## The Problem

A verification session runs three sequential checks: **liveness**, **document scan**, and **face match**. Each check produces result data that needs to be stored. There are three ways to model this.

---

## Approach 1 — Current Architecture (9 Tables, Fully Normalized)

Each check type gets its own result table with a 1:1 unique FK back to `biometric_checks`.

```
verification_sessions
└── biometric_checks
    ├── liveness_results      (1:1 via check_id UNIQUE)
    ├── document_scan_results (1:1 via check_id UNIQUE)
    └── face_match_results    (1:1 via check_id UNIQUE)
identity_hashes
consent_records
verified_data
audit_logs
```

**`liveness_results`**
```sql
result_id, check_id (UNIQUE FK), verdict, confidence_score,
failure_reason, sdk_version, reference_image (TEXT), raw_response (JSONB)
```

**`document_scan_results`**
```sql
scan_id, check_id (UNIQUE FK), document_type, issuing_country,
id_number_hmac, extracted_fields (JSONB), raw_response (JSONB)
```

**`face_match_results`**
```sql
match_id, check_id (UNIQUE FK), confidence, threshold,
passed, source_a, source_b, raw_response (JSONB)
```

### Strengths
- **Full audit trail** — `raw_response` on each table stores the exact bytes returned by AWS/Azure. If a user disputes their verification, you have everything.
- **Schema enforces correctness** — `UNIQUE (check_id)` on each result table means the DB prevents duplicate results at the constraint level.
- **Independent evolution** — adding `sdk_version` to liveness or `mrz_data` to doc_scan doesn't touch the other tables.
- **Type-safe columns** — `confidence_score numeric(5,4)`, `reference_image text` (Postgres TOAST), `passed boolean` — each field has the right type and storage class.
- **Compliance-ready** — for an identity platform, regulators may ask for a full history of what each provider returned. It's already there.

### Weaknesses
- **3 extra repo files** — `LivenessResultRepo`, `DocumentScanRepo`, `FaceMatchRepo` — each with their own interface, implementation, and converter.
- **5 repos injected into `document_service`** — the service constructor is hard to read and test.
- **2–3 DB roundtrips per request** — get check, then get result. Simple but adds up.
- **Most result data is write-only** — `raw_response`, `failure_reason`, `sdk_version`, `threshold`, `source_a`, `source_b` are never read back operationally. They exist for audit only, but the same code that writes them also maintains the repo interfaces.

---

## Approach 2 — Consolidate into `biometric_checks` (Option A, 6 Tables)

Drop the 3 result tables. Add nullable typed columns directly to `biometric_checks`.

```
verification_sessions
└── biometric_checks  ← all result data lives here
identity_hashes
consent_records
verified_data
audit_logs
```

**`biometric_checks` (expanded)**
```sql
check_id, session_id, user_id, check_type, status, attempt_number, attempted_at,

-- liveness result (NULL for doc_scan and face_match rows)
verdict          varchar(50),
confidence_score numeric(5,4),
reference_image  text,

-- doc scan result (NULL for liveness and face_match rows)
extracted_fields jsonb,

-- face match result (NULL for liveness and doc_scan rows)
face_confidence  numeric(5,4),
face_passed      boolean,

created_at, updated_at
```

### Strengths
- **Simpler service layer** — `document_service` goes from 5 injected repos to 2 (`sessionRepo`, `checkRepo`).
- **Single DB query** — `GetBySessionAndType` returns everything needed, no second lookup.
- **Less code** — 3 repo files deleted, 3 repo interfaces removed from service structs.
- **Proper column types** — `reference_image text` stays as a first-class TEXT column (Postgres TOAST), not embedded in JSONB.

### Weaknesses
- **Sparse rows** — a liveness row has `NULL` for `extracted_fields`, `face_confidence`, `face_passed`. A doc_scan row has `NULL` for `verdict`, `confidence_score`, `reference_image`. This is honest (different check types have different data) but visually messy.
- **No audit trail** — `raw_response` and write-only audit fields are dropped entirely. If you need compliance later, this is a migration.
- **Schema changes when adding check types** — a new check type (e.g. address verification) means `ALTER TABLE biometric_checks ADD COLUMN ...` for its result fields.
- **DB no longer enforces uniqueness** — the `UNIQUE (check_id)` constraint that prevented duplicate results is gone. Logic moves to the application layer.

---

## Approach 3 — Polymorphic JSONB (`identity_type` + `identity_value`)

Single `result` JSONB column holds a type-specific payload. Each check type defines its own struct.

```sql
biometric_checks:
  check_id, session_id, user_id, check_type, status, attempt_number,
  result          jsonb,        -- type-specific payload
  reference_image text,         -- kept separate (see below)
  created_at, updated_at
```

**`result` payload per check type:**
```json
// liveness
{ "verdict": "live", "confidence": 0.95 }

// doc_scan
{ "firstName": "John", "lastName": "Doe", "dob": "1990-01-01",
  "idNumber": "X123456", "expiry": "2030-01-01", "issuingCountry": "US" }

// face_match
{ "confidence": 0.87, "passed": true }
```

`reference_image` is kept as a separate `text` column (not inside JSONB) because it is a 100–200 KB base64 data URL — embedding it in JSONB means Postgres must deserialize the entire blob on every row load, losing TOAST storage optimization.

### Strengths
- **Clean schema** — no nullable columns. Every row has `check_type` and `result`.
- **Adding new check types** requires no `ALTER TABLE` — define a new Go struct and write to `result`.
- **Go structs per type** — type-switch on `check_type`, unmarshal into the right struct. Explicit and type-safe at the application layer.

### Weaknesses
- **No column-level constraints** — `confidence` can't be `numeric(5,4)` inside JSONB; it's just a JSON number.
- **Harder to query in SQL** — filtering or sorting by `result->>'confidence'` requires JSONB operators; no direct column index.
- **`reference_image` is still a special case** — you end up with a mixed schema: most data in JSONB, image as a text column. This awkwardness reveals that the data isn't uniformly "one value per check".
- **Application-layer type safety** — nothing stops writing a `{ "wrongField": true }` to a liveness result. The DB won't catch it.

---

## Comparison Table

| Dimension | Current (9 tables) | Option A (6 tables) | Option B (JSONB) |
|---|---|---|---|
| Table count | 9 | 6 | 6 |
| Repos in document_service | 5 | 2 | 2 |
| DB queries per request | 2–3 | 1 | 1 |
| Audit trail (raw_response) | ✅ Full | ❌ Dropped | ❌ Dropped |
| Column-level type safety | ✅ | ✅ | ❌ |
| DB-enforced uniqueness | ✅ | ❌ | ❌ |
| Schema change for new check type | New table | ALTER TABLE | No change |
| reference_image storage | TEXT (TOAST) | TEXT (TOAST) | TEXT (TOAST, separate) |
| Compliance-ready | ✅ | ❌ (migration needed) | ❌ (migration needed) |
| Code simplicity | Low | High | Medium |
| Schema clarity | High | Medium (sparse rows) | High |

---

## Recommendation

**Keep the current architecture (9 tables) — simplify the Go layer instead.**

The schema is correct for the domain. Identity verification is a compliance-sensitive operation. `raw_response` is not optional luxury — it is the audit trail that proves what AWS or Azure returned for a given user at a given time. If a verification is disputed, or a regulator asks for records, you need it.

The pain point is not the schema. It is the **repo-per-table pattern** applied to tables that are always accessed together. The fix:

**Merge the 3 result repos into `BiometricCheckRepo`.**

Instead of:
```go
// document_service today: 5 repos
sessionRepo  VerificationSessionRepoInterface
checkRepo    BiometricCheckRepoInterface
livenessRepo LivenessResultRepoInterface   // ← always used with checkRepo
docScanRepo  DocumentScanRepoInterface     // ← always used with checkRepo
faceRepo     FaceMatchRepoInterface        // ← always used with checkRepo
```

Do:
```go
// document_service simplified: 2 repos
sessionRepo  VerificationSessionRepoInterface
checkRepo    BiometricCheckRepoInterface   // ← now owns all result operations
```

`BiometricCheckRepo` gains methods like `SaveLivenessResult`, `GetLivenessResult`, `SaveDocScanResult`, `GetDocScanResult`, `SaveFaceResult`. Internally these do the second lookup into the result tables — that detail is hidden from the service layer.

**What you get:**
- Services stay simple (2 repos instead of 5)
- Schema stays normalized and compliance-ready
- `raw_response` and audit fields are preserved at no extra cost to callers
- DB constraints still enforce uniqueness at the result level
- When compliance requirements arrive (and they will for an identity platform), nothing needs to change

**When to reconsider Option A:**
If after 6 months the result tables genuinely contain nothing but the operationally-used fields (i.e., you never query `raw_response`) and compliance is not a concern, consolidating is a reasonable cleanup. But that is a migration you do with data in hand — not a decision to make before launch.

---

## Decision Log

| Date | Decision | Reason |
|---|---|---|
| 2026-04-19 | Evaluated 3 schema approaches | Prompted by review of service layer complexity |
| 2026-04-19 | Recommend: keep current 9-table schema | Compliance audit trail, DB-enforced uniqueness, type-safe columns outweigh code overhead |
| 2026-04-19 | Action: merge 3 result repos into BiometricCheckRepo | Reduces service constructor from 5 repos to 2 without changing the schema |
