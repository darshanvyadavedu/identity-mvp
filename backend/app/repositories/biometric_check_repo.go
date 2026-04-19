package repositories

import (
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// BiometricCheckRepoInterface defines data access for biometric checks.
type BiometricCheckRepoInterface interface {
	Create(db *gorm.DB, check *models.BiometricCheck) (*models.BiometricCheck, error)
	GetBySessionAndType(db *gorm.DB, sessionID string, entityType models.EntityType) (*models.BiometricCheck, error)
	GetLatestBySessionAndType(db *gorm.DB, sessionID string, entityType models.EntityType) (*models.BiometricCheck, error)
	CountBySessionAndType(db *gorm.DB, sessionID string, entityType models.EntityType) (int64, error)
	UpdateStatus(db *gorm.DB, checkID string, status models.CheckStatus) error
	UpdateEntityValue(db *gorm.DB, checkID string, entityValue []byte, rawResponse []byte, referenceImage *string) error
}

type biometricCheckRepo struct{}

// NewBiometricCheckRepo returns the default implementation.
func NewBiometricCheckRepo() BiometricCheckRepoInterface {
	return &biometricCheckRepo{}
}

func (r *biometricCheckRepo) Create(db *gorm.DB, check *models.BiometricCheck) (*models.BiometricCheck, error) {
	row := toDBCheck(check)
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create biometric check: %w", err)
	}
	return fromDBCheck(row), nil
}

func (r *biometricCheckRepo) GetLatestBySessionAndType(db *gorm.DB, sessionID string, entityType models.EntityType) (*models.BiometricCheck, error) {
	var row dbmodels.BiometricCheck
	if err := db.Where("session_id = ? AND entity_type = ?", sessionID, string(entityType)).
		Order("attempt_number DESC").
		First(&row).Error; err != nil {
		return nil, fmt.Errorf("get latest biometric check: %w", err)
	}
	return fromDBCheck(&row), nil
}

func (r *biometricCheckRepo) GetBySessionAndType(db *gorm.DB, sessionID string, entityType models.EntityType) (*models.BiometricCheck, error) {
	var row dbmodels.BiometricCheck
	if err := db.Where("session_id = ? AND entity_type = ?", sessionID, string(entityType)).
		First(&row).Error; err != nil {
		return nil, fmt.Errorf("get biometric check: %w", err)
	}
	return fromDBCheck(&row), nil
}

func (r *biometricCheckRepo) CountBySessionAndType(db *gorm.DB, sessionID string, entityType models.EntityType) (int64, error) {
	var count int64
	err := db.Model(&dbmodels.BiometricCheck{}).
		Where("session_id = ? AND entity_type = ?", sessionID, string(entityType)).
		Count(&count).Error
	return count, err
}

func (r *biometricCheckRepo) UpdateStatus(db *gorm.DB, checkID string, status models.CheckStatus) error {
	return db.Model(&dbmodels.BiometricCheck{}).
		Where("check_id = ?", checkID).
		Update("status", string(status)).Error
}

func (r *biometricCheckRepo) UpdateEntityValue(db *gorm.DB, checkID string, entityValue []byte, rawResponse []byte, referenceImage *string) error {
	updates := map[string]any{
		"entity_value": entityValue,
		"raw_response": rawResponse,
	}
	if referenceImage != nil {
		updates["reference_image"] = *referenceImage
	}
	return db.Model(&dbmodels.BiometricCheck{}).
		Where("check_id = ?", checkID).
		Updates(updates).Error
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBCheck(m *models.BiometricCheck) *dbmodels.BiometricCheck {
	var refImg *string
	if m.ReferenceImage != "" {
		refImg = &m.ReferenceImage
	}
	return &dbmodels.BiometricCheck{
		CheckID:        m.CheckID,
		SessionID:      m.SessionID,
		UserID:         m.UserID,
		EntityType:     string(m.EntityType),
		Status:         string(m.Status),
		AttemptNumber:  m.AttemptNumber,
		AttemptedAt:    m.AttemptedAt,
		EntityValue:    m.EntityValue,
		ReferenceImage: refImg,
		RawResponse:    m.RawResponse,
	}
}

func fromDBCheck(row *dbmodels.BiometricCheck) *models.BiometricCheck {
	var refImg string
	if row.ReferenceImage != nil {
		refImg = *row.ReferenceImage
	}
	return &models.BiometricCheck{
		CheckID:        row.CheckID,
		SessionID:      row.SessionID,
		UserID:         row.UserID,
		EntityType:     models.EntityType(row.EntityType),
		Status:         models.CheckStatus(row.Status),
		AttemptNumber:  row.AttemptNumber,
		AttemptedAt:    row.AttemptedAt,
		EntityValue:    row.EntityValue,
		ReferenceImage: refImg,
		RawResponse:    row.RawResponse,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
