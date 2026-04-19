package middlewares

import (
	"net/http"

	"user-authentication/lib/web"
)

// CORS is a middleware that sets CORS headers and short-circuits OPTIONS preflight requests.
// Because the web.Middleware type wraps Endpoints, CORS is handled at the httprouter level
// via WithCORS instead (see routes). This no-op keeps the package consistent with the pattern.
func CORS(next web.Endpoint) web.Endpoint {
	return next
}

// WithCORS wraps any http.Handler with CORS headers and OPTIONS handling.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
