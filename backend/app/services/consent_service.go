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

// StoreConsentParams holds the input for StoreConsent.
type StoreConsentParams struct {
	SessionID string
	UserID    string
	Fields    []string
}

// StoreConsentResult holds the output for StoreConsent.
type StoreConsentResult struct {
	Stored bool
}

// ConsentServiceInterface defines the contract for consent operations.
type ConsentServiceInterface interface {
	StoreConsent(ctx context.Context, db *gorm.DB, params *StoreConsentParams) (*StoreConsentResult, ServiceErrorInterface)
}

type consentService struct {
	sessionRepo repositories.VerificationSessionRepoInterface
	checkRepo   repositories.BiometricCheckRepoInterface
	consentRepo repositories.ConsentRepoInterface
	hashRepo    repositories.IdentityHashRepoInterface
	auditRepo   repositories.AuditRepoInterface
	p           provider.IdentityProvider
}

// ConsentServiceOption configures a consentService.
type ConsentServiceOption func(*consentService)

// NewConsentService returns a new ConsentServiceInterface with default dependencies.
func NewConsentService(opts ...ConsentServiceOption) ConsentServiceInterface {
	svc := &consentService{
		sessionRepo: repositories.NewVerificationSessionRepo(),
		checkRepo:   repositories.NewBiometricCheckRepo(),
		consentRepo: repositories.NewConsentRepo(),
		hashRepo:    repositories.NewIdentityHashRepo(),
		auditRepo:   repositories.NewAuditRepo(),
		p:           Active(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func ConfigureConsentSessionRepo(r repositories.VerificationSessionRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.sessionRepo = r }
}

func ConfigureConsentCheckRepo(r repositories.BiometricCheckRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.checkRepo = r }
}

func ConfigureConsentRepo(r repositories.ConsentRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.consentRepo = r }
}

func ConfigureConsentHashRepo(r repositories.IdentityHashRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.hashRepo = r }
}

func ConfigureConsentAuditRepo(r repositories.AuditRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.auditRepo = r }
}

