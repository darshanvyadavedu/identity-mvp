package azure

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"user-authentication/lib/provider"
)

const (
	faceAPIVersion     = "v1.2"
	faceV10APIVersion  = "v1.0"
	azureFaceThreshold = float64(0.70)
	azureSearchMinConf = float64(0.90)
)

type faceClient struct {
	endpoint string
	key      string
}

// ── Liveness ──────────────────────────────────────────────────────────────────

type createLivenessSessionRequest struct {
	LivenessOperationMode        string `json:"livenessOperationMode"`
	DeviceCorrelationID          string `json:"deviceCorrelationId,omitempty"`
	AuthTokenTimeToLiveInSeconds int    `json:"authTokenTimeToLiveInSeconds,omitempty"`
	EnableSessionImage           bool   `json:"enableSessionImage,omitempty"`
}

type createLivenessSessionResponse struct {
	SessionID string `json:"sessionId"`
	AuthToken string `json:"authToken"`
}

func (c *faceClient) createLivenessSession(ctx context.Context, userID string) (*provider.CreateSessionResult, error) {
	body, _ := json.Marshal(createLivenessSessionRequest{
		LivenessOperationMode:        "PassiveActive",
		DeviceCorrelationID:          userID,
		AuthTokenTimeToLiveInSeconds: 600,
		EnableSessionImage:           true,
	})
	resp, err := c.do(ctx, http.MethodPost,
		"/face/"+faceAPIVersion+"/detectLiveness-sessions", "application/json", body)
	if err != nil {
		return nil, err
	}
	var out createLivenessSessionResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, err
	}
	return &provider.CreateSessionResult{
		ProviderSessionID: out.SessionID,
		AuthToken:         out.AuthToken,
	}, nil
}

type livenessSessionResult struct {
	Status  string `json:"status"`
	Results struct {
		Attempts []struct {
			AttemptID     int    `json:"attemptId"`
			AttemptStatus string `json:"attemptStatus"`
			Result        *struct {
				LivenessDecision string `json:"livenessDecision"`
				SessionImageID   string `json:"sessionImageId"`
			} `json:"result"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"attempts"`
	} `json:"results"`
}

func (c *faceClient) getLivenessResult(ctx context.Context, providerSessionID string) (*provider.LivenessResult, error) {
	raw, err := c.do(ctx, http.MethodGet,
		"/face/"+faceAPIVersion+"/detectLiveness-sessions/"+providerSessionID, "", nil)
	if err != nil {
		return nil, err
	}

	var result livenessSessionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}

	if result.Status != "Succeeded" && result.Status != "ResultAvailable" {
		return &provider.LivenessResult{Complete: false, ProviderStatus: result.Status}, nil
	}

	var verdict, sessionImageID string
	if attempts := result.Results.Attempts; len(attempts) > 0 {
		latest := attempts[len(attempts)-1]
		if latest.Result != nil {
			sessionImageID = latest.Result.SessionImageID
			if latest.Result.LivenessDecision == "realface" {
				verdict = "live"
			} else {
				verdict = "failed"
			}
		}
		if latest.Error != nil {
			verdict = "failed"
		}
	}

	var refBytes []byte
	var refDataURL string
	if verdict == "live" && sessionImageID != "" {
		log.Printf("[azure] downloading session image: %s", sessionImageID)
		imgBytes, imgErr := c.do(ctx, http.MethodGet,
			"/face/"+faceAPIVersion+"/sessionImages/"+sessionImageID, "", nil)
		if imgErr != nil {
			log.Printf("[azure] warn: could not download session image: %v", imgErr)
		} else {
			refBytes = imgBytes
			refDataURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imgBytes)
			log.Printf("[azure] session image downloaded: %d bytes", len(imgBytes))
		}
	}

	return &provider.LivenessResult{
		Complete:       true,
		ProviderStatus: result.Status,
		Verdict:        verdict,
		ReferenceImage: refDataURL,
		ReferenceBytes: refBytes,
		RawResponse:    raw,
	}, nil
}

// ── Face compare & dedup ──────────────────────────────────────────────────────

type detectedFace struct {
	FaceID string `json:"faceId"`
}

type verifyResponse struct {
	IsIdentical bool    `json:"isIdentical"`
	Confidence  float64 `json:"confidence"`
}

