package endpoints

import (
	"net/http"

	"user-authentication/app/services"
	"user-authentication/lib/db"
	"user-authentication/lib/web"
)

type livenessEndpoint struct {
	svc services.LivenessServiceInterface
}

// LivenessEndpointOption configures a livenessEndpoint.
type LivenessEndpointOption func(*livenessEndpoint)

// ConfigureLivenessService overrides the default service (for testing).
func ConfigureLivenessService(svc services.LivenessServiceInterface) LivenessEndpointOption {
	return func(e *livenessEndpoint) { e.svc = svc }
}

// GetLivenessResultEndpoint returns an httprouter-compatible handler for GET /api/sessions/:sessionId/result.
func GetLivenessResultEndpoint(opts ...LivenessEndpointOption) func(*web.Request) web.Response {
	e := &livenessEndpoint{svc: services.NewLivenessService()}
	for _, opt := range opts {
		opt(e)
	}
	return e.getLivenessResult
}

// GetLivenessImageEndpoint returns a handler for GET /api/sessions/:sessionId/liveness-image.
func GetLivenessImageEndpoint(opts ...LivenessEndpointOption) func(*web.Request) web.Response {
	e := &livenessEndpoint{svc: services.NewLivenessService()}
	for _, opt := range opts {
		opt(e)
	}
	return e.getLivenessImage
}

func (e *livenessEndpoint) getLivenessResult(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}

	sessionID := req.Param("sessionId")

	result, svcErr := e.svc.GetLivenessResult(req.Context(), db.Get(), &services.GetLivenessResultParams{
		SessionID: sessionID,
		UserID:    userID,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	// Session still in progress — return status without confidence/image.
	if !result.Complete {
		return web.NewResponse(map[string]any{
			"sessionId":      result.SessionID,
			"livenessStatus": result.LivenessStatus,
		}, true, http.StatusOK, web.API_V1)
	}

	return web.NewResponse(map[string]any{
		"sessionId":          result.SessionID,
		"livenessStatus":     result.LivenessStatus,
		"livenessConfidence": result.LivenessConfidence,
		"referenceImage":     result.ReferenceImage,
	}, true, http.StatusOK, web.API_V1)
}

func (e *livenessEndpoint) getLivenessImage(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}

	sessionID := req.Param("sessionId")

	imgBytes, svcErr := e.svc.GetLivenessImage(req.Context(), db.Get(), &services.GetLivenessImageParams{
		SessionID: sessionID,
		UserID:    userID,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewRawResponse(imgBytes, "image/jpeg", http.StatusOK)
}
