package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const apiVersion = "v1.2"

// FaceClient wraps Azure Face API calls.
// Endpoint example: https://eastus.api.cognitive.microsoft.com/
type FaceClient struct {
	Endpoint string
	Key      string
}

// ── Liveness ──────────────────────────────────────────────────────────────────

type CreateLivenessSessionRequest struct {
	LivenessOperationMode          string `json:"livenessOperationMode"` // "Passive" | "PassiveActive"
	DeviceCorrelationID            string `json:"deviceCorrelationId,omitempty"`
	DeviceCorrelationIDSetInClient bool   `json:"deviceCorrelationIdSetInClient,omitempty"`
	AuthTokenTimeToLiveInSeconds   int    `json:"authTokenTimeToLiveInSeconds,omitempty"`
	EnableSessionImage             bool   `json:"enableSessionImage,omitempty"`
}

type CreateLivenessSessionResponse struct {
	SessionID string `json:"sessionId"`
	AuthToken string `json:"authToken"`
	Status    string `json:"status"`
}

// CreateLivenessSession creates a liveness session on Azure.
// Returns sessionId (for backend polling) + authToken (to give to the client SDK).
// Endpoint: POST /face/v1.2/detectLiveness-sessions
func (c *FaceClient) CreateLivenessSession(ctx context.Context, userID string) (*CreateLivenessSessionResponse, error) {
	body, _ := json.Marshal(CreateLivenessSessionRequest{
		LivenessOperationMode:        "PassiveActive", // works on both web and mobile
		DeviceCorrelationID:          userID,
		AuthTokenTimeToLiveInSeconds: 600, // 10 minutes
		EnableSessionImage:           true,
	})
	resp, err := c.do(ctx, http.MethodPost,
		"/face/"+apiVersion+"/detectLiveness-sessions", "application/json", body)
	if err != nil {
		return nil, err
	}
	var out CreateLivenessSessionResponse
	return &out, json.Unmarshal(resp, &out)
}

// LivenessSessionResult is the response from GET /face/v1.2/detectLiveness-sessions/{sessionId}
type LivenessSessionResult struct {
	SessionID    string `json:"sessionId"`
	Status       string `json:"status"` // "NotStarted" | "Running" | "ResultAvailable" | "Failed" | "Canceled"
	ModelVersion string `json:"modelVersion"`
	Results      struct {
		Attempts []struct {
			AttemptID     int    `json:"attemptId"`
			AttemptStatus string `json:"attemptStatus"`
			Result        *struct {
				LivenessDecision string `json:"livenessDecision"` // "realface" | "spoofface" | "uncertain"
				Digest           string `json:"digest"`
				SessionImageID   string `json:"sessionImageId"`
				VerifyImageHash  string `json:"verifyImageHash"`
				Targets          *struct {
					Color *struct {
						FaceRectangle struct {
							Top    int `json:"top"`
							Left   int `json:"left"`
							Width  int `json:"width"`
							Height int `json:"height"`
						} `json:"faceRectangle"`
					} `json:"color"`
				} `json:"targets"`
			} `json:"result"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"attempts"`
	} `json:"results"`
}

// GetLivenessSessionResult fetches the result of a liveness session.
// Endpoint: GET /face/v1.2/detectLiveness-sessions/{sessionId}
func (c *FaceClient) GetLivenessSessionResult(ctx context.Context, sessionID string) (*LivenessSessionResult, error) {
	resp, err := c.do(ctx, http.MethodGet,
		"/face/"+apiVersion+"/detectLiveness-sessions/"+sessionID, "", nil)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[Azure liveness raw] %s\n", string(resp))
	var out LivenessSessionResult
	return &out, json.Unmarshal(resp, &out)
}

// GetSessionImage downloads the captured face image using its sessionImageId.
// Endpoint: GET /face/v1.2/sessionImages/{sessionImageId}
func (c *FaceClient) GetSessionImage(ctx context.Context, sessionImageID string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, "/face/"+apiVersion+"/sessionImages/"+sessionImageID, "", nil)
}

// DeleteLivenessSession deletes a session after it's no longer needed.
// Endpoint: DELETE /face/v1.2/detectLiveness-sessions/{sessionId}
func (c *FaceClient) DeleteLivenessSession(ctx context.Context, sessionID string) error {
	_, err := c.do(ctx, http.MethodDelete,
		"/face/"+apiVersion+"/detectLiveness-sessions/"+sessionID, "", nil)
	return err
}

// ── Face detect ───────────────────────────────────────────────────────────────

type DetectedFace struct {
	FaceID string `json:"faceId"`
}