func (c *faceClient) detectFace(ctx context.Context, imgBytes []byte) (string, error) {
	resp, err := c.do(ctx, http.MethodPost,
		"/face/"+faceV10APIVersion+"/detect?detectionModel=detection_03&returnFaceId=true&recognitionModel=recognition_04",
		"application/octet-stream", imgBytes)
	if err != nil {
		return "", err
	}
	var faces []detectedFace
	if err := json.Unmarshal(resp, &faces); err != nil {
		return "", err
	}
	if len(faces) == 0 {
		return "", fmt.Errorf("no face detected in image")
	}
	return faces[0].FaceID, nil
}

func (c *faceClient) compareFaces(ctx context.Context, refBytes, docBytes []byte) (*provider.CompareFacesResult, error) {
	selfieID, err1 := c.detectFace(ctx, refBytes)
	docFaceID, err2 := c.detectFace(ctx, docBytes)
	log.Printf("[azure] face compare: selfieID=%q err=%v  docFaceID=%q err=%v", selfieID, err1, docFaceID, err2)
	if err1 != nil || err2 != nil {
		return &provider.CompareFacesResult{}, nil
	}
	body, _ := json.Marshal(map[string]string{"faceId1": selfieID, "faceId2": docFaceID})
	resp, err := c.do(ctx, http.MethodPost, "/face/"+faceV10APIVersion+"/verify", "application/json", body)
	if err != nil {
		return nil, err
	}
	var out verifyResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, err
	}
	similarity := out.Confidence * 100
	return &provider.CompareFacesResult{
		Similarity: similarity,
		Passed:     out.Confidence >= azureFaceThreshold,
	}, nil
}

type findSimilarRequest struct {
	FaceID                     string `json:"faceId"`
	FaceListID                 string `json:"faceListId"`
	MaxNumOfCandidatesReturned int    `json:"maxNumOfCandidatesReturned"`
	Mode                       string `json:"mode"`
}

type similarFace struct {
	PersistedFaceID string  `json:"persistedFaceId"`
	Confidence      float64 `json:"confidence"`
	UserData        string  `json:"userData"`
}

func (c *faceClient) searchFacesByImage(ctx context.Context, imgBytes []byte, faceListID string) (*provider.SearchFacesResult, error) {
	faceID, err := c.detectFace(ctx, imgBytes)
	if err != nil {
		return &provider.SearchFacesResult{Found: false}, nil
	}
	body, _ := json.Marshal(findSimilarRequest{
		FaceID: faceID, FaceListID: faceListID,
		MaxNumOfCandidatesReturned: 1, Mode: "matchPerson",
	})
	resp, err := c.do(ctx, http.MethodPost, "/face/"+faceV10APIVersion+"/findsimilars", "application/json", body)
	if err != nil {
		return &provider.SearchFacesResult{Found: false}, nil
	}
	var matches []similarFace
	if err := json.Unmarshal(resp, &matches); err != nil || len(matches) == 0 {
		return &provider.SearchFacesResult{Found: false}, nil
	}
	m := matches[0]
	if m.Confidence < azureSearchMinConf {
		return &provider.SearchFacesResult{Found: false}, nil
	}
	return &provider.SearchFacesResult{Found: true, FaceID: m.PersistedFaceID, MatchedUserID: m.UserData}, nil
}

type addFaceResponse struct {
	PersistedFaceID string `json:"persistedFaceId"`
}

func (c *faceClient) indexFace(ctx context.Context, imgBytes []byte, faceListID, userID string) (string, error) {
	path := fmt.Sprintf("/face/"+faceV10APIVersion+"/facelists/%s/persistedFaces?userData=%s", faceListID, userID)
	resp, err := c.do(ctx, http.MethodPost, path, "application/octet-stream", imgBytes)
	if err != nil {
		return "", err
	}
	var out addFaceResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return "", err
	}
	return out.PersistedFaceID, nil
}

func (c *faceClient) deleteFace(ctx context.Context, faceListID, persistedFaceID string) error {
	_, err := c.do(ctx, http.MethodDelete,
		fmt.Sprintf("/face/"+faceV10APIVersion+"/facelists/%s/persistedFaces/%s", faceListID, persistedFaceID),
		"", nil)
	return err
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (c *faceClient) do(ctx context.Context, method, path, contentType string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	endpoint := strings.TrimRight(c.endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", c.key)
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
