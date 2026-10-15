package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
)

// writeMiddlewareError serializes a standard JSON error response matching the API architecture specification.
func writeMiddlewareError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	reqID := GetRequestID(r.Context())
	resp := map[string]interface{}{
		"success": false,
		"error": map[string]interface{}{
			"code":       code,
			"message":    message,
			"request_id": reqID,
		},
		"meta": map[string]interface{}{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": reqID,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// Authenticate returns a middleware that validates Bearer JWT access tokens and injects AuthIdentity into request context.
func Authenticate(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeMiddlewareError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "missing or malformed authorization header")
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := security.VerifyAccessToken(jwtSecret, tokenStr)
			if err != nil {
				if errors.Is(err, security.ErrExpiredToken) {
					writeMiddlewareError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "access token has expired")
					return
				}
				writeMiddlewareError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "invalid access token")
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				writeMiddlewareError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "invalid subject token claim")
				return
			}

			identity := &AuthIdentity{
				UserID:       userID,
				Email:        claims.Email,
				Capabilities: claims.Capabilities,
				TokenID:      claims.ID,
			}

			ctx := WithAuthIdentity(r.Context(), identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