// DetectFaceFromBytes detects a face in raw image bytes and returns the faceId.
// Endpoint: POST /face/v1.0/detect
func (c *FaceClient) DetectFaceFromBytes(ctx context.Context, imgBytes []byte) (string, error) {
	resp, err := c.do(ctx, http.MethodPost,
		"/face/v1.0/detect?detectionModel=detection_03&returnFaceId=true&recognitionModel=recognition_04",
		"application/octet-stream", imgBytes)
	if err != nil {
		return "", err
	}
	var faces []DetectedFace
	if err := json.Unmarshal(resp, &faces); err != nil {
		return "", err
	}
	if len(faces) == 0 {
		return "", fmt.Errorf("no face detected in image")
	}
	return faces[0].FaceID, nil
}

// ── Face verify ───────────────────────────────────────────────────────────────

type VerifyResponse struct {
	IsIdentical bool    `json:"isIdentical"`
	Confidence  float64 `json:"confidence"` // 0.0–1.0
}

// VerifyFaces compares two faceIds and returns similarity.
// Endpoint: POST /face/v1.0/verify
func (c *FaceClient) VerifyFaces(ctx context.Context, faceID1, faceID2 string) (*VerifyResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"faceId1": faceID1,
		"faceId2": faceID2,
	})
	resp, err := c.do(ctx, http.MethodPost, "/face/v1.0/verify", "application/json", body)
	if err != nil {
		return nil, err
	}
	var out VerifyResponse
	return &out, json.Unmarshal(resp, &out)
}

// ── FaceList (biometric dedup) ────────────────────────────────────────────────

// EnsureFaceList creates the FaceList if it doesn't exist (idempotent).
// Endpoint: PUT /face/v1.0/facelists/{faceListId}
func (c *FaceClient) EnsureFaceList(ctx context.Context, faceListID string) error {
	body, _ := json.Marshal(map[string]string{
		"name":             faceListID,
		"recognitionModel": "recognition_04",
	})
	_, err := c.do(ctx, http.MethodPut, "/face/v1.0/facelists/"+faceListID, "application/json", body)
	// 409 Conflict = already exists — treat as success.
	if err != nil && strings.Contains(err.Error(), "409") {
		return nil
	}
	return err
}

type AddFaceResponse struct {
	PersistedFaceID string `json:"persistedFaceId"`
}

// AddFaceToList enrolls a face image in the FaceList and returns the persistedFaceId.
// Endpoint: POST /face/v1.0/facelists/{faceListId}/persistedFaces
func (c *FaceClient) AddFaceToList(ctx context.Context, faceListID string, imgBytes []byte, userData string) (string, error) {
	path := fmt.Sprintf("/face/v1.0/facelists/%s/persistedFaces?userData=%s", faceListID, userData)
	resp, err := c.do(ctx, http.MethodPost, path, "application/octet-stream", imgBytes)
	if err != nil {
		return "", err
	}
	var out AddFaceResponse
	return out.PersistedFaceID, json.Unmarshal(resp, &out)
}

// DeleteFaceFromList removes a persisted face from the FaceList.
// Endpoint: DELETE /face/v1.0/facelists/{faceListId}/persistedFaces/{persistedFaceId}
func (c *FaceClient) DeleteFaceFromList(ctx context.Context, faceListID, persistedFaceID string) error {
	_, err := c.do(ctx, http.MethodDelete,
		fmt.Sprintf("/face/v1.0/facelists/%s/persistedFaces/%s", faceListID, persistedFaceID), "", nil)
	return err
}

type FindSimilarRequest struct {
	FaceID                     string `json:"faceId"`
	FaceListID                 string `json:"faceListId"`
	MaxNumOfCandidatesReturned int    `json:"maxNumOfCandidatesReturned"`
	Mode                       string `json:"mode"` // "matchPerson"
}

type SimilarFace struct {
	PersistedFaceID string  `json:"persistedFaceId"`
	Confidence      float64 `json:"confidence"`
	UserData        string  `json:"userData"`
}

// FindSimilar searches the FaceList for a matching face.
// Endpoint: POST /face/v1.0/findsimilars
func (c *FaceClient) FindSimilar(ctx context.Context, faceID, faceListID string) ([]SimilarFace, error) {
	body, _ := json.Marshal(FindSimilarRequest{
		FaceID:                     faceID,
		FaceListID:                 faceListID,
		MaxNumOfCandidatesReturned: 1,
		Mode:                       "matchPerson",
	})
	resp, err := c.do(ctx, http.MethodPost, "/face/v1.0/findsimilars", "application/json", body)
	if err != nil {
		return nil, err
	}
	var out []SimilarFace
	return out, json.Unmarshal(resp, &out)
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (c *FaceClient) do(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	// Ensure endpoint has no trailing slash before appending path.
	endpoint := strings.TrimRight(c.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", c.Key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("azure face API %s %s → %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
