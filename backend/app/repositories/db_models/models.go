package db_models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setUUID(id *string) {
	if *id == "" {
		*id = uuid.NewString()
	}
}

// ── VerificationSession ───────────────────────────────────────────────────────

type VerificationSession struct {
	SessionID         string     `gorm:"primaryKey;column:session_id"`
	UserID            string     `gorm:"column:user_id;not null;index"`
	ModuleType        string     `gorm:"column:module_type;default:ID"`
	Status            string     `gorm:"column:status;default:pending"`
	DecisionStatus    string     `gorm:"column:decision_status;default:pending"`
	Provider          string     `gorm:"column:provider"`
	ProviderSessionID string     `gorm:"column:provider_session_id"`
	RetryCount        int        `gorm:"column:retry_count;default:0"`
	ExpiresAt         *time.Time `gorm:"column:expires_at"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (s *VerificationSession) BeforeCreate(_ *gorm.DB) error {
	setUUID(&s.SessionID)
	return nil
}

// ── BiometricCheck ────────────────────────────────────────────────────────────

type BiometricCheck struct {
	CheckID        string     `gorm:"primaryKey;column:check_id"`
	SessionID      string     `gorm:"column:session_id;not null;index"`
	UserID         string     `gorm:"column:user_id;not null;index"`
	EntityType     string     `gorm:"column:entity_type;not null"`
	Status         string     `gorm:"column:status;default:pending"`
	AttemptNumber  int        `gorm:"column:attempt_number;default:1"`
	AttemptedAt    *time.Time `gorm:"column:attempted_at"`
	EntityValue    []byte     `gorm:"column:entity_value;type:jsonb"`
	ReferenceImage *string    `gorm:"column:reference_image;type:text"`
	RawResponse    []byte     `gorm:"column:raw_response;type:jsonb"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (b *BiometricCheck) BeforeCreate(_ *gorm.DB) error {
	setUUID(&b.CheckID)
	return nil
}

// ── IdentityHash ──────────────────────────────────────────────────────────────

type IdentityHash struct {
	HashID    string `gorm:"primaryKey;column:hash_id"`
	UserID    string `gorm:"column:user_id;not null;index"`
	FieldName string `gorm:"column:field_name;not null"`
	HashValue string `gorm:"column:hash_value;not null"`
	HashAlgo  string `gorm:"column:hash_algo;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (h *IdentityHash) BeforeCreate(_ *gorm.DB) error {
	setUUID(&h.HashID)
	return nil
}

// ── ConsentRecord ─────────────────────────────────────────────────────────────

type ConsentRecord struct {
	ConsentID string `gorm:"primaryKey;column:consent_id"`
	UserID    string `gorm:"column:user_id;index"`
	SessionID string `gorm:"column:session_id;index"`
	FieldName string `gorm:"column:field_name"`
	Consented bool   `gorm:"column:consented"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c *ConsentRecord) BeforeCreate(_ *gorm.DB) error {
	setUUID(&c.ConsentID)
	return nil
}

// ── VerifiedData ──────────────────────────────────────────────────────────────

type VerifiedData struct {
	DataID         string `gorm:"primaryKey;column:data_id"`
	UserID         string `gorm:"column:user_id;index"`
	SessionID      string `gorm:"column:session_id;index"`
	ConsentID      string `gorm:"column:consent_id"`
	FieldName      string `gorm:"column:field_name"`
	EncryptedValue string `gorm:"column:encrypted_value;type:text"`
	EncryptionIV   string `gorm:"column:encryption_iv"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (v *VerifiedData) BeforeCreate(_ *gorm.DB) error {
	setUUID(&v.DataID)
	return nil
}

// ── AuditLog ──────────────────────────────────────────────────────────────────

type AuditLog struct {
	LogID     string `gorm:"primaryKey;column:log_id"`
	UserID    string `gorm:"column:user_id;index"`
	Action    string `gorm:"column:action;not null"`
	SessionID string `gorm:"column:session_id;index"`
	Details   []byte `gorm:"column:details;type:jsonb"`
	CreatedAt time.Time
}

func (a *AuditLog) BeforeCreate(_ *gorm.DB) error {
	setUUID(&a.LogID)
	return nil
}
