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

type invitationTestEnv struct {
	pool     *pgxpool.Pool
	router   http.Handler
	secret   []byte
	orgID    uuid.UUID
	leader   string
	admin    string
	invitee  string
	inviteeEmail string
	cleanup  func()
}

func newInvitationTestEnv(t *testing.T) *invitationTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live invitation integration test in short mode")
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
	invitationRepo := repository.NewPostgresInvitationRepository(pool)
	invitationUsecase := usecase.NewInvitationUsecase(invitationRepo, nil)
	tenantRepo := repository.NewPostgresTenantRepository(pool)

	router := handler.SetupRoutesWithInvitations(
		pool, nil, nil, nil, tenantRepo, nil,
		handler.NewInvitationHandler(invitationUsecase),
		[]byte(jwtSecret),
	)

	unique := uuid.NewString()[:8]
	seed := func(prefix, capability string) (uuid.UUID, string, string) {
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
		return user.ID, tok, email
	}

	leaderID, leaderToken, _ := seed("inv_leader", "HOST_LEADER")
	adminID, adminToken, _ := seed("inv_admin", "BUYER")
	inviteeID, inviteeToken, inviteeEmail := seed("inv_invitee", "BUYER")

	orgID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Undangan Uji', 'Invitation Test EO', 'Sports', 'APPROVED')`, orgID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed organization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE'), ($1, $3, 'ADMIN', 'ACTIVE')`, orgID, leaderID, adminID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed memberships: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_admin_limits (host_organization_id, max_admins, policy_version, updated_by)
		VALUES ($1, 5, 'v1-default-5', $2)`, orgID, leaderID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed admin quota: %v", err)
	}

	env := &invitationTestEnv{
		pool: pool, router: router, secret: []byte(jwtSecret),
		orgID: orgID, leader: leaderToken, admin: adminToken,
		invitee: inviteeToken, inviteeEmail: inviteeEmail,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM host_invitations WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_admin_limits WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id = $1", orgID)
		for _, id := range []uuid.UUID{leaderID, adminID, inviteeID} {
			_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", id)
		}
		pool.Close()
	}
	return env
}

