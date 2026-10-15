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
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type discoveryTestEnv struct {
	pool       *pgxpool.Pool
	router     http.Handler
	buyer      string
	buyerID    uuid.UUID
	seller     string
	secret     []byte
	otpIssuer  *security.OTPIssuer
	cleanup    func()
}

func newDiscoveryTestEnv(t *testing.T) *discoveryTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live discovery integration test in short mode")
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

	jwtSecret := "test_super_secret_jwt_key_32_bytes_long_12345"
	userRepo := repository.NewPostgresUserRepository(pool)
	discoveryRepo := repository.NewPostgresDiscoveryRepository(pool)
	otpIssuer, _ := security.NewOTPIssuer([]byte("test_otp_secret_key_32_bytes_long_67890"))
	discoveryUsecase := usecase.NewDiscoveryUsecase(discoveryRepo, otpIssuer)

	router := handler.SetupRoutesWithDiscovery(
		pool, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		handler.NewDiscoveryHandler(discoveryUsecase),
		[]byte(jwtSecret),
	)

	unique := uuid.NewString()[:8]
	seed := func(prefix, capability string) (uuid.UUID, string) {
		email := fmt.Sprintf("%s_%s@galeriku.com", prefix, unique)
		user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
		profile := &domain.UserProfile{DisplayName: prefix, Locale: "id-ID"}
		creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
		if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, capability); err != nil {
			pool.Close()
			t.Fatalf("failed to seed %s: %v", prefix, err)
		}
		tok, err := security.GenerateAccessToken([]byte(jwtSecret), user.ID.String(), email, []string{capability}, 15*time.Minute)
		if err != nil {
			pool.Close()
			t.Fatalf("failed to mint %s token: %v", prefix, err)
		}
		return user.ID, tok
	}

	buyerID, buyerToken := seed("disc_buyer", "BUYER")
	_, sellerToken := seed("disc_seller", "CREATOR")

	env := &discoveryTestEnv{
		pool: pool, router: router,
		buyer: buyerToken, buyerID: buyerID, seller: sellerToken, secret: []byte(jwtSecret),
		otpIssuer: otpIssuer,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM order_idempotency_keys WHERE buyer_user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE buyer_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM carts WHERE buyer_user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM order_status_events WHERE order_id IN (SELECT id FROM orders WHERE buyer_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM orders WHERE buyer_user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM checkout_verification_grants WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM verification_challenges WHERE checkout_verification_id IN (SELECT id FROM checkout_verifications WHERE user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM checkout_verifications WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM ai_search_sessions WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM photo_bib_tags WHERE photo_id IN (SELECT id FROM photos WHERE uploader_user_id = $1)", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM photos WHERE uploader_user_id = $1", buyerID)
		for _, id := range []uuid.UUID{buyerID, uuid.Must(uuid.NewV7())} {
			_ = id
		}
		_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", buyerID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", buyerID)
		pool.Close()
		_ = sellerToken
	}
	return env
}

// seedPhoto creates one READY photo owned by the buyer and returns its id.
func (e *discoveryTestEnv) seedPhoto(t *testing.T, bibNumber string) string {
	t.Helper()
	photoID := uuid.Must(uuid.NewV7())
	if _, err := e.pool.Exec(context.Background(), `
		INSERT INTO photos (id, uploader_user_id, status, width_px, height_px, moderation_status)
		VALUES ($1, $2, 'READY', 800, 600, 'APPROVED')`, photoID, e.buyerID); err != nil {
		t.Fatalf("failed to seed photo: %v", err)
	}
	if bibNumber != "" {
		if _, err := e.pool.Exec(context.Background(), `
			INSERT INTO photo_bib_tags (photo_id, bib_normalized, source, review_status)
			VALUES ($1, $2, 'AI_OCR', 'CONFIRMED')`, photoID, bibNumber); err != nil {
			t.Fatalf("failed to seed bib tag: %v", err)
		}
	}
	return photoID.String()
}

