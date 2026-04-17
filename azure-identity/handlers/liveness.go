package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"time"

	"azure-identity/azure"
	"azure-identity/store"

	"github.com/julienschmidt/httprouter"
)

// GET /api/sessions/:sessionId/result
// Header: X-User-ID: <uuid>
func GetLivenessResult(face *azure.FaceClient, st *store.Store) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		internalID := ps.ByName("sessionId")
		sess, ok := st.GetSession(internalID)
		if !ok || sess.UserID != userID {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// 1. Poll Azure for the liveness result.
		result, err := face.GetLivenessSessionResult(ctx, sess.AzureSessionID)
		if err != nil {
			http.Error(w, "get liveness result: "+err.Error(), http.StatusBadGateway)
			return
		}
		fmt.Println(result)

		if result.Status != "Succeeded" && result.Status != "ResultAvailable" {
			WriteJSON(w, http.StatusOK, map[string]any{
				"sessionId":      internalID,
				"livenessStatus": result.Status,
			})
			return
		}

		// 2. Parse verdict from latest attempt.
		var verdict string
		var sessionImageID string
		attempts := result.Results.Attempts
		if len(attempts) > 0 {
			latest := attempts[len(attempts)-1]
			if latest.Result != nil {
				sessionImageID = latest.Result.SessionImageID
				if latest.Result.LivenessDecision == "realface" {
					verdict = "succeeded"
				} else {
					verdict = "failed"
				}
			}
			if latest.Error != nil {
				verdict = "failed"
			}
		}

		// 3. Update session status.
		sessionStatus := "liveness_failed"
		decisionStatus := "failed"
		if verdict == "succeeded" {
			sessionStatus = "liveness_passed"
			decisionStatus = "pending"
		}
		st.UpdateSession(internalID, sessionStatus, decisionStatus)

		// 4. Save liveness verdict immediately so documents.go can find it.
		st.SaveLivenessResult(&store.LivenessResult{
			SessionID:      internalID,
			Verdict:        verdict,
			SessionImageID: sessionImageID,
		})

		// 5. Download the captured face image (only when liveness passed).
		//    Endpoint: GET /face/v1.2/sessionImages/{sessionImageId}
		if verdict == "succeeded" && sessionImageID != "" {
			log.Printf("downloading session image: imageID=%s", sessionImageID)
			imgBytes, imgErr := face.GetSessionImage(ctx, sessionImageID)
			if imgErr != nil {
				log.Printf("warn: could not download session image: %v", imgErr)
			} else {
				log.Printf("session image downloaded: %d bytes", len(imgBytes))
				lr, ok := st.GetLivenessResult(internalID)
				if ok {
					lr.SessionImageBytes = imgBytes
				}
			}
		}

		// Include liveness image as base64 if available.
		var livenessImageB64 string
		if lr, ok := st.GetLivenessResult(internalID); ok && len(lr.SessionImageBytes) > 0 {
			livenessImageB64 = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(lr.SessionImageBytes)
		}

		WriteJSON(w, http.StatusOK, map[string]any{
			"sessionId":      internalID,
			"livenessStatus": verdict,
			"livenessImage":  livenessImageB64,
		})
	}
}

// GET /api/sessions/:sessionId/liveness-image
// Returns the captured liveness face image as JPEG for inspection.
func GetLivenessImage(st *store.Store) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		internalID := ps.ByName("sessionId")
		lr, ok := st.GetLivenessResult(internalID)
		if !ok || len(lr.SessionImageBytes) == 0 {
			http.Error(w, "liveness image not available", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(lr.SessionImageBytes)
	}
}
