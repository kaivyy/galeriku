package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_EchoesApprovedOrigin(t *testing.T) {
	handler := CORS([]string{"http://192.168.1.44:6464"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos", nil)
	req.Header.Set("Origin", "http://192.168.1.44:6464")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://192.168.1.44:6464" {
		t.Fatalf("expected origin echo, got %q", got)
	}
}

func TestCORS_RejectsUnapprovedOrigin(t *testing.T) {
	handler := CORS([]string{"http://192.168.1.44:6464"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS header for unapproved origin, got %q", got)
	}
}

func TestCORS_PreflightShortCircuits(t *testing.T) {
	reached := false
	handler := CORS([]string{"*"})(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/photos", nil)
	req.Header.Set("Origin", "http://192.168.1.44:6464")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if reached {
		t.Fatal("preflight must not reach the wrapped handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rec.Code)
	}
}

func TestCORS_AllWildcard(t *testing.T) {
	handler := CORS([]string{"*"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://any-host:9999")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://any-host:9999" {
		t.Fatalf("expected wildcard echo, got %q", got)
	}
}
