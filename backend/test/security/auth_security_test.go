// Package security_test holds the Phase 2 security regression suite. Each test
// corresponds to one documented attack class from the Task 2.9 acceptance criteria.
package security_test

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
	"github.com/redis/go-redis/v9"
)

const (
	testJWTSecret  = "test_super_secret_jwt_key_32_bytes_long_12345"
	testOTPSecret  = "test_otp_secret_key_32_bytes_long_67890"
	testBankKey    = "test_bank_cipher_key_32_bytes_ok"
)

type suiteEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
	clean  func()
}

func newSuiteEnv(t *testing.T) *suiteEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live security suite in short mode")
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

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6380"
	}
	rdb := redis.NewClient(&redis.Options{Addr: fmt.Sprintf("%s:%s", redisHost, redisPort)})

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

	userRepo := repository.NewPostgresUserRepository(pool)
	authRepo := repository.NewPostgresAuthRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)
	hostRepo := repository.NewPostgresHostRepository(pool)
	invitationRepo := repository.NewPostgresInvitationRepository(pool)
	bankRepo := repository.NewPostgresBankAccountRepository(pool)
	bankCipher, err := security.NewBankAccountCipher([]byte(testBankKey))
	if err != nil {
		pool.Close()
		t.Fatalf("failed to build bank cipher: %v", err)
	}

	auditSvc := audit.NewService(pool)
	limiter := security.NewRedisRateLimiter(rdb)

	authUsecase := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, auditSvc)

	router := handler.SetupRoutesWithInvitations(
		pool,
		handler.NewAuthHandler(authUsecase, testJWTSecret),
		handler.NewHostApplicationHandler(usecase.NewHostApplicationUsecase(hostRepo, s3)),
		handler.NewHostMembershipHandler(usecase.NewHostMembershipUsecase(hostRepo, auditSvc)),
		tenantRepo,
		handler.NewBankAccountHandler(usecase.NewBankAccountUsecase(bankRepo, bankCipher)),
		handler.NewInvitationHandler(usecase.NewInvitationUsecase(invitationRepo, auditSvc)),
		[]byte(testJWTSecret),
	)

	env := &suiteEnv{pool: pool, router: router}
	env.clean = func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE action_code LIKE 'TEST_%'")
		rdb.Close()
		pool.Close()
	}
	return env
}

func (e *suiteEnv) do(t *testing.T, method, path, token string, body interface{}, headers map[string]string) (*httptest.ResponseRecorder, map[string]interface{}) {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func errCode(parsed map[string]interface{}) string {
	body, _ := parsed["error"].(map[string]interface{})
	code, _ := body["code"].(string)
	return code
}

func (e *suiteEnv) seedUser(t *testing.T, prefix, capability string) (uuid.UUID, string, string) {
	t.Helper()
	ctx := context.Background()
	userRepo := repository.NewPostgresUserRepository(e.pool)
	email := fmt.Sprintf("%s_%s@galeriku.com", prefix, uuid.NewString()[:8])
	user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: prefix, Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, capability); err != nil {
		t.Fatalf("failed to seed %s: %v", prefix, err)
	}
	tok, err := security.GenerateAccessToken([]byte(testJWTSecret), user.ID.String(), email, []string{capability}, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to mint %s token: %v", prefix, err)
	}
	return user.ID, tok, email
}