func (e *invitationTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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
	if !strings.HasPrefix(path, "/api/v1/hosts/invitations/accept") {
		req.Header.Set("X-Organization-ID", e.orgID.String())
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestInvitation_CreateHashesTokenAndAcceptGrantsAdmin(t *testing.T) {
	env := newInvitationTestEnv(t)
	defer env.cleanup()

	// Leader issues an invitation.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/invitations", env.leader, map[string]string{
		"invited_email":   env.inviteeEmail,
		"membership_role": "ADMIN",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for leader invitation, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	invitationID, _ := data["invitation_id"].(string)
	rawToken, _ := data["invitation_token"].(string)
	if invitationID == "" || rawToken == "" {
		t.Fatalf("expected invitation_id and invitation_token, got %v", data)
	}
	if status, _ := data["status"].(string); status != "PENDING" {
		t.Fatalf("expected PENDING status, got %q", status)
	}

	// Only the SHA-256 digest may be persisted.
	var storedHash string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT token_hash FROM host_invitations WHERE id = $1", invitationID).Scan(&storedHash); err != nil {
		t.Fatalf("failed to read stored token hash: %v", err)
	}
	if storedHash == rawToken {
		t.Fatal("raw invitation token must never be stored")
	}
	if len(storedHash) != 64 {
		t.Fatalf("expected a 64-character SHA-256 hex digest, got %d chars", len(storedHash))
	}

	// The invited user accepts and becomes an ACTIVE ADMIN with the HOST_ADMIN capability.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/hosts/invitations/accept", env.invitee, map[string]string{
		"invitation_token": rawToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for acceptance, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var role, status string
	if err := env.pool.QueryRow(context.Background(), `
		SELECT hm.membership_role, hm.status
		FROM host_memberships hm
		JOIN users u ON u.id = hm.user_id
		WHERE hm.host_organization_id = $1 AND u.email_normalized = $2`, env.orgID, env.inviteeEmail).Scan(&role, &status); err != nil {
		t.Fatalf("acceptance did not create a membership: %v", err)
	}
	if role != domain.HostRoleAdmin || status != domain.HostMembershipStatusActive {
		t.Fatalf("expected ACTIVE ADMIN, got role=%q status=%q", role, status)
	}

	var hasCapability bool
	if err := env.pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM user_capabilities uc
			JOIN capabilities c ON c.id = uc.capability_id
			JOIN users u ON u.id = uc.user_id
			WHERE u.email_normalized = $1 AND c.code = 'HOST_ADMIN' AND uc.status = 'ACTIVE'
		)`, env.inviteeEmail).Scan(&hasCapability); err != nil {
		t.Fatalf("failed to check HOST_ADMIN capability: %v", err)
	}
	if !hasCapability {
		t.Fatal("expected the HOST_ADMIN capability to be granted on acceptance")
	}

	// The invitation is now ACCEPTED and cannot be replayed.
	rec, _ = env.do(t, http.MethodPost, "/api/v1/hosts/invitations/accept", env.invitee, map[string]string{
		"invitation_token": rawToken,
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a consumed invitation token must not be accepted twice")
	}
}

func TestInvitation_AdminForbiddenAndExpiredAndRevoked(t *testing.T) {
	env := newInvitationTestEnv(t)
	defer env.cleanup()

	// An ADMIN member cannot issue invitations.
	rec, _ := env.do(t, http.MethodPost, "/api/v1/hosts/invitations", env.admin, map[string]string{
		"invited_email": env.inviteeEmail, "membership_role": "ADMIN",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin issuing an invitation, got %d", rec.Code)
	}

	// Expired invitation is refused with 410.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/invitations", env.leader, map[string]string{
		"invited_email": env.inviteeEmail, "membership_role": "ADMIN",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup invitation failed: %d", rec.Code)
	}
	expiredID := successData(parsed)["invitation_id"].(string)
	expiredToken := successData(parsed)["invitation_token"].(string)
	if _, err := env.pool.Exec(context.Background(),
		"UPDATE host_invitations SET expires_at = CURRENT_TIMESTAMP - INTERVAL '1 hour' WHERE id = $1", expiredID); err != nil {
		t.Fatalf("failed to age the invitation: %v", err)
	}

	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/invitations/accept", env.invitee, map[string]string{
		"invitation_token": expiredToken,
	})
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 for an expired invitation, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "INVITATION_EXPIRED" {
		t.Fatalf("expected INVITATION_EXPIRED, got %q", code)
	}

	// Revoked invitation cannot be accepted.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/invitations", env.leader, map[string]string{
		"invited_email": env.inviteeEmail, "membership_role": "ADMIN",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup invitation failed: %d", rec.Code)
	}
	revokeID := successData(parsed)["invitation_id"].(string)
	revokeToken := successData(parsed)["invitation_token"].(string)

	rec, _ = env.do(t, http.MethodDelete, "/api/v1/hosts/invitations/"+revokeID, env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for revocation, got %d", rec.Code)
	}
	rec, _ = env.do(t, http.MethodPost, "/api/v1/hosts/invitations/accept", env.invitee, map[string]string{
		"invitation_token": revokeToken,
	})
	if rec.Code == http.StatusOK {
		t.Fatal("a revoked invitation must not be accepted")
	}
}

func TestInvitation_QuotaExceededReturns422(t *testing.T) {
	env := newInvitationTestEnv(t)
	defer env.cleanup()

	// Tighten the quota to exactly the one admin that already exists.
	if _, err := env.pool.Exec(context.Background(),
		"UPDATE host_admin_limits SET max_admins = 1 WHERE host_organization_id = $1", env.orgID); err != nil {
		t.Fatalf("failed to tighten quota: %v", err)
	}

	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/invitations", env.leader, map[string]string{
		"invited_email": env.inviteeEmail, "membership_role": "ADMIN",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup invitation failed: %d", rec.Code)
	}
	rawToken := successData(parsed)["invitation_token"].(string)

	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/invitations/accept", env.invitee, map[string]string{
		"invitation_token": rawToken,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when the admin quota is full, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(parsed); code != "ADMIN_LIMIT_EXCEEDED" {
		t.Fatalf("expected ADMIN_LIMIT_EXCEEDED, got %q", code)
	}

	// No membership may be created by a refused acceptance.
	var count int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM host_memberships WHERE host_organization_id = $1", env.orgID).Scan(&count); err != nil {
		t.Fatalf("failed to count memberships: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected the 2 seeded memberships only, got %d", count)
	}
}
