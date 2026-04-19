package services

import (
	"context"
	"encoding/json"
	"fmt"

	"user-authentication/app/models"
	"user-authentication/app/repositories"
	"user-authentication/config"
	"user-authentication/lib/provider"

	"gorm.io/gorm"
)

// UploadDocumentParams holds the input for UploadDocument.
type UploadDocumentParams struct {
	SessionID string
	UserID    string
	DocBytes  []byte
}

// UploadDocumentResult holds the output for UploadDocument.
type UploadDocumentResult struct {
	SessionID      string
	DecisionStatus string
	Document       *provider.DocumentData
	FaceMatch      FaceMatchSummary
}

// FaceMatchSummary is a lightweight summary returned to the caller.
type FaceMatchSummary struct {
	Similarity float64
	Passed     bool
	Threshold  float64
}

// DocumentServiceInterface defines the contract for document operations.
type DocumentServiceInterface interface {
	UploadDocument(ctx context.Context, db *gorm.DB, params *UploadDocumentParams) (*UploadDocumentResult, ServiceErrorInterface)
}

type documentService struct {
	sessionRepo repositories.VerificationSessionRepoInterface
	checkRepo   repositories.BiometricCheckRepoInterface
	hashRepo    repositories.IdentityHashRepoInterface
	auditRepo   repositories.AuditRepoInterface
	p           provider.IdentityProvider
}

// DocumentServiceOption configures a documentService.
type DocumentServiceOption func(*documentService)

// NewDocumentService returns a new DocumentServiceInterface with default dependencies.
func NewDocumentService(opts ...DocumentServiceOption) DocumentServiceInterface {
	svc := &documentService{
		sessionRepo: repositories.NewVerificationSessionRepo(),
		checkRepo:   repositories.NewBiometricCheckRepo(),
		hashRepo:    repositories.NewIdentityHashRepo(),
		auditRepo:   repositories.NewAuditRepo(),
		p:           Active(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func ConfigureDocSessionRepo(r repositories.VerificationSessionRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.sessionRepo = r }
}

func ConfigureDocCheckRepo(r repositories.BiometricCheckRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.checkRepo = r }
}

func ConfigureDocHashRepo(r repositories.IdentityHashRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.hashRepo = r }
}

func ConfigureDocAuditRepo(r repositories.AuditRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.auditRepo = r }
}

