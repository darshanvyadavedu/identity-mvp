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
	sessionRepo  repositories.VerificationSessionRepoInterface
	checkRepo    repositories.BiometricCheckRepoInterface
	livenessRepo repositories.LivenessResultRepoInterface
	docScanRepo  repositories.DocumentScanRepoInterface
	consentRepo  repositories.ConsentRepoInterface
	hashRepo     repositories.IdentityHashRepoInterface
	auditRepo    repositories.AuditRepoInterface
	faceClient   clients.FaceClientInterface
}

// ConsentServiceOption configures a consentService.
type ConsentServiceOption func(*consentService)

// NewConsentService returns a new ConsentServiceInterface with default dependencies.
func NewConsentService(opts ...ConsentServiceOption) ConsentServiceInterface {
	svc := &consentService{
		sessionRepo:  repositories.NewVerificationSessionRepo(),
		checkRepo:    repositories.NewBiometricCheckRepo(),
		livenessRepo: repositories.NewLivenessResultRepo(),
		docScanRepo:  repositories.NewDocumentScanRepo(),
		consentRepo:  repositories.NewConsentRepo(),
		hashRepo:     repositories.NewIdentityHashRepo(),
		auditRepo:    repositories.NewAuditRepo(),
		faceClient:   newFaceClient(),
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

func ConfigureConsentLivenessRepo(r repositories.LivenessResultRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.livenessRepo = r }
}

func ConfigureConsentDocScanRepo(r repositories.DocumentScanRepoInterface) ConsentServiceOption {
	return func(s *consentService) { s.docScanRepo = r }
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

func ConfigureConsentFaceClient(c clients.FaceClientInterface) ConsentServiceOption {
	return func(s *consentService) { s.faceClient = c }
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

	// 2. Load latest doc_scan result.
	docCheck, err := svc.checkRepo.GetLatestBySessionAndType(db, params.SessionID, models.CheckTypeDocScan)
	if err != nil {
		return nil, ErrInternalServer("doc scan check not found")
	}
	docScan, err := svc.docScanRepo.GetByCheckID(db, docCheck.CheckID)
	if err != nil {
		return nil, ErrInternalServer("doc scan result not found")
	}

	// 3. Reconstruct field→value map from extracted fields.
	var extracted models.DocumentData
	if len(docScan.ExtractedFields) > 0 {
		_ = json.Unmarshal(docScan.ExtractedFields, &extracted)
	}
	if docScan.IssuingCountry != "" {
		extracted.IssuingCountry = docScan.IssuingCountry
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

	// 4. Document duplicate check via doc_number HMAC.
	docNumber := extracted.IDNumber
	if docNumber != "" {
		docHash := computeHMAC(docNumber, cfg.HMACSecret)
		existing, findErr := svc.hashRepo.FindByFieldAndHash(db, "doc_number", docHash)
		if findErr == nil && existing != nil {
			if existing.UserID != params.UserID {
				return nil, ErrConflict("This document has already been used to verify another account.")
			}
			// Same user re-verifying — clean up old identity data.
			_ = svc.hashRepo.DeleteByUser(db, params.UserID)
			_ = svc.consentRepo.DeleteConsentByUser(db, params.UserID)
			_ = svc.consentRepo.DeleteVerifiedDataByUser(db, params.UserID)
		}
	}

	// 5. Load liveness reference image for biometric dedup.
	livenessCheck, err := svc.checkRepo.GetBySessionAndType(db, params.SessionID, models.CheckTypeLiveness)
	if err != nil {
		return nil, ErrInternalServer("liveness check not found")
	}
	livenessResult, err := svc.livenessRepo.GetByCheckID(db, livenessCheck.CheckID)
	if err != nil {
		return nil, ErrInternalServer("liveness result not found")
	}
	refBytes, err := dataURLToBytes(livenessResult.ReferenceImage)
	if err != nil {
		return nil, ErrInternalServer("decode reference image: " + err.Error())
	}

	// 6. Biometric duplicate check — SearchFacesByImage.
	searchResult, searchErr := svc.faceClient.SearchFacesByImage(ctx, refBytes, collectionID)
	if searchErr == nil && searchResult != nil && searchResult.Found {
		if searchResult.MatchedUserID != params.UserID {
			return nil, ErrConflict("This face has already been used to verify another account.")
		}
		// Same user re-verifying — delete old face before re-enrolling.
		_ = svc.faceClient.DeleteFace(ctx, collectionID, searchResult.FaceID)
	}

	// 7. Store consent_records + encrypted verified_data.
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

	// 8. Store identity hashes.
	if docNumber != "" {
		docHash := computeHMAC(docNumber, cfg.HMACSecret)
		_ = svc.hashRepo.Create(db, &models.IdentityHash{
			UserID:    params.UserID,
			FieldName: "doc_number",
			HashValue: docHash,
			HashAlgo:  "hmac-sha256",
		})
	}
	if extracted.FirstName != "" && extracted.DOB != "" {
		combo := extracted.FirstName + "|" + extracted.DOB
		hash := computeHMAC(combo, cfg.HMACSecret)
		_ = svc.hashRepo.DeleteByUserAndField(db, params.UserID, "first_name_dob")
		_ = svc.hashRepo.Create(db, &models.IdentityHash{
			UserID:    params.UserID,
			FieldName: "first_name_dob",
			HashValue: hash,
			HashAlgo:  "hmac-sha256",
		})
	}

	// 9. Enroll face in collection/FaceList.
	faceID, indexErr := svc.faceClient.IndexFace(ctx, refBytes, collectionID, params.UserID)
	if indexErr == nil && faceID != "" {
		algo := "rekognition_collection"
		if cfg.Provider == config.ProviderAzure {
			algo = "azure_face_list"
		}
		_ = svc.hashRepo.Create(db, &models.IdentityHash{
			UserID:    params.UserID,
			FieldName: "face_id",
			HashValue: faceID,
			HashAlgo:  algo,
		})
	}

	// 10. Audit log (best-effort).
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
