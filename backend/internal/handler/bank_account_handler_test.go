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

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type bankTestEnv struct {
	pool    *pgxpool.Pool
	router  http.Handler
	secret  []byte
	leader  string
	admin   string
	stranger string
	orgID   uuid.UUID
	leaderID uuid.UUID
	cleanup func()
}

func newBankTestEnv(t *testing.T) *bankTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live bank account integration test in short mode")
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
	cipherKey := "test_bank_cipher_key_32_bytes_ok"

	userRepo := repository.NewPostgresUserRepository(pool)
	bankRepo := repository.NewPostgresBankAccountRepository(pool)
	bankCipher, err := security.NewBankAccountCipher([]byte(cipherKey))
	if err != nil {
		pool.Close()
		t.Fatalf("failed to build bank cipher: %v", err)
	}
	bankUsecase := usecase.NewBankAccountUsecase(bankRepo, bankCipher)

	tenantRepo := repository.NewPostgresTenantRepository(pool)
	router := handler.SetupRoutesWithBankAccounts(
		pool, nil, nil, nil, tenantRepo,
		handler.NewBankAccountHandler(bankUsecase),
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

	leaderID, leaderToken := seed("bank_leader", "HOST_LEADER")
	adminID, adminToken := seed("bank_admin", "BUYER")
	strangerID, strangerToken := seed("bank_stranger", "BUYER")

	// Organization fixture with one LEADER and one ADMIN membership.
	orgID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Bank Uji', 'Bank Test EO', 'Sports', 'APPROVED')`, orgID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE'), ($1, $3, 'ADMIN', 'ACTIVE')`, orgID, leaderID, adminID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed memberships: %v", err)
	}

	env := &bankTestEnv{
		pool: pool, router: router, secret: []byte(jwtSecret),
		leader: leaderToken, admin: adminToken, stranger: strangerToken,
		orgID: orgID, leaderID: leaderID,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM host_bank_accounts WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM audit_logs WHERE target_id = $1 OR actor_user_id = $2", orgID, leaderID)
		_, _ = pool.Exec(bg, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id = $1", orgID)
		for _, id := range []uuid.UUID{leaderID, adminID, strangerID} {
			_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", id)
		}
		pool.Close()
	}
	return env
}

func (e *bankTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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

func TestBankAccount_LeaderRegistersAndReadsMasked(t *testing.T) {
	env := newBankTestEnv(t)
	defer env.cleanup()

	// Leader registers an account.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/bank-accounts", env.leader, map[string]string{
		"bank_code":           "BCA",
		"account_number":      "1234567890123456",
		"account_holder_name": "PT Bank Uji",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for leader registration, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	if masked, _ := data["account_number_masked"].(string); masked != "1234xxxxxxxx3456" {
		t.Fatalf("expected masked account 1234xxxxxxxx3456, got %q", masked)
	}
	if _, leaked := data["account_number"]; leaked {
		t.Fatal("response must not echo the raw account number")
	}

	// Database must hold ciphertext, never the plaintext.
	var stored string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT account_number_encrypted FROM host_bank_accounts WHERE host_organization_id = $1", env.orgID,
	).Scan(&stored); err != nil {
		t.Fatalf("failed to read stored account: %v", err)
	}
	if strings.Contains(stored, "1234567890123456") {
		t.Fatal("stored value must be encrypted, found plaintext account number")
	}

	// Leader reads the masked list.
	rec, parsed = env.do(t, http.MethodGet, "/api/v1/hosts/bank-accounts", env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for leader read, got %d", rec.Code)
	}
	accounts, _ := successData(parsed)["bank_accounts"].([]interface{})
	if len(accounts) != 1 {
		t.Fatalf("expected 1 bank account, got %d", len(accounts))
	}
	first, _ := accounts[0].(map[string]interface{})
	if masked, _ := first["account_number_masked"].(string); masked != "1234xxxxxxxx3456" {
		t.Fatalf("expected masked number in list, got %q", masked)
	}

	// The write must be recorded in audit_logs against the created account.
	accountID, _ := data["id"].(string)
	if accountID == "" {
		t.Fatal("expected the created account id in the response")
	}
	var auditCount int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM audit_logs
		WHERE action_code = 'HOST_BANK_ACCOUNT_REGISTERED' AND target_type = 'host_bank_account' AND target_id = $1`, accountID).Scan(&auditCount); err != nil {
		t.Fatalf("failed to query audit log: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected exactly 1 audit log entry, got %d", auditCount)
	}

	// The audit metadata must not contain the raw account number.
	var metadata string
	if err := env.pool.QueryRow(context.Background(), `
		SELECT metadata::text FROM audit_logs
		WHERE action_code = 'HOST_BANK_ACCOUNT_REGISTERED' AND target_id = $1`, accountID).Scan(&metadata); err != nil {
		t.Fatalf("failed to read audit metadata: %v", err)
	}
	if strings.Contains(metadata, "1234567890123456") {
		t.Fatalf("audit metadata must never contain the raw account number: %s", metadata)
	}
}

func TestBankAccount_HostAdminAndStrangerForbidden(t *testing.T) {
	env := newBankTestEnv(t)
	defer env.cleanup()

	body := map[string]string{
		"bank_code":           "BNI",
		"account_number":      "9999888877776666",
		"account_holder_name": "PT Bank Uji",
	}

	// HOST_ADMIN membership must be refused on both verbs.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/bank-accounts", env.admin, body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin POST, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "AUTH_FORBIDDEN" {
		t.Fatalf("expected AUTH_FORBIDDEN, got %q", code)
	}

	rec, parsed = env.do(t, http.MethodGet, "/api/v1/hosts/bank-accounts", env.admin, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin GET, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "AUTH_FORBIDDEN" {
		t.Fatalf("expected AUTH_FORBIDDEN, got %q", code)
	}

	// A non-member cannot reach the endpoint at all.
	rec, _ = env.do(t, http.MethodGet, "/api/v1/hosts/bank-accounts", env.stranger, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d", rec.Code)
	}

	// Nothing was persisted by the denied attempts.
	var count int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM host_bank_accounts WHERE host_organization_id = $1", env.orgID).Scan(&count); err != nil {
		t.Fatalf("failed to count accounts: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected zero persisted accounts after denials, got %d", count)
	}
}
