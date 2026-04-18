package services

import (
	"context"
	"encoding/json"

	"user-authentication/app/clients"
	"user-authentication/app/models"
	"user-authentication/app/repositories"

	"gorm.io/gorm"
)

// GetLivenessResultParams holds the input for GetLivenessResult.
type GetLivenessResultParams struct {
	SessionID string
	UserID    string
}

// GetLivenessResultOutput holds the output for GetLivenessResult.
type GetLivenessResultOutput struct {
	SessionID          string
	Complete           bool   // false = still in progress; caller should return status as-is
	LivenessStatus     string // raw provider status
	LivenessConfidence float64
	ReferenceImage     string
}

// GetLivenessImageParams holds the input for GetLivenessImage.
type GetLivenessImageParams struct {
	SessionID string
	UserID    string
}

// LivenessServiceInterface defines the contract for liveness operations.
type LivenessServiceInterface interface {
	GetLivenessResult(ctx context.Context, db *gorm.DB, params *GetLivenessResultParams) (*GetLivenessResultOutput, ServiceErrorInterface)
	GetLivenessImage(ctx context.Context, db *gorm.DB, params *GetLivenessImageParams) ([]byte, ServiceErrorInterface)
}

type livenessService struct {
	sessionRepo  repositories.VerificationSessionRepoInterface
	checkRepo    repositories.BiometricCheckRepoInterface
	livenessRepo repositories.LivenessResultRepoInterface
	auditRepo    repositories.AuditRepoInterface
	faceClient   clients.FaceClientInterface
}

// LivenessServiceOption configures a livenessService.
type LivenessServiceOption func(*livenessService)

// NewLivenessService returns a new LivenessServiceInterface with default dependencies.
func NewLivenessService(opts ...LivenessServiceOption) LivenessServiceInterface {
	svc := &livenessService{
		sessionRepo:  repositories.NewVerificationSessionRepo(),
		checkRepo:    repositories.NewBiometricCheckRepo(),
		livenessRepo: repositories.NewLivenessResultRepo(),
		auditRepo:    repositories.NewAuditRepo(),
		faceClient:   newFaceClient(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func ConfigureLivenessSessionRepo(r repositories.VerificationSessionRepoInterface) LivenessServiceOption {
	return func(s *livenessService) { s.sessionRepo = r }
}

func ConfigureLivenessCheckRepo(r repositories.BiometricCheckRepoInterface) LivenessServiceOption {
	return func(s *livenessService) { s.checkRepo = r }
}

func ConfigureLivenessResultRepo(r repositories.LivenessResultRepoInterface) LivenessServiceOption {
	return func(s *livenessService) { s.livenessRepo = r }
}

func ConfigureLivenessAuditRepo(r repositories.AuditRepoInterface) LivenessServiceOption {
	return func(s *livenessService) { s.auditRepo = r }
}

func ConfigureLivenessFaceClient(c clients.FaceClientInterface) LivenessServiceOption {
	return func(s *livenessService) { s.faceClient = c }
}

func (svc *livenessService) GetLivenessResult(ctx context.Context, db *gorm.DB, params *GetLivenessResultParams) (*GetLivenessResultOutput, ServiceErrorInterface) {
	// 1. Load the verification session.
	session, err := svc.sessionRepo.GetBySessionAndUser(db, params.SessionID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("session not found")
	}

	// 2. Poll the provider for liveness result.
	result, err := svc.faceClient.GetLivenessResult(ctx, session.ProviderSessionID)
	if err != nil {
		return nil, ErrBadGateway("get liveness results: " + err.Error())
	}

	// 3. If still in progress, return early — no DB writes.
	if !result.Complete {
		return &GetLivenessResultOutput{
			SessionID:      params.SessionID,
			Complete:       false,
			LivenessStatus: result.ProviderStatus,
		}, nil
	}

	verdict := result.Verdict
	confidence := result.Confidence

	checkStatus := models.CheckStatusPending
	sessionStatus := models.SessionStatusPending
	decisionStatus := models.DecisionStatusPending

	switch verdict {
	case "live":
		checkStatus = models.CheckStatusSucceeded
		sessionStatus = models.SessionStatusLivenessPassed
		decisionStatus = models.DecisionStatusPending
	case "failed":
		checkStatus = models.CheckStatusFailed
		sessionStatus = models.SessionStatusLivenessFailed
		decisionStatus = models.DecisionStatusFailed
	}

	// 4. Find the biometric check for this session.
	check, _ := svc.checkRepo.GetBySessionAndType(db, params.SessionID, models.CheckTypeLiveness)

	// 5. Upsert liveness result.
	if check != nil {
		_, err = svc.livenessRepo.Upsert(db, &models.LivenessResult{
			CheckID:         check.CheckID,
			Verdict:         verdict,
			ConfidenceScore: confidence / 100.0,
			ReferenceImage:  result.ReferenceImage,
			RawResponse:     result.RawResponse,
		})
		if err != nil {
			return nil, ErrInternalServer("save liveness result: " + err.Error())
		}

		// 6. Update biometric check status.
		_ = svc.checkRepo.UpdateStatus(db, check.CheckID, checkStatus)
	}

	// 7. Update verification session.
	_ = svc.sessionRepo.UpdateStatus(db, params.SessionID, string(sessionStatus), string(decisionStatus))

	// 8. Audit log (best-effort).
	details, _ := json.Marshal(map[string]any{
		"verdict":        verdict,
		"confidence":     confidence,
		"providerStatus": result.ProviderStatus,
	})
	_ = svc.auditRepo.Create(db, &models.AuditLog{
		UserID:    params.UserID,
		Action:    "liveness_result_fetched",
		SessionID: params.SessionID,
		Details:   details,
	})

	return &GetLivenessResultOutput{
		SessionID:          params.SessionID,
		Complete:           true,
		LivenessStatus:     result.ProviderStatus,
		LivenessConfidence: confidence,
		ReferenceImage:     result.ReferenceImage,
	}, nil
}

func (svc *livenessService) GetLivenessImage(ctx context.Context, db *gorm.DB, params *GetLivenessImageParams) ([]byte, ServiceErrorInterface) {
	// 1. Verify session belongs to user.
	_, err := svc.sessionRepo.GetBySessionAndUser(db, params.SessionID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("session not found")
	}

	// 2. Load liveness check + result.
	check, err := svc.checkRepo.GetBySessionAndType(db, params.SessionID, models.CheckTypeLiveness)
	if err != nil {
		return nil, ErrNotFound("liveness check not found")
	}
	livenessResult, err := svc.livenessRepo.GetByCheckID(db, check.CheckID)
	if err != nil || livenessResult.ReferenceImage == "" {
		return nil, ErrNotFound("liveness image not available")
	}

	// 3. Decode the data URL to raw bytes.
	imgBytes, err := dataURLToBytes(livenessResult.ReferenceImage)
	if err != nil {
		return nil, ErrInternalServer("decode reference image: " + err.Error())
	}
	return imgBytes, nil
}
