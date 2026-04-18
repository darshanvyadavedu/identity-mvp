package repositories

import (
	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// AuditRepoInterface defines data access for audit logs.
type AuditRepoInterface interface {
	Create(db *gorm.DB, entry *models.AuditLog) error
}

type auditRepo struct{}

// NewAuditRepo returns the default implementation.
func NewAuditRepo() AuditRepoInterface {
	return &auditRepo{}
}

func (r *auditRepo) Create(db *gorm.DB, entry *models.AuditLog) error {
	row := &dbmodels.AuditLog{
		LogID:     entry.LogID,
		UserID:    entry.UserID,
		Action:    entry.Action,
		SessionID: entry.SessionID,
		Details:   entry.Details,
	}
	// Best-effort — ignore error; audit failure should not fail the request.
	db.Create(row)
	return nil
}
