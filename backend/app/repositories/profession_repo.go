package repositories

import (
	"errors"
	"fmt"
	"time"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// ProfessionRepoInterface defines data access for profession verification.
type ProfessionRepoInterface interface {
	Create(db *gorm.DB, v *models.ProfessionVerification) (*models.ProfessionVerification, error)
	GetByIDAndUser(db *gorm.DB, verificationID, userID string) (*models.ProfessionVerification, error)
	UpdateScoreAndStatus(db *gorm.DB, verificationID string, score int, level, status string) error

	UpsertEvidence(db *gorm.DB, e *models.ProfessionEvidence) (*models.ProfessionEvidence, error)
	GetEvidenceByID(db *gorm.DB, evidenceID, verificationID string) (*models.ProfessionEvidence, error)
	UpdateEvidenceStatus(db *gorm.DB, evidenceID, status string, scoreContribution int) error
	ListEvidence(db *gorm.DB, verificationID string) ([]*models.ProfessionEvidence, error)
}

type professionRepo struct{}

// NewProfessionRepo returns the default implementation.
func NewProfessionRepo() ProfessionRepoInterface {
	return &professionRepo{}
}

// ── ProfessionVerification ────────────────────────────────────────────────────

func (r *professionRepo) Create(db *gorm.DB, v *models.ProfessionVerification) (*models.ProfessionVerification, error) {
	row := toDB(v)
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create profession_verification: %w", err)
	}
	return fromDB(row), nil
}

func (r *professionRepo) GetByIDAndUser(db *gorm.DB, verificationID, userID string) (*models.ProfessionVerification, error) {
	var row dbmodels.ProfessionVerification
	err := db.Where("verification_id = ? AND user_id = ?", verificationID, userID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("profession verification not found")
		}
		return nil, fmt.Errorf("get profession_verification: %w", err)
	}
	return fromDB(&row), nil
}

func (r *professionRepo) UpdateScoreAndStatus(db *gorm.DB, verificationID string, score int, level, status string) error {
	return db.Model(&dbmodels.ProfessionVerification{}).
		Where("verification_id = ?", verificationID).
		Updates(map[string]any{
			"confidence_score": score,
			"confidence_level": level,
			"status":           status,
			"updated_at":       time.Now(),
		}).Error
}

// ── ProfessionEvidence ────────────────────────────────────────────────────────

// UpsertEvidence creates a new evidence row or replaces the existing one for the
// same (verification_id, method) pair, enforcing the unique constraint in Go to
// remain compatible with older PostgreSQL versions that lack ON CONFLICT DO UPDATE
// for partial unique indexes.
func (r *professionRepo) UpsertEvidence(db *gorm.DB, e *models.ProfessionEvidence) (*models.ProfessionEvidence, error) {
	var existing dbmodels.ProfessionEvidence
	err := db.Where("verification_id = ? AND method = ?", e.VerificationID, string(e.Method)).First(&existing).Error
	if err == nil {
		// Update in-place.
		updates := map[string]any{
			"status":             string(e.Status),
			"score_contribution": e.ScoreContribution,
			"metadata":           e.Metadata,
			"email_otp_hash":     e.EmailOTPHash,
			"email_otp_expires":  e.EmailOTPExpires,
			"updated_at":         time.Now(),
		}
		if err2 := db.Model(&existing).Updates(updates).Error; err2 != nil {
			return nil, fmt.Errorf("update profession_evidence: %w", err2)
		}
		e.EvidenceID = existing.EvidenceID
		return e, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup profession_evidence: %w", err)
	}
	row := toEvidenceDB(e)
	if err2 := db.Create(row).Error; err2 != nil {
		return nil, fmt.Errorf("create profession_evidence: %w", err2)
	}
	e.EvidenceID = row.EvidenceID
	return e, nil
}

func (r *professionRepo) GetEvidenceByID(db *gorm.DB, evidenceID, verificationID string) (*models.ProfessionEvidence, error) {
	var row dbmodels.ProfessionEvidence
	err := db.Where("evidence_id = ? AND verification_id = ?", evidenceID, verificationID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("profession evidence not found")
		}
		return nil, fmt.Errorf("get profession_evidence: %w", err)
	}
	return fromEvidenceDB(&row), nil
}

func (r *professionRepo) UpdateEvidenceStatus(db *gorm.DB, evidenceID, status string, scoreContribution int) error {
	return db.Model(&dbmodels.ProfessionEvidence{}).
		Where("evidence_id = ?", evidenceID).
		Updates(map[string]any{
			"status":             status,
			"score_contribution": scoreContribution,
			"updated_at":         time.Now(),
		}).Error
}

func (r *professionRepo) ListEvidence(db *gorm.DB, verificationID string) ([]*models.ProfessionEvidence, error) {
	var rows []dbmodels.ProfessionEvidence
	if err := db.Where("verification_id = ?", verificationID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list profession_evidence: %w", err)
	}
	out := make([]*models.ProfessionEvidence, len(rows))
	for i := range rows {
		out[i] = fromEvidenceDB(&rows[i])
	}
	return out, nil
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDB(m *models.ProfessionVerification) *dbmodels.ProfessionVerification {
	return &dbmodels.ProfessionVerification{
		VerificationID:   m.VerificationID,
		UserID:           m.UserID,
		ProfessionType:   string(m.ProfessionType),
		ProfessionTitle:  m.ProfessionTitle,
		EmployerName:     m.EmployerName,
		Status:           string(m.Status),
		ConfidenceScore:  m.ConfidenceScore,
		ConfidenceLevel:  string(m.ConfidenceLevel),
		ConsentStoreData: m.ConsentStoreData,
		ExpiresAt:        m.ExpiresAt,
	}
}

func fromDB(row *dbmodels.ProfessionVerification) *models.ProfessionVerification {
	return &models.ProfessionVerification{
		VerificationID:   row.VerificationID,
		UserID:           row.UserID,
		ProfessionType:   models.ProfessionType(row.ProfessionType),
		ProfessionTitle:  row.ProfessionTitle,
		EmployerName:     row.EmployerName,
		Status:           models.ProfessionStatus(row.Status),
		ConfidenceScore:  row.ConfidenceScore,
		ConfidenceLevel:  models.ConfidenceLevel(row.ConfidenceLevel),
		ConsentStoreData: row.ConsentStoreData,
		ExpiresAt:        row.ExpiresAt,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func toEvidenceDB(e *models.ProfessionEvidence) *dbmodels.ProfessionEvidence {
	return &dbmodels.ProfessionEvidence{
		EvidenceID:        e.EvidenceID,
		VerificationID:    e.VerificationID,
		UserID:            e.UserID,
		Method:            string(e.Method),
		Status:            string(e.Status),
		ScoreContribution: e.ScoreContribution,
		Metadata:          e.Metadata,
		EmailOTPHash:      e.EmailOTPHash,
		EmailOTPExpires:   e.EmailOTPExpires,
	}
}

func fromEvidenceDB(row *dbmodels.ProfessionEvidence) *models.ProfessionEvidence {
	return &models.ProfessionEvidence{
		EvidenceID:        row.EvidenceID,
		VerificationID:    row.VerificationID,
		UserID:            row.UserID,
		Method:            models.VerificationMethod(row.Method),
		Status:            models.EvidenceStatus(row.Status),
		ScoreContribution: row.ScoreContribution,
		Metadata:          row.Metadata,
		EmailOTPHash:      row.EmailOTPHash,
		EmailOTPExpires:   row.EmailOTPExpires,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}
