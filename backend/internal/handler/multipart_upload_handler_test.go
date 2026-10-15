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
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type multipartTestEnv struct {
	pool      *pgxpool.Pool
	router    http.Handler
	orgID     uuid.UUID
	accountID uuid.UUID
	quotaID   uuid.UUID
	policyID  uuid.UUID
	leader    string
	outsider  string
	batchID   string
	cleanup   func()
}

func newMultipartTestEnv(t *testing.T) *multipartTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live multipart upload integration test in short mode")
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
	uploadRepo := repository.NewPostgresUploadRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)
	auditSvc := audit.NewService(pool)

	router := handler.SetupRoutesWithMultipart(
		pool, nil, nil, nil, tenantRepo, nil, nil, nil,
		handler.NewUploadBatchHandler(usecase.NewUploadBatchUsecase(uploadRepo, auditSvc)),
		handler.NewMultipartUploadHandler(usecase.NewMultipartUploadUsecase(repository.NewPostgresMultipartRepository(pool), uploadRepo, s3, auditSvc)),
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

	leaderID, leaderToken := seed("mp_leader", "HOST_LEADER")
	_, outsiderToken := seed("mp_outsider", "BUYER")

	orgID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Multipart Uji', 'Multipart Test EO', 'Sports', 'APPROVED')`, orgID); err != nil {
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
		VALUES ($1, 'MULTIPART_TEST', 1, '["RAW"]'::jsonb, 10737418240, CURRENT_TIMESTAMP)`, policyID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed quota policy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO storage_quotas (id, storage_account_id, policy_version_id, quota_limit_bytes, billable_bytes, reserved_bytes)
		VALUES ($1, $2, $3, 10737418240, 0, 0)`, quotaID, accountID, policyID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed storage quota: %v", err)
	}

	env := &multipartTestEnv{
		pool: pool, router: router, orgID: orgID, accountID: accountID,
		quotaID: quotaID, policyID: policyID,
		leader: leaderToken, outsider: outsiderToken,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM multipart_upload_sessions WHERE upload_item_id IN (SELECT id FROM upload_items WHERE upload_batch_id IN (SELECT id FROM upload_batches WHERE storage_account_id = $1))", accountID)
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

func (e *multipartTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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

// initiateBatch creates one batch with a single file slot and returns its identifiers.
func (e *multipartTestEnv) initiateBatch(t *testing.T, token string, bytes int64) (batchID, itemToken string) {
	t.Helper()
	rec, parsed := e.do(t, http.MethodPost, "/api/v1/storage/upload-batches/initiate", token, map[string]interface{}{
		"total_files": 1, "total_bytes": bytes,
		"files": []map[string]interface{}{
			{"client_file_token": "mp_tok_" + uuid.NewString()[:8], "original_filename": "DSC_0001.JPG", "expected_bytes": bytes},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to initiate batch: %d %s", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	batchID, _ = data["upload_batch_id"].(string)

	var tok string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT client_file_token FROM upload_items WHERE upload_batch_id = $1 LIMIT 1", batchID).Scan(&tok); err != nil {
		t.Fatalf("failed to read item token: %v", err)
	}
	return batchID, tok
}

func TestMultipart_InitiateSignAndCompleteAgainstLiveS3(t *testing.T) {
	env := newMultipartTestEnv(t)
	defer env.cleanup()

	const fileBytes = 6 * 1024 * 1024 // two parts of 5 MiB plus remainder
	batchID, itemToken := env.initiateBatch(t, env.leader, fileBytes)

	// 1. Initiate the multipart session.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", env.leader, map[string]string{
		"upload_batch_id": batchID, "client_file_token": itemToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for multipart initiate, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	itemID, _ := data["upload_item_id"].(string)
	uploadID, _ := data["provider_upload_id"].(string)
	objectKey, _ := data["s3_object_key"].(string)
	if itemID == "" || uploadID == "" || objectKey == "" {
		t.Fatalf("expected item id, upload id, and object key, got %v", data)
	}
	if !strings.HasPrefix(objectKey, "raw-uploads/") {
		t.Fatalf("object key must live under raw-uploads/, got %q", objectKey)
	}

	// 2. Request a presigned URL for part 1 and upload real bytes to MinIO.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/storage/multipart/sign-part", env.leader, map[string]interface{}{
		"upload_item_id": itemID, "part_number": 1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for sign-part, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	part1URL, _ := successData(parsed)["presigned_url"].(string)
	if !strings.Contains(part1URL, "partNumber=1") || !strings.Contains(part1URL, "X-Amz-Signature") {
		t.Fatalf("expected a signed part URL for part 1, got %s", part1URL)
	}

	part1Body := bytes.Repeat([]byte("A"), 5*1024*1024)
	etag1 := putToPresignedURL(t, part1URL, part1Body)

	// 3. Part 2 carries the remainder.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/storage/multipart/sign-part", env.leader, map[string]interface{}{
		"upload_item_id": itemID, "part_number": 2,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for part 2, got %d", rec.Code)
	}
	part2URL, _ := successData(parsed)["presigned_url"].(string)
	part2Body := bytes.Repeat([]byte("B"), 1*1024*1024)
	etag2 := putToPresignedURL(t, part2URL, part2Body)

	// 4. Complete the multipart session with the real ETags from MinIO.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/storage/multipart/complete", env.leader, map[string]interface{}{
		"upload_item_id": itemID,
		"parts": []map[string]interface{}{
			{"part_number": 1, "etag": etag1},
			{"part_number": 2, "etag": etag2},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for complete, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if status, _ := successData(parsed)["status"].(string); status != "COMPLETED" {
		t.Fatalf("expected COMPLETED status, got %q", successData(parsed)["status"])
	}

	// 5. The item and the session reflect the completed upload.
	var itemStatus, sessionStatus string
	var storedKey string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT status, object_key FROM upload_items WHERE id = $1", itemID).Scan(&itemStatus, &storedKey); err != nil {
		t.Fatalf("failed to read item: %v", err)
	}
	if itemStatus != "UPLOADED" || storedKey != objectKey {
		t.Fatalf("expected UPLOADED with key %q, got %q with key %q", objectKey, itemStatus, storedKey)
	}
	if err := env.pool.QueryRow(context.Background(),
		"SELECT status FROM multipart_upload_sessions WHERE upload_item_id = $1", itemID).Scan(&sessionStatus); err != nil {
		t.Fatalf("failed to read session: %v", err)
	}
	if sessionStatus != "COMPLETED" {
		t.Fatalf("expected COMPLETED session, got %q", sessionStatus)
	}

	// 6. The completion is audited.
	var auditCount int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM audit_logs WHERE action_code = 'UPLOAD_ITEM_COMPLETED' AND target_id = $1", itemID).Scan(&auditCount); err != nil {
		t.Fatalf("failed to query audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 completion audit entry, got %d", auditCount)
	}
}

func TestMultipart_AuthorizationAndValidation(t *testing.T) {
	env := newMultipartTestEnv(t)
	defer env.cleanup()

	batchID, itemToken := env.initiateBatch(t, env.leader, 1024)

	// A non-member cannot initiate a multipart session for this organization's batch.
	rec, _ := env.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", env.outsider, map[string]string{
		"upload_batch_id": batchID, "client_file_token": itemToken,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d", rec.Code)
	}

	// An unknown batch is a 404.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", env.leader, map[string]string{
		"upload_batch_id": uuid.Must(uuid.NewV7()).String(), "client_file_token": itemToken,
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown batch, got %d", rec.Code)
	}

	// An unknown file token is a 404, so a caller cannot address another batch's item.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", env.leader, map[string]string{
		"upload_batch_id": batchID, "client_file_token": "not_a_real_token",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown file token, got %d", rec.Code)
	}

	// Establish a real session for the part-number checks.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", env.leader, map[string]string{
		"upload_batch_id": batchID, "client_file_token": itemToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup initiate failed: %d %s", rec.Code, rec.Body.String())
	}
	itemID, _ := successData(parsed)["upload_item_id"].(string)

	// Part numbers are bounded by the S3 limit of 10000.
	for _, bad := range []int{0, -1, 10001} {
		rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/multipart/sign-part", env.leader, map[string]interface{}{
			"upload_item_id": itemID, "part_number": bad,
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("part_number %d: expected 400, got %d", bad, rec.Code)
		}
	}

	// Completing with an empty part list is refused.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/multipart/complete", env.leader, map[string]interface{}{
		"upload_item_id": itemID, "parts": []map[string]interface{}{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty parts, got %d", rec.Code)
	}

	// A batch that already has an active session refuses a second initiation.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/storage/multipart/initiate", env.leader, map[string]string{
		"upload_batch_id": batchID, "client_file_token": itemToken,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a second active session, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// putToPresignedURL uploads bytes to a presigned S3 URL and returns the ETag header.
func putToPresignedURL(t *testing.T, url string, body []byte) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to build upload request: %v", err)
	}
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to upload part: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("part upload failed with status %d", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("part upload returned no ETag")
	}
	return etag
}
