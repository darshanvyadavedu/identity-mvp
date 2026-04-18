package endpoints

import (
	"net/http"

	"user-authentication/app/services"
	"user-authentication/lib/db"
	"user-authentication/lib/web"
)

type consentEndpoint struct {
	svc services.ConsentServiceInterface
}

// ConsentEndpointOption configures a consentEndpoint.
type ConsentEndpointOption func(*consentEndpoint)

// ConfigureConsentService overrides the default service (for testing).
func ConfigureConsentService(svc services.ConsentServiceInterface) ConsentEndpointOption {
	return func(e *consentEndpoint) { e.svc = svc }
}

// StoreConsentEndpoint returns an httprouter-compatible handler for POST /api/sessions/:sessionId/consent.
func StoreConsentEndpoint(opts ...ConsentEndpointOption) func(*web.Request) web.Response {
	e := &consentEndpoint{svc: services.NewConsentService()}
	for _, opt := range opts {
		opt(e)
	}
	return e.storeConsent
}

func (e *consentEndpoint) storeConsent(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}

	sessionID := req.Param("sessionId")

	var body struct {
		Fields []string `json:"fields"`
	}
	if err := req.DecodeBody(&body); err != nil {
		return web.ErrBadRequest("invalid JSON body")
	}
	if len(body.Fields) == 0 {
		return web.ErrBadRequest("at least one field must be consented")
	}

	result, svcErr := e.svc.StoreConsent(req.Context(), db.Get(), &services.StoreConsentParams{
		SessionID: sessionID,
		UserID:    userID,
		Fields:    body.Fields,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(map[string]any{
		"stored": result.Stored,
	}, true, http.StatusOK, web.API_V1)
}
