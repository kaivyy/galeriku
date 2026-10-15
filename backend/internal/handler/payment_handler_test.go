package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type paymentTestEnv struct {
	pool    *pgxpool.Pool
	router  http.Handler
	buyer   string
	buyerID uuid.UUID
	s3      *storage.S3Service
	cleanup func()
}

func newPaymentTestEnv(t *testing.T) *paymentTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live payment integration test in short mode")
	}

	cfg := config.LoadConfig()
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5433"
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, dbHost, dbPort, cfg.DBName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}

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
		pool.Close()
		t.Skipf("cannot build S3 service: %v", err)
	}

	jwtSecret := "test_super_secret_jwt_key_32_bytes_long_12345"
	userRepo := repository.NewPostgresUserRepository(pool)
	discoveryRepo := repository.NewPostgresDiscoveryRepository(pool)
	paymentRepo := repository.NewPostgresPaymentRepository(pool)
	otpIssuer, _ := security.NewOTPIssuer([]byte("test_otp_secret_key_32_bytes_long_67890"))

	discoveryUsecase := usecase.NewDiscoveryUsecase(discoveryRepo, otpIssuer)
	paymentGateway := usecase.StubPaymentGateway{}
	paymentUsecase := usecase.NewPaymentUsecase(paymentRepo, paymentGateway, "test_midtrans_server_key", "http://localhost:8080", nil)

	router := handler.SetupRoutesWithPayment(
		pool, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		handler.NewDiscoveryHandler(discoveryUsecase),
		handler.NewPaymentHandler(paymentUsecase, s3),
		[]byte(jwtSecret),
	)

	unique := uuid.NewString()[:8]
	buyerID := uuid.Must(uuid.NewV7())
	buyerEmail := fmt.Sprintf("pay_buyer_%s@galeriku.com", unique)
	user := &domain.User{EmailNormalized: &buyerEmail, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: "Pay Buyer", Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "BUYER"); err != nil {
		pool.Close()
		t.Fatalf("failed to seed buyer: %v", err)
	}
	buyerID = user.ID
	tok, err := security.GenerateAccessToken([]byte(jwtSecret), buyerID.String(), buyerEmail, []string{"BUYER"}, 15*time.Minute)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to mint token: %v", err)
	}

	env := &paymentTestEnv{
		pool: pool, router: router, buyer: tok, buyerID: buyerID, s3: s3,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM download_events WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM download_grants WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM fulfillment_events WHERE fulfillment_job_id IN (SELECT id FROM fulfillment_jobs WHERE order_item_id IN (SELECT id FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1)))", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM fulfillment_jobs WHERE order_item_id IN (SELECT id FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1))", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM payment_events WHERE payment_id IN (SELECT id FROM payments WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1))", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM payments WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM order_status_events WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM order_idempotency_keys WHERE buyer_user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM orders WHERE buyer_user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM photo_bib_tags WHERE photo_id IN (SELECT id FROM photos WHERE uploader_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM photo_prices WHERE photo_id IN (SELECT id FROM photos WHERE uploader_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM photos WHERE uploader_user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", buyerID)
		pool.Close()
	}
	return env
}

