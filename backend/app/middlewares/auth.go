package middlewares

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"user-authentication/app/repositories"
	"user-authentication/config"
	"user-authentication/lib/db"
	"user-authentication/lib/web"
)

type jwtClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
	Role  string `json:"role"`
}

// JWTAuth validates the Supabase JWT from the Authorization: Bearer header.
//
// Behaviour:
//   - SUPABASE_JWT_SECRET not set → middleware is a no-op (dev / test mode).
//   - Token missing or invalid     → 401 Unauthorized.
//   - Token valid                  → user row is upserted in public.users so
//     all FK constraints are satisfied, then the request continues.
//
// The X-User-ID header, when present, must match the JWT "sub" claim to
// prevent a logged-in user from impersonating another.
func JWTAuth(next web.Endpoint) web.Endpoint {
	return func(req *web.Request) web.Response {
		secret := config.Get().SupabaseJWTSecret
		if secret == "" {
			// No JWT secret configured — still ensure the user row exists for FK constraints.
			if userID := req.Header("X-User-ID"); userID != "" {
				_ = repositories.EnsureUser(db.Get(), userID, "")
			}
			return next(req)
		}

		authHeader := req.Header("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return web.ErrWithStatus("authorization required", http.StatusUnauthorized)
		}

		claims, err := parseSupabaseJWT(strings.TrimPrefix(authHeader, "Bearer "), secret)
		if err != nil {
			return web.ErrWithStatus("invalid token: "+err.Error(), http.StatusUnauthorized)
		}

		// Prevent header spoofing: X-User-ID must match the JWT subject.
		if userID := req.Header("X-User-ID"); userID != "" && userID != claims.Sub {
			return web.ErrWithStatus("user ID mismatch", http.StatusUnauthorized)
		}

		// Ensure user row exists (best-effort — auth failure must not block the request).
		_ = repositories.EnsureUser(db.Get(), claims.Sub, claims.Email)

		return next(req)
	}
}

// parseSupabaseJWT validates an HS256 JWT signed with the Supabase JWT secret.
// It does not rely on any external JWT library — only the standard library.
func parseSupabaseJWT(token, secret string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token")
	}

	// Verify HMAC-SHA256 signature.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	if !hmac.Equal(sig, expected) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Decode payload.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}
