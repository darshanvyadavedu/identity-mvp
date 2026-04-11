package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"user-authentication/db"
	"user-authentication/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/julienschmidt/httprouter"
)

// POST /api/sessions
// Header: X-User-ID: <uuid>
// Creates an AWS Rekognition liveness session and persists it to the DB.
func CreateSession(rekClient *rekognition.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// 1. Create AWS liveness session.
		out, err := rekClient.CreateFaceLivenessSession(ctx, &rekognition.CreateFaceLivenessSessionInput{})
		if err != nil {
			http.Error(w, fmt.Sprintf("create liveness session: %v", err), http.StatusBadGateway)
			return
		}
		awsSessionID := aws.ToString(out.SessionId)

		// 2. Persist verification session.
		expires := time.Now().Add(10 * time.Minute)
		session := models.VerificationSession{
			UserID:            userID,
			Provider:          "aws",
			ProviderSessionID: awsSessionID,
			Status:            "pending",
			DecisionStatus:    "pending",
			ExpiresAt:         &expires,
		}
		if err := db.DB.Create(&session).Error; err != nil {
			http.Error(w, fmt.Sprintf("save session: %v", err), http.StatusInternalServerError)
			return
		}

		// 3. Create the liveness biometric check record.
		now := time.Now()
		check := models.BiometricCheck{
			SessionID:   session.SessionID,
			UserID:      userID,
			CheckType:   "liveness",
			Status:      "pending",
			AttemptedAt: &now,
		}
		if err := db.DB.Create(&check).Error; err != nil {
			http.Error(w, fmt.Sprintf("save biometric check: %v", err), http.StatusInternalServerError)
			return
		}

		// 4. Audit log.
		writeAudit(userID, "liveness_session_created", session.SessionID, map[string]any{
			"provider":          "aws",
			"providerSessionId": awsSessionID,
		})

		WriteJSON(w, http.StatusCreated, map[string]string{
			"sessionId":         session.SessionID,
			"providerSessionId": awsSessionID,
			"userId":            userID,
		})
	}
}

// writeAudit inserts an audit log row (best-effort — never blocks the response).
func writeAudit(userID, action, sessionID string, details map[string]any) {
	raw, _ := json.Marshal(details)
	entry := models.AuditLog{
		UserID:    userID,
		Action:    action,
		SessionID: sessionID,
		Details:   raw,
	}
	db.DB.Create(&entry) // ignore error — audit failure should not fail the request
}
