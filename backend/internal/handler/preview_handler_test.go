package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPreview_PresignURLRules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live preview test in short mode")
	}
	cfg := config.LoadConfig()
	connStr := fmt.Sprintf("postgres://%s:%s@localhost:5433/%s?sslmode=disable", cfg.DBUser, cfg.DBPassword, cfg.DBName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	defer pool.Close()

	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "localhost:9010"
	}
	accessKey := os.Getenv("MINIO_ROOT_USER")
	if accessKey == "" {
		accessKey = "galeriku_admin"
	}
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if secretKey == "" {
		secretKey = "galeriku_minio_secret"
	}
	s3, err := storage.NewS3Service(s3Endpoint, accessKey, secretKey, false)
	if err != nil {
		t.Skipf("cannot build S3 service: %v", err)
	}

	// Seed: user + READY photo + a real preview object in MinIO.
	userID := uuid.Must(uuid.NewV7())
	email := fmt.Sprintf("preview_%s@galeriku-test.invalid", uuid.NewString()[:8])
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email_normalized, account_status) VALUES ($1, $2, 'ACTIVE')`, userID, email); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)

	photoID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `INSERT INTO photos (id, uploader_user_id, status, width_px, height_px, moderation_status) VALUES ($1, $2, 'READY', 800, 600, 'APPROVED')`, photoID, userID); err != nil {
		t.Fatalf("failed to seed photo: %v", err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM photo_prices WHERE photo_id = $1", photoID)
	defer pool.Exec(context.Background(), "DELETE FROM photos WHERE id = $1", photoID)

	if err := s3.PutPreviewObject(ctx, "previews/"+photoID.String()+".jpg", []byte("fake-jpeg-preview")); err != nil {
		t.Fatalf("failed to seed preview object: %v", err)
	}

	catalogRepo := repository.NewPostgresPhotoCatalogRepository(pool)
	router := handler.SetupRoutesWithFinance(
		pool, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		handler.NewCatalogHandler(catalogRepo),
		nil, // financeHandler
		handler.NewPreviewHandler(catalogRepo, s3),
		nil, // statsHandler
		[]byte("test_secret_32_bytes_long_ok!!"),
	)

	// 1. Happy path: a presigned URL with signature query params is returned.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/"+photoID.String()+"/preview-url", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for preview-url, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	data, _ := parsed["data"].(map[string]interface{})
	previewURL, _ := data["preview_url"].(string)
	if previewURL == "" {
		t.Fatalf("expected preview_url, got %v", data)
	}
	if !strings.Contains(previewURL, "X-Amz-Signature") {
		t.Fatalf("expected a presigned URL with signature, got %s", previewURL)
	}
	if key, _ := data["object_key"].(string); !strings.HasPrefix(key, "previews/") {
		t.Fatalf("expected object_key under previews/, got %q", key)
	}

	// 2. TTL clamp: requesting 9999 minutes must still succeed (clamped), not error.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/photos/"+photoID.String()+"/preview-url?ttl_minutes=9999", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with clamped TTL, got %d", rec.Code)
	}

	// 3. A photo that is not in the catalog must never be presigned.
	unknownID := uuid.Must(uuid.NewV7())
	req = httptest.NewRequest(http.MethodGet, "/api/v1/photos/"+unknownID.String()+"/preview-url", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown photo, got %d", rec.Code)
	}

	// 4. Malformed id returns 400.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/photos/not-a-uuid/preview-url", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed id, got %d", rec.Code)
	}
}
