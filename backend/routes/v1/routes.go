package v1

import (
	"fmt"
	"net/http"

	appmiddlewares "user-authentication/app/middlewares"
	"user-authentication/app/endpoints"
	"user-authentication/config"
	"user-authentication/lib/web"
	"user-authentication/lib/web/middlewares"

	"github.com/julienschmidt/httprouter"
)

var commonMiddlewares = []web.Middleware{middlewares.CORS, appmiddlewares.JWTAuth}

// Init registers all v1 API routes. Provider selection and client wiring happen
// automatically inside each service via config.
func Init(router *httprouter.Router) {
	provider := string(config.Get().Provider)

	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Identity Verification API (provider: %s)\n\n"+
			"  POST /api/sessions                                        — create liveness session\n"+
			"  GET  /api/sessions/:sessionId/result                      — fetch liveness result\n"+
			"  GET  /api/sessions/:sessionId/liveness-image              — raw face capture (JPEG)\n"+
			"  POST /api/documents                                       — upload ID document\n"+
			"  POST /api/sessions/:sessionId/consent                     — store consent & verified data\n\n"+
			"  Profession verification:\n"+
			"  POST /api/professions                                     — start profession verification\n"+
			"  POST /api/profession/:verificationId/submit/email         — submit work email (sends OTP)\n"+
			"  POST /api/profession/:verificationId/confirm/email        — confirm OTP\n"+
			"  POST /api/profession/:verificationId/submit/document      — upload license/certificate\n"+
			"  POST /api/profession/:verificationId/submit/portfolio     — submit portfolio URL\n"+
			"  GET  /api/profession/:verificationId/status               — get verification status\n"+
			"  GET  /api/profession/:verificationId/linkedin/auth-url    — get LinkedIn OAuth URL\n"+
			"  POST /api/profession/:verificationId/linkedin/callback    — handle LinkedIn callback\n",
			provider)
	})

	router.POST("/api/sessions",
		web.Serve(commonMiddlewares, endpoints.CreateSessionEndpoint()))

	router.GET("/api/sessions/:sessionId/result",
		web.Serve(commonMiddlewares, endpoints.GetLivenessResultEndpoint()))

	router.GET("/api/sessions/:sessionId/liveness-image",
		web.Serve(commonMiddlewares, endpoints.GetLivenessImageEndpoint()))

	router.POST("/api/documents",
		web.Serve(commonMiddlewares, endpoints.UploadDocumentEndpoint()))

	router.POST("/api/sessions/:sessionId/consent",
		web.Serve(commonMiddlewares, endpoints.StoreConsentEndpoint()))

	// ── Profession verification ────────────────────────────────────────────────
	router.POST("/api/professions",
		web.Serve(commonMiddlewares, endpoints.StartProfessionVerificationEndpoint()))

	router.POST("/api/profession/:verificationId/submit/email",
		web.Serve(commonMiddlewares, endpoints.SubmitWorkEmailEndpoint()))

	router.POST("/api/profession/:verificationId/confirm/email",
		web.Serve(commonMiddlewares, endpoints.ConfirmEmailEndpoint()))

	router.POST("/api/profession/:verificationId/submit/document",
		web.Serve(commonMiddlewares, endpoints.SubmitProfessionDocumentEndpoint()))

	router.POST("/api/profession/:verificationId/submit/portfolio",
		web.Serve(commonMiddlewares, endpoints.SubmitPortfolioEndpoint()))

	router.GET("/api/profession/:verificationId/status",
		web.Serve(commonMiddlewares, endpoints.GetProfessionStatusEndpoint()))

	router.GET("/api/profession/:verificationId/linkedin/auth-url",
		web.Serve(commonMiddlewares, endpoints.LinkedInAuthURLEndpoint()))

	router.POST("/api/profession/:verificationId/linkedin/callback",
		web.Serve(commonMiddlewares, endpoints.LinkedInCallbackEndpoint()))
}
