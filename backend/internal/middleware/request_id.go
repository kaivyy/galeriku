package middleware

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

var requestIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// CorrelationIDMiddleware extracts or generates X-Request-ID and injects it into context and response headers.
// Client-supplied IDs are strictly validated with ^[a-zA-Z0-9_-]{1,64}$ to prevent header injection or splitting.
func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if reqID == "" || !requestIDRegex.MatchString(reqID) {
			reqID = "req_" + strings.ReplaceAll(uuid.New().String(), "-", "")
		}

		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		w.Header().Set("X-Request-ID", reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from the context if present.
func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(RequestIDKey).(string); ok {
		return val
	}
	return ""
}
