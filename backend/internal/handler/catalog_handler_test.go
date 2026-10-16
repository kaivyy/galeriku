package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCatalog_PublicPhotoList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live catalog test in short mode")
	}
	cfg := config.LoadConfig()
	connStr := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, os.Getenv("DB_PORT_FALLBACK"), cfg.DBName)
	if os.Getenv("DB_PORT_FALLBACK") == "" {
		connStr = fmt.Sprintf("postgres://%s:%s@localhost:5433/%s?sslmode=disable",
			cfg.DBUser, cfg.DBPassword, cfg.DBName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	defer pool.Close()

	// Seed test data
	buyerID := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx, `INSERT INTO users (id, email_normalized, account_status) VALUES ($1, $2, 'ACTIVE')`,
		buyerID, fmt.Sprintf("catalog_test_%s@galeriku-test.invalid", uuid.New().String()[:8]))
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", buyerID)

	photoID := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx, `INSERT INTO photos (id, uploader_user_id, status, width_px, height_px, moderation_status) VALUES ($1, $2, 'READY', 800, 600, 'APPROVED')`, photoID, buyerID)
	if err != nil {
		t.Fatalf("failed to seed photo: %v", err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM photo_prices WHERE photo_id = $1", photoID)
	defer pool.Exec(context.Background(), "DELETE FROM photos WHERE id = $1", photoID)

	_, err = pool.Exec(ctx, `INSERT INTO photo_prices (photo_id, price_idr) VALUES ($1, 45000)`, photoID)
	if err != nil {
		t.Fatalf("failed to seed price: %v", err)
	}

	// Build router with catalog handler (nil for unused handlers; catalog is
	// the 14th positional arg, jwtSecret the 15th)
	catalogRepo := repository.NewPostgresPhotoCatalogRepository(pool)
	router := handler.SetupRoutesWithPayment(
		pool, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // paymentHandler
		handler.NewCatalogHandler(catalogRepo),
		[]byte("test_secret_32_bytes_long_ok!!"),
	)

	// GET /api/v1/photos should return the seeded photo
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos?limit=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for photo list, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	data, _ := parsed["data"].(map[string]interface{})
	photos, _ := data["photos"].([]interface{})
	if len(photos) == 0 {
		t.Fatal("expected at least 1 photo in catalog")
	}
	first, _ := photos[0].(map[string]interface{})
	if _, ok := first["photo_id"].(string); !ok {
		t.Fatal("expected photo_id in result")
	}

	// GET /api/v1/photos/{id} should return the specific photo
	req = httptest.NewRequest(http.MethodGet, "/api/v1/photos/"+photoID.String(), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for photo detail, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	data, _ = parsed["data"].(map[string]interface{})
	if data["photo_id"].(string) != photoID.String() {
		t.Fatalf("expected photo_id %s, got %v", photoID, data["photo_id"])
	}

	// Nonexistent photo returns 404
	req = httptest.NewRequest(http.MethodGet, "/api/v1/photos/"+uuid.New().String(), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent photo, got %d", rec.Code)
	}
}
