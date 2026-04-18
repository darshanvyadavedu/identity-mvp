package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ── Base ──────────────────────────────────────────────────────────────────────

type Base struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// BeforeCreate sets a UUID primary key if empty.
func setUUID(id *string) {
	if *id == "" {
		*id = uuid.NewString()
	}
}

// ── VerificationSession ───────────────────────────────────────────────────────

type VerificationSession struct {
	SessionID         string     `gorm:"primaryKey;column:session_id"                json:"sessionId"`
	UserID            string     `gorm:"column:user_id;not null;index"               json:"userId"`
	ModuleType        string     `gorm:"column:module_type;default:ID"               json:"moduleType"`
	Status            string     `gorm:"column:status;default:pending"               json:"status"`
	DecisionStatus    string     `gorm:"column:decision_status;default:pending"      json:"decisionStatus"`
	Provider          string     `gorm:"column:provider"                             json:"provider"`
	ProviderSessionID string     `gorm:"column:provider_session_id"                  json:"providerSessionId"`
	RetryCount        int        `gorm:"column:retry_count;default:0"                json:"retryCount"`
	ExpiresAt         *time.Time `gorm:"column:expires_at"                           json:"expiresAt,omitempty"`
	Base
}

func (s *VerificationSession) BeforeCreate(_ *gorm.DB) error {
	setUUID(&s.SessionID)
	return nil
}

// ── BiometricCheck ────────────────────────────────────────────────────────────

type BiometricCheck struct {
	CheckID       string     `gorm:"primaryKey;column:check_id"           json:"checkId"`
	SessionID     string     `gorm:"column:session_id;not null;index"     json:"sessionId"`
	UserID        string     `gorm:"column:user_id;not null;index"        json:"userId"`
	CheckType     string     `gorm:"column:check_type;not null"           json:"checkType"` // liveness | doc_scan | face_match
	Status        string     `gorm:"column:status;default:pending"        json:"status"`
	AttemptNumber int        `gorm:"column:attempt_number;default:1"      json:"attemptNumber"`
	AttemptedAt   *time.Time `gorm:"column:attempted_at"                  json:"attemptedAt,omitempty"`
	Base
}

func (b *BiometricCheck) BeforeCreate(_ *gorm.DB) error {
	setUUID(&b.CheckID)
	return nil
}

// ── LivenessResult ────────────────────────────────────────────────────────────

type LivenessResult struct {
	ResultID        string  `gorm:"primaryKey;column:result_id"                   json:"resultId"`
	CheckID         string  `gorm:"uniqueIndex;column:check_id;not null"          json:"checkId"`
	Verdict         string  `gorm:"column:verdict;not null"                       json:"verdict"` // live | spoofed | inconclusive | error
	ConfidenceScore float64 `gorm:"column:confidence_score;type:numeric(5,4)"    json:"confidenceScore"`
	FailureReason   string  `gorm:"column:failure_reason"                         json:"failureReason,omitempty"`
	SDKVersion      string  `gorm:"column:sdk_version"                            json:"sdkVersion,omitempty"`
	ReferenceImage  string  `gorm:"column:reference_image;type:text"              json:"referenceImage,omitempty"` // data URL
	RawResponse     []byte  `gorm:"column:raw_response;type:jsonb"                json:"-"`
	Base
}

func (l *LivenessResult) BeforeCreate(_ *gorm.DB) error {
	setUUID(&l.ResultID)
	return nil
}

// ── DocumentScanResult ────────────────────────────────────────────────────────

type DocumentScanResult struct {
	ScanID          string `gorm:"primaryKey;column:scan_id"                        json:"scanId"`
	CheckID         string `gorm:"uniqueIndex;column:check_id;not null"             json:"checkId"`
	DocumentType    string `gorm:"column:document_type"                             json:"documentType,omitempty"`
	IssuingCountry  string `gorm:"column:issuing_country"                           json:"issuingCountry,omitempty"`
	IDNumberHMAC    string `gorm:"column:id_number_hmac"                            json:"idNumberHmac,omitempty"`
	ExtractedFields []byte `gorm:"column:extracted_fields;type:jsonb"               json:"extractedFields,omitempty"`
	RawResponse     []byte `gorm:"column:raw_response;type:jsonb"                   json:"-"`
	Base
}