func (svc *consentService) StoreConsent(ctx context.Context, db *gorm.DB, params *StoreConsentParams) (*StoreConsentResult, ServiceErrorInterface) {
	cfg := config.Get()
	collectionID := cfg.CollectionID()

	// 1. Load and validate session.
	session, err := svc.sessionRepo.GetBySessionAndUser(db, params.SessionID, params.UserID)
	if err != nil {
		return nil, ErrNotFound("session not found")
	}
	if session.DecisionStatus != models.DecisionStatusVerified {
		return nil, ErrBadRequest(fmt.Sprintf("session is not verified (current: %s)", session.DecisionStatus))
	}

	// 2. Load doc scan entity value from biometric check.
	docCheck, err := svc.checkRepo.GetLatestBySessionAndType(db, params.SessionID, models.EntityTypeDocScan)
	if err != nil {
		return nil, ErrInternalServer("doc scan check not found")
	}
	var extracted provider.DocumentData
	if len(docCheck.EntityValue) > 0 {
		_ = json.Unmarshal(docCheck.EntityValue, &extracted)
	}

	fieldValues := map[string]string{
		"first_name":      extracted.FirstName,
		"last_name":       extracted.LastName,
		"dob":             extracted.DOB,
		"doc_number":      extracted.IDNumber,
		"expiry_date":     extracted.Expiry,
		"issuing_country": extracted.IssuingCountry,
		"address":         extracted.Address,
	}

	// 3. Identity duplicate check via first_name+DOB blind index.
	// Relies on the person, not the document — prevents the same person verifying with different documents.
	if extracted.FirstName != "" && extracted.DOB != "" {
		combo := extracted.FirstName + "|" + extracted.DOB
		blindIdx := computeHMAC(combo, cfg.HMACSecret)
		existing, findErr := svc.hashRepo.FindByFieldAndBlindIndex(db, "first_name_dob", blindIdx)
		if findErr == nil && existing != nil {
			if existing.UserID != params.UserID {
				return nil, ErrConflict("An account is already verified with this identity.")
			}
			// Same user re-verifying — clean up old identity data.
			_ = svc.hashRepo.DeleteByUser(db, params.UserID)
			_ = svc.consentRepo.DeleteConsentByUser(db, params.UserID)
			_ = svc.consentRepo.DeleteVerifiedDataByUser(db, params.UserID)
		}
	}

	// 4. Load liveness reference image from biometric check.
	livenessCheck, err := svc.checkRepo.GetBySessionAndType(db, params.SessionID, models.EntityTypeLiveness)
	if err != nil {
		return nil, ErrInternalServer("liveness check not found")
	}
	refBytes, err := dataURLToBytes(livenessCheck.ReferenceImage)
	if err != nil {
		return nil, ErrInternalServer("decode reference image: " + err.Error())
	}

	// 5. Biometric duplicate check — SearchFacesByImage.
	searchResult, searchErr := svc.p.SearchFacesByImage(ctx, refBytes, collectionID)
	if searchErr == nil && searchResult != nil && searchResult.Found {
		if searchResult.MatchedUserID != params.UserID {
			return nil, ErrConflict("This face has already been used to verify another account.")
		}
		// Same user re-verifying — delete old face before re-enrolling.
		_ = svc.p.DeleteFace(ctx, collectionID, searchResult.FaceID)
	}

	// 6. Store consent_records + encrypted verified_data.
	for _, fieldName := range params.Fields {
		value, ok := fieldValues[fieldName]
		if !ok || value == "" {
			continue
		}

		consent, err := svc.consentRepo.CreateConsent(db, &models.ConsentRecord{
			UserID:    params.UserID,
			SessionID: params.SessionID,
			FieldName: fieldName,
			Consented: true,
		})
		if err != nil {
			return nil, ErrInternalServer(fmt.Sprintf("store consent for %s: %v", fieldName, err))
		}

		cipherB64, ivB64, encErr := encryptAESGCM(value, cfg.EncryptionKey)
		if encErr != nil {
			return nil, ErrInternalServer(fmt.Sprintf("encrypt %s: %v", fieldName, encErr))
		}

		_, err = svc.consentRepo.CreateVerifiedData(db, &models.VerifiedData{
			UserID:         params.UserID,
			SessionID:      params.SessionID,
			ConsentID:      consent.ConsentID,
			FieldName:      fieldName,
			EncryptedValue: cipherB64,
			EncryptionIV:   ivB64,
		})
		if err != nil {
			return nil, ErrInternalServer(fmt.Sprintf("store verified data for %s: %v", fieldName, err))
		}
	}

	// 7. Store identity hash — first_name+DOB anchors identity to the person, not the document.
	// hash_value  = HMAC(value, userID+":"+secret) — user-specific, private
	// blind_index = HMAC(value, secret)            — global, dedup only
	if extracted.FirstName != "" && extracted.DOB != "" {
		combo := extracted.FirstName + "|" + extracted.DOB
		_ = svc.hashRepo.DeleteByUserAndField(db, params.UserID, "first_name_dob")
		_ = svc.hashRepo.Create(db, &models.IdentityHash{
			UserID:     params.UserID,
			FieldName:  "first_name_dob",
			HashValue:  computeHMAC(combo, params.UserID+":"+cfg.HMACSecret),
			BlindIndex: computeHMAC(combo, cfg.HMACSecret),
			HashAlgo:   "hmac-sha256",
		})
	}

	// 8. Enroll face in collection/FaceList.
	_, _ = svc.p.IndexFace(ctx, refBytes, collectionID, params.UserID)

	// 9. Audit log (best-effort).
	details, _ := json.Marshal(map[string]any{
		"fields": params.Fields,
	})
	_ = svc.auditRepo.Create(db, &models.AuditLog{
		UserID:    params.UserID,
		Action:    "consent_stored",
		SessionID: params.SessionID,
		Details:   details,
	})

	return &StoreConsentResult{Stored: true}, nil
}
