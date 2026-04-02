# Database Schema — Identity Verification Platform (Module 1)

> Cloud-Agnostic PostgreSQL · Polymorphic `biometric_checks` pattern · AES-256-GCM app-layer encryption · No PII in plain text

---

## Table of Contents

1. [The Big Picture](#the-big-picture)
2. [Entity Relationship Overview](#entity-relationship-overview)
3. [ENUMs](#enums)
4. [Table-by-Table Walkthrough](#table-by-table-walkthrough)
   - [users](#1-users--who-is-this-person)
   - [verification_sessions](#2-verification_sessions--what-verification-run-is-this)
   - [biometric_checks](#3-biometric_checks--the-polymorphic-parent)
   - [liveness_results](#4-liveness_results--what-happened-during-the-video-challenge)
   - [document_scan_results](#5-document_scan_results--what-did-the-id-document-say)
   - [face_match_results](#6-face_match_results--do-the-liveness-face-and-id-photo-match)
   - [consent_records](#7-consent_records--what-did-the-user-agree-to-share)
   - [verified_data](#8-verified_data--the-final-encrypted-pii-store)
   - [audit_logs](#9-audit_logs--everything-that-ever-happened)
   - [secret_refs](#10-secret_refs--where-api-keys-live)
5. [End-to-End Data Flow](#end-to-end-data-flow)
6. [Security Design Decisions](#security-design-decisions)
7. [Scalability Notes](#scalability-notes)
8. [DBML Source](#dbml-source)

---

## The Big Picture

Each table answers one question:

| Question | Table |
|----------|-------|
| Who is the user? | `users` |
| What verification run is this? | `verification_sessions` |
| Which checks are tracked? | `biometric_checks` (parent) |
| What did liveness detect? | `liveness_results` |
| What did the ID document contain? | `document_scan_results` |
| Did the faces match? | `face_match_results` |
| What did the user agree to store? | `consent_records` |
| What PII is actually stored? | `verified_data` |
| What happened overall? | `audit_logs` |
| Where are API secrets kept? | `secret_refs` |

---

## Entity Relationship Overview

```
users
  │
  ├──< verification_sessions
  │         │
  │         └──< biometric_checks  ◄─── parent (polymorphic)
  │                   │
  │          ┌────────┼────────────┐
  │          ▼        ▼            ▼
  │   liveness_  document_    face_match_
  │   results    scan_results  results
  │
  ├──< consent_records
  │
  ├──< verified_data
  │
  └──< audit_logs
```

**Relationship rules:**
- `users` `1 ──< many` `verification_sessions`
- `verification_sessions` `1 ──< many` `biometric_checks`
- `biometric_checks` `1 ──1` `liveness_results` *(one-to-one via UNIQUE check_id)*
- `biometric_checks` `1 ──1` `document_scan_results`
- `biometric_checks` `1 ──1` `face_match_results`
- `verification_sessions` `1 ──< many` `consent_records`
- `verification_sessions` `1 ──< many` `verified_data`
- `consent_records` `1 ──< many` `verified_data` *(via consent_id — links stored field to its authorising consent)*
- `users` `1 ──< many` `audit_logs`

---

## ENUMs

```sql
module_type_enum      → ID | ADDRESS | SALARY | PROFESSION
session_status_enum   → pending | in_progress | complete | failed
decision_status_enum  → pending | approved | rejected | manual_review
check_type_enum       → liveness | doc_scan | face_match
check_status_enum     → pending | passed | failed | skipped
document_type_enum    → passport | drivers_license | national_id
liveness_verdict_enum → live | spoofed | inconclusive | error | timeout
```

> `inconclusive` is distinct from `error`/`timeout` — it means the provider could not
> make a determination (e.g. poor lighting, head not fully visible). It is **retryable**
> and does **not** count as a spoof attempt.

> `decision_status` is separate from `session_status`. A session can be `complete` (all
> checks ran) but `manual_review` (a human must approve). `approved` / `rejected` /
> `manual_review` map directly to the decision engine output.

---

## Table-by-Table Walkthrough

---

### 1. `users` — Who is this person?

```sql
user_id         UUID          PK
custom_username TEXT          UNIQUE NOT NULL   -- plain-text public handle chosen by user
username_hash   TEXT          NOT NULL          -- Argon2id hash
password_hash   TEXT          NOT NULL          -- Argon2id hash, separate salt
email_hash      TEXT          NULLABLE          -- Argon2id hash; nullable if not provided
id_verified     BOOL          DEFAULT false
retry_count     INT           DEFAULT 0
locked_until    TIMESTAMPTZ   NULLABLE          -- set to now()+24h after 3 failed attempts
created_at      TIMESTAMPTZ
updated_at      TIMESTAMPTZ
```

**Why hashes and not plain text?**

```
User types:   john@email.com
We store:     $argon2id$v=19$m=65536...   ← mathematically irreversible

To check login:
  hash(what_user_typed) == stored_hash?  →  yes = authenticated
  The original email is never needed again after registration.
```

**What is NOT in this table:**
- Real name, address, date of birth → stored encrypted in `verified_data`
- Session data, verification results → separate tables, separate concerns

---

### 2. `verification_sessions` — What verification run is this?

One row = one complete verification attempt for one module.

```sql
session_id          UUID                  PK DEFAULT gen_random_uuid()
user_id             UUID                  NOT NULL FK → users
module_type         module_type_enum      NOT NULL
status              session_status_enum   NOT NULL DEFAULT 'pending'
decision_status     decision_status_enum  NOT NULL DEFAULT 'pending'
provider            TEXT                  -- 'azure' | 'aws' | 'gcp' | 'onfido' | 'stripe'
provider_session_id TEXT                  -- opaque vendor session ref
retry_count         INT                   NOT NULL DEFAULT 0
expires_at          TIMESTAMPTZ
created_at          TIMESTAMPTZ           NOT NULL DEFAULT now()
completed_at        TIMESTAMPTZ
```

**Why `decision_status` separate from `status`?**

```
status          = did the session run to completion? (process state)
decision_status = what was the outcome?             (business verdict)

A session can be:
  status=complete + decision_status=approved      → all checks passed
  status=complete + decision_status=manual_review → checks ran, human needed
  status=complete + decision_status=rejected      → hard failure (spoof detected)
  status=failed   + decision_status=pending       → session crashed before decision
```

**Why `provider` + `provider_session_id` instead of `azure_session_id`?**

```
BAD  (vendor lock-in):
  azure_session_id TEXT   ← column breaks if you switch to AWS Rekognition

GOOD (cloud-agnostic):
  provider            = "azure"
  provider_session_id = "az_sess_abc123"   ← same columns, any vendor
```

**A user can have multiple sessions:**

```
users
  └── session 1: module=ID,      status=failed    (blurry photo)
  └── session 2: module=ID,      status=complete  (success)
  └── session 3: module=ADDRESS, status=pending   (started later)
```

---

### 3. `biometric_checks` — The Polymorphic Parent

> This is the centrepiece of the design.

```sql
check_id        UUID                PK DEFAULT gen_random_uuid()
session_id      UUID                NOT NULL FK → verification_sessions
user_id         UUID                NOT NULL FK → users   (denormalized for fast user-level queries)
check_type      check_type_enum     NOT NULL
status          check_status_enum   NOT NULL DEFAULT 'pending'
provider        TEXT                -- can differ per check within the same session
provider_ref_id TEXT                -- vendor-side operation/job ID for this check
attempted_at    TIMESTAMPTZ
completed_at    TIMESTAMPTZ

UNIQUE (session_id, check_type)     -- enforced as a real DB index, not just a comment
```

**Why "polymorphic"?**

Each check type has a different result structure but shares common tracking fields.
The pattern works like this:

```
biometric_checks  (check_type = 'liveness')
    └──▶ liveness_results       { verdict, confidence, sdk_version }

biometric_checks  (check_type = 'doc_scan')
    └──▶ document_scan_results  { document_type, mrz_validated, extracted_fields }

biometric_checks  (check_type = 'face_match')
    └──▶ face_match_results     { confidence, threshold, passed, source_a, source_b }
```

**Check session progress with a single query:**

```sql
SELECT check_type, status
FROM   biometric_checks
WHERE  session_id = $1;

-- Returns:
-- liveness   | passed
-- doc_scan   | passed
-- face_match | pending   ← not done yet
```

**Why enforce `UNIQUE(session_id, check_type)` as a real index?**

```
Without it, you can accidentally INSERT two liveness rows for the same session.
The Note field in DBML is just documentation — it does NOT enforce anything.
The unique index is what actually prevents duplicate checks.
```

**Extending for Module 2 (Address) — zero schema changes to existing tables:**

```
Add ENUM value:  check_type = 'gps_check'
Add child table: gps_check_results
biometric_checks stays completely unchanged.
```

---

### 4. `liveness_results` — What happened during the video challenge?

```sql
result_id        UUID                  PK
check_id         UUID                  UNIQUE FK → biometric_checks   (1:1)
verdict          liveness_verdict      -- live | spoofed | inconclusive | error | timeout
confidence_score NUMERIC(5,4)          -- 0.0000 – 1.0000
failure_reason   TEXT                  -- 'spoof_detected' | 'timeout' | 'poor_lighting'
sdk_version      TEXT                  -- e.g. '1.23.4'
raw_response     JSONB                 -- full provider JSON response
created_at       TIMESTAMPTZ
```

**Why store `sdk_version`?**

```
If Azure releases SDK v2.0 with different scoring behaviour,
you can query: "did this user verify before or after the SDK update?"
Critical for compliance audits and debugging false positive spikes.
```

**Why `raw_response JSONB`?**

```
Azure returns rich metadata. Instead of adding columns every time
the provider adds a new field, the full JSON is preserved.
The app extracts what it needs; the rest is available for debugging.
```

**Why UNIQUE on `check_id`?**

```
Enforces exactly one liveness_results row per biometric_checks row.
Prevents duplicate results from being inserted for the same check.
```

---

### 5. `document_scan_results` — What did the ID document say?

```sql
scan_id          UUID                  PK
check_id         UUID                  UNIQUE FK → biometric_checks   (1:1)
document_type    document_type_enum    -- passport | drivers_license | national_id
issuing_country  CHAR(2)               -- ISO 3166-1 alpha-2 (US, IN, GB...)
id_number_hmac   TEXT                  -- HMAC-SHA256 only; never reversible
expiry_date      DATE
mrz_validated    BOOL                  DEFAULT false
extracted_fields JSONB                 -- { "first_name": "<AES-GCM>", "dob": "<AES-GCM>" }
raw_response     JSONB                 -- full OCR provider JSON
scanned_at       TIMESTAMPTZ
```

**Why `id_number_hmac` and not encrypted value?**

```
Encryption:  encrypt("AB123456") → ciphertext → can decrypt back to "AB123456"
HMAC:        hmac("AB123456")    → fingerprint → can NEVER be reversed

We only need to answer: "has this ID been used before?"
  hmac(new_id) == any existing hmac?  →  yes = duplicate detected

We never need to read the actual number back, so HMAC is the correct tool.
```

**Why `extracted_fields JSONB` instead of individual encrypted columns?**

```
Instead of:
  extracted_first_name  BYTEA   ← schema migration every time OCR adds a field
  extracted_last_name   BYTEA
  extracted_dob         BYTEA

We store:
  extracted_fields = {
    "first_name": "AES-GCM-ciphertext-base64",
    "dob":        "AES-GCM-ciphertext-base64",
    "address":    "AES-GCM-ciphertext-base64"
  }

Adding a new OCR field = no migration needed.
Provider-agnostic: Azure, AWS, GCP all return different field names —
map them to canonical names in the app before encrypting.
```

**What is MRZ (`mrz_validated`)?**

```
The bottom two lines of a passport:
  P<GBRSMITH<<JOHN<<<<<<<<<<<<<<<<<<<<<<<<<<<
  AB1234567GBR8001151M2801155<<<<<<<<<<<<<<<4

mrz_validated = true means the provider parsed and checksummed these
lines successfully. Strong signal that the document is genuine and
not a photocopy or digital forgery.
```

---

### 6. `face_match_results` — Do the liveness face and ID photo match?

```sql
match_id      UUID          PK
check_id      UUID          UNIQUE FK → biometric_checks   (1:1)
confidence    NUMERIC(5,4)  -- e.g. 0.9750
threshold     NUMERIC(5,4)  -- configurable; default 0.9000
passed        BOOL NOT NULL -- confidence >= threshold
source_a      TEXT          -- 'liveness_frame'
source_b      TEXT          -- 'id_document_photo'
raw_response  JSONB         -- score + verdict only; no images stored
checked_at    TIMESTAMPTZ
```

**Why `source_a` / `source_b` text fields instead of image URLs?**

```
We never store actual face images (privacy-correct).
But we record WHAT was compared:
  source_a = "liveness_frame"     → best frame captured from the video
  source_b = "id_document_photo"  → photo on the ID card

If audited: "how was this match done?" → answered without any biometric data.
```

**Why store `threshold` per row and not just as a global config?**

```
Today:       threshold = 0.90
Next month:  raised to 0.95 for high-risk users

If you only store the score, you cannot tell whether a historical
match would have passed under the new threshold.

Storing threshold per row = you know exactly what rule was in force
at the time of verification. Essential for audit trails.
```

---

### 7. `consent_records` — What did the user agree to share?

One row per field per consent decision.

```sql
consent_id     UUID          PK
user_id        UUID          FK → users
session_id     UUID          FK → verification_sessions
field_name     TEXT NOT NULL -- 'first_name' | 'dob' | 'doc_number' | 'address'
consented      BOOL NOT NULL -- true = agreed to store; false = declined
signed_payload TEXT          -- JWT-signed consent bundle
ip_address     INET NOT NULL
user_agent     TEXT
consented_at   TIMESTAMPTZ
```

**Example: user shown 5 fields, approves 3:**

```
consent_id | field_name  | consented | consented_at
-----------+-------------+-----------+-------------
uuid-1     | first_name  | true      | 2026-03-29
uuid-2     | dob         | true      | 2026-03-29
uuid-3     | doc_number  | false     | 2026-03-29   ← declined
uuid-4     | address     | true      | 2026-03-29
uuid-5     | nationality | false     | 2026-03-29   ← declined
```

**Why `signed_payload`?**

```
signed_payload = JWT signed by the backend private key:
{
  "user_id":   "...",
  "session_id":"...",
  "fields":    ["first_name", "dob", "address"],
  "consented": true,
  "ip":        "192.168.1.1",
  "timestamp": "2026-03-29T10:00:00Z"
}

If a user later claims "I never agreed to share my date of birth"
→ the cryptographically signed record proves otherwise.
This property is called non-repudiation.
```

**Why immutable (no UPDATE/DELETE)?**

```
GDPR right to erasure applies to personal DATA, not to consent RECORDS.
The consent record itself is a legal document.

To revoke:  set verified_data.revoked_at = now()
            (data becomes inaccessible, consent record remains)

RLS policy enforces this at the database level:
  CREATE POLICY consent_insert_only ON consent_records FOR INSERT WITH CHECK (true);
  -- No UPDATE or DELETE policy = those operations are denied by default
```

---

### 8. `verified_data` — The Final Encrypted PII Store

Only fields the user explicitly consented to end up here.

```sql
data_id             UUID          PK DEFAULT gen_random_uuid()
user_id             UUID          NOT NULL FK → users
session_id          UUID          NOT NULL FK → verification_sessions
consent_id          UUID          NOT NULL FK → consent_records  -- links field to the consent that authorised it
field_name          TEXT          NOT NULL  -- 'first_name' | 'last_name' | 'dob' | 'address' | ...
encrypted_value     TEXT          NOT NULL  -- AES-256-GCM ciphertext (base64-encoded)
encryption_iv       TEXT          NOT NULL  -- 12-byte random nonce (base64-encoded); unique per record
verification_method TEXT                    -- 'azure_doc_intelligence' | 'manual' | ...
verified_at         TIMESTAMPTZ   NOT NULL DEFAULT now()
expires_at          TIMESTAMPTZ             -- nullable; some fields require periodic re-verification
revoked_at          TIMESTAMPTZ             -- NULL = active; non-null = user withdrew consent
```

**Why separate `encryption_iv` from `encrypted_value`?**

```
AES-GCM encryption requires two inputs to decrypt:
  1. The ciphertext
  2. The nonce (IV) used during encryption

  encrypted_value = ciphertext   (what Azure found)
  encryption_iv   = nonce        (random 12 bytes generated at encryption time)

Storing them separately makes the contract explicit.
Both are required; neither is useful alone.
```

**Why `consent_id` FK on `verified_data`?**

```
Without consent_id, you can't answer: "under what consent was this field stored?"

With consent_id, you can:
  verified_data.consent_id → consent_records row
  → signed_payload (JWT proving user agreed)
  → consented_at, ip_address, user_agent (full audit context)

Required for GDPR Article 7 compliance — you must be able to demonstrate
the lawful basis for processing each piece of personal data.
```

**The two-stage flow from OCR → verified_data:**

```
Stage 1 — OCR output (temporary, shown on consent screen):
  document_scan_results.extracted_fields = {
    "first_name": "<encrypted>",
    "dob":        "<encrypted>",
    "doc_number": "<encrypted>"
  }

Stage 2 — User consents to store first_name and dob only:
  verified_data row: field_name="first_name", consent_id=uuid-1, encrypted_value=..., encryption_iv=...
  verified_data row: field_name="dob",        consent_id=uuid-2, encrypted_value=..., encryption_iv=...
  ← doc_number was declined; it stays only in document_scan_results, no verified_data row created
```

---

### 9. `audit_logs` — Everything That Ever Happened

```sql
log_id      UUID          PK
user_id     UUID          FK → users (nullable; some events have no user context)
action      TEXT NOT NULL -- see action values below
entity_type TEXT          -- which table was affected
entity_id   UUID          -- which row was affected
ip_address  INET
metadata    JSONB         -- contextual data per action type
created_at  TIMESTAMPTZ
```

**Action values:**

| Action | When Written | Example `metadata` |
|--------|-------------|-------------------|
| `register` | User account created | `{ "method": "email" }` |
| `liveness` | Liveness check completed | `{ "verdict": "live", "score": 0.97, "sdk": "1.2" }` |
| `upload` | Document uploaded | `{ "document_type": "passport", "country": "GB" }` |
| `facematch` | Face comparison run | `{ "confidence": 0.97, "passed": true }` |
| `consent` | Consent screen submitted | `{ "accepted": ["name","dob"], "declined": ["address"] }` |
| `revoke` | User withdrew consent | `{ "field": "dob" }` |
| `delete` | User requested data deletion | `{ "fields_deleted": 3 }` |
| `lock` | Account locked after max retries | `{ "retry_count": 3, "locked_until": "..." }` |

**Why `metadata JSONB` instead of typed columns?**

```
Each action has completely different relevant context.
JSONB lets each action store exactly what matters
without adding columns for every possible combination.
```

---

### 10. `secret_refs` — Where API Keys Live (if no cloud Key Vault)

```sql
ref_id          UUID          PK
key_name        TEXT          UNIQUE NOT NULL   -- 'AZURE_FACE_API_KEY'
encrypted_value TEXT NOT NULL                  -- encrypted secret value
provider        TEXT                           -- 'doppler' | 'infisical' | 'db' | 'azure_kv'
environment     TEXT                           -- 'development' | 'staging' | 'production'
last_rotated_at TIMESTAMPTZ
created_at      TIMESTAMPTZ
```

**When to use this table:**

```
Have Azure Key Vault?     → Skip this table. Use Key Vault directly.
On Railway / Render?      → Use this table OR Doppler / Infisical.
Local development?        → Use .env file. Never commit to git.
```

---

## End-to-End Data Flow

```
[1] Register
    POST /v1/auth/register
      → users (INSERT: username_hash, password_hash)
      → audit_logs (action='register')

[2] Start ID Verification
    POST /v1/liveness/sessions
      → verification_sessions (INSERT: module_type=ID, status=pending)
      → biometric_checks x3 (INSERT: liveness/doc_scan/face_match, status=pending)

[3] Liveness Check  ← Azure SDK ↔ Azure directly (never hits our server)
    GET /v1/liveness/sessions/:id/verdict
      → biometric_checks (UPDATE: liveness → status=passed)
      → liveness_results (INSERT: verdict=live, confidence=0.97)
      → audit_logs (action='liveness')

[4] Document Upload + OCR
    POST /v1/documents/upload
      → Azure Blob Storage (upload image)
      → Azure Document Intelligence (OCR job)
      → biometric_checks (UPDATE: doc_scan → status=passed)
      → document_scan_results (INSERT: extracted_fields={encrypted JSON})
      → audit_logs (action='upload')

[5] Face Match
    POST /v1/verification/face-match
      → Azure Face API (compare liveness frame vs ID photo)
      → biometric_checks (UPDATE: face_match → status=passed)
      → face_match_results (INSERT: confidence=0.97, passed=true)
      → audit_logs (action='facematch')

[6] All 3 checks passed
      → verification_sessions (UPDATE: status=complete)
      → users (UPDATE: id_verified=true)

[7] Consent Screen
    GET  /v1/consent/prepare  → decrypt document_scan_results.extracted_fields → show to user
    POST /v1/consent/submit   → user selects fields to keep
      → consent_records (INSERT: 1 row per field, immutable)
      → verified_data   (INSERT: 1 row per CONSENTED field, encrypted)
      → Azure Blob Storage (DELETE: raw ID image — no longer needed)
      → audit_logs (action='consent')
```

---

## Security Design Decisions

| Concern | Decision | Reason |
|---------|----------|--------|
| Password storage | Argon2id (m=65536, t=3, p=4) | Memory-hard; resistant to GPU brute-force |
| Email storage | Argon2id hash only | Original never needed after registration |
| ID document number | HMAC-SHA256 | Only need uniqueness check, not reversibility |
| Extracted PII | AES-256-GCM per-field with HKDF-derived keys | Compromise of one field doesn't expose others |
| Nonce (IV) | Random 12 bytes per record | Prevents nonce reuse across ciphertexts |
| Face images | Never stored | Only score + verdict stored; `source_a`/`source_b` document what was compared |
| Consent records | Immutable (RLS INSERT-only) | Legal document; cannot be altered |
| Audit logs | Append-only (RLS INSERT-only) | Tamper-evident action trail |
| API secrets | Key Vault or `secret_refs` | Never hardcoded in source or env files committed to git |
| Biometric video | Never touches our servers | Azure SDK ↔ Azure direct; we only receive the verdict |

---

## Scalability Notes

| Concern | Approach |
|---------|---------|
| Cloud vendor change | `provider` + `provider_session_id` on sessions and checks — swap vendor with config change only |
| New check type (Module 2–4) | Add ENUM value + new child table; `biometric_checks` unchanged |
| New OCR field | Add key to `extracted_fields` JSONB — no migration |
| High write volume on audit_logs | Partition by `created_at` (monthly); use BRIN index on timestamp |
| Session token hot reads | Cache `provider_session_id` in Redis (TTL = session expiry); DB is source of truth |
| PII key rotation | `encryption_iv` stored per-record; re-encrypt only affected records when rotating master key |

---

## DBML Source

> Paste into [dbdiagram.io](https://dbdiagram.io) for an interactive visual.

```dbml
Project identity_verification {
  database_type: 'PostgreSQL'
  Note: 'Module 1 – ID Verification (Final MVP Schema). Cloud-agnostic. No PII in plain text. AES-256-GCM app-layer encryption.'
}

// ─── ENUMs ────────────────────────────────────────────────────

Enum module_type_enum {
  ID
  ADDRESS
  SALARY
  PROFESSION
}

Enum session_status_enum {
  pending
  in_progress
  complete
  failed
}

Enum decision_status_enum {
  pending       [note: 'Checks still in progress']
  approved      [note: 'All checks passed threshold']
  rejected      [note: 'One or more checks failed hard threshold']
  manual_review [note: 'Soft threshold — requires human review']
}

Enum check_type_enum {
  liveness
  doc_scan
  face_match
}

Enum check_status_enum {
  pending
  passed
  failed
  skipped
}

Enum document_type_enum {
  passport
  drivers_license
  national_id
}

Enum liveness_verdict_enum {
  live
  spoofed
  inconclusive  [note: 'Retryable — poor lighting or head not fully visible. Not a spoof attempt.']
  error
  timeout
}

// ─── TABLES ───────────────────────────────────────────────────

Table users {
  user_id         uuid        [pk, default: `gen_random_uuid()`]
  custom_username text        [unique, not null,  note: 'Plain-text public handle chosen by user']
  username_hash   text        [not null,           note: 'Argon2id hash']
  password_hash   text        [not null,           note: 'Argon2id hash; separate salt per user']
  email_hash      text        [null,               note: 'Argon2id hash; nullable if email not provided']
  id_verified     boolean     [not null, default: false]
  retry_count     int         [not null, default: 0,    note: 'Increments on each failed attempt']
  locked_until    timestamptz [null,               note: 'Set to now()+24h after 3 failed attempts']
  created_at      timestamptz [not null, default: `now()`]
  updated_at      timestamptz [not null, default: `now()`]

  Note: 'No PII stored in plain text. Argon2id hashes only.'
}

Table verification_sessions {
  session_id          uuid                  [pk, default: `gen_random_uuid()`]
  user_id             uuid                  [not null, ref: > users.user_id]
  module_type         module_type_enum      [not null]
  status              session_status_enum   [not null, default: 'pending']
  decision_status     decision_status_enum  [not null, default: 'pending']
  provider            text                  [null, note: 'azure | aws | gcp | onfido | stripe']
  provider_session_id text                  [null, note: 'Opaque vendor session ref — no vendor lock-in']
  retry_count         int                   [not null, default: 0]
  expires_at          timestamptz           [null]
  created_at          timestamptz           [not null, default: `now()`]
  completed_at        timestamptz           [null]

  indexes {
    (user_id)         [name: 'idx_sessions_user_id']
    (status)          [name: 'idx_sessions_status']
    (decision_status) [name: 'idx_sessions_decision_status']
  }

  Note: 'provider + provider_session_id make this schema cloud-agnostic.'
}

Table biometric_checks {
  check_id        uuid              [pk, default: `gen_random_uuid()`]
  session_id      uuid              [not null, ref: > verification_sessions.session_id]
  user_id         uuid              [not null, ref: > users.user_id, note: 'Denormalized for fast user-level queries']
  check_type      check_type_enum   [not null]
  status          check_status_enum [not null, default: 'pending']
  provider        text              [null, note: 'Can differ per check within same session']
  provider_ref_id text              [null, note: 'Vendor-side operation/job ID']
  attempted_at    timestamptz       [null]
  completed_at    timestamptz       [null]

  indexes {
    (session_id)             [name: 'idx_checks_session_id']
    (user_id)                [name: 'idx_checks_user_id']
    (check_type)             [name: 'idx_checks_type']
    (session_id, check_type) [unique, name: 'uq_checks_session_check_type']
  }

  Note: 'Polymorphic parent for all check types. UNIQUE(session_id, check_type) enforced via index. To add Module 2–4 checks: add ENUM value + new child table only.'
}

Table liveness_results {
  result_id        uuid                  [pk, default: `gen_random_uuid()`]
  check_id         uuid                  [unique, not null, ref: - biometric_checks.check_id]
  verdict          liveness_verdict_enum [not null]
  confidence_score numeric(5,4)          [null,  note: '0.0000 – 1.0000']
  failure_reason   text                  [null,  note: 'spoof_detected | timeout | poor_lighting | inconclusive']
  sdk_version      text                  [null,  note: 'Provider SDK version — required for compliance audits and debugging score regressions']
  raw_response     jsonb                 [null,  note: 'Full provider JSON response']
  created_at       timestamptz           [not null, default: `now()`]

  Note: '1:1 with biometric_checks via check_id (UNIQUE enforced).'
}

Table document_scan_results {
  scan_id          uuid               [pk, default: `gen_random_uuid()`]
  check_id         uuid               [unique, not null, ref: - biometric_checks.check_id]
  document_type    document_type_enum [not null]
  issuing_country  char(2)            [null,  note: 'ISO 3166-1 alpha-2 (US, IN, GB...)']
  id_number_hmac   text               [null,  note: 'HMAC-SHA256 only — never reversible. Uniqueness checks only.']
  expiry_date      date               [null]
  mrz_validated    boolean            [not null, default: false, note: 'Machine Readable Zone validation (passports)']
  extracted_fields jsonb              [null,  note: 'Encrypted field map: {"first_name": "<AES-GCM>", "dob": "<AES-GCM>"}. App-layer only.']
  raw_response     jsonb              [null,  note: 'Full OCR provider JSON']
  scanned_at       timestamptz        [not null, default: `now()`]

  indexes {
    (check_id) [name: 'idx_doc_scans_check_id']
  }
}

Table face_match_results {
  match_id     uuid         [pk, default: `gen_random_uuid()`]
  check_id     uuid         [unique, not null, ref: - biometric_checks.check_id]
  confidence   numeric(5,4) [not null, note: 'e.g. 0.9750']
  threshold    numeric(5,4) [not null, default: 0.9000, note: 'Stored per-row to audit historical decisions against the rule in force at the time']
  passed       boolean      [not null]
  source_a     text         [null, note: 'liveness_frame — documents what was compared without storing images']
  source_b     text         [null, note: 'id_document_photo — documents what was compared without storing images']
  raw_response jsonb        [null, note: 'Score + verdict only. No face images ever stored.']
  checked_at   timestamptz  [not null, default: `now()`]

  Note: 'No face images stored. source_a/source_b document what was compared for audit purposes.'
}

Table consent_records {
  consent_id     uuid        [pk, default: `gen_random_uuid()`]
  user_id        uuid        [not null, ref: > users.user_id]
  session_id     uuid        [not null, ref: > verification_sessions.session_id]
  field_name     text        [not null, note: 'first_name | last_name | dob | address | doc_number | nationality']
  consented      boolean     [not null, note: 'true = agreed to store; false = explicitly declined']
  signed_payload text        [null,     note: 'JWT signed by backend private key — non-repudiation. User cannot deny consent was given.']
  ip_address     inet        [null]
  user_agent     text        [null]
  consented_at   timestamptz [not null, default: `now()`]

  indexes {
    (user_id)    [name: 'idx_consent_user_id']
    (session_id) [name: 'idx_consent_session_id']
  }

  Note: 'Immutable audit log. Append-only — no UPDATE or DELETE ever. RLS: INSERT only. One row per field per consent decision.'
}

Table verified_data {
  data_id             uuid        [pk, default: `gen_random_uuid()`]
  user_id             uuid        [not null, ref: > users.user_id]
  session_id          uuid        [not null, ref: > verification_sessions.session_id]
  consent_id          uuid        [not null, ref: > consent_records.consent_id, note: 'Links stored field to the consent record that authorised it']
  field_name          text        [not null, note: 'first_name | last_name | dob | address | doc_number | nationality']
  encrypted_value     text        [not null, note: 'AES-256-GCM ciphertext (base64-encoded)']
  encryption_iv       text        [not null, note: '12-byte random nonce (base64-encoded). Unique per field per record.']
  verification_method text        [null,     note: 'azure_doc_intelligence | manual | ...']
  verified_at         timestamptz [not null, default: `now()`]
  expires_at          timestamptz [null,     note: 'Nullable — some fields require periodic re-verification']
  revoked_at          timestamptz [null,     note: 'NULL = active. Non-null = user withdrew consent.']

  indexes {
    (user_id)    [name: 'idx_verified_user_id']
    (session_id) [name: 'idx_verified_session_id']
    (consent_id) [name: 'idx_verified_consent_id']
  }

  Note: 'AES-256-GCM. Per-field IV. App-layer only. DB never sees plaintext PII. Only populated after explicit user consent.'
}

Table audit_logs {
  log_id      uuid        [pk, default: `gen_random_uuid()`]
  user_id     uuid        [null, ref: > users.user_id, note: 'Nullable — some events have no user context']
  action      text        [not null, note: 'register | liveness | upload | consent | facematch | revoke | delete | lock']
  entity_type text        [null, note: 'verification_sessions | biometric_checks | verified_data | ...']
  entity_id   uuid        [null]
  ip_address  inet        [null]
  metadata    jsonb       [null, note: 'Contextual data per action: {provider, score, decision, sdk_version, ...}']
  created_at  timestamptz [not null, default: `now()`]

  indexes {
    (user_id)    [name: 'idx_audit_user_id']
    (action)     [name: 'idx_audit_action']
    (created_at) [name: 'idx_audit_created_at']
  }

  Note: 'Append-only. Tracks every sensitive action. Never update or delete.'
}

Table secret_refs {
  ref_id          uuid        [pk, default: `gen_random_uuid()`]
  key_name        text        [unique, not null, note: 'e.g. AZURE_FACE_API_KEY']
  encrypted_value text        [not null]
  provider        text        [null, note: 'doppler | infisical | db | azure_kv']
  environment     text        [null, note: 'development | staging | production']
  last_rotated_at timestamptz [null]
  created_at      timestamptz [not null, default: `now()`]

  Note: 'Optional — skip if using Azure Key Vault or Doppler/Infisical. Never store plain-text secrets. Never commit to git.'
}
```
