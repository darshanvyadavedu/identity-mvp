# User Authentication — Identity Verification API

A Go backend for biometric identity verification. Users complete a liveness check, upload an ID document, and the service confirms the face on the document matches the liveness selfie.

## How It Works

```
1. Create liveness session  →  provider returns a session token
2. Client completes liveness check (selfie capture handled by provider SDK)
3. Poll for liveness result  →  reference image stored on success
4. Upload ID document        →  OCR extracts identity fields
5. Face match                →  liveness selfie vs document face
6. Store consent + verified data (encrypted)
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/sessions` | Create a liveness session |
| `GET` | `/api/sessions/:sessionId/result` | Poll liveness result |
| `GET` | `/api/sessions/:sessionId/liveness-image` | Fetch reference face image (JPEG) |
| `POST` | `/api/documents` | Upload ID document (triggers OCR + face match) |
| `POST` | `/api/sessions/:sessionId/consent` | Store consent and verified identity data |

## Providers

Set `PROVIDER` in `.env` to select a backend:

| Value | Face / Liveness | Document OCR |
|-------|----------------|--------------|
| `aws` (default) | AWS Rekognition | AWS Textract |
| `azure` | Azure Face API | Azure Document Intelligence |
| `hybrid` | AWS Rekognition | Azure Document Intelligence |

## Prerequisites

- Go 1.24+
- PostgreSQL 14+
- AWS account (for `aws` or `hybrid` provider)
- Azure account (for `azure` or `hybrid` provider)

## Setup

```bash
# 1. Clone and enter the backend directory
cd backend

# 2. Copy and fill in environment variables
cp .env.sample .env

# 3. Create the database
psql -U postgres -c "CREATE DATABASE identification;"

# 4. Apply the schema
psql -U postgres -d identification -f design/schema.sql

# 5. Install dependencies
go mod download

# 6. Run
go run ./cmd/api
```

The server starts on `http://localhost:8080` by default.

## Environment Variables

See [`.env.sample`](.env.sample) for the full list. Key variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `PROVIDER` | No | `aws` / `azure` / `hybrid` (default: `aws`) |
| `PORT` | No | HTTP port (default: `8080`) |
| `DATABASE_URL` | No* | Full postgres DSN — overrides `DB_*` vars |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | No* | Individual DB connection params |
| `HMAC_SECRET` | Yes | Random secret for identity blind index |
| `ENCRYPTION_KEY` | Yes | 64 hex chars (32 bytes) for AES-256-GCM |
| `AWS_REGION` | aws/hybrid | AWS region (default: `us-east-1`) |
| `REKOGNITION_COLLECTION_ID` | aws/hybrid | Rekognition face collection name |
| `AZURE_FACE_ENDPOINT` / `AZURE_FACE_KEY` | azure/hybrid | Azure Face API credentials |
| `AZURE_DOCUMENT_ENDPOINT` / `AZURE_DOCUMENT_KEY` | azure/hybrid | Azure Document Intelligence credentials |

*Either `DATABASE_URL` or the individual `DB_*` vars must be set.

## Project Structure

```
backend/
├── cmd/api/            # Entry point
├── app/
│   ├── endpoints/      # HTTP handlers
│   ├── services/       # Business logic
│   ├── repositories/   # Database access
│   └── models/         # Domain models
├── lib/
│   ├── aws/            # AWS Rekognition + Textract client
│   ├── azure/          # Azure Face + Document Intelligence client
│   ├── provider/       # IdentityProvider interface + shared types
│   ├── db/             # Database connection
│   └── web/            # HTTP utilities and middleware
├── config/             # Environment-based configuration
├── routes/v1/          # Route registration
└── design/
    └── schema.sql      # PostgreSQL schema
```

## Database Schema

| Table | Purpose |
|-------|---------|
| `users` | User accounts |
| `verification_sessions` | One session per verification attempt |
| `biometric_checks` | Individual checks: liveness, doc_scan, face_match |
| `identity_hashes` | One-way HMAC hashes for identity deduplication |
| `consent_records` | User consent per data field |
| `verified_data` | AES-256-GCM encrypted verified identity fields |
| `audit_logs` | Append-only action log |

## Security Notes

- Identity fields (name, DOB) are never stored in plaintext — only HMAC-SHA256 blind indexes for dedup
- Verified data is encrypted at rest with AES-256-GCM (`ENCRYPTION_KEY`)
- Reference images are stored as base64 data URLs in the `biometric_checks` table
- Face comparison thresholds: AWS 80% similarity / Azure 70% confidence
