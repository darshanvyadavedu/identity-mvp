package models

import "time"

// ── VerificationSession ───────────────────────────────────────────────────────

type SessionStatus string

const (
	SessionStatusPending        SessionStatus = "pending"
	SessionStatusLivenessPassed SessionStatus = "liveness_passed"
	SessionStatusLivenessFailed SessionStatus = "liveness_failed"
	SessionStatusCompleted      SessionStatus = "completed"
)

type DecisionStatus string

const (
	DecisionStatusPending  DecisionStatus = "pending"
	DecisionStatusVerified DecisionStatus = "verified"
	DecisionStatusFailed   DecisionStatus = "failed"
)

type VerificationSession struct {
	SessionID         string
	UserID            string
	ModuleType        string
	Status            SessionStatus
	DecisionStatus    DecisionStatus
	Provider          string
	ProviderSessionID string
	RetryCount        int
	ExpiresAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ── BiometricCheck ────────────────────────────────────────────────────────────

type CheckType string

const (
	CheckTypeLiveness  CheckType = "liveness"
	CheckTypeDocScan   CheckType = "doc_scan"
	CheckTypeFaceMatch CheckType = "face_match"
)

type CheckStatus string

const (
	CheckStatusPending   CheckStatus = "pending"
	CheckStatusSucceeded CheckStatus = "succeeded"
	CheckStatusFailed    CheckStatus = "failed"
)

type BiometricCheck struct {
	CheckID       string
	SessionID     string
	UserID        string
	CheckType     CheckType
	Status        CheckStatus
	AttemptNumber int
	AttemptedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ── LivenessResult ────────────────────────────────────────────────────────────

type LivenessResult struct {
	ResultID        string
	CheckID         string
	Verdict         string
	ConfidenceScore float64
	FailureReason   string
	SDKVersion      string
	ReferenceImage  string
	RawResponse     []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ── DocumentScanResult ────────────────────────────────────────────────────────

type DocumentScanResult struct {
	ScanID          string
	CheckID         string
	DocumentType    string
	IssuingCountry  string
	IDNumberHMAC    string
	ExtractedFields []byte
	RawResponse     []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ── FaceMatchResult ───────────────────────────────────────────────────────────

type FaceMatchResult struct {
	MatchID     string
	CheckID     string
	Confidence  float64
	Threshold   float64
	Passed      bool
	SourceA     string
	SourceB     string
	RawResponse []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ── IdentityHash ──────────────────────────────────────────────────────────────

type IdentityHash struct {
	HashID    string
	UserID    string
	FieldName string
	HashValue string
	HashAlgo  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ── ConsentRecord ─────────────────────────────────────────────────────────────

type ConsentRecord struct {
	ConsentID string
	UserID    string
	SessionID string
	FieldName string
	Consented bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ── VerifiedData ──────────────────────────────────────────────────────────────

type VerifiedData struct {
	DataID         string
	UserID         string
	SessionID      string
	ConsentID      string
	FieldName      string
	EncryptedValue string
	EncryptionIV   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ── AuditLog ──────────────────────────────────────────────────────────────────

type AuditLog struct {
	LogID     string
	UserID    string
	Action    string
	SessionID string
	Details   []byte
	CreatedAt time.Time
}

// ── DocumentData ──────────────────────────────────────────────────────────────

// DocumentData holds structured fields extracted from an identity document.
type DocumentData struct {
	FirstName      string `json:"firstName,omitempty"`
	LastName       string `json:"lastName,omitempty"`
	DOB            string `json:"dob,omitempty"`
	IDNumber       string `json:"idNumber,omitempty"`
	Expiry         string `json:"expiry,omitempty"`
	IssuingCountry string `json:"issuingCountry,omitempty"`
	Address        string `json:"address,omitempty"`
	DocumentType   string `json:"documentType,omitempty"`
}