func (e *paymentTestEnv) seedPhotoAndOrder(t *testing.T) (orderID string, orderItemID string) {
	t.Helper()
	ctx := context.Background()
	photoID := uuid.Must(uuid.NewV7())
	if _, err := e.pool.Exec(ctx, `
		INSERT INTO photos (id, uploader_user_id, status, width_px, height_px, moderation_status)
		VALUES ($1, $2, 'READY', 800, 600, 'APPROVED')`, photoID, e.buyerID); err != nil {
		t.Fatalf("failed to seed photo: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO photo_prices (photo_id, price_idr) VALUES ($1, 45000)`, photoID); err != nil {
		t.Fatalf("failed to seed price: %v", err)
	}

	// Upload a preview object to MinIO so the download path can stream it.
	previewKey := "previews/" + photoID.String() + ".jpg"
	if err := e.s3.PutPreviewObject(ctx, previewKey, []byte("fake-jpeg-data")); err != nil {
		t.Fatalf("failed to seed preview object: %v", err)
	}

	// Create the order through the API so idempotency keys work.
	body, _ := json.Marshal(map[string]interface{}{"photo_ids": []string{photoID.String()}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.buyer)
	req.Header.Set("Idempotency-Key", "pay-test-"+uuid.NewString()[:8])
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create order: %d %s", rec.Code, rec.Body.String())
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	orderID = successData(parsed)["order_id"].(string)

	// Fetch the order item id from DB.
	err := e.pool.QueryRow(ctx, "SELECT id::text FROM order_items WHERE order_id = $1 LIMIT 1", orderID).Scan(&orderItemID)
	if err != nil {
		t.Fatalf("failed to read order item: %v", err)
	}
	return
}

func (e *paymentTestEnv) do(t *testing.T, method, path string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.buyer)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestPayment_InitiateWebhookDownload(t *testing.T) {
	env := newPaymentTestEnv(t)
	defer env.cleanup()

	orderID, orderItemID := env.seedPhotoAndOrder(t)

	// 1. Initiate payment.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/payments/initiate", map[string]string{
		"order_id":       orderID,
		"payment_method": "QRIS",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for initiate, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	paymentID, _ := successData(parsed)["payment_id"].(string)
	if paymentID == "" {
		t.Fatalf("expected payment_id, got %v", successData(parsed))
	}
	if got, _ := successData(parsed)["amount_idr"].(float64); got != 45000 {
		t.Fatalf("expected 45000 amount, got %v", successData(parsed)["amount_idr"])
	}

	// 2. Simulate a Midtrans settlement webhook with the signature the gateway expects.
	gateway := usecase.StubPaymentGateway{}
	event := domain.WebhookEvent{
		OrderID:           orderID, // full order UUID; the webhook resolves it directly
		TransactionStatus: "settlement",
		GrossAmount:       "45000.00",
		TransactionID:     uuid.NewString(),
	}
	event.SignatureKey = gatewaySignatureForTest(gateway, event)

	webhookBody, _ := json.Marshal(event)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/payments/midtrans", bytes.NewReader(webhookBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for webhook, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var parsed2 map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed2)
	if got, _ := successData(parsed2)["processed"].(bool); !got {
		t.Fatalf("expected webhook processed=true, got %v", successData(parsed2))
	}

	// 3. Verify the order is now PAID.
	var orderStatus string
	if err := env.pool.QueryRow(context.Background(), "SELECT status FROM orders WHERE id = $1", orderID).Scan(&orderStatus); err != nil {
		t.Fatalf("failed to read order: %v", err)
	}
	if orderStatus != "PAID" {
		t.Fatalf("expected PAID, got %q", orderStatus)
	}

	// 4. Verify fulfillment jobs were created with a 48-hour deadline.
	var jobCount int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM fulfillment_jobs WHERE order_item_id = $1`, orderItemID).Scan(&jobCount); err != nil {
		t.Fatalf("failed to query fulfillment jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("expected 1 fulfillment job, got %d", jobCount)
	}
	var hiresDueAt time.Time
	if err := env.pool.QueryRow(context.Background(),
		"SELECT hires_due_at FROM fulfillment_jobs WHERE order_item_id = $1", orderItemID).Scan(&hiresDueAt); err != nil {
		t.Fatalf("failed to read SLA deadline: %v", err)
	}
	if time.Until(hiresDueAt) > 49*time.Hour || time.Until(hiresDueAt) < 47*time.Hour {
		t.Fatalf("expected ~48h SLA, got %v until %v", time.Until(hiresDueAt), hiresDueAt)
	}

	// 5. Request a Non-Hi-Res download grant (DEC-Q01).
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/downloads/grant/"+orderItemID, map[string]string{
		"requested_asset": "NON_HIRES_DOWNLOAD",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for download grant, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	downloadToken, _ := successData(parsed)["download_token"].(string)
	if downloadToken == "" {
		t.Fatalf("expected download_token, got %v", successData(parsed))
	}
	if wm, _ := successData(parsed)["watermark_applied"].(string); wm == "" {
		t.Fatalf("expected watermark_applied for non-hires, got empty")
	}

	// 6. Download the file using the token (public endpoint).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/downloads/file/"+downloadToken, nil)
	rec = httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for download, got %d", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty download body")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("expected image/jpeg content type, got %q", ct)
	}

	// 7. A download event was recorded for audit.
	var downloadEvents int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM download_events WHERE download_grant_id = $1`, downloadToken).Scan(&downloadEvents); err != nil {
		t.Fatalf("failed to query download events: %v", err)
	}
	if downloadEvents != 1 {
		t.Fatalf("expected 1 download event, got %d", downloadEvents)
	}
}

func TestPayment_FulfillmentUploadMaster(t *testing.T) {
	env := newPaymentTestEnv(t)
	defer env.cleanup()

	orderID, orderItemID := env.seedPhotoAndOrder(t)

	// Settle the order directly so fulfillment jobs exist.
	if _, err := env.pool.Exec(context.Background(),
		"UPDATE orders SET status = 'PAID' WHERE id = $1", orderID); err != nil {
		t.Fatalf("failed to mark order paid: %v", err)
	}
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO fulfillment_jobs (order_item_id, status, hires_due_at, attempt_count)
		VALUES ($1, 'WAITING_MASTER', CURRENT_TIMESTAMP + INTERVAL '48 hours', 0)`, orderItemID); err != nil {
		t.Fatalf("failed to seed fulfillment job: %v", err)
	}

	// The creator uploads the master.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/fulfillment/upload-master/"+orderItemID, map[string]string{
		"storage_object_key": "hires/test/master.jpg",
		"sha256_checksum":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for upload master, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if got, _ := successData(parsed)["fulfillment_status"].(string); got != "FULFILLED" {
		t.Fatalf("expected FULFILLED, got %q", got)
	}
	if ready, _ := successData(parsed)["hires_download_ready"].(bool); !ready {
		t.Fatalf("expected hires_download_ready=true, got %v", successData(parsed)["hires_download_ready"])
	}

	// A second upload is refused.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/fulfillment/upload-master/"+orderItemID, map[string]string{
		"storage_object_key": "hires/test/master2.jpg",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for repeated fulfillment, got %d", rec.Code)
	}
}

// gatewaySignatureForTest computes the exact signature VerifyWebhookSignature expects.
func gatewaySignatureForTest(_ usecase.PaymentGateway, event domain.WebhookEvent) string {
	return computeStubSignature("test_midtrans_server_key", event)
}
