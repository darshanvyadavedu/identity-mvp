package repositories

import (
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// ConsentRepoInterface defines data access for consent records and verified data.
type ConsentRepoInterface interface {
	CreateConsent(db *gorm.DB, consent *models.ConsentRecord) (*models.ConsentRecord, error)
	CreateVerifiedData(db *gorm.DB, vd *models.VerifiedData) (*models.VerifiedData, error)
	DeleteConsentByUser(db *gorm.DB, userID string) error
	DeleteVerifiedDataByUser(db *gorm.DB, userID string) error
}

type consentRepo struct{}

// NewConsentRepo returns the default implementation.
func NewConsentRepo() ConsentRepoInterface {
	return &consentRepo{}
}

func (r *consentRepo) CreateConsent(db *gorm.DB, consent *models.ConsentRecord) (*models.ConsentRecord, error) {
	row := &dbmodels.ConsentRecord{
		ConsentID: consent.ConsentID,
		UserID:    consent.UserID,
		SessionID: consent.SessionID,
		FieldName: consent.FieldName,
		Consented: consent.Consented,
		HashValue: consent.HashValue,
	}
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create consent record: %w", err)
	}
	consent.ConsentID = row.ConsentID
	return consent, nil
}

func (r *consentRepo) CreateVerifiedData(db *gorm.DB, vd *models.VerifiedData) (*models.VerifiedData, error) {
	row := &dbmodels.VerifiedData{
		DataID:         vd.DataID,
		UserID:         vd.UserID,
		SessionID:      vd.SessionID,
		ConsentID:      vd.ConsentID,
		FieldName:      vd.FieldName,
		EncryptedValue: vd.EncryptedValue,
		EncryptionIV:   vd.EncryptionIV,
	}
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create verified data: %w", err)
	}
	return vd, nil
}

func (r *consentRepo) DeleteConsentByUser(db *gorm.DB, userID string) error {
	return db.Where("user_id = ?", userID).Delete(&dbmodels.ConsentRecord{}).Error
}

func (r *consentRepo) DeleteVerifiedDataByUser(db *gorm.DB, userID string) error {
	return db.Where("user_id = ?", userID).Delete(&dbmodels.VerifiedData{}).Error
}
