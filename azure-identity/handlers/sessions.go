package handlers

import (
	"context"
	"net/http"
	"time"

	"azure-identity/azure"
	"azure-identity/store"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
)

// POST /api/sessions
// Header: X-User-ID: <uuid>
func CreateSession(face *azure.FaceClient, st *store.Store) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// 1. Create Azure liveness session.
		azureSession, err := face.CreateLivenessSession(ctx, userID)
		if err != nil {
			http.Error(w, "create liveness session: "+err.Error(), http.StatusBadGateway)
			return
		}

		// 2. Store in memory.
		internalID := uuid.NewString()
		st.SaveSession(&store.Session{
			InternalID:     internalID,
			AzureSessionID: azureSession.SessionID,
			UserID:         userID,
			Status:         "pending",
			DecisionStatus: "pending",
			CreatedAt:      time.Now(),
		})

		WriteJSON(w, http.StatusCreated, map[string]string{
			"sessionId":         internalID,
			"providerSessionId": azureSession.SessionID,
			"authToken":         azureSession.AuthToken,
			"provider":          "azure",
			"userId":            userID,
		})
	}
}