func (svc *documentService) UploadDocument(ctx context.Context, db *gorm.DB, params *UploadDocumentParams) (*UploadDocumentResult, ServiceErrorInterface) {
	cfg := config.Get()

	// 1. Load session and verify state.
	session, err := svc.sessionRepo.GetBySessionAndUser(db, params.SessionID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("session not found")
	}
	if session.Status != models.SessionStatusLivenessPassed {
		return nil, ErrBadRequest(fmt.Sprintf("liveness check must pass first (current status: %s)", session.Status))
	}

	// 2. Load liveness reference image from biometric check.
	livenessCheck, err := svc.checkRepo.GetBySessionAndType(db, params.SessionID, models.EntityTypeLiveness)
	if err != nil {
		return nil, ErrInternalServer("liveness check record not found")
	}
	if livenessCheck.ReferenceImage == "" {
		return nil, ErrBadRequest("no liveness reference image on record")
	}
	refBytes, err := dataURLToBytes(livenessCheck.ReferenceImage)
	if err != nil {
		return nil, ErrInternalServer("decode reference image: " + err.Error())
	}

	// 3. Create doc_scan biometric check.
	docAttempts, _ := svc.checkRepo.CountBySessionAndType(db, params.SessionID, models.EntityTypeDocScan)
	docCheck, err := svc.checkRepo.Create(db, &models.BiometricCheck{
		SessionID:     params.SessionID,
		UserID:        params.UserID,
		EntityType:    models.EntityTypeDocScan,
		Status:        models.CheckStatusPending,
		AttemptNumber: int(docAttempts) + 1,
	})
	if err != nil {
		return nil, ErrInternalServer("create doc scan check: " + err.Error())
	}

	// 4. OCR — AnalyzeID via provider.
	docData, rawDocJSON, docExtractErr := svc.p.AnalyzeID(ctx, params.DocBytes)
	docCheckStatus := models.CheckStatusSucceeded
	if docExtractErr != nil {
		docCheckStatus = models.CheckStatusFailed
	}
	_ = svc.checkRepo.UpdateStatus(db, docCheck.CheckID, docCheckStatus)

	entityJSON, _ := json.Marshal(docData)
	_ = svc.checkRepo.UpdateEntityValue(db, docCheck.CheckID, entityJSON, rawDocJSON, nil)

	// 5. Identity duplicate check + hash storage.
	if docData.FirstName != "" && docData.DOB != "" {
		combo := docData.FirstName + "|" + docData.DOB
		blindIdx := computeHMAC(combo, cfg.HMACSecret)
		existing, findErr := svc.hashRepo.FindByFieldAndBlindIndex(db, "first_name_dob", blindIdx)
		if findErr == nil && existing != nil {
			if existing.UserID != params.UserID {
				return nil, ErrConflict("An account is already verified with this identity.")
			}
			return nil, ErrConflict("This account has already been verified.")
		}
		_ = svc.hashRepo.Create(db, &models.IdentityHash{
			UserID:     params.UserID,
			FieldName:  "first_name_dob",
			HashValue:  computeHMAC(combo, params.UserID+":"+cfg.HMACSecret),
			BlindIndex: blindIdx,
			HashAlgo:   "hmac-sha256",
		})
	}

	// 6. Face match — create check and compare faces.
	fmAttempts, _ := svc.checkRepo.CountBySessionAndType(db, params.SessionID, models.EntityTypeFaceMatch)
	fmCheck, err := svc.checkRepo.Create(db, &models.BiometricCheck{
		SessionID:     params.SessionID,
		UserID:        params.UserID,
		EntityType:    models.EntityTypeFaceMatch,
		Status:        models.CheckStatusPending,
		AttemptNumber: int(fmAttempts) + 1,
	})
	if err != nil {
		return nil, ErrInternalServer("create face match check: " + err.Error())
	}

	compareResult, fmErr := svc.p.CompareFaces(ctx, refBytes, params.DocBytes)
	var similarity float64
	var passed bool
	var rawFMJSON []byte
	if fmErr == nil && compareResult != nil {
		similarity = compareResult.Similarity
		passed = compareResult.Passed
		rawFMJSON = compareResult.RawResponse
	}
	fmStatus := models.CheckStatusSucceeded
	if fmErr != nil {
		fmStatus = models.CheckStatusFailed
	}
	_ = svc.checkRepo.UpdateStatus(db, fmCheck.CheckID, fmStatus)

	fmEntityJSON, _ := json.Marshal(models.FaceMatchEntityValue{
		Confidence: similarity / 100.0,
		Passed:     passed,
	})
	_ = svc.checkRepo.UpdateEntityValue(db, fmCheck.CheckID, fmEntityJSON, rawFMJSON, nil)

	// 7. Update session decision.
	decisionStatus := models.DecisionStatusVerified
	if !passed {
		decisionStatus = models.DecisionStatusFailed
	}
	_ = svc.sessionRepo.UpdateStatus(db, params.SessionID, string(models.SessionStatusCompleted), string(decisionStatus))

	// 8. Audit log (best-effort).
	details, _ := json.Marshal(map[string]any{
		"faceMatchPassed": passed,
		"similarity":      similarity,
		"decisionStatus":  string(decisionStatus),
	})
	_ = svc.auditRepo.Create(db, &models.AuditLog{
		UserID:    params.UserID,
		Action:    "document_verified",
		SessionID: params.SessionID,
		Details:   details,
	})

	return &UploadDocumentResult{
		SessionID:      params.SessionID,
		DecisionStatus: string(decisionStatus),
		Document:       docData,
		FaceMatch: FaceMatchSummary{
			Similarity: similarity,
			Passed:     passed,
			Threshold:  similarity,
		},
	}, nil
}
