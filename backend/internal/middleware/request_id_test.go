package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorrelationIDMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected request ID in context, got empty")
		}
		w.WriteHeader(http.StatusOK)
	})

	handlerToTest := CorrelationIDMiddleware(dummyHandler)

	// Case 1: Client provides X-Request-ID
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "custom-req-12345")
	rec := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "custom-req-12345" {
		t.Errorf("expected echoed X-Request-ID 'custom-req-12345', got '%s'", rec.Header().Get("X-Request-ID"))
	}

	// Case 2: Client does not provide X-Request-ID -> Auto-generate with prefix req_
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec2, req2)

	resID := rec2.Header().Get("X-Request-ID")
	if !strings.HasPrefix(resID, "req_") {
		t.Errorf("expected auto-generated request ID to start with 'req_', got '%s'", resID)
	}
	if len(resID) != 36 {
		t.Errorf("expected auto-generated request ID length 36, got %d ('%s')", len(resID), resID)
	}
}
