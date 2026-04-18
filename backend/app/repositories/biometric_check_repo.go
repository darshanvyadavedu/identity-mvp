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
	GetBySessionAndType(db *gorm.DB, sessionID string, checkType models.CheckType) (*models.BiometricCheck, error)
	GetLatestBySessionAndType(db *gorm.DB, sessionID string, checkType models.CheckType) (*models.BiometricCheck, error)
	CountBySessionAndType(db *gorm.DB, sessionID string, checkType models.CheckType) (int64, error)
	UpdateStatus(db *gorm.DB, checkID string, status models.CheckStatus) error
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

func (r *biometricCheckRepo) GetLatestBySessionAndType(db *gorm.DB, sessionID string, checkType models.CheckType) (*models.BiometricCheck, error) {
	var row dbmodels.BiometricCheck
	if err := db.Where("session_id = ? AND check_type = ?", sessionID, string(checkType)).
		Order("attempt_number DESC").
		First(&row).Error; err != nil {
		return nil, fmt.Errorf("get latest biometric check: %w", err)
	}
	return fromDBCheck(&row), nil
}

func (r *biometricCheckRepo) GetBySessionAndType(db *gorm.DB, sessionID string, checkType models.CheckType) (*models.BiometricCheck, error) {
	var row dbmodels.BiometricCheck
	if err := db.Where("session_id = ? AND check_type = ?", sessionID, string(checkType)).
		First(&row).Error; err != nil {
		return nil, fmt.Errorf("get biometric check: %w", err)
	}
	return fromDBCheck(&row), nil
}

func (r *biometricCheckRepo) CountBySessionAndType(db *gorm.DB, sessionID string, checkType models.CheckType) (int64, error) {
	var count int64
	err := db.Model(&dbmodels.BiometricCheck{}).
		Where("session_id = ? AND check_type = ?", sessionID, string(checkType)).
		Count(&count).Error
	return count, err
}

func (r *biometricCheckRepo) UpdateStatus(db *gorm.DB, checkID string, status models.CheckStatus) error {
	return db.Model(&dbmodels.BiometricCheck{}).
		Where("check_id = ?", checkID).
		Update("status", string(status)).Error
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBCheck(m *models.BiometricCheck) *dbmodels.BiometricCheck {
	return &dbmodels.BiometricCheck{
		CheckID:       m.CheckID,
		SessionID:     m.SessionID,
		UserID:        m.UserID,
		CheckType:     string(m.CheckType),
		Status:        string(m.Status),
		AttemptNumber: m.AttemptNumber,
		AttemptedAt:   m.AttemptedAt,
	}
}

func fromDBCheck(row *dbmodels.BiometricCheck) *models.BiometricCheck {
	return &models.BiometricCheck{
		CheckID:       row.CheckID,
		SessionID:     row.SessionID,
		UserID:        row.UserID,
		CheckType:     models.CheckType(row.CheckType),
		Status:        models.CheckStatus(row.Status),
		AttemptNumber: row.AttemptNumber,
		AttemptedAt:   row.AttemptedAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
