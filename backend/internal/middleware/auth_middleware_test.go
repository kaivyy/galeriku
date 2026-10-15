package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
)

func TestAuthenticateMiddleware(t *testing.T) {
	jwtSecret := []byte("test_super_secret_jwt_key_32_bytes_long_12345")
	authMw := middleware.Authenticate(jwtSecret)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := middleware.GetAuthIdentity(r.Context())
		if !ok || identity == nil {
			t.Error("expected AuthIdentity in request context, got nil")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(identity)
	})

	protectedHandler := authMw(dummyHandler)

	// 1. Missing Authorization header -> 401 AUTH_UNAUTHORIZED
	req1 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec1 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing header, got %d", rec1.Code)
	}
	var errResp1 map[string]interface{}
	_ = json.Unmarshal(rec1.Body.Bytes(), &errResp1)
	errBody1 := errResp1["error"].(map[string]interface{})
	if errBody1["code"] != "AUTH_UNAUTHORIZED" {
		t.Errorf("expected error code AUTH_UNAUTHORIZED, got %v", errBody1["code"])
	}

	// 2. Malformed Authorization header -> 401 AUTH_UNAUTHORIZED
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec2 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for malformed header, got %d", rec2.Code)
	}

	// 3. Expired token -> 401 AUTH_UNAUTHORIZED
	userID := uuid.New().String()
	expiredToken, err := security.GenerateAccessToken(jwtSecret, userID, "user@example.com", []string{"BUYER"}, -10*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req3.Header.Set("Authorization", "Bearer "+expiredToken)
	rec3 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", rec3.Code)
	}
	var errResp3 map[string]interface{}
	_ = json.Unmarshal(rec3.Body.Bytes(), &errResp3)
	errBody3 := errResp3["error"].(map[string]interface{})
	if errBody3["message"] != "access token has expired" {
		t.Errorf("expected message 'access token has expired', got %v", errBody3["message"])
	}

	// 4. Tampered token -> 401 AUTH_UNAUTHORIZED
	validToken, err := security.GenerateAccessToken(jwtSecret, userID, "user@example.com", []string{"BUYER", "CREATOR"}, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate valid token: %v", err)
	}

	tamperedToken := validToken[:len(validToken)-5] + "XXXXX"
	req4 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req4.Header.Set("Authorization", "Bearer "+tamperedToken)
	rec4 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec4, req4)

	if rec4.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for tampered token, got %d", rec4.Code)
	}

	// 5. Valid token -> 200 OK with AuthIdentity in context
	req5 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req5.Header.Set("Authorization", "Bearer "+validToken)
	rec5 := httptest.NewRecorder()
	protectedHandler.ServeHTTP(rec5, req5)

	if rec5.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token, got %d (body: %s)", rec5.Code, rec5.Body.String())
	}
	var identityResp middleware.AuthIdentity
	if err := json.Unmarshal(rec5.Body.Bytes(), &identityResp); err != nil {
		t.Fatalf("failed to parse identity response: %v", err)
	}
	if identityResp.UserID.String() != userID {
		t.Errorf("expected UserID %s, got %s", userID, identityResp.UserID.String())
	}
	if identityResp.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got %s", identityResp.Email)
	}
	if len(identityResp.Capabilities) != 2 || !identityResp.HasCapability("CREATOR") {
		t.Errorf("expected capabilities [BUYER, CREATOR], got %v", identityResp.Capabilities)
	}
}
