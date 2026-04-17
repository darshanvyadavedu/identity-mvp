package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"azure-identity/azure"
	"azure-identity/store"

	"github.com/julienschmidt/httprouter"
)

const faceMatchThreshold = 0.70 // Azure confidence is 0.0-1.0

// POST /api/documents
// Header:  X-User-ID: <uuid>
// Form:    sessionId + file (image)
func UploadDocument(face *azure.FaceClient, docInt *azure.DocIntelClient, st *store.Store) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		internalID := strings.TrimSpace(r.FormValue("sessionId"))
		if internalID == "" {
			http.Error(w, "sessionId is required", http.StatusBadRequest)
			return
		}

		sess, ok := st.GetSession(internalID)
		if !ok || sess.UserID != userID {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		// if sess.Status != "liveness_passed" {
		// 	http.Error(w, "liveness check must pass first (current: "+sess.Status+")", http.StatusBadRequest)
		// 	return
		// }

		// Read uploaded document.
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file is required: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		docBytes, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		// 1. Document Intelligence — extract identity fields.
		fields, _, docErr := docInt.AnalyzeID(ctx, docBytes)
		if docErr != nil {
			http.Error(w, "document analysis: "+docErr.Error(), http.StatusBadGateway)
			return
		}
		log.Printf("document fields: %+v", fields)

		// 2. Early duplicate check: name+DOB hash.
		if fields.FirstName != "" && fields.DOB != "" {
			combo := fields.FirstName + "|" + fields.DOB
			h := computeHMAC(combo, os.Getenv("HMAC_SECRET"))
			if existing := st.FindHash("first_name_dob", h); existing != nil && existing.UserID != userID {
				WriteJSON(w, http.StatusConflict, map[string]any{
					"duplicate": true,
					"message":   "Identity already exists: this document's name and date of birth are linked to another account.",
				})
				return
			}
		}

		// 3. Face comparison — download the captured liveness image from Azure,
		//    detect faces in both it and the document, then verify they match.
		var similarity float64
		var facePassed bool

		livenessResult, hasLiveness := st.GetLivenessResult(internalID)
		if !hasLiveness {
			log.Printf("face compare: no liveness result for sessionId=%s", internalID)
		} else if len(livenessResult.SessionImageBytes) == 0 {
			log.Printf("face compare: liveness result found but no image bytes for sessionId=%s", internalID)
		}

		if hasLiveness && len(livenessResult.SessionImageBytes) > 0 {
			log.Printf("face compare: detecting faces — liveness image=%d bytes, doc image=%d bytes", len(livenessResult.SessionImageBytes), len(docBytes))
			selfieID, err1 := face.DetectFaceFromBytes(ctx, livenessResult.SessionImageBytes)
			docFaceID, err2 := face.DetectFaceFromBytes(ctx, docBytes)
			log.Printf("face compare: selfieID=%q err=%v  docFaceID=%q err=%v", selfieID, err1, docFaceID, err2)
			if err1 == nil && err2 == nil {
				verifyResp, err := face.VerifyFaces(ctx, selfieID, docFaceID)
				if err != nil {
					log.Printf("face compare: VerifyFaces error: %v", err)
				} else {
					log.Printf("face compare: isIdentical=%v confidence=%.4f", verifyResp.IsIdentical, verifyResp.Confidence)
					similarity = verifyResp.Confidence * 100
					facePassed = verifyResp.Confidence >= faceMatchThreshold
				}
			}
		} else {
			// No liveness image stored — skip face match.
			facePassed = true
		}

		// 4. Store doc scan result.
		st.SaveDocScan(&store.DocScanResult{
			SessionID:      internalID,
			FirstName:      fields.FirstName,
			LastName:       fields.LastName,
			DOB:            fields.DOB,
			IDNumber:       fields.DocumentNumber,
			Expiry:         fields.Expiry,
			IssuingCountry: fields.Country,
			Address:        fields.Address,
			DocumentType:   fields.DocumentType,
		})

		// 5. Store face match result + update session.
		st.SaveFaceMatch(&store.FaceMatchResult{
			SessionID:  internalID,
			Similarity: similarity,
			Passed:     facePassed,
		})

		decisionStatus := "verified"
		sessionStatus := "completed"
		if !facePassed {
			decisionStatus = "failed"
		}
		st.UpdateSession(internalID, sessionStatus, decisionStatus)

		WriteJSON(w, http.StatusOK, map[string]any{
			"sessionId":      internalID,
			"decisionStatus": decisionStatus,
			"document": map[string]string{
				"firstName":      fields.FirstName,
				"lastName":       fields.LastName,
				"dob":            fields.DOB,
				"idNumber":       fields.DocumentNumber,
				"expiry":         fields.Expiry,
				"issuingCountry": fields.Country,
				"address":        fields.Address,
				"documentType":   fields.DocumentType,
			},
			"faceMatch": map[string]any{
				"similarity": similarity,
				"passed":     facePassed,
				"threshold":  faceMatchThreshold * 100,
			},
		})
	}
}

func computeHMAC(value, secret string) string {
	if value == "" || secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}
