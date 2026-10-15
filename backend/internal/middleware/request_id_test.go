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

	// Case 1: Client provides valid X-Request-ID
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

	// Case 3: Client provides header with invalid characters (CRLF / injection attempt) -> auto-generated replacement
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.Header.Set("X-Request-ID", "injected\r\nX-Injected-Header: evil")
	rec3 := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec3, req3)

	resID3 := rec3.Header().Get("X-Request-ID")
	if !strings.HasPrefix(resID3, "req_") || strings.Contains(resID3, "injected") {
		t.Errorf("expected malicious request ID to be replaced with auto-generated ID, got '%s'", resID3)
	}

	// Case 4: Client provides oversized ID (> 64 chars) -> auto-generated replacement
	oversizedID := strings.Repeat("a", 65)
	req4 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req4.Header.Set("X-Request-ID", oversizedID)
	rec4 := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec4, req4)

	resID4 := rec4.Header().Get("X-Request-ID")
	if !strings.HasPrefix(resID4, "req_") {
		t.Errorf("expected oversized request ID to be replaced with auto-generated ID, got '%s'", resID4)
	}

	// Case 5: Client provides exactly 64 valid characters -> preserved
	valid64ID := strings.Repeat("b", 64)
	req5 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req5.Header.Set("X-Request-ID", valid64ID)
	rec5 := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec5, req5)

	if rec5.Header().Get("X-Request-ID") != valid64ID {
		t.Errorf("expected valid 64-char request ID to be preserved, got '%s'", rec5.Header().Get("X-Request-ID"))
	}
}
