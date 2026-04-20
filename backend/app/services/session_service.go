package services

import (
	"context"
	"encoding/json"

	"user-authentication/app/models"
	"user-authentication/app/repositories"
	"user-authentication/config"
	"user-authentication/lib/provider"

	"gorm.io/gorm"
)

// CreateSessionParams holds the input for CreateSession.
type CreateSessionParams struct {
	UserID string
}

// CreateSessionResult holds the output for CreateSession.
type CreateSessionResult struct {
	SessionID         string
	ProviderSessionID string
	AuthToken         string // Azure only; empty for AWS
	Provider          string
	UserID            string
}

// SessionServiceInterface defines the contract for session operations.
type SessionServiceInterface interface {
	CreateSession(ctx context.Context, db *gorm.DB, params *CreateSessionParams) (*CreateSessionResult, ServiceErrorInterface)
}

type sessionService struct {
	sessionRepo repositories.VerificationSessionRepoInterface
	checkRepo   repositories.BiometricCheckRepoInterface
	auditRepo   repositories.AuditRepoInterface
	face        provider.FaceProvider
}

// SessionServiceOption configures a sessionService.
type SessionServiceOption func(*sessionService)

// NewSessionService returns a new SessionServiceInterface with default dependencies.
func NewSessionService(opts ...SessionServiceOption) SessionServiceInterface {
	svc := &sessionService{
		sessionRepo: repositories.NewVerificationSessionRepo(),
		checkRepo:   repositories.NewBiometricCheckRepo(),
		auditRepo:   repositories.NewAuditRepo(),
		face:        ActiveFace(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func ConfigureSessionRepo(r repositories.VerificationSessionRepoInterface) SessionServiceOption {
	return func(s *sessionService) { s.sessionRepo = r }
}

func ConfigureSessionCheckRepo(r repositories.BiometricCheckRepoInterface) SessionServiceOption {
	return func(s *sessionService) { s.checkRepo = r }
}

func ConfigureSessionAuditRepo(r repositories.AuditRepoInterface) SessionServiceOption {
	return func(s *sessionService) { s.auditRepo = r }
}

func (svc *sessionService) CreateSession(ctx context.Context, db *gorm.DB, params *CreateSessionParams) (*CreateSessionResult, ServiceErrorInterface) {
	provider := string(config.Get().Provider)

	// 1. Create provider liveness session.
	providerSession, err := svc.face.CreateLivenessSession(ctx, params.UserID)
	if err != nil {
		return nil, ErrBadGateway("create liveness session: " + err.Error())
	}

	// 2. Persist verification session.
	session, err := svc.sessionRepo.Create(db, &models.VerificationSession{
		UserID:            params.UserID,
		Provider:          provider,
		ProviderSessionID: providerSession.ProviderSessionID,
		Status:            models.SessionStatusPending,
		DecisionStatus:    models.DecisionStatusPending,
	})
	if err != nil {
		return nil, ErrInternalServer("save session: " + err.Error())
	}

	// 3. Create the liveness biometric check record.
	_, err = svc.checkRepo.Create(db, &models.BiometricCheck{
		SessionID:     session.SessionID,
		UserID:        params.UserID,
		EntityType:    models.EntityTypeLiveness,
		Status:        models.CheckStatusPending,
		AttemptNumber: 1,
	})
	if err != nil {
		return nil, ErrInternalServer("save biometric check: " + err.Error())
	}

	// 4. Audit log (best-effort).
	details, _ := json.Marshal(map[string]any{
		"provider":          provider,
		"providerSessionId": providerSession.ProviderSessionID,
	})
	_ = svc.auditRepo.Create(db, &models.AuditLog{
		UserID:    params.UserID,
		Action:    "liveness_session_created",
		SessionID: session.SessionID,
		Details:   details,
	})

	return &CreateSessionResult{
		SessionID:         session.SessionID,
		ProviderSessionID: providerSession.ProviderSessionID,
		AuthToken:         providerSession.AuthToken,
		Provider:          provider,
		UserID:            params.UserID,
	}, nil
}
