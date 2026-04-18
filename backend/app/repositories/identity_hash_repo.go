package repositories

import (
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// IdentityHashRepoInterface defines data access for identity hashes.
type IdentityHashRepoInterface interface {
	Create(db *gorm.DB, hash *models.IdentityHash) error
	FindByFieldAndHash(db *gorm.DB, fieldName, hashValue string) (*models.IdentityHash, error)
	DeleteByUser(db *gorm.DB, userID string) error
	DeleteByUserAndField(db *gorm.DB, userID, fieldName string) error
}

type identityHashRepo struct{}

// NewIdentityHashRepo returns the default implementation.
func NewIdentityHashRepo() IdentityHashRepoInterface {
	return &identityHashRepo{}
}

func (r *identityHashRepo) Create(db *gorm.DB, hash *models.IdentityHash) error {
	row := toDBIdentityHash(hash)
	if err := db.Create(row).Error; err != nil {
		return fmt.Errorf("create identity hash: %w", err)
	}
	return nil
}

func (r *identityHashRepo) FindByFieldAndHash(db *gorm.DB, fieldName, hashValue string) (*models.IdentityHash, error) {
	var row dbmodels.IdentityHash
	err := db.Where("field_name = ? AND hash_value = ?", fieldName, hashValue).First(&row).Error
	if err != nil {
		return nil, err
	}
	return fromDBIdentityHash(&row), nil
}

func (r *identityHashRepo) DeleteByUser(db *gorm.DB, userID string) error {
	return db.Where("user_id = ?", userID).Delete(&dbmodels.IdentityHash{}).Error
}

func (r *identityHashRepo) DeleteByUserAndField(db *gorm.DB, userID, fieldName string) error {
	return db.Where("user_id = ? AND field_name = ?", userID, fieldName).Delete(&dbmodels.IdentityHash{}).Error
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBIdentityHash(m *models.IdentityHash) *dbmodels.IdentityHash {
	return &dbmodels.IdentityHash{
		HashID:    m.HashID,
		UserID:    m.UserID,
		FieldName: m.FieldName,
		HashValue: m.HashValue,
		HashAlgo:  m.HashAlgo,
	}
}

func fromDBIdentityHash(row *dbmodels.IdentityHash) *models.IdentityHash {
	return &models.IdentityHash{
		HashID:    row.HashID,
		UserID:    row.UserID,
		FieldName: row.FieldName,
		HashValue: row.HashValue,
		HashAlgo:  row.HashAlgo,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
