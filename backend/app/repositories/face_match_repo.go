package repositories

import (
	"fmt"

	"user-authentication/app/models"
	dbmodels "user-authentication/app/repositories/db_models"

	"gorm.io/gorm"
)

// FaceMatchRepoInterface defines data access for face match results.
type FaceMatchRepoInterface interface {
	Create(db *gorm.DB, result *models.FaceMatchResult) (*models.FaceMatchResult, error)
}

type faceMatchRepo struct{}

// NewFaceMatchRepo returns the default implementation.
func NewFaceMatchRepo() FaceMatchRepoInterface {
	return &faceMatchRepo{}
}

func (r *faceMatchRepo) Create(db *gorm.DB, result *models.FaceMatchResult) (*models.FaceMatchResult, error) {
	row := toDBFaceMatch(result)
	if err := db.Create(row).Error; err != nil {
		return nil, fmt.Errorf("create face match result: %w", err)
	}
	return fromDBFaceMatch(row), nil
}

// ── Converters ────────────────────────────────────────────────────────────────

func toDBFaceMatch(m *models.FaceMatchResult) *dbmodels.FaceMatchResult {
	return &dbmodels.FaceMatchResult{
		MatchID:     m.MatchID,
		CheckID:     m.CheckID,
		Confidence:  m.Confidence,
		Threshold:   m.Threshold,
		Passed:      m.Passed,
		SourceA:     m.SourceA,
		SourceB:     m.SourceB,
		RawResponse: m.RawResponse,
	}
}

func fromDBFaceMatch(row *dbmodels.FaceMatchResult) *models.FaceMatchResult {
	return &models.FaceMatchResult{
		MatchID:     row.MatchID,
		CheckID:     row.CheckID,
		Confidence:  row.Confidence,
		Threshold:   row.Threshold,
		Passed:      row.Passed,
		SourceA:     row.SourceA,
		SourceB:     row.SourceB,
		RawResponse: row.RawResponse,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
