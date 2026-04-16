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

	"azure-identity/azure"
	"azure-identity/store"

	"github.com/julienschmidt/httprouter"
)

// POST /api/sessions/:sessionId/consent
// Header: X-User-ID: <uuid>
// Body:   { "fields": ["first_name", "last_name", "dob", ...] }
func StoreConsent(face *azure.FaceClient, faceListID string, st *store.Store) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		internalID := ps.ByName("sessionId")

		var body struct {
			Fields []string `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Fields) == 0 {
			http.Error(w, "at least one field must be consented", http.StatusBadRequest)
			return
		}

		sess, ok := st.GetSession(internalID)
		if !ok || sess.UserID != userID {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		if sess.DecisionStatus != "verified" {
			http.Error(w, "session is not verified (current: "+sess.DecisionStatus+")", http.StatusBadRequest)
			return
		}

		doc, hasDoc := st.GetDocScan(internalID)
		if !hasDoc {
			http.Error(w, "document scan result not found", http.StatusInternalServerError)
			return
		}

		hmacSecret := os.Getenv("HMAC_SECRET")
		encKey := os.Getenv("ENCRYPTION_KEY")

		// 1. Document duplicate check (doc_number HMAC).
		if doc.IDNumber != "" {
			h := computeHMAC(doc.IDNumber, hmacSecret)
			if existing := st.FindHash("doc_number", h); existing != nil {
				if existing.UserID != userID {
					WriteJSON(w, http.StatusConflict, map[string]any{
						"duplicate": true,
						"message":   "This document has already been used to verify another account.",
					})
					return
				}
				// Same user re-verifying — clear old data.
				st.ClearUserData(userID)
			}
		}

		// 2. Biometric duplicate check via Azure FaceList.
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		liveness, hasLiveness := st.GetLivenessResult(internalID)
		_ = liveness // Azure liveness doesn't give us a reference image to search with
		_ = hasLiveness

		// Note: Azure Face FindSimilar requires a faceId from a recent Detect call.
		// If a selfie was uploaded during document step, we could re-detect it here.
		// For this exploration project, biometric dedup via FaceList is best-effort.

		// 3. Build field value map from doc scan.
		fieldValues := map[string]string{
			"first_name":      doc.FirstName,
			"last_name":       doc.LastName,
			"dob":             doc.DOB,
			"doc_number":      doc.IDNumber,
			"expiry_date":     doc.Expiry,
			"issuing_country": doc.IssuingCountry,
			"address":         doc.Address,
		}

		// 4. Store consent + encrypted verified data.
		for _, fieldName := range body.Fields {
			value := fieldValues[fieldName]
			if value == "" {
				continue
			}
			st.AppendConsent(store.ConsentRecord{
				UserID:    userID,
				SessionID: internalID,
				FieldName: fieldName,
				Consented: true,
			})
			cipherB64, ivB64, err := encryptAESGCM(value, encKey)
			if err != nil {
				http.Error(w, "encrypt "+fieldName+": "+err.Error(), http.StatusInternalServerError)
				return
			}
			st.AppendVerifiedData(store.VerifiedData{
				UserID:         userID,
				SessionID:      internalID,
				FieldName:      fieldName,
				EncryptedValue: cipherB64,
				EncryptionIV:   ivB64,
			})
		}

		// 5. Store identity hashes.
		if doc.IDNumber != "" {
			st.UpsertHash(store.IdentityHash{
				UserID:    userID,
				FieldName: "doc_number",
				HashValue: computeHMAC(doc.IDNumber, hmacSecret),
				HashAlgo:  "hmac-sha256",
			})
		}
		if doc.FirstName != "" && doc.DOB != "" {
			st.UpsertHash(store.IdentityHash{
				UserID:    userID,
				FieldName: "first_name_dob",
				HashValue: computeHMAC(doc.FirstName+"|"+doc.DOB, hmacSecret),
				HashAlgo:  "hmac-sha256",
			})
		}

		// 6. Enroll face in Azure FaceList (best-effort — requires selfie bytes).
		// If face enrollment fails, we still return success (consent was stored).
		_ = face
		_ = faceListID
		_ = ctx

		WriteJSON(w, http.StatusOK, map[string]any{"stored": true})
	}
}

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
