package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/database"
)

func TestHealthEndpointResponse(t *testing.T) {
	router := SetupRoutes(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp SuccessEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}

	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", resp.Data)
	}

	if dataMap["status"] != "UP" {
		t.Errorf("expected status 'UP', got '%v'", dataMap["status"])
	}
	if dataMap["service"] != "galeriku-core-api" {
		t.Errorf("expected service 'galeriku-core-api', got '%v'", dataMap["service"])
	}
	if dataMap["database"] != "UNKNOWN" {
		t.Errorf("expected database 'UNKNOWN' when pool is nil, got '%v'", dataMap["database"])
	}
	if resp.Meta.RequestID == "" || !strings.HasPrefix(resp.Meta.RequestID, "req_") {
		t.Errorf("expected auto-generated request ID with prefix 'req_', got '%s'", resp.Meta.RequestID)
	}
	if rec.Header().Get("X-Request-ID") != resp.Meta.RequestID {
		t.Errorf("expected response header X-Request-ID '%s', got '%s'", resp.Meta.RequestID, rec.Header().Get("X-Request-ID"))
	}
}

func TestHealthEndpoint_CustomRequestID(t *testing.T) {
	router := SetupRoutes(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-ID", "custom-trace-id-999")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp SuccessEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}

	if resp.Meta.RequestID != "custom-trace-id-999" {
		t.Errorf("expected request_id 'custom-trace-id-999', got '%s'", resp.Meta.RequestID)
	}
	if rec.Header().Get("X-Request-ID") != "custom-trace-id-999" {
		t.Errorf("expected response header X-Request-ID 'custom-trace-id-999', got '%s'", rec.Header().Get("X-Request-ID"))
	}
}

func TestHealthEndpoint_WithDatabasePool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live database test in short mode")
	}

	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}

	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5433"
	}

	cfg := &config.Config{
		DBHost:     dbHost,
		DBPort:     dbPort,
		DBUser:     "galeriku",
		DBPassword: "galeriku_secret_pass",
		DBName:     "galeriku_db",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		t.Skipf("skipping live database test: %v", err)
	}

	// 1. Test live pool (status UP)
	router := SetupRoutes(pool)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp SuccessEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}

	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", resp.Data)
	}

	if dataMap["status"] != "UP" {
		t.Errorf("expected status 'UP', got '%v'", dataMap["status"])
	}
	if dataMap["database"] != "UP" {
		t.Errorf("expected database status 'UP', got '%v'", dataMap["database"])
	}

	// 2. Test closed pool (status DOWN)
	pool.Close()
	recClosed := httptest.NewRecorder()
	router.ServeHTTP(recClosed, req)

	if recClosed.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", recClosed.Code)
	}

	var respClosed SuccessEnvelope
	if err := json.Unmarshal(recClosed.Body.Bytes(), &respClosed); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}

	dataClosedMap, ok := respClosed.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", respClosed.Data)
	}

	if dataClosedMap["status"] != "DEGRADED" {
		t.Errorf("expected status 'DEGRADED' after pool closed, got '%v'", dataClosedMap["status"])
	}
	if dataClosedMap["database"] != "DOWN" {
		t.Errorf("expected database status 'DOWN' after pool closed, got '%v'", dataClosedMap["database"])
	}
}
