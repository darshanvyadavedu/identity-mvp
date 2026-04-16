package handlers

import (
	"context"
	"fmt"
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

		if result.Status != "ResultAvailable" {
			WriteJSON(w, http.StatusOK, map[string]any{
				"sessionId":      internalID,
				"livenessStatus": result.Status,
			})
			return
		}

		// 2. Parse verdict from latest attempt.
		var verdict string
		var confidence float64
		attempts := result.Results.Attempts
		if len(attempts) > 0 {
			latest := attempts[len(attempts)-1]
			if latest.Result != nil {
				if latest.Result.LivenessDecision == "realface" {
					verdict = "succeeded"
					confidence = 95.0 // Azure doesn't return a confidence score directly
				} else {
					verdict = "failed"
					confidence = 0
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

		// 4. Store liveness result (no reference image from Azure liveness — will use uploaded doc photo for face compare).
		st.SaveLivenessResult(&store.LivenessResult{
			SessionID:  internalID,
			Verdict:    verdict,
			Confidence: confidence,
		})

		WriteJSON(w, http.StatusOK, map[string]any{
			"sessionId":          internalID,
			"livenessStatus":     verdict,
			"livenessConfidence": confidence,
		})
	}
}