// Criterion 1: no account enumeration through the login path.
// Unknown identifier and wrong password must be indistinguishable to the caller.
func TestSecurity_NoAccountEnumerationOnLogin(t *testing.T) {
	env := newSuiteEnv(t)
	defer env.clean()

	_, _, email := env.seedUser(t, "sec_enum", "BUYER")

	// Wrong password for an existing account.
	recKnown, parsedKnown := env.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"identifier": email, "password": "WrongPassword123!",
	}, nil)
	// Well-formed identifier that does not exist.
	recUnknown, parsedUnknown := env.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"identifier": fmt.Sprintf("nobody_%s@galeriku.com", uuid.NewString()[:8]), "password": "WrongPassword123!",
	}, nil)

	if recKnown.Code != http.StatusUnauthorized || recUnknown.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for both cases, got known=%d unknown=%d", recKnown.Code, recUnknown.Code)
	}
	if errCode(parsedKnown) != errCode(parsedUnknown) {
		t.Fatalf("login responses leak account existence: known=%q unknown=%q", errCode(parsedKnown), errCode(parsedUnknown))
	}
	if errCode(parsedKnown) != "INVALID_CREDENTIALS" {
		t.Fatalf("expected INVALID_CREDENTIALS, got %q", errCode(parsedKnown))
	}
}

// Criterion 2: presenting a rotated refresh token revokes the whole family.
func TestSecurity_NoRefreshTokenFamilyReuse(t *testing.T) {
	env := newSuiteEnv(t)
	defer env.clean()

	// Register a fresh account through the public endpoint to obtain real tokens.
	freshEmail := fmt.Sprintf("sec_reuse_login_%s@galeriku.com", uuid.NewString()[:8])
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": freshEmail, "password": "ValidPassword123!", "display_name": "Reuse Tester",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to register fixture account: %d %s", rec.Code, rec.Body.String())
	}

	rec, parsed = env.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"identifier": freshEmail, "password": "ValidPassword123!",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("failed to log in fixture account: %d %s", rec.Code, rec.Body.String())
	}
	data, _ := parsed["data"].(map[string]interface{})
	firstRefresh, _ := data["refresh_token"].(string)
	if firstRefresh == "" {
		t.Fatalf("expected a refresh token, got %v", data)
	}

	// Legitimate rotation consumes the first token.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refresh_token": firstRefresh,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("first rotation failed: %d %s", rec.Code, rec.Body.String())
	}

	// Replaying the consumed token must be rejected and must revoke the family.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{
		"refresh_token": firstRefresh,
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on refresh token replay, got %d", rec.Code)
	}
	if code := errCode(parsed); code != "TOKEN_REUSE_DETECTED" {
		t.Fatalf("expected TOKEN_REUSE_DETECTED, got %q", code)
	}

	defer func() {
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM auth_refresh_tokens WHERE user_id IN (SELECT id FROM users WHERE email_normalized LIKE 'sec_reuse%')")
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_capabilities WHERE user_id IN (SELECT id FROM users WHERE email_normalized LIKE 'sec_reuse%')")
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_profiles WHERE user_id IN (SELECT id FROM users WHERE email_normalized LIKE 'sec_reuse%')")
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_credentials WHERE user_id IN (SELECT id FROM users WHERE email_normalized LIKE 'sec_reuse%')")
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM users WHERE email_normalized LIKE 'sec_reuse%'")
	}()
}

