package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORSEnvironmentOrigins reads APPROVED_ORIGINS (comma-separated) from the
// environment, defaulting to the wildcard for development.
func CORSEnvironmentOrigins() []string {
	raw := os.Getenv("APPROVED_ORIGINS")
	if raw == "" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

// CORS allows the Galeriku web storefront (Astro) and any approved browser
// origin to call the API from another host, which is how phones on the LAN
// reach the dev server. APPROVED_ORIGINS is a comma-separated list; the
// special value "*" echoes any origin (dev only).
func CORS(approvedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]bool, len(approvedOrigins))
	for _, origin := range approvedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowed[origin] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Organization-ID, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Preflight short-circuits before routing.
			if r.Method == http.MethodOptions && origin != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
