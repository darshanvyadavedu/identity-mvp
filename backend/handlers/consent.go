package handlers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"user-authentication/db"
	"user-authentication/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/julienschmidt/httprouter"
)

// POST /api/sessions/:sessionId/consent
// Header: X-User-ID: <uuid>
// Body:   { "fields": ["first_name", "last_name", "dob", "doc_number", "expiry_date", "issuing_country", "address"] }
//
// 1. Validate session is verified.
// 2. Duplicate check: HMAC of doc_number across accounts.
// 3. Biometric duplicate check: SearchFacesByImage in Rekognition collection.
// 4. Store consent_records + encrypted verified_data per consented field.
// 5. Store identity_hashes (doc_number HMAC + first_name_dob HMAC + face_id).
// 6. IndexFaces to enroll biometric in collection.
func StoreConsent(rekClient *rekognition.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		internalSessionID := ps.ByName("sessionId")

		var body struct {
			Fields []string `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if len(body.Fields) == 0 {
			http.Error(w, "at least one field must be consented", http.StatusBadRequest)
			return
		}

		// 1. Load and validate session.
		var session models.VerificationSession
		if err := db.DB.Where("session_id = ? AND user_id = ?", internalSessionID, userID).
			First(&session).Error; err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		if session.DecisionStatus != "verified" {
			http.Error(w, fmt.Sprintf("session is not verified (current: %s)", session.DecisionStatus), http.StatusBadRequest)
			return
		}

		// 2. Load latest doc_scan result.
		var docCheck models.BiometricCheck
		if err := db.DB.Where("session_id = ? AND check_type = ?", internalSessionID, "doc_scan").
			Order("attempt_number DESC").First(&docCheck).Error; err != nil {
			http.Error(w, "doc scan check not found", http.StatusInternalServerError)
			return
		}
		var docScan models.DocumentScanResult
		if err := db.DB.Where("check_id = ?", docCheck.CheckID).First(&docScan).Error; err != nil {
			http.Error(w, "doc scan result not found", http.StatusInternalServerError)
			return
		}

		// 3. Reconstruct field→value map from extracted fields.
		var extracted DocumentData
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

		hmacSecret := os.Getenv("HMAC_SECRET")
		encKey := os.Getenv("ENCRYPTION_KEY")
		collectionID := os.Getenv("REKOGNITION_COLLECTION_ID")
		if collectionID == "" {
			collectionID = "identity-verification"
		}

		// 4. Document duplicate check via doc_number HMAC.
		docNumber := extracted.IDNumber
		if docNumber != "" {
			docHash := computeHMAC(docNumber, hmacSecret)
			var existing models.IdentityHash
			err := db.DB.Where("field_name = ? AND hash_value = ?", "doc_number", docHash).
				First(&existing).Error
			if err == nil {
				if existing.UserID != userID {
					WriteJSON(w, http.StatusConflict, map[string]any{
						"duplicate": true,
						"message":   "This document has already been used to verify another account.",
					})
					return
				}
				// Same user re-verifying — clean up old identity data.
				db.DB.Where("user_id = ?", userID).Delete(&models.IdentityHash{})
				db.DB.Where("user_id = ?", userID).Delete(&models.ConsentRecord{})
				db.DB.Where("user_id = ?", userID).Delete(&models.VerifiedData{})
			}
		}

		// 5. Load liveness reference image for biometric dedup.
		var livenessCheck models.BiometricCheck
		if err := db.DB.Where("session_id = ? AND check_type = ?", internalSessionID, "liveness").
			First(&livenessCheck).Error; err != nil {
			http.Error(w, "liveness check not found", http.StatusInternalServerError)
			return
		}
		var livenessResult models.LivenessResult
		if err := db.DB.Where("check_id = ?", livenessCheck.CheckID).First(&livenessResult).Error; err != nil {
			http.Error(w, "liveness result not found", http.StatusInternalServerError)
			return
		}
		refBytes, err := dataURLToBytes(livenessResult.ReferenceImage)
		if err != nil {
			http.Error(w, "decode reference image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		// 6. Biometric duplicate check — SearchFacesByImage.
		var existingFaceID string
		searchOut, searchErr := rekClient.SearchFacesByImage(ctx, &rekognition.SearchFacesByImageInput{
			CollectionId:       aws.String(collectionID),
			Image:              &rektypes.Image{Bytes: refBytes},
			FaceMatchThreshold: aws.Float32(95.0),
			MaxFaces:           aws.Int32(1),
		})
		if searchErr == nil && len(searchOut.FaceMatches) > 0 {
			match := searchOut.FaceMatches[0]
			matchedUserID := aws.ToString(match.Face.ExternalImageId)
			if matchedUserID != userID {
				WriteJSON(w, http.StatusConflict, map[string]any{
					"duplicate": true,
					"message":   "This face has already been used to verify another account.",
				})
				return
			}
			// Same user re-verifying — delete old face from collection before re-enrolling.
			existingFaceID = aws.ToString(match.Face.FaceId)
			rekClient.DeleteFaces(ctx, &rekognition.DeleteFacesInput{ //nolint
				CollectionId: aws.String(collectionID),
				FaceIds:      []string{existingFaceID},
			})
		}

		// 7. Store consent_records + encrypted verified_data.
		for _, fieldName := range body.Fields {
			value, ok := fieldValues[fieldName]
			if !ok || value == "" {
				continue
			}

			consent := models.ConsentRecord{
				UserID:    userID,
				SessionID: internalSessionID,
				FieldName: fieldName,
				Consented: true,
			}
			if err := db.DB.Create(&consent).Error; err != nil {
				http.Error(w, fmt.Sprintf("store consent for %s: %v", fieldName, err), http.StatusInternalServerError)
				return
			}

			cipherB64, ivB64, err := encryptAESGCM(value, encKey)
			if err != nil {
				http.Error(w, fmt.Sprintf("encrypt %s: %v", fieldName, err), http.StatusInternalServerError)
				return
			}

			vd := models.VerifiedData{
				UserID:         userID,
				SessionID:      internalSessionID,
				ConsentID:      consent.ConsentID,
				FieldName:      fieldName,
				EncryptedValue: cipherB64,
				EncryptionIV:   ivB64,
			}
			if err := db.DB.Create(&vd).Error; err != nil {
				http.Error(w, fmt.Sprintf("store verified data for %s: %v", fieldName, err), http.StatusInternalServerError)
				return
			}
		}

		// 8. Store identity hashes.
		if docNumber != "" {
			docHash := computeHMAC(docNumber, hmacSecret)
			db.DB.Create(&models.IdentityHash{
				UserID:    userID,
				FieldName: "doc_number",
				HashValue: docHash,
				HashAlgo:  "hmac-sha256",
			})
		}
		if extracted.FirstName != "" && extracted.DOB != "" {
			combo := extracted.FirstName + "|" + extracted.DOB
			db.DB.Create(&models.IdentityHash{
				UserID:    userID,
				FieldName: "first_name_dob",
				HashValue: computeHMAC(combo, hmacSecret),
				HashAlgo:  "hmac-sha256",
			})
		}

		// 9. Enroll face in Rekognition collection — IndexFaces.
		indexOut, indexErr := rekClient.IndexFaces(ctx, &rekognition.IndexFacesInput{
			CollectionId:        aws.String(collectionID),
			Image:               &rektypes.Image{Bytes: refBytes},
			ExternalImageId:     aws.String(userID),
			MaxFaces:            aws.Int32(1),
			DetectionAttributes: []rektypes.Attribute{},
		})
		if indexErr == nil && len(indexOut.FaceRecords) > 0 {
			faceID := aws.ToString(indexOut.FaceRecords[0].Face.FaceId)
			db.DB.Create(&models.IdentityHash{
				UserID:    userID,
				FieldName: "face_id",
				HashValue: faceID,
				HashAlgo:  "rekognition_collection",
			})
		}

		// 10. Audit log.
		writeAudit(userID, "consent_stored", internalSessionID, map[string]any{
			"fields": body.Fields,
		})

		WriteJSON(w, http.StatusOK, map[string]any{"stored": true})
	}
}

// encryptAESGCM encrypts plaintext with AES-256-GCM using the hex-encoded key.
// Returns base64-encoded ciphertext and nonce.
func encryptAESGCM(plaintext, keyHex string) (cipherB64, ivB64 string, err error) {
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil || len(keyBytes) != 32 {
		return "", "", fmt.Errorf("ENCRYPTION_KEY must be 64 hex chars (32 bytes)")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(nonce),
		nil
}
