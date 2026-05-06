package models

import "time"

// ── ProfessionVerification ────────────────────────────────────────────────────

type ProfessionType string

const (
	ProfessionTypeCorporate ProfessionType = "corporate"
	ProfessionTypeFreelance ProfessionType = "freelance"
	ProfessionTypeRegulated ProfessionType = "regulated"
)

type ProfessionStatus string

const (
	ProfessionStatusInProgress ProfessionStatus = "in_progress"
	ProfessionStatusVerified   ProfessionStatus = "verified"
	ProfessionStatusFailed     ProfessionStatus = "failed"
	ProfessionStatusExpired    ProfessionStatus = "expired"
)

type ConfidenceLevel string

const (
	ConfidenceLevelNone   ConfidenceLevel = "none"
	ConfidenceLevelLow    ConfidenceLevel = "low"
	ConfidenceLevelMedium ConfidenceLevel = "medium"
	ConfidenceLevelHigh   ConfidenceLevel = "high"
)

type ProfessionVerification struct {
	VerificationID   string
	UserID           string
	ProfessionType   ProfessionType
	ProfessionTitle  string
	EmployerName     string
	Status           ProfessionStatus
	ConfidenceScore  int
	ConfidenceLevel  ConfidenceLevel
	ConsentStoreData bool
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ── ProfessionEvidence ────────────────────────────────────────────────────────

type VerificationMethod string

const (
	MethodWorkEmail       VerificationMethod = "work_email"
	MethodLinkedInOAuth   VerificationMethod = "linkedin_oauth"
	MethodDocumentUpload  VerificationMethod = "document_upload"
	MethodPortfolioSocial VerificationMethod = "portfolio_social"
)

type EvidenceStatus string

const (
	EvidenceStatusPending   EvidenceStatus = "pending"
	EvidenceStatusVerified  EvidenceStatus = "verified"
	EvidenceStatusFailed    EvidenceStatus = "failed"
)

// Metadata keys stored per method (JSONB, non-sensitive only):
//   work_email:       email_domain, is_professional
//   linkedin_oauth:   linkedin_id, display_name, headline
//   document_upload:  document_type, file_name, file_size_kb
//   portfolio_social: url, platform

type ProfessionEvidence struct {
	EvidenceID        string
	VerificationID    string
	UserID            string
	Method            VerificationMethod
	Status            EvidenceStatus
	ScoreContribution int
	Metadata          []byte // JSONB
	EmailOTPHash      string
	EmailOTPExpires   *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
