package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type uploadTestEnv struct {
	pool      *pgxpool.Pool
	router    http.Handler
	orgID     uuid.UUID
	accountID uuid.UUID
	leader    string
	outsider  string
	cleanup   func()
}

func newUploadTestEnv(t *testing.T) *uploadTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live upload batch integration test in short mode")
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
	uploadRepo := repository.NewPostgresUploadRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)

	router := handler.SetupRoutesWithUploads(
		pool, nil, nil, nil, tenantRepo, nil, nil, nil,
		handler.NewUploadBatchHandler(usecase.NewUploadBatchUsecase(uploadRepo, audit.NewService(pool))),
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

	leaderID, leaderToken := seed("upload_leader", "HOST_LEADER")
	_, outsiderToken := seed("upload_outsider", "BUYER")

	orgID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Upload Uji', 'Upload Test EO', 'Sports', 'APPROVED')`, orgID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE')`, orgID, leaderID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed membership: %v", err)
	}

	// Storage account for the organization, with a quota policy and a 1 GiB quota.
	accountID := uuid.Must(uuid.NewV7())
	policyID := uuid.Must(uuid.NewV7())
	quotaID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO storage_accounts (id, owner_type, owner_id, status)
		VALUES ($1, 'HOST_ORGANIZATION', $2, 'ACTIVE')`, accountID, orgID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed storage account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quota_policy_versions (id, policy_code, version_number, billable_categories, limit_bytes, effective_from)
		VALUES ($1, 'UPLOAD_TEST', 1, '["RAW","PREVIEW"]'::jsonb, 1073741824, CURRENT_TIMESTAMP)`, policyID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed quota policy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO storage_quotas (id, storage_account_id, policy_version_id, quota_limit_bytes, billable_bytes, reserved_bytes)
		VALUES ($1, $2, $3, 1073741824, 0, 0)`, quotaID, accountID, policyID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed storage quota: %v", err)
	}

	env := &uploadTestEnv{
		pool: pool, router: router, orgID: orgID, accountID: accountID,
		leader: leaderToken, outsider: outsiderToken,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM quota_reservations WHERE storage_quota_id = $1", quotaID)
		_, _ = pool.Exec(bg, "DELETE FROM upload_items WHERE upload_batch_id IN (SELECT id FROM upload_batches WHERE storage_account_id = $1)", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM upload_batches WHERE storage_account_id = $1", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM audit_logs WHERE action_code LIKE 'UPLOAD_%' AND metadata->>'storage_account_id' = $1", accountID.String())
		_, _ = pool.Exec(bg, "DELETE FROM storage_quotas WHERE storage_account_id = $1", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM quota_policy_versions WHERE id = $1", policyID)
		_, _ = pool.Exec(bg, "DELETE FROM storage_accounts WHERE id = $1", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", leaderID)
		_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", leaderID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", leaderID)
		pool.Close()
	}
	return env
}

