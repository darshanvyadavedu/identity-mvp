package repositories

import (
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// DocumentScanRepoInterface defines data access for document scan results.
type DocumentScanRepoInterface interface {
	Create(db *gorm.DB, result *models.DocumentScanResult) (*models.DocumentScanResult, error)
	GetByCheckID(db *gorm.DB, checkID string) (*models.DocumentScanResult, error)
}

type documentScanRepo struct{}

// NewDocumentScanRepo returns the default implementation.
func NewDocumentScanRepo() DocumentScanRepoInterface {
	return &documentScanRepo{}
}

func (r *documentScanRepo) Create(db *gorm.DB, result *models.DocumentScanResult) (*models.DocumentScanResult, error) {
	row := toDBDocScan(result)
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create document scan result: %w", err)
	}
	return fromDBDocScan(row), nil
}

func (r *documentScanRepo) GetByCheckID(db *gorm.DB, checkID string) (*models.DocumentScanResult, error) {
	var row dbmodels.DocumentScanResult
	if err := db.Where("check_id = ?", checkID).First(&row).Error; err != nil {
		return nil, fmt.Errorf("get document scan result: %w", err)
	}
	return fromDBDocScan(&row), nil
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBDocScan(m *models.DocumentScanResult) *dbmodels.DocumentScanResult {
	return &dbmodels.DocumentScanResult{
		ScanID:          m.ScanID,
		CheckID:         m.CheckID,
		DocumentType:    m.DocumentType,
		IssuingCountry:  m.IssuingCountry,
		IDNumberHMAC:    m.IDNumberHMAC,
		ExtractedFields: m.ExtractedFields,
		RawResponse:     m.RawResponse,
	}
}

func fromDBDocScan(row *dbmodels.DocumentScanResult) *models.DocumentScanResult {
	return &models.DocumentScanResult{
		ScanID:          row.ScanID,
		CheckID:         row.CheckID,
		DocumentType:    row.DocumentType,
		IssuingCountry:  row.IssuingCountry,
		IDNumberHMAC:    row.IDNumberHMAC,
		ExtractedFields: row.ExtractedFields,
		RawResponse:     row.RawResponse,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
