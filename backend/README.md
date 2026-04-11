# User Authentication — Face Liveness + Document Verification

End-to-end identity verification using **AWS Rekognition Face Liveness** and **AWS Textract**.

## Architecture

```
Browser
  │
  ├─ POST /api/liveness/session   → Go backend → AWS Rekognition (create session)
  ├─ AWS Amplify Liveness SDK     → streams video directly to AWS (never hits backend)
  ├─ GET  /api/liveness/result/:id → Go backend → AWS Rekognition (get result + face image)
  └─ POST /api/documents          → Go backend → AWS Textract (OCR the ID document)
```

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- AWS account with:
  - Rekognition Face Liveness enabled (`us-east-1`)
  - Textract enabled
  - IAM user with `rekognition:*` and `textract:*` permissions

## Setup

### 1. Clone and install

```bash
git clone <repo-url>
cd user-authentication

# Install Go dependencies
go mod download

# Install frontend dependencies
cd frontend && npm install && cd ..
```

### 2. Configure environment variables

Copy the example and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:

```env
# AWS
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key

# Azure Face API (optional — not used in current MVP)
AZURE_FACE_ENDPOINT=https://eastus.api.cognitive.microsoft.com
AZURE_FACE_KEY=your_azure_key

# Backend port
PORT=8080

# Frontend (Vite exposes VITE_ prefixed vars to the browser)
VITE_AWS_REGION=us-east-1
VITE_AWS_ACCESS_KEY_ID=your_access_key
VITE_AWS_SECRET_ACCESS_KEY=your_secret_key
```

Get your AWS credentials:
```bash
aws configure get aws_access_key_id
aws configure get aws_secret_access_key
```

### 3. Run the backend

```bash
go run main.go
# Server starts on http://localhost:8080
```

### 4. Run the frontend

```bash
cd frontend
npm run dev
# Opens on http://localhost:5173
```

## Usage

1. Open `http://localhost:5173`
2. Enter your name and click **Start Verification**
3. Allow camera access — the AWS liveness check runs in the browser
4. Your face image is shown with a confidence score
5. Upload a photo of your ID document
6. Results are displayed with extracted fields

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/liveness/session` | Create a Rekognition Face Liveness session |
| `GET`  | `/api/liveness/result/:sessionId` | Get liveness result + reference image |
| `POST` | `/api/documents` | Upload ID document for Textract OCR |

## Manual AWS CLI testing

```bash
# Create a liveness session
aws rekognition create-face-liveness-session --region us-east-1

# Poll the result (replace SESSION_ID)
aws rekognition get-face-liveness-session-results \
  --session-id SESSION_ID \
  --region us-east-1
```

## Project Structure

```
user-authentication/
├── main.go                  # Go backend (HTTP server + AWS SDK calls)
├── go.mod / go.sum          # Go dependencies
├── .env                     # Environment variables (git-ignored)
├── .env.example             # Template — commit this, not .env
├── docs/
│   ├── api.puml             # API sequence diagram (PlantUML)
│   └── db-schema.md         # Planned PostgreSQL schema
└── frontend/
    ├── index.html
    ├── vite.config.js
    ├── package.json
    └── src/
        ├── main.js              # App state machine + UI
        └── LivenessCapture.jsx  # AWS Amplify FaceLivenessDetector component
```
