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
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ── BiometricCheck ────────────────────────────────────────────────────────────

type EntityType string

const (
	EntityTypeLiveness  EntityType = "liveness"
	EntityTypeDocScan   EntityType = "doc_scan"
	EntityTypeFaceMatch EntityType = "face_match"
)

type CheckStatus string

const (
	CheckStatusPending   CheckStatus = "pending"
	CheckStatusSucceeded CheckStatus = "succeeded"
	CheckStatusFailed    CheckStatus = "failed"
)

type BiometricCheck struct {
	CheckID        string
	SessionID      string
	UserID         string
	EntityType     EntityType
	Status         CheckStatus
	AttemptNumber  int
	EntityValue    []byte // unmarshal into typed payload struct based on EntityType
	ReferenceImage string // liveness image data URL
	RawResponse    []byte // full provider response — audit only
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ── Entity value payload structs (not DB models) ──────────────────────────────

type LivenessEntityValue struct {
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
}

type FaceMatchEntityValue struct {
	Confidence float64 `json:"confidence"`
	Passed     bool    `json:"passed"`
}

// DocScan entity value uses provider.DocumentData — same JSON shape.

// ── IdentityHash ──────────────────────────────────────────────────────────────

type IdentityHash struct {
	HashID     string
	UserID     string
	FieldName  string
	HashValue  string // HMAC(value, userID+":"+secret) — user-specific, private
	BlindIndex string // HMAC(value, secret) — global, used only for cross-user dedup
	HashAlgo   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ── ConsentRecord ─────────────────────────────────────────────────────────────

type ConsentRecord struct {
	ConsentID string
	UserID    string
	SessionID string
	FieldName string
	Consented bool
	HashValue string
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
