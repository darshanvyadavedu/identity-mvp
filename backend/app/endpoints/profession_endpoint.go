package endpoints

import (
	"io"
	"net/http"
	"strings"

	"user-authentication/app/services"
	"user-authentication/lib/db"
	"user-authentication/lib/web"
)

type professionEndpoint struct {
	svc services.ProfessionServiceInterface
}

type ProfessionEndpointOption func(*professionEndpoint)

func ConfigureProfessionService(svc services.ProfessionServiceInterface) ProfessionEndpointOption {
	return func(e *professionEndpoint) { e.svc = svc }
}

func newProfessionEndpoint(opts []ProfessionEndpointOption) *professionEndpoint {
	e := &professionEndpoint{svc: services.NewProfessionService()}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ── StartProfessionVerificationEndpoint ── POST /api/profession/start ─────────

func StartProfessionVerificationEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).startVerification
}

func (e *professionEndpoint) startVerification(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}

	var body struct {
		ProfessionType   string `json:"professionType"`
		ProfessionTitle  string `json:"professionTitle"`
		EmployerName     string `json:"employerName"`
		ConsentStoreData bool   `json:"consentStoreData"`
	}
	if err := req.DecodeBody(&body); err != nil {
		return web.ErrBadRequest("invalid request body: " + err.Error())
	}
	if strings.TrimSpace(body.ProfessionType) == "" {
		return web.ErrBadRequest("professionType is required")
	}

	result, svcErr := e.svc.StartVerification(req.Context(), db.Get(), &services.StartProfessionParams{
		UserID:           userID,
		ProfessionType:   strings.TrimSpace(body.ProfessionType),
		ProfessionTitle:  strings.TrimSpace(body.ProfessionTitle),
		EmployerName:     strings.TrimSpace(body.EmployerName),
		ConsentStoreData: body.ConsentStoreData,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusCreated, web.API_V1)
}

// ── SubmitWorkEmailEndpoint ── POST /api/profession/:vId/submit/email ─────────

func SubmitWorkEmailEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).submitWorkEmail
}

func (e *professionEndpoint) submitWorkEmail(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	var body struct {
		WorkEmail string `json:"workEmail"`
	}
	if err := req.DecodeBody(&body); err != nil {
		return web.ErrBadRequest("invalid request body: " + err.Error())
	}
	if strings.TrimSpace(body.WorkEmail) == "" {
		return web.ErrBadRequest("workEmail is required")
	}

	result, svcErr := e.svc.SubmitWorkEmail(req.Context(), db.Get(), &services.SubmitWorkEmailParams{
		VerificationID: verificationID,
		UserID:         userID,
		WorkEmail:      strings.TrimSpace(body.WorkEmail),
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}

// ── ConfirmEmailEndpoint ── POST /api/profession/:vId/confirm/email ───────────

func ConfirmEmailEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).confirmEmail
}

func (e *professionEndpoint) confirmEmail(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	var body struct {
		EvidenceID string `json:"evidenceId"`
		OTP        string `json:"otp"`
	}
	if err := req.DecodeBody(&body); err != nil {
		return web.ErrBadRequest("invalid request body: " + err.Error())
	}
	if body.EvidenceID == "" || body.OTP == "" {
		return web.ErrBadRequest("evidenceId and otp are required")
	}

	result, svcErr := e.svc.ConfirmEmail(req.Context(), db.Get(), &services.ConfirmEmailParams{
		VerificationID: verificationID,
		UserID:         userID,
		EvidenceID:     body.EvidenceID,
		OTP:            strings.TrimSpace(body.OTP),
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}

// ── SubmitDocumentEndpoint ── POST /api/profession/:vId/submit/document ───────

func SubmitProfessionDocumentEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).submitDocument
}

func (e *professionEndpoint) submitDocument(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	if err := req.ParseMultipartForm(10 << 20); err != nil {
		return web.ErrBadRequest("parse form: " + err.Error())
	}

	file, header, err := req.FormFile("file")
	if err != nil {
		return web.ErrBadRequest("file is required: " + err.Error())
	}
	defer file.Close()

	docBytes, err := io.ReadAll(file)
	if err != nil {
		return web.ErrInternalServerError("read file: " + err.Error())
	}

	documentType := strings.TrimSpace(req.FormValue("documentType"))
	if documentType == "" {
		documentType = "professional_document"
	}

	result, svcErr := e.svc.SubmitDocument(req.Context(), db.Get(), &services.SubmitDocumentParams{
		VerificationID: verificationID,
		UserID:         userID,
		DocBytes:       docBytes,
		DocumentType:   documentType,
		FileName:       header.Filename,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}

// ── SubmitPortfolioEndpoint ── POST /api/profession/:vId/submit/portfolio ──────

func SubmitPortfolioEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).submitPortfolio
}

func (e *professionEndpoint) submitPortfolio(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	var body struct {
		URL      string `json:"url"`
		Platform string `json:"platform"`
	}
	if err := req.DecodeBody(&body); err != nil {
		return web.ErrBadRequest("invalid request body: " + err.Error())
	}
	if strings.TrimSpace(body.URL) == "" {
		return web.ErrBadRequest("url is required")
	}

	result, svcErr := e.svc.SubmitPortfolio(req.Context(), db.Get(), &services.SubmitPortfolioParams{
		VerificationID: verificationID,
		UserID:         userID,
		URL:            strings.TrimSpace(body.URL),
		Platform:       strings.TrimSpace(body.Platform),
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}

// ── GetProfessionStatusEndpoint ── GET /api/profession/:vId/status ────────────

func GetProfessionStatusEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).getStatus
}

func (e *professionEndpoint) getStatus(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	result, svcErr := e.svc.GetStatus(req.Context(), db.Get(), &services.GetProfessionStatusParams{
		VerificationID: verificationID,
		UserID:         userID,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}

// ── LinkedInAuthURLEndpoint ── GET /api/profession/:vId/linkedin/auth-url ─────

func LinkedInAuthURLEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).linkedInAuthURL
}

func (e *professionEndpoint) linkedInAuthURL(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	result, svcErr := e.svc.GetLinkedInAuthURL(req.Context(), db.Get(), &services.LinkedInAuthURLParams{
		VerificationID: verificationID,
		UserID:         userID,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}

// ── LinkedInCallbackEndpoint ── POST /api/profession/:vId/linkedin/callback ───

func LinkedInCallbackEndpoint(opts ...ProfessionEndpointOption) func(*web.Request) web.Response {
	return newProfessionEndpoint(opts).linkedInCallback
}

func (e *professionEndpoint) linkedInCallback(req *web.Request) web.Response {
	userID := req.Header("X-User-ID")
	if userID == "" {
		return web.ErrBadRequest("X-User-ID header is required")
	}
	verificationID := req.Param("verificationId")
	if verificationID == "" {
		return web.ErrBadRequest("verificationId path parameter is required")
	}

	var body struct {
		AuthCode string `json:"authCode"`
		State    string `json:"state"`
	}
	if err := req.DecodeBody(&body); err != nil {
		return web.ErrBadRequest("invalid request body: " + err.Error())
	}
	if body.AuthCode == "" {
		return web.ErrBadRequest("authCode is required")
	}

	result, svcErr := e.svc.HandleLinkedInCallback(req.Context(), db.Get(), &services.LinkedInCallbackParams{
		VerificationID: verificationID,
		UserID:         userID,
		AuthCode:       body.AuthCode,
		State:          body.State,
	})
	if svcErr != nil {
		return web.ErrWithStatus(svcErr.Description(), svcErr.HttpStatus())
	}

	return web.NewResponse(result, true, http.StatusOK, web.API_V1)
}
