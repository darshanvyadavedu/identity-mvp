package endpoints

import (
	"io"
	"net/http"
	"strings"

	"user-authentication/app/services"
	"user-authentication/lib/db"
	"user-authentication/lib/web"
)

type documentEndpoint struct {
	svc services.DocumentServiceInterface
}

// DocumentEndpointOption configures a documentEndpoint.
type DocumentEndpointOption func(*documentEndpoint)

// ConfigureDocumentService overrides the default service (for testing).
func ConfigureDocumentService(svc services.DocumentServiceInterface) DocumentEndpointOption {
	return func(e *documentEndpoint) { e.svc = svc }
}

// UploadDocumentEndpoint returns an httprouter-compatible handler for POST /api/documents.
func UploadDocumentEndpoint(opts ...DocumentEndpointOption) func(*web.Request) web.Response {
	e := &documentEndpoint{svc: services.NewDocumentService()}
	for _, opt := range opts {
		opt(e)
	}
	return e.uploadDocument
}

func (e *documentEndpoint) uploadDocument(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}

	if err := req.ParseMultipartForm(10 << 20); err != nil {
		return web.ErrBadRequest("parse form: " + err.Error())
	}

	sessionID := strings.TrimSpace(req.FormValue("sessionId"))
	if sessionID == "" {
		return web.ErrBadRequest("sessionId is required")
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		return web.ErrBadRequest("file is required: " + err.Error())
	}
	defer file.Close()

	docBytes, err := io.ReadAll(file)
	if err != nil {
		return web.ErrInternalServerError("read file: " + err.Error())
	}

	result, svcErr := e.svc.UploadDocument(req.Context(), db.Get(), &services.UploadDocumentParams{
		SessionID: sessionID,
		UserID:    userID,
		DocBytes:  docBytes,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(map[string]any{
		"sessionId":      result.SessionID,
		"decisionStatus": result.DecisionStatus,
		"document":       result.Document,
		"faceMatch": map[string]any{
			"similarity": result.FaceMatch.Similarity,
			"passed":     result.FaceMatch.Passed,
			"threshold":  result.FaceMatch.Threshold,
		},
	}, true, http.StatusOK, web.API_V1)
}
