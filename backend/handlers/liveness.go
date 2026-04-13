package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"user-authentication/db"
	"user-authentication/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/julienschmidt/httprouter"
)

// GET /api/sessions/:sessionId/result
// Header: X-User-ID: <uuid>
// Fetches the liveness result from AWS using the stored provider session ID,
// then persists liveness_results and updates biometric_checks + verification_sessions.
func GetLivenessResult(rekClient *rekognition.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		internalSessionID := ps.ByName("sessionId")

		// 1. Load the verification session to get the AWS provider session ID.
		var session models.VerificationSession
		if err := db.DB.Where("session_id = ? AND user_id = ?", internalSessionID, userID).
			First(&session).Error; err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// 2. Call AWS with the provider session ID.
		out, err := rekClient.GetFaceLivenessSessionResults(ctx, &rekognition.GetFaceLivenessSessionResultsInput{
			SessionId: aws.String(session.ProviderSessionID),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("get liveness results: %v", err), http.StatusBadGateway)
			return
		}

		// 3. Decode reference image.
		var refImage string
		if out.ReferenceImage != nil && len(out.ReferenceImage.Bytes) > 0 {
			refImage = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.ReferenceImage.Bytes)
		}

		awsStatus := strings.ToLower(string(out.Status))
		confidence := float64(aws.ToFloat32(out.Confidence))

		verdict := awsStatus // succeeded | failed
		checkStatus := "pending"
		sessionStatus := "pending"
		decisionStatus := "pending"

		switch awsStatus {
		case "succeeded":
			verdict = "live"
			checkStatus = "succeeded"
			sessionStatus = "liveness_passed"
			decisionStatus = "pending" // still waiting for doc
		case "failed":
			verdict = "failed"
			checkStatus = "failed"
			sessionStatus = "liveness_failed"
			decisionStatus = "failed"
		}

		// 4. Find the biometric check for this session.
		var check models.BiometricCheck
		db.DB.Where("session_id = ? AND check_type = ?", internalSessionID, "liveness").First(&check)

		// 5. Raw response for storage.
		rawJSON, _ := json.Marshal(out)

		// 6. Upsert liveness result (re-poll safe).
		livenessResult := models.LivenessResult{
			CheckID:         check.CheckID,
			Verdict:         verdict,
			ConfidenceScore: confidence / 100.0,
			ReferenceImage:  refImage,
			RawResponse:     rawJSON,
		}
		if err := db.DB.Where(models.LivenessResult{CheckID: check.CheckID}).
			Assign(models.LivenessResult{
				Verdict:         verdict,
				ConfidenceScore: confidence / 100.0,
				ReferenceImage:  refImage,
				RawResponse:     rawJSON,
			}).
			FirstOrCreate(&livenessResult).Error; err != nil {
			http.Error(w, fmt.Sprintf("save liveness result: %v", err), http.StatusInternalServerError)
			return
		}

		// 7. Update biometric check status.
		db.DB.Model(&check).Updates(map[string]any{"status": checkStatus})

		// 8. Update verification session.
		db.DB.Model(&session).Updates(map[string]any{
			"status":          sessionStatus,
			"decision_status": decisionStatus,
		})

		// 9. Audit log.
		writeAudit(userID, "liveness_result_fetched", internalSessionID, map[string]any{
			"verdict":    verdict,
			"confidence": confidence,
			"awsStatus":  awsStatus,
		})

		WriteJSON(w, http.StatusOK, map[string]any{
			"sessionId":          internalSessionID,
			"livenessStatus":     awsStatus,
			"livenessConfidence": confidence,
			"referenceImage":     refImage,
		})
	}
}
