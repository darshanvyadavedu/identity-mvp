package endpoints

import (
	"net/http"

	"user-authentication/app/services"
	"user-authentication/lib/db"
	"user-authentication/lib/web"
)

type sessionEndpoint struct {
	svc services.SessionServiceInterface
}

// SessionEndpointOption configures a sessionEndpoint.
type SessionEndpointOption func(*sessionEndpoint)

// ConfigureSessionService overrides the default service (for testing).
func ConfigureSessionService(svc services.SessionServiceInterface) SessionEndpointOption {
	return func(e *sessionEndpoint) { e.svc = svc }
}

// CreateSessionEndpoint returns an httprouter-compatible handler for POST /api/sessions.
func CreateSessionEndpoint(opts ...SessionEndpointOption) func(*web.Request) web.Response {
	e := &sessionEndpoint{svc: services.NewSessionService()}
	for _, opt := range opts {
		opt(e)
	}
	return e.createSession
}

func (e *sessionEndpoint) createSession(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}

	result, svcErr := e.svc.CreateSession(req.Context(), db.Get(), &services.CreateSessionParams{
		UserID: userID,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	resp := map[string]string{
		"sessionId":         result.SessionID,
		"providerSessionId": result.ProviderSessionID,
		"provider":          result.Provider,
		"userId":            result.UserID,
	}
	if result.AuthToken != "" {
		resp["authToken"] = result.AuthToken
	}

	return web.NewResponse(resp, true, http.StatusCreated, web.API_V1)
}