func (e *uploadTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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
	req.Header.Set("X-Organization-ID", e.orgID.String())
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestUploadBatch_QuotaReadAndReservation(t *testing.T) {
	env := newUploadTestEnv(t)
	defer env.cleanup()

	// 1. Quota read shows the seeded 1 GiB limit with nothing consumed.
	rec, parsed := env.do(t, http.MethodGet, "/api/v1/storage/quota", env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for quota read, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	q := successData(parsed)
	if got, _ := q["quota_limit_bytes"].(float64); got != 1073741824 {
		t.Fatalf("expected limit 1073741824, got %v", q["quota_limit_bytes"])
	}
	if got, _ := q["remaining_bytes"].(float64); got != 1073741824 {
		t.Fatalf("expected full remaining quota, got %v", q["remaining_bytes"])
	}

	// 2. Initiate a batch; the reservation must be recorded atomically.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", env.leader, map[string]interface{}{
		"total_files": 2,
		"total_bytes": 200000000,
		"files": []map[string]interface{}{
			{"client_file_token": "tok_a", "original_filename": "A.JPG", "expected_bytes": 100000000},
			{"client_file_token": "tok_b", "original_filename": "B.JPG", "expected_bytes": 100000000},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for initiate, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	batchID, _ := data["upload_batch_id"].(string)
	if batchID == "" {
		t.Fatalf("expected upload_batch_id, got %v", data)
	}
	if status, _ := data["status"].(string); status != "RESERVED" {
		t.Fatalf("expected RESERVED status, got %q", status)
	}
	if got, _ := data["reserved_bytes"].(float64); got != 200000000 {
		t.Fatalf("expected reserved_bytes 200000000, got %v", data["reserved_bytes"])
	}

	// 3. The quota row must show the reservation.
	var reserved int64
	if err := env.pool.QueryRow(context.Background(),
		"SELECT reserved_bytes FROM storage_quotas WHERE storage_account_id = $1", env.accountID).Scan(&reserved); err != nil {
		t.Fatalf("failed to read quota: %v", err)
	}
	if reserved != 200000000 {
		t.Fatalf("expected reserved_bytes 200000000 in database, got %d", reserved)
	}

	// 4. Quota read now reports the reservation against the remaining balance.
	rec, parsed = env.do(t, http.MethodGet, "/api/v1/storage/quota", env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for quota read, got %d", rec.Code)
	}
	q = successData(parsed)
	if got, _ := q["reserved_bytes"].(float64); got != 200000000 {
		t.Fatalf("expected reserved_bytes 200000000, got %v", q["reserved_bytes"])
	}
	if got, _ := q["remaining_bytes"].(float64); got != 873741824 {
		t.Fatalf("expected remaining 873741824, got %v", q["remaining_bytes"])
	}

	// 5. The batch and its items exist, and the initiation is audited.
	var itemCount int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM upload_items WHERE upload_batch_id = $1", batchID).Scan(&itemCount); err != nil {
		t.Fatalf("failed to count upload items: %v", err)
	}
	if itemCount != 2 {
		t.Fatalf("expected 2 upload items, got %d", itemCount)
	}
	var auditCount int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM audit_logs WHERE action_code = 'UPLOAD_BATCH_RESERVED' AND target_id = $1", batchID).Scan(&auditCount); err != nil {
		t.Fatalf("failed to query audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 reservation audit entry, got %d", auditCount)
	}
}

func TestUploadBatch_OversubscriptionAndLimitsRejected(t *testing.T) {
	env := newUploadTestEnv(t)
	defer env.cleanup()

	// Exceeding the remaining quota is refused and reserves nothing.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", env.leader, map[string]interface{}{
		"total_files": 1, "total_bytes": 2000000000,
		"files": []map[string]interface{}{{"client_file_token": "tok_big", "expected_bytes": 2000000000}},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for oversubscription, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(parsed); code != "QUOTA_EXCEEDED" {
		t.Fatalf("expected QUOTA_EXCEEDED, got %q", code)
	}
	var reserved int64
	if err := env.pool.QueryRow(context.Background(),
		"SELECT reserved_bytes FROM storage_quotas WHERE storage_account_id = $1", env.accountID).Scan(&reserved); err != nil {
		t.Fatalf("failed to read quota: %v", err)
	}
	if reserved != 0 {
		t.Fatalf("a refused batch must not reserve bytes, got %d", reserved)
	}

	// A batch larger than the documented 2000-file ceiling is refused.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", env.leader, map[string]interface{}{
		"total_files": 2001, "total_bytes": 1000,
		"files": []map[string]interface{}{{"client_file_token": "tok_x", "expected_bytes": 1000}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 2001 files, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT, got %q", code)
	}

	// A batch of zero files is refused.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", env.leader, map[string]interface{}{
		"total_files": 0, "total_bytes": 0, "files": []map[string]interface{}{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty batch, got %d", rec.Code)
	}

	// A non-member cannot initiate against this organization's account.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", env.outsider, map[string]interface{}{
		"total_files": 1, "total_bytes": 1000,
		"files": []map[string]interface{}{{"client_file_token": "tok_y", "expected_bytes": 1000}},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d", rec.Code)
	}
}

func TestUploadBatch_ConcurrentInitiationRespectsQuota(t *testing.T) {
	env := newUploadTestEnv(t)
	defer env.cleanup()

	// Ten concurrent batches each request 30 percent of the quota. At most three can fit.
	const workers = 10
	const eachBytes = 322122547 // 30 percent of 1 GiB

	var wg sync.WaitGroup
	wg.Add(workers)
	var mu sync.Mutex
	accepted := 0

	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]interface{}{
				"total_files": 1, "total_bytes": eachBytes,
				"files": []map[string]interface{}{
					{"client_file_token": fmt.Sprintf("tok_%d", n), "expected_bytes": eachBytes},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/storage/upload-batches/initiate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+env.leader)
			req.Header.Set("X-Organization-ID", env.orgID.String())
			rec := httptest.NewRecorder()
			env.router.ServeHTTP(rec, req)
			if rec.Code == http.StatusCreated {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if accepted > 3 {
		t.Fatalf("quota accounting is not atomic: %d of %d concurrent batches were accepted, at most 3 fit", accepted, workers)
	}

	var reserved int64
	if err := env.pool.QueryRow(context.Background(),
		"SELECT reserved_bytes FROM storage_quotas WHERE storage_account_id = $1", env.accountID).Scan(&reserved); err != nil {
		t.Fatalf("failed to read quota: %v", err)
	}
	if reserved != int64(accepted)*eachBytes {
		t.Fatalf("reserved_bytes %d does not match %d accepted batches of %d bytes", reserved, accepted, eachBytes)
	}
	if reserved > 1073741824 {
		t.Fatalf("reserved_bytes %d exceeds the quota limit", reserved)
	}
}