// setPhotoPrice seeds the product price by inserting an order_item-free pricing hook.
// Pricing in this phase is derived from the photo_events catalog; the test uses a
// fixed test price carried by the usecase.
func (e *discoveryTestEnv) setPhotoPrice(t *testing.T, photoID string, priceIDR int64) {
	t.Helper()
	// Prices live in order_items as snapshots; for discovery the catalog uses a flat
	// test price table seeded via the discovery repository.
	if _, err := e.pool.Exec(context.Background(), `
		INSERT INTO photo_prices (photo_id, price_idr)
		VALUES ($1, $2)
		ON CONFLICT (photo_id) DO UPDATE SET price_idr = EXCLUDED.price_idr
	`, uuid.Must(uuid.Parse(photoID)), priceIDR); err != nil {
		t.Fatalf("failed to seed photo price: %v", err)
	}
}

func (e *discoveryTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestDiscovery_BIBSearchPublic(t *testing.T) {
	env := newDiscoveryTestEnv(t)
	defer env.cleanup()

	photoID := env.seedPhoto(t, "10452")
	env.setPhotoPrice(t, photoID, 45000)

	// A different bib must not match.
	env.seedPhoto(t, "99999")

	// The endpoint is public: no token required.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search/bib", bytes.NewReader([]byte(`{"bib_number":"10452"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for bib search, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	data := successData(parsed)
	if got, _ := data["bib_normalized"].(string); got != "10452" {
		t.Fatalf("expected normalized bib 10452, got %q", got)
	}
	if got, _ := data["total_photos"].(float64); got != 1 {
		t.Fatalf("expected 1 photo, got %v", data["total_photos"])
	}
	photos, _ := data["photos"].([]interface{})
	first, _ := photos[0].(map[string]interface{})
	if requires, _ := first["bib_checkout_requires_verification"].(bool); !requires {
		t.Fatalf("expected BIB-bound checkout to require verification, got %v", first)
	}
}

func TestDiscovery_CartFlow(t *testing.T) {
	env := newDiscoveryTestEnv(t)
	defer env.cleanup()

	photoID := env.seedPhoto(t, "")
	env.setPhotoPrice(t, photoID, 45000)

	// Adding the same photo twice must not duplicate the cart line.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/cart/items", env.buyer, map[string]string{"photo_id": photoID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for add to cart, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	rec, _ = env.do(t, http.MethodPost, "/api/v1/cart/items", env.buyer, map[string]string{"photo_id": photoID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for duplicate add, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	t.Logf("first add body: %s", rec.Body.String())

	rec, parsed = env.do(t, http.MethodGet, "/api/v1/cart", env.buyer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cart read, got %d", rec.Code)
	}
	data := successData(parsed)
	if got, _ := data["total_items"].(float64); got != 1 {
		t.Fatalf("expected 1 cart item after duplicate add, got %v", data["total_items"])
	}
	if got, _ := data["total_amount_idr"].(float64); got != 45000 {
		t.Fatalf("expected 45000 total, got %v", data["total_amount_idr"])
	}

	items, _ := data["items"].([]interface{})
	itemID, _ := items[0].(map[string]interface{})["cart_item_id"].(string)

	// Delete the item.
	rec, _ = env.do(t, http.MethodDelete, "/api/v1/cart/items/"+itemID, env.buyer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cart item delete, got %d", rec.Code)
	}
	rec, parsed = env.do(t, http.MethodGet, "/api/v1/cart", env.buyer, nil)
	if got, _ := successData(parsed)["total_items"].(float64); got != 0 {
		t.Fatalf("expected empty cart after delete, got %v", successData(parsed)["total_items"])
	}
}

func TestDiscovery_BIBVerificationGrantFlow(t *testing.T) {
	env := newDiscoveryTestEnv(t)
	defer env.cleanup()

	// 1. Initiate a checkout verification challenge.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/checkout/verify-bib/initiate", env.buyer, map[string]string{
		"bib_number": "10452",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for initiate, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	verificationID, _ := successData(parsed)["verification_id"].(string)
	if verificationID == "" {
		t.Fatalf("expected verification_id, got %v", successData(parsed))
	}

	// 2. The OTP code is only in the challenge hash; a wrong code is refused.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/checkout/verify-bib/confirm", env.buyer, map[string]string{
		"verification_id": verificationID, "code": "000000",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong code, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 3. Recover the code from the OTP issuer log in the test env (the issuer stores
	//    the last code for tests through a deterministic override).
	realCode := env.lastIssuedCode(t, verificationID)
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/checkout/verify-bib/confirm", env.buyer, map[string]string{
		"verification_id": verificationID, "code": realCode,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct code, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	grantToken, _ := successData(parsed)["grant_token"].(string)
	if grantToken == "" {
		t.Fatalf("expected grant_token, got %v", successData(parsed))
	}

	// 4. The grant is one-shot: consuming it at order placement is later verified
	//    by the order usecase; here we confirm it exists as a row.
	var grantCount int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM checkout_verification_grants WHERE verification_id = $1", verificationID).Scan(&grantCount); err != nil {
		t.Fatalf("failed to query grants: %v", err)
	}
	if grantCount != 1 {
		t.Fatalf("expected 1 grant row, got %d", grantCount)
	}
}

func TestDiscovery_OrdersWithIdempotency(t *testing.T) {
	env := newDiscoveryTestEnv(t)
	defer env.cleanup()

	photo1 := env.seedPhoto(t, "10452")
	photo2 := env.seedPhoto(t, "")
	env.setPhotoPrice(t, photo1, 45000)
	env.setPhotoPrice(t, photo2, 45000)

	body := map[string]interface{}{
		"photo_ids": []string{photo1, photo2},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.buyer)
	req.Header.Set("Idempotency-Key", "order-key-001")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for order create, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var first map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for order create, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	orderID, _ := successData(first)["order_id"].(string)
	if orderID == "" {
		t.Fatalf("expected order_id, got %v", successData(first))
	}
	totalIDR := successData(first)["total_amount_idr"].(float64)
	if totalIDR != 90000 {
		t.Fatalf("expected 90000 total, got %v", totalIDR)
	}

	// Replaying the same Idempotency-Key returns the same order, not a new one.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.buyer)
	req.Header.Set("Idempotency-Key", "order-key-001")
	rec = httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	t.Logf("replay status: %d body: %s", rec.Code, rec.Body.String())
	var second map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	secondOrderID, _ := successData(second)["order_id"].(string)
	if secondOrderID != orderID {
		t.Fatalf("expected the same order id for a replayed idempotency key, got %v vs %v",
			successData(second)["order_id"], orderID)
	}

	// Order status flow must be tracked.
	var statusCount int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM order_status_events WHERE order_id = $1", orderID).Scan(&statusCount); err != nil {
		t.Fatalf("failed to query status events: %v", err)
	}
	if statusCount < 1 {
		t.Fatalf("expected order status history, got %d", statusCount)
	}

	// A different key creates a different order.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.buyer)
	req.Header.Set("Idempotency-Key", "order-key-002")
	rec = httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	var third map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &third)
	if successData(third)["order_id"].(string) == orderID {
		t.Fatal("a different idempotency key must create a new order")
	}
}

// lastIssuedCode returns the code the issuer stored for this verification entity,
// so the confirm step can succeed with the real plaintext. Production never exposes this.
func (e *discoveryTestEnv) lastIssuedCode(t *testing.T, verificationID string) string {
	t.Helper()
	code, ok := e.otpIssuer.PeekSink(verificationID)
	if !ok {
		t.Fatalf("no issued code found for verification %s in the test sink", verificationID)
	}
	return code
}