func (d *DocumentScanResult) BeforeCreate(_ *gorm.DB) error {
	setUUID(&d.ScanID)
	return nil
}

// ── FaceMatchResult ───────────────────────────────────────────────────────────

type FaceMatchResult struct {
	MatchID     string  `gorm:"primaryKey;column:match_id"                json:"matchId"`
	CheckID     string  `gorm:"uniqueIndex;column:check_id;not null"      json:"checkId"`
	Confidence  float64 `gorm:"column:confidence;type:numeric(5,4)"       json:"confidence"`
	Threshold   float64 `gorm:"column:threshold;type:numeric(5,4);default:0.8000" json:"threshold"`
	Passed      bool    `gorm:"column:passed"                             json:"passed"`
	SourceA     string  `gorm:"column:source_a"                           json:"sourceA,omitempty"` // liveness_frame
	SourceB     string  `gorm:"column:source_b"                           json:"sourceB,omitempty"` // id_document
	RawResponse []byte  `gorm:"column:raw_response;type:jsonb"            json:"-"`
	Base
}

func (f *FaceMatchResult) BeforeCreate(_ *gorm.DB) error {
	setUUID(&f.MatchID)
	return nil
}

// ── IdentityHash ──────────────────────────────────────────────────────────────

type IdentityHash struct {
	HashID    string `gorm:"primaryKey;column:hash_id"      json:"hashId"`
	UserID    string `gorm:"column:user_id;not null;index"  json:"userId"`
	FieldName string `gorm:"column:field_name;not null"     json:"fieldName"`
	HashValue string `gorm:"column:hash_value;not null"     json:"hashValue"`
	HashAlgo  string `gorm:"column:hash_algo;not null"      json:"hashAlgo"`
	Base
}

func (h *IdentityHash) BeforeCreate(_ *gorm.DB) error {
	setUUID(&h.HashID)
	return nil
}

// ── ConsentRecord ─────────────────────────────────────────────────────────────

type ConsentRecord struct {
	ConsentID string `gorm:"primaryKey;column:consent_id"  json:"consentId"`
	UserID    string `gorm:"column:user_id;index"          json:"userId"`
	SessionID string `gorm:"column:session_id;index"       json:"sessionId"`
	FieldName string `gorm:"column:field_name"             json:"fieldName"`
	Consented bool   `gorm:"column:consented"              json:"consented"`
	Base
}

func (c *ConsentRecord) BeforeCreate(_ *gorm.DB) error {
	setUUID(&c.ConsentID)
	return nil
}

// ── VerifiedData ──────────────────────────────────────────────────────────────

type VerifiedData struct {
	DataID         string `gorm:"primaryKey;column:data_id"        json:"dataId"`
	UserID         string `gorm:"column:user_id;index"             json:"userId"`
	SessionID      string `gorm:"column:session_id;index"          json:"sessionId"`
	ConsentID      string `gorm:"column:consent_id"               json:"consentId"`
	FieldName      string `gorm:"column:field_name"               json:"fieldName"`
	EncryptedValue string `gorm:"column:encrypted_value;type:text" json:"-"`
	EncryptionIV   string `gorm:"column:encryption_iv"            json:"-"`
	Base
}

func (v *VerifiedData) BeforeCreate(_ *gorm.DB) error {
	setUUID(&v.DataID)
	return nil
}

// ── AuditLog ──────────────────────────────────────────────────────────────────

type AuditLog struct {
	LogID     string `gorm:"primaryKey;column:log_id"              json:"logId"`
	UserID    string `gorm:"column:user_id;index"                  json:"userId"`
	Action    string `gorm:"column:action;not null"                json:"action"`
	SessionID string `gorm:"column:session_id;index"               json:"sessionId,omitempty"`
	Details   []byte `gorm:"column:details;type:jsonb"             json:"details,omitempty"`
	CreatedAt time.Time
}

func (a *AuditLog) BeforeCreate(_ *gorm.DB) error {
	setUUID(&a.LogID)
	return nil
}
