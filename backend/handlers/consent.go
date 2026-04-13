package handlers

import (
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

	"user-authentication/db"
	"user-authentication/models"

	"github.com/julienschmidt/httprouter"
)

// POST /api/sessions/:sessionId/consent
// Header: X-User-ID: <uuid>
// Body:   { "fields": ["first_name", "last_name", "dob", "doc_number", "expiry_date", "issuing_country"] }
//
// Takes user consent, checks for duplicate documents across accounts,
// stores encrypted verified_data, consent_records, and identity_hashes.
func StoreConsent() httprouter.Handle {
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

		// 2. Load the latest doc_scan biometric check for this session.
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

		// 3. Unmarshal extracted fields to build field → value map.
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
		}

		// 4. Duplicate check via doc_number HMAC.
		hmacSecret := os.Getenv("HMAC_SECRET")
		encKey := os.Getenv("ENCRYPTION_KEY")

		docNumber := extracted.IDNumber
		if docNumber != "" {
			docHash := computeHMAC(docNumber, hmacSecret)
			var existing models.IdentityHash
			err := db.DB.Where("field_name = ? AND hash_value = ?", "doc_number", docHash).
				First(&existing).Error
			if err == nil {
				// Hash found — check ownership.
				if existing.UserID != userID {
					WriteJSON(w, http.StatusConflict, map[string]any{
						"duplicate": true,
						"message":   "This document has already been used to verify another account.",
					})
					return
				}
				// Same user re-verifying — clean up old data for this user.
				db.DB.Where("user_id = ?", userID).Delete(&models.IdentityHash{})
				db.DB.Where("user_id = ?", userID).Delete(&models.ConsentRecord{})
				db.DB.Where("user_id = ?", userID).Delete(&models.VerifiedData{})
			}
		}

		// 5. Store consent_records + verified_data for each consented field.
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

		// 6. Store identity hashes.
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

		// 7. Audit log.
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
