package repositories

import (
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// LivenessResultRepoInterface defines data access for liveness results.
type LivenessResultRepoInterface interface {
	Upsert(db *gorm.DB, result *models.LivenessResult) (*models.LivenessResult, error)
	GetByCheckID(db *gorm.DB, checkID string) (*models.LivenessResult, error)
}

type livenessResultRepo struct{}

// NewLivenessResultRepo returns the default implementation.
func NewLivenessResultRepo() LivenessResultRepoInterface {
	return &livenessResultRepo{}
}

func (r *livenessResultRepo) Upsert(db *gorm.DB, result *models.LivenessResult) (*models.LivenessResult, error) {
	row := toDBLiveness(result)
	err := db.Where(dbmodels.LivenessResult{CheckID: result.CheckID}).
		Assign(dbmodels.LivenessResult{
			Verdict:         row.Verdict,
			ConfidenceScore: row.ConfidenceScore,
			ReferenceImage:  row.ReferenceImage,
			RawResponse:     row.RawResponse,
		}).
		FirstOrCreate(row).Error
	if err != nil {
		return nil, fmt.Errorf("upsert liveness result: %w", err)
	}
	return fromDBLiveness(row), nil
}

func (r *livenessResultRepo) GetByCheckID(db *gorm.DB, checkID string) (*models.LivenessResult, error) {
	var row dbmodels.LivenessResult
	if err := db.Where("check_id = ?", checkID).First(&row).Error; err != nil {
		return nil, fmt.Errorf("get liveness result: %w", err)
	}
	return fromDBLiveness(&row), nil
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBLiveness(m *models.LivenessResult) *dbmodels.LivenessResult {
	return &dbmodels.LivenessResult{
		ResultID:        m.ResultID,
		CheckID:         m.CheckID,
		Verdict:         m.Verdict,
		ConfidenceScore: m.ConfidenceScore,
		FailureReason:   m.FailureReason,
		SDKVersion:      m.SDKVersion,
		ReferenceImage:  m.ReferenceImage,
		RawResponse:     m.RawResponse,
	}
}

func fromDBLiveness(row *dbmodels.LivenessResult) *models.LivenessResult {
	return &models.LivenessResult{
		ResultID:        row.ResultID,
		CheckID:         row.CheckID,
		Verdict:         row.Verdict,
		ConfidenceScore: row.ConfidenceScore,
		FailureReason:   row.FailureReason,
		SDKVersion:      row.SDKVersion,
		ReferenceImage:  row.ReferenceImage,
		RawResponse:     row.RawResponse,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
