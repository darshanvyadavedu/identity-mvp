package db_models

import (
	"time"

	"gorm.io/gorm"
)

// ── ProfessionVerification ────────────────────────────────────────────────────

type ProfessionVerification struct {
	VerificationID   string     `gorm:"primaryKey;column:verification_id"`
	UserID           string     `gorm:"column:user_id;not null;index"`
	ProfessionType   string     `gorm:"column:profession_type;not null"`
	ProfessionTitle  string     `gorm:"column:profession_title"`
	EmployerName     string     `gorm:"column:employer_name"`
	Status           string     `gorm:"column:status;default:in_progress"`
	ConfidenceScore  int        `gorm:"column:confidence_score;default:0"`
	ConfidenceLevel  string     `gorm:"column:confidence_level;default:none"`
	ConsentStoreData bool       `gorm:"column:consent_store_data;default:false"`
	ExpiresAt        *time.Time `gorm:"column:expires_at"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (p *ProfessionVerification) BeforeCreate(_ *gorm.DB) error {
	setUUID(&p.VerificationID)
	return nil
}

// ── ProfessionEvidence ────────────────────────────────────────────────────────

type ProfessionEvidence struct {
	EvidenceID        string     `gorm:"primaryKey;column:evidence_id"`
	VerificationID    string     `gorm:"column:verification_id;not null;index"`
	UserID            string     `gorm:"column:user_id;not null;index"`
	Method            string     `gorm:"column:method;not null"`
	Status            string     `gorm:"column:status;default:pending"`
	ScoreContribution int        `gorm:"column:score_contribution;default:0"`
	Metadata          []byte     `gorm:"column:metadata;type:jsonb"`
	EmailOTPHash      string     `gorm:"column:email_otp_hash"`
	EmailOTPExpires   *time.Time `gorm:"column:email_otp_expires"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (e *ProfessionEvidence) BeforeCreate(_ *gorm.DB) error {
	setUUID(&e.EvidenceID)
	return nil
}

func (ProfessionEvidence) TableName() string { return "profession_evidence" }
