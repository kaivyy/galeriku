// Package loadtest implements the Phase 9 Task 9.1 acceptance test: 50
// concurrent photographers initiating batches that together would upload
// 2000 files, proving zero gateway choke on the initiate path.
//
// The test exercises the most resource-sensitive endpoint under load:
// POST /storage/upload-batches/initiate, which holds a FOR UPDATE lock
// on the quota row. If the gateway chokes, the test fails with a
// non-201 response from at least one worker.
package security_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
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

func TestLoadTest_50ConcurrentPhotographers_2000FilesTotal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	cfg := config.LoadConfig()
	connStr := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, os.Getenv("DB_PORT_FALLBACK"), cfg.DBName)
	if os.Getenv("DB_PORT_FALLBACK") == "" {
		connStr = fmt.Sprintf("postgres://%s:%s@localhost:5433/%s?sslmode=disable",
			cfg.DBUser, cfg.DBPassword, cfg.DBName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres ping failed: %v", err)
	}

	jwtSecret := "test_super_secret_jwt_key_32_bytes_long_12345"
	userRepo := repository.NewPostgresUserRepository(pool)
	uploadRepo := repository.NewPostgresUploadRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)

	router := handler.SetupRoutesWithFinalize(
		pool, nil, nil, nil, tenantRepo, nil, nil, nil,
		handler.NewUploadBatchHandler(usecase.NewUploadBatchUsecase(uploadRepo, nil)),
		nil, nil,
		[]byte(jwtSecret),
	)

	// Seed one organization + one LEADER membership + storage account + quota.
	orgID := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx, `INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status) VALUES ($1, 'PT Load Test', 'Load Test EO', 'Sports', 'APPROVED')`, orgID)
	if err != nil {
		t.Fatalf("failed to seed org: %v", err)
	}
	defer func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM upload_items WHERE upload_batch_id IN (SELECT id FROM upload_batches WHERE storage_account_id IN (SELECT id FROM storage_accounts WHERE owner_id = $1))", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM upload_batches WHERE storage_account_id IN (SELECT id FROM storage_accounts WHERE owner_id = $1)", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM storage_quotas WHERE storage_account_id IN (SELECT id FROM storage_accounts WHERE owner_id = $1)", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM storage_accounts WHERE owner_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id = $1", orgID)
	}()

	leaderID := uuid.Must(uuid.NewV7())
	leaderEmail := fmt.Sprintf("load_leader_%s@galeriku.com", uuid.NewString()[:8])
	user := &domain.User{EmailNormalized: &leaderEmail, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: "Load Leader", Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "HOST_LEADER"); err != nil {
		t.Fatalf("failed to seed leader: %v", err)
	}
	leaderID = user.ID
	defer func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", leaderID)
		_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", leaderID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", leaderID)
	}()

	_, err = pool.Exec(ctx, `INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status) VALUES ($1, $2, 'LEADER', 'ACTIVE')`, orgID, leaderID)
	if err != nil {
		t.Fatalf("failed to seed membership: %v", err)
	}

	accountID := uuid.Must(uuid.NewV7())
	policyID := uuid.Must(uuid.NewV7())
	quotaID := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(ctx, `INSERT INTO storage_accounts (id, owner_type, owner_id, status) VALUES ($1, 'HOST_ORGANIZATION', $2, 'ACTIVE')`, accountID, orgID)
	if err != nil {
		t.Fatalf("failed to seed storage account: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO quota_policy_versions (id, policy_code, version_number, billable_categories, limit_bytes, effective_from) VALUES ($1, 'LOAD_TEST', 1, '["RAW"]'::jsonb, 1099511627776, CURRENT_TIMESTAMP)`, policyID)
	if err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO storage_quotas (id, storage_account_id, policy_version_id, quota_limit_bytes, billable_bytes, reserved_bytes) VALUES ($1, $2, $3, 1099511627776, 0, 0)`, quotaID, accountID, policyID)
	if err != nil {
		t.Fatalf("failed to seed quota: %v", err)
	}

	tok, _ := security.GenerateAccessToken([]byte(jwtSecret), leaderID.String(), leaderEmail, []string{"HOST_LEADER"}, 30*time.Minute)

	// 50 workers, each initiating one batch of 40 files = 2000 total.
	const workers = 50
	const filesPerBatch = 40
	const bytesPerFile = 15_000_000 // 15 MB per file, total ~600 GB within the 1 TiB quota

	var wg sync.WaitGroup
	wg.Add(workers)
	var accepted int32
	var rejected int32

	for i := 0; i < workers; i++ {
		go func(n int) {
			defer wg.Done()
			files := make([]map[string]interface{}, filesPerBatch)
			for j := 0; j < filesPerBatch; j++ {
				files[j] = map[string]interface{}{
					"client_file_token":  fmt.Sprintf("load_%d_%d", n, j),
					"original_filename":  fmt.Sprintf("DSC_%04d.JPG", j),
					"expected_bytes":     bytesPerFile,
				}
			}
			body, _ := json.Marshal(map[string]interface{}{
				"total_files": filesPerBatch,
				"total_bytes": filesPerBatch * bytesPerFile,
				"files":       files,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/storage/upload-batches/initiate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("X-Organization-ID", orgID.String())
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusCreated {
				atomic.AddInt32(&accepted, 1)
			} else {
				atomic.AddInt32(&rejected, 1)
			}
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt32(&accepted) != workers {
		t.Fatalf("expected all %d workers to be accepted, got %d accepted and %d rejected", workers, accepted, rejected)
	}

	// Verify that the total reserved bytes matches 2000 files * 15 MB.
	var reserved int64
	err = pool.QueryRow(ctx, "SELECT reserved_bytes FROM storage_quotas WHERE id = $1", quotaID).Scan(&reserved)
	if err != nil {
		t.Fatalf("failed to read reserved bytes: %v", err)
	}
	expectedReserved := int64(workers * filesPerBatch * bytesPerFile)
	if reserved != expectedReserved {
		t.Fatalf("expected reserved_bytes = %d, got %d", expectedReserved, reserved)
	}

	t.Logf("Load test PASS: %d workers, %d files total, %d bytes reserved, zero rejections", workers, workers*filesPerBatch, reserved)
}
