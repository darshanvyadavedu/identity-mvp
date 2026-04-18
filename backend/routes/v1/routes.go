package v1

import (
	"fmt"
	"net/http"

	"user-authentication/app/endpoints"
	"user-authentication/config"
	"user-authentication/lib/web"
	"user-authentication/lib/web/middlewares"

	"github.com/julienschmidt/httprouter"
)

var commonMiddlewares = []web.Middleware{middlewares.CORS}

// Init registers all v1 API routes. Provider selection and client wiring happen
// automatically inside each service via config.
func Init(router *httprouter.Router) {
	provider := string(config.Get().Provider)

	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Identity Verification API (provider: %s)\n\n"+
			"  POST /api/sessions                            — create liveness session\n"+
			"  GET  /api/sessions/:sessionId/result          — fetch liveness result\n"+
			"  GET  /api/sessions/:sessionId/liveness-image  — raw face capture (JPEG)\n"+
			"  POST /api/documents                           — upload ID document\n"+
			"  POST /api/sessions/:sessionId/consent         — store consent & verified data\n",
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
}
