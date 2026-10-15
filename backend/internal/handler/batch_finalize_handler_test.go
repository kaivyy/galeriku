package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/messaging"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type finalizeTestEnv struct {
	pool      *pgxpool.Pool
	router    http.Handler
	orgID     uuid.UUID
	accountID uuid.UUID
	quotaID   uuid.UUID
	policyID  uuid.UUID
	leader    string
	outsider  string
	s3        *storage.S3Service
	cleanup   func()
}

func newFinalizeTestEnv(t *testing.T) *finalizeTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live batch finalize integration test in short mode")
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

	rabbitHost := os.Getenv("RABBITMQ_HOST")
	if rabbitHost == "" {
		rabbitHost = "localhost"
	}
	rabbitPort := os.Getenv("RABBITMQ_PORT")
	if rabbitPort == "" {
		rabbitPort = "5672"
	}
	rabbitUser := os.Getenv("RABBITMQ_USER")
	if rabbitUser == "" {
		rabbitUser = "guest"
	}
	rabbitPass := os.Getenv("RABBITMQ_PASSWORD")
	if rabbitPass == "" {
		rabbitPass = "guest"
	}
	publisher, err := messaging.NewRabbitPublisher(
		fmt.Sprintf("amqp://%s:%s@%s:%s/", rabbitUser, rabbitPass, rabbitHost, rabbitPort),
		"ai.photo.events.test."+uuid.NewString()[:8],
	)
	if err != nil {
		pool.Close()
		t.Skipf("cannot connect to rabbitmq: %v", err)
	}

	jwtSecret := "test_super_secret_jwt_key_32_bytes_long_12345"
	userRepo := repository.NewPostgresUserRepository(pool)
	uploadRepo := repository.NewPostgresUploadRepository(pool)
	multipartRepo := repository.NewPostgresMultipartRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)
	auditSvc := audit.NewService(pool)

	router := handler.SetupRoutesWithFinalize(
		pool, nil, nil, nil, tenantRepo, nil, nil, nil,
		handler.NewUploadBatchHandler(usecase.NewUploadBatchUsecase(uploadRepo, auditSvc)),
		handler.NewMultipartUploadHandler(usecase.NewMultipartUploadUsecase(multipartRepo, uploadRepo, s3, auditSvc)),
		handler.NewBatchFinalizeHandler(usecase.NewBatchFinalizeUsecase(uploadRepo, s3, publisher, auditSvc)),
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

	leaderID, leaderToken := seed("fin_leader", "HOST_LEADER")
	_, outsiderToken := seed("fin_outsider", "BUYER")

	orgID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Finalize Uji', 'Finalize Test EO', 'Sports', 'APPROVED')`, orgID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE')`, orgID, leaderID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed membership: %v", err)
	}

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
		VALUES ($1, 'FINALIZE_TEST', 1, '["RAW"]'::jsonb, 10737418240, CURRENT_TIMESTAMP)`, policyID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed quota policy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO storage_quotas (id, storage_account_id, policy_version_id, quota_limit_bytes, billable_bytes, reserved_bytes)
		VALUES ($1, $2, $3, 10737418240, 0, 0)`, quotaID, accountID, policyID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed storage quota: %v", err)
	}

	env := &finalizeTestEnv{
		pool: pool, router: router, orgID: orgID, accountID: accountID,
		quotaID: quotaID, policyID: policyID,
		leader: leaderToken, outsider: outsiderToken, s3: s3,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM multipart_upload_sessions WHERE upload_item_id IN (SELECT id FROM upload_items WHERE upload_batch_id IN (SELECT id FROM upload_batches WHERE storage_account_id = $1))", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM quota_reservations WHERE storage_quota_id = $1", quotaID)
		_, _ = pool.Exec(bg, "DELETE FROM upload_items WHERE upload_batch_id IN (SELECT id FROM upload_batches WHERE storage_account_id = $1)", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM upload_batches WHERE storage_account_id = $1", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM storage_usage_entries WHERE storage_account_id = $1", accountID)
		_, _ = pool.Exec(bg, "DELETE FROM storage_objects WHERE storage_account_id = $1", accountID)
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

func (e *finalizeTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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

// uploadOneFile runs the whole path for a single file: initiate batch, open a multipart
// session, upload real bytes to MinIO, and complete it.
func (e *finalizeTestEnv) uploadOneFile(t *testing.T, fileBytes int) (batchID string, expected int64) {
	t.Helper()
	rec, parsed := e.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", e.leader, map[string]interface{}{
		"total_files": 1, "total_bytes": fileBytes,
		"files": []map[string]interface{}{
			{"client_file_token": "fin_tok_" + uuid.NewString()[:8], "original_filename": "F.JPG", "expected_bytes": fileBytes},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("initiate failed: %d %s", rec.Code, rec.Body.String())
	}
	batchID = successData(parsed)["upload_batch_id"].(string)

	var itemToken string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT client_file_token FROM upload_items WHERE upload_batch_id = $1", batchID).Scan(&itemToken); err != nil {
		t.Fatalf("failed to read item token: %v", err)
	}

	rec, parsed = e.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", e.leader, map[string]string{
		"upload_batch_id": batchID, "client_file_token": itemToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("multipart initiate failed: %d %s", rec.Code, rec.Body.String())
	}
	itemID := successData(parsed)["upload_item_id"].(string)

	rec, parsed = e.do(t, http.MethodPost, "/api/v1/storage/multipart/sign-part", e.leader, map[string]interface{}{
		"upload_item_id": itemID, "part_number": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-part failed: %d", rec.Code)
	}
	etag := putToPresignedURL(t, successData(parsed)["presigned_url"].(string), bytes.Repeat([]byte("Z"), fileBytes))

	rec, parsed = e.do(t, http.MethodPost, "/api/v1/storage/multipart/complete", e.leader, map[string]interface{}{
		"upload_item_id": itemID,
		"parts":          []map[string]interface{}{{"part_number": 1, "etag": etag}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("complete failed: %d %s", rec.Code, rec.Body.String())
	}
	return batchID, int64(fileBytes)
}

func TestBatchFinalize_ReconcilesQuotaAndPublishesEvent(t *testing.T) {
	env := newFinalizeTestEnv(t)
	defer env.cleanup()

	batchID, uploadedBytes := env.uploadOneFile(t, 2*1024*1024)

	// Before finalization the bytes sit in reserved_bytes, not billable_bytes.
	var reserved, billable int64
	if err := env.pool.QueryRow(context.Background(),
		"SELECT reserved_bytes, billable_bytes FROM storage_quotas WHERE storage_account_id = $1", env.accountID,
	).Scan(&reserved, &billable); err != nil {
		t.Fatalf("failed to read quota: %v", err)
	}
	if billable != 0 {
		t.Fatalf("expected nothing billable before finalize, got %d", billable)
	}
	if reserved < uploadedBytes {
		t.Fatalf("expected the reservation to cover the upload, got reserved=%d for %d bytes", reserved, uploadedBytes)
	}

	// Finalize.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/"+batchID+"/finalize", env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for finalize, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	if status, _ := data["status"].(string); status != "COMPLETED" {
		t.Fatalf("expected COMPLETED batch status, got %q", status)
	}
	if got, _ := data["total_files_uploaded"].(float64); got != 1 {
		t.Fatalf("expected 1 uploaded file, got %v", data["total_files_uploaded"])
	}
	if got, _ := data["actual_bytes"].(float64); got != float64(uploadedBytes) {
		t.Fatalf("expected actual_bytes %d, got %v", uploadedBytes, data["actual_bytes"])
	}
	if reconciled, _ := data["quota_reconciled"].(bool); !reconciled {
		t.Fatalf("expected quota_reconciled true, got %v", data["quota_reconciled"])
	}
	if published, _ := data["event_published"].(bool); !published {
		t.Fatalf("expected event_published true, got %v", data["event_published"])
	}

	// Quota moved from reserved to billable, and the reservation is committed.
	if err := env.pool.QueryRow(context.Background(),
		"SELECT reserved_bytes, billable_bytes FROM storage_quotas WHERE storage_account_id = $1", env.accountID,
	).Scan(&reserved, &billable); err != nil {
		t.Fatalf("failed to read quota: %v", err)
	}
	if billable != uploadedBytes {
		t.Fatalf("expected billable_bytes %d, got %d", uploadedBytes, billable)
	}
	if reserved != 0 {
		t.Fatalf("expected reserved_bytes back to 0, got %d", reserved)
	}

	var reservationStatus string
	var committedBytes int64
	if err := env.pool.QueryRow(context.Background(),
		"SELECT status, committed_bytes FROM quota_reservations WHERE upload_batch_id = $1", batchID,
	).Scan(&reservationStatus, &committedBytes); err != nil {
		t.Fatalf("failed to read reservation: %v", err)
	}
	if reservationStatus != "COMMITTED" || committedBytes != uploadedBytes {
		t.Fatalf("expected COMMITTED reservation of %d bytes, got %q / %d", uploadedBytes, reservationStatus, committedBytes)
	}

	// The batch and item statuses are final, and the usage entry is append-only evidence.
	var batchStatus string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT status FROM upload_batches WHERE id = $1", batchID).Scan(&batchStatus); err != nil {
		t.Fatalf("failed to read batch: %v", err)
	}
	if batchStatus != "COMPLETED" {
		t.Fatalf("expected COMPLETED batch, got %q", batchStatus)
	}
	var usageCount int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM storage_usage_entries
		WHERE storage_account_id = $1 AND event_type = 'UPLOAD_COMMITTED'`, env.accountID).Scan(&usageCount); err != nil {
		t.Fatalf("failed to query usage entries: %v", err)
	}
	if usageCount != 1 {
		t.Fatalf("expected 1 committed usage entry, got %d", usageCount)
	}

	// Each uploaded object is queued as a pending AI job so the Phase 4 worker can pick it up.
	// The idempotency key ties one job to one object, so a retried finalize cannot duplicate work.
	var jobCount int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM ai_jobs
		WHERE job_type = 'PHOTO_INGEST' AND status = 'PENDING' AND idempotency_key LIKE $1`,
		"upload:"+batchID+":%").Scan(&jobCount); err != nil {
		t.Fatalf("ai_jobs lookup failed: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("expected exactly 1 pending AI job for the uploaded object, got %d", jobCount)
	}

	// Finalizing twice is refused rather than double-billing.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/"+batchID+"/finalize", env.leader, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on a repeated finalize, got %d", rec.Code)
	}
	if err := env.pool.QueryRow(context.Background(),
		"SELECT billable_bytes FROM storage_quotas WHERE storage_account_id = $1", env.accountID).Scan(&billable); err != nil {
		t.Fatalf("failed to read quota: %v", err)
	}
	if billable != uploadedBytes {
		t.Fatalf("a refused second finalize must not double-bill: billable=%d, expected %d", billable, uploadedBytes)
	}
}

func TestBatchFinalize_AuthorizationAndUnknownBatch(t *testing.T) {
	env := newFinalizeTestEnv(t)
	defer env.cleanup()

	batchID, _ := env.uploadOneFile(t, 512*1024)

	rec, _ := env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/"+batchID+"/finalize", env.outsider, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-member, got %d", rec.Code)
	}

	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/"+uuid.Must(uuid.NewV7()).String()+"/finalize", env.leader, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown batch, got %d", rec.Code)
	}

	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/upload-batches/not-a-uuid/finalize", env.leader, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a malformed batch id, got %d", rec.Code)
	}

	if !strings.Contains(strings.ToLower(rec.Body.String()), "uuid") {
		t.Fatalf("expected the error to name the offending field, got %s", rec.Body.String())
	}
}
