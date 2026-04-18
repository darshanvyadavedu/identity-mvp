package services

import (
	"context"
	"encoding/json"
	"fmt"

	"user-authentication/app/clients"
	"user-authentication/app/models"
	"user-authentication/app/repositories"
	"user-authentication/config"

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
	Document       *models.DocumentData
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
	sessionRepo  repositories.VerificationSessionRepoInterface
	checkRepo    repositories.BiometricCheckRepoInterface
	livenessRepo repositories.LivenessResultRepoInterface
	docScanRepo  repositories.DocumentScanRepoInterface
	faceRepo     repositories.FaceMatchRepoInterface
	hashRepo     repositories.IdentityHashRepoInterface
	auditRepo    repositories.AuditRepoInterface
	faceClient   clients.FaceClientInterface
	docClient    clients.DocumentClientInterface
}

// DocumentServiceOption configures a documentService.
type DocumentServiceOption func(*documentService)

// NewDocumentService returns a new DocumentServiceInterface with default dependencies.
func NewDocumentService(opts ...DocumentServiceOption) DocumentServiceInterface {
	svc := &documentService{
		sessionRepo:  repositories.NewVerificationSessionRepo(),
		checkRepo:    repositories.NewBiometricCheckRepo(),
		livenessRepo: repositories.NewLivenessResultRepo(),
		docScanRepo:  repositories.NewDocumentScanRepo(),
		faceRepo:     repositories.NewFaceMatchRepo(),
		hashRepo:     repositories.NewIdentityHashRepo(),
		auditRepo:    repositories.NewAuditRepo(),
		faceClient:   newFaceClient(),
		docClient:    newDocumentClient(),
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

func ConfigureDocLivenessRepo(r repositories.LivenessResultRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.livenessRepo = r }
}

func ConfigureDocScanRepo(r repositories.DocumentScanRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.docScanRepo = r }
}

func ConfigureDocFaceMatchRepo(r repositories.FaceMatchRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.faceRepo = r }
}

func ConfigureDocHashRepo(r repositories.IdentityHashRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.hashRepo = r }
}

func ConfigureDocAuditRepo(r repositories.AuditRepoInterface) DocumentServiceOption {
	return func(s *documentService) { s.auditRepo = r }
}

func ConfigureDocFaceClient(c clients.FaceClientInterface) DocumentServiceOption {
	return func(s *documentService) { s.faceClient = c }
}

func ConfigureDocClient(c clients.DocumentClientInterface) DocumentServiceOption {
	return func(s *documentService) { s.docClient = c }
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

	// 2. Load liveness reference image.
	livenessCheck, err := svc.checkRepo.GetBySessionAndType(db, params.SessionID, models.CheckTypeLiveness)
	if err != nil {
		return nil, ErrInternalServer("liveness check record not found")
	}
	livenessResult, err := svc.livenessRepo.GetByCheckID(db, livenessCheck.CheckID)
	if err != nil {
		return nil, ErrInternalServer("liveness result not found")
	}
	if livenessResult.ReferenceImage == "" {
		return nil, ErrBadRequest("no liveness reference image on record")
	}
	refBytes, err := dataURLToBytes(livenessResult.ReferenceImage)
	if err != nil {
		return nil, ErrInternalServer("decode reference image: " + err.Error())
	}

	// 3. Create doc_scan biometric check.
	docAttempts, _ := svc.checkRepo.CountBySessionAndType(db, params.SessionID, models.CheckTypeDocScan)
	docCheck, err := svc.checkRepo.Create(db, &models.BiometricCheck{
		SessionID:     params.SessionID,
		UserID:        params.UserID,
		CheckType:     models.CheckTypeDocScan,
		Status:        models.CheckStatusPending,
		AttemptNumber: int(docAttempts) + 1,
	})
	if err != nil {
		return nil, ErrInternalServer("create doc scan check: " + err.Error())
	}

	// 4. OCR — AnalyzeID via provider document client.
	docData, rawDocJSON, docExtractErr := svc.docClient.AnalyzeID(ctx, params.DocBytes)
	docCheckStatus := models.CheckStatusSucceeded
	if docExtractErr != nil {
		docCheckStatus = models.CheckStatusFailed
	}
	_ = svc.checkRepo.UpdateStatus(db, docCheck.CheckID, docCheckStatus)

	extractedJSON, _ := json.Marshal(docData)
	_, err = svc.docScanRepo.Create(db, &models.DocumentScanResult{
		CheckID:         docCheck.CheckID,
		DocumentType:    docData.DocumentType,
		IssuingCountry:  docData.IssuingCountry,
		IDNumberHMAC:    computeHMAC(docData.IDNumber, cfg.HMACSecret),
		ExtractedFields: extractedJSON,
		RawResponse:     rawDocJSON,
	})
	if err != nil {
		return nil, ErrInternalServer("save document scan result: " + err.Error())
	}

	// 5. Early duplicate check: name+DOB HMAC.
	if docData.FirstName != "" && docData.DOB != "" {
		combo := docData.FirstName + "|" + docData.DOB
		nameDOBHash := computeHMAC(combo, cfg.HMACSecret)
		existing, findErr := svc.hashRepo.FindByFieldAndHash(db, "first_name_dob", nameDOBHash)
		if findErr == nil && existing != nil && existing.UserID != params.UserID {
			return nil, ErrConflict("Identity already exists: this document's name and date of birth are linked to another account.")
		}
	}

	// 6. Face match — create check and compare faces via provider.
	fmAttempts, _ := svc.checkRepo.CountBySessionAndType(db, params.SessionID, models.CheckTypeFaceMatch)
	fmCheck, err := svc.checkRepo.Create(db, &models.BiometricCheck{
		SessionID:     params.SessionID,
		UserID:        params.UserID,
		CheckType:     models.CheckTypeFaceMatch,
		Status:        models.CheckStatusPending,
		AttemptNumber: int(fmAttempts) + 1,
	})
	if err != nil {
		return nil, ErrInternalServer("create face match check: " + err.Error())
	}

	compareResult, fmErr := svc.faceClient.CompareFaces(ctx, refBytes, params.DocBytes)
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

	var threshold float64
	if compareResult != nil {
		threshold = compareResult.Similarity
	}

	_, err = svc.faceRepo.Create(db, &models.FaceMatchResult{
		CheckID:     fmCheck.CheckID,
		Confidence:  similarity / 100.0,
		Threshold:   threshold / 100.0,
		Passed:      passed,
		SourceA:     "liveness_frame",
		SourceB:     "id_document",
		RawResponse: rawFMJSON,
	})
	if err != nil {
		return nil, ErrInternalServer("save face match result: " + err.Error())
	}

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
			Threshold:  threshold,
		},
	}, nil
}
