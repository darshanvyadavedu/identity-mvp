package repositories

import (
	"errors"
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// VerificationSessionRepoInterface defines data access for verification sessions.
type VerificationSessionRepoInterface interface {
	Create(db *gorm.DB, session *models.VerificationSession) (*models.VerificationSession, error)
	GetBySessionAndUser(db *gorm.DB, sessionID, userID string) (*models.VerificationSession, error)
	UpdateStatus(db *gorm.DB, sessionID, status, decisionStatus string) error
}

type verificationSessionRepo struct{}

// NewVerificationSessionRepo returns the default implementation.
func NewVerificationSessionRepo() VerificationSessionRepoInterface {
	return &verificationSessionRepo{}
}

func (r *verificationSessionRepo) Create(db *gorm.DB, session *models.VerificationSession) (*models.VerificationSession, error) {
	row := toDBSession(session)
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create verification session: %w", err)
	}
	return fromDBSession(row), nil
}

func (r *verificationSessionRepo) GetBySessionAndUser(db *gorm.DB, sessionID, userID string) (*models.VerificationSession, error) {
	var row dbmodels.VerificationSession
	err := db.Where("session_id = ? AND user_id = ?", sessionID, userID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("get verification session: %w", err)
	}
	return fromDBSession(&row), nil
}

func (r *verificationSessionRepo) UpdateStatus(db *gorm.DB, sessionID, status, decisionStatus string) error {
	updates := map[string]any{}
	if status != "" {
		updates["status"] = status
	}
	if decisionStatus != "" {
		updates["decision_status"] = decisionStatus
	}
	return db.Model(&dbmodels.VerificationSession{}).
		Where("session_id = ?", sessionID).
		Updates(updates).Error
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBSession(m *models.VerificationSession) *dbmodels.VerificationSession {
	return &dbmodels.VerificationSession{
		SessionID:         m.SessionID,
		UserID:            m.UserID,
		ModuleType:        m.ModuleType,
		Status:            string(m.Status),
		DecisionStatus:    string(m.DecisionStatus),
		Provider:          m.Provider,
		ProviderSessionID: m.ProviderSessionID,
		RetryCount:        m.RetryCount,
		ExpiresAt:         m.ExpiresAt,
	}
}

func fromDBSession(row *dbmodels.VerificationSession) *models.VerificationSession {
	return &models.VerificationSession{
		SessionID:         row.SessionID,
		UserID:            row.UserID,
		ModuleType:        row.ModuleType,
		Status:            models.SessionStatus(row.Status),
		DecisionStatus:    models.DecisionStatus(row.DecisionStatus),
		Provider:          row.Provider,
		ProviderSessionID: row.ProviderSessionID,
		RetryCount:        row.RetryCount,
		ExpiresAt:         row.ExpiresAt,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}