// Criterion 3: a member of one organization cannot read another organization's resources.
func TestSecurity_NoIDORAcrossOrganizations(t *testing.T) {
	env := newSuiteEnv(t)
	defer env.clean()

	ctx := context.Background()
	leaderID, leaderToken, _ := env.seedUser(t, "sec_idor_leader", "HOST_LEADER")

	orgA := uuid.Must(uuid.NewV7())
	orgB := uuid.Must(uuid.NewV7())
	if _, err := env.pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT IDOR A', 'IDOR A', 'Sports', 'APPROVED'),
		       ($2, 'PT IDOR B', 'IDOR B', 'Music', 'APPROVED')`, orgA, orgB); err != nil {
		t.Fatalf("failed to seed organizations: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE')`, orgA, leaderID); err != nil {
		t.Fatalf("failed to seed membership: %v", err)
	}
	defer func() {
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM host_memberships WHERE host_organization_id IN ($1, $2)", orgA, orgB)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM host_organizations WHERE id IN ($1, $2)", orgA, orgB)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_capabilities WHERE user_id = $1", leaderID)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_profiles WHERE user_id = $1", leaderID)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", leaderID)
	}()

	// Same authenticated user, but claiming the other tenant.
	rec, _ := env.do(t, http.MethodGet, "/api/v1/hosts/organization", leaderToken, nil, map[string]string{
		"X-Organization-ID": orgB.String(),
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant read, got %d", rec.Code)
	}

	// The same request against the caller's own organization succeeds, proving the
	// rejection above was authorization and not a broken route.
	rec, _ = env.do(t, http.MethodGet, "/api/v1/hosts/organization", leaderToken, nil, map[string]string{
		"X-Organization-ID": orgA.String(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for own-tenant read, got %d", rec.Code)
	}
}

// Criterion 4: a Host ADMIN can never reach bank account routes or organization mutation.
func TestSecurity_NoHostAdminAccessToFinancialSettings(t *testing.T) {
	env := newSuiteEnv(t)
	defer env.clean()

	ctx := context.Background()
	adminID, adminToken, _ := env.seedUser(t, "sec_fin_admin", "BUYER")

	orgID := uuid.Must(uuid.NewV7())
	if _, err := env.pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Finance Guard', 'Finance Guard', 'Sports', 'APPROVED')`, orgID); err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'ADMIN', 'ACTIVE')`, orgID, adminID); err != nil {
		t.Fatalf("failed to seed admin membership: %v", err)
	}
	defer func() {
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM host_organizations WHERE id = $1", orgID)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_capabilities WHERE user_id = $1", adminID)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM user_profiles WHERE user_id = $1", adminID)
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", adminID)
	}()

	hdr := map[string]string{"X-Organization-ID": orgID.String()}

	for _, tc := range []struct {
		method, path string
		body         interface{}
	}{
		{http.MethodGet, "/api/v1/hosts/bank-accounts", nil},
		{http.MethodPost, "/api/v1/hosts/bank-accounts", map[string]string{
			"bank_code": "BCA", "account_number": "1234567890", "account_holder_name": "PT Finance Guard",
		}},
		{http.MethodPatch, "/api/v1/hosts/organization", map[string]string{"legal_name": "PT Diubah"}},
		{http.MethodDelete, "/api/v1/hosts/members/" + uuid.Must(uuid.NewV7()).String(), nil},
		{http.MethodPost, "/api/v1/hosts/invitations", map[string]string{
			"invited_email": "someone@galeriku.com", "membership_role": "ADMIN",
		}},
	} {
		rec, parsed := env.do(t, tc.method, tc.path, adminToken, tc.body, hdr)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: expected 403 for Host ADMIN, got %d", tc.method, tc.path, rec.Code)
		}
		if code := errCode(parsed); code != "AUTH_FORBIDDEN" {
			t.Fatalf("%s %s: expected AUTH_FORBIDDEN, got %q", tc.method, tc.path, code)
		}
	}
}

// Criterion 5: a hostile X-Request-ID can never be reflected or split the response.
func TestSecurity_NoHeaderInjectionOnRequestID(t *testing.T) {
	env := newSuiteEnv(t)
	defer env.clean()

	for _, hostile := range []string{
		"injected\r\nX-Injected-Header: evil",
		"injected\nX-Injected-Header: evil",
		strings.Repeat("a", 200),
		"bad id with spaces",
		"<script>alert(1)</script>",
	} {
		rec, _ := env.do(t, http.MethodGet, "/api/v1/health", "", nil, map[string]string{"X-Request-ID": hostile})
		echoed := rec.Header().Get("X-Request-ID")

		if strings.ContainsAny(echoed, "\r\n") {
			t.Fatalf("hostile request id was reflected with a control character: %q", echoed)
		}
		if echoed == hostile {
			t.Fatalf("hostile request id was echoed verbatim: %q", hostile)
		}
		if !strings.HasPrefix(echoed, "req_") {
			t.Fatalf("expected an auto-generated replacement id, got %q", echoed)
		}
		if rec.Header().Get("X-Injected-Header") != "" {
			t.Fatal("response was split: injected header is present")
		}
	}
}
