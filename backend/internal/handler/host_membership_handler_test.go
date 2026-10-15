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

type membershipTestEnv struct {
	pool     *pgxpool.Pool
	router   http.Handler
	secret   []byte
	leaderID uuid.UUID
	leader   string
	adminID  uuid.UUID
	admin    string
	orgID    uuid.UUID
	cleanup  func()
}

func newMembershipTestEnv(t *testing.T) *membershipTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live host membership integration test in short mode")
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
	hostRepo := repository.NewPostgresHostRepository(pool)
	hostUsecase := usecase.NewHostApplicationUsecase(hostRepo, s3)
	membershipUsecase := usecase.NewHostMembershipUsecase(hostRepo, audit.NewService(pool))

	tenantRepo := repository.NewPostgresTenantRepository(pool)
	router := handler.SetupRoutesWithMembership(
		pool,
		nil,
		handler.NewHostApplicationHandler(hostUsecase),
		handler.NewHostMembershipHandler(membershipUsecase),
		tenantRepo,
		[]byte(jwtSecret),
	)

	unique := uuid.NewString()[:8]
	seed := func(prefix, capability string) (*domain.User, string) {
		email := fmt.Sprintf("%s_%s@galeriku.com", prefix, unique)
		user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
		profile := &domain.UserProfile{DisplayName: prefix, Locale: "id-ID"}
		creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
		if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, capability); err != nil {
			pool.Close()
			t.Fatalf("failed to seed %s user: %v", prefix, err)
		}
		tok, err := security.GenerateAccessToken([]byte(jwtSecret), user.ID.String(), email, []string{capability}, 15*time.Minute)
		if err != nil {
			pool.Close()
			t.Fatalf("failed to mint %s token: %v", prefix, err)
		}
		return user, tok
	}

	applicant, applicantToken := seed("membership_applicant", "BUYER")
	adminUser, adminToken := seed("membership_admin", "SUPER_ADMIN")

	// Applicant submits an application through the Task 2.5 endpoint.
	applyBody, _ := json.Marshal(map[string]string{
		"legal_name": "PT Uji Keanggotaan", "display_name": "Membership Test EO", "business_field": "Sports",
	})
	applyReq := httptest.NewRequest(http.MethodPost, "/api/v1/hosts/apply", bytes.NewReader(applyBody))
	applyReq.Header.Set("Content-Type", "application/json")
	applyReq.Header.Set("Authorization", "Bearer "+applicantToken)
	applyRec := httptest.NewRecorder()
	router.ServeHTTP(applyRec, applyReq)
	if applyRec.Code != http.StatusCreated {
		pool.Close()
		t.Fatalf("failed to create application fixture: %d %s", applyRec.Code, applyRec.Body.String())
	}
	var applyParsed map[string]interface{}
	_ = json.Unmarshal(applyRec.Body.Bytes(), &applyParsed)
	applicationID := successData(applyParsed)["application_id"].(string)

	var orgID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT host_organization_id FROM host_applications WHERE id = $1", applicationID).Scan(&orgID); err != nil {
		pool.Close()
		t.Fatalf("failed to read application org: %v", err)
	}

	env := &membershipTestEnv{
		pool:     pool,
		router:   router,
		secret:   []byte(jwtSecret),
		leaderID: applicant.ID,
		leader:   applicantToken,
		adminID:  adminUser.ID,
		admin:    adminToken,
		orgID:    orgID,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM audit_logs WHERE action_code LIKE 'HOST_%' AND metadata->>'organization_id' = $1", orgID.String())
		_, _ = pool.Exec(bg, "DELETE FROM host_admin_limits WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_legal_documents WHERE host_application_id IN (SELECT id FROM host_applications WHERE host_organization_id = $1)", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_applications WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id = $1", orgID)
		for _, id := range []uuid.UUID{applicant.ID, adminUser.ID} {
			_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", id)
		}
		pool.Close()
	}
	return env
}

func (e *membershipTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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
	// Tenant-scoped routes resolve the organization through the Task 2.4 middleware.
	if !strings.HasPrefix(path, "/api/v1/admin/") {
		req.Header.Set("X-Organization-ID", e.orgID.String())
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestHostMembership_ApprovalIsAtomicAndGrantsLeader(t *testing.T) {
	env := newMembershipTestEnv(t)
	defer env.cleanup()

	applicationID := env.applicationIDFor(t, env.orgID)

	// Non-platform-admin cannot approve.
	rec, _ := env.do(t, http.MethodPost, "/api/v1/admin/hosts/applications/"+applicationID+"/approve", env.leader, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-platform-admin approval, got %d", rec.Code)
	}

	// Platform admin approves.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/admin/hosts/applications/"+applicationID+"/approve", env.admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for approval, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	_ = parsed

	bg := context.Background()
	var orgStatus, appStatus, membershipRole, membershipStatus string
	err := env.pool.QueryRow(bg, `
		SELECT ho.verification_status, ha.status, hm.membership_role, hm.status
		FROM host_applications ha
		JOIN host_organizations ho ON ho.id = ha.host_organization_id
		JOIN host_memberships hm ON hm.host_organization_id = ho.id AND hm.user_id = ha.submitted_by
		WHERE ha.id = $1`, applicationID).Scan(&orgStatus, &appStatus, &membershipRole, &membershipStatus)
	if err != nil {
		t.Fatalf("approval did not produce membership: %v", err)
	}
	if orgStatus != "APPROVED" || appStatus != "APPROVED" {
		t.Fatalf("expected APPROVED statuses, got org=%q app=%q", orgStatus, appStatus)
	}
	if membershipRole != domain.HostRoleLeader || membershipStatus != domain.HostMembershipStatusActive {
		t.Fatalf("expected ACTIVE LEADER membership, got role=%q status=%q", membershipRole, membershipStatus)
	}

	var hasCapability bool
	if err := env.pool.QueryRow(bg, `
		SELECT EXISTS (
			SELECT 1 FROM user_capabilities uc JOIN capabilities c ON c.id = uc.capability_id
			WHERE uc.user_id = $1 AND c.code = 'HOST_LEADER' AND uc.status = 'ACTIVE'
		)`, env.leaderID).Scan(&hasCapability); err != nil {
		t.Fatalf("failed to check HOST_LEADER capability: %v", err)
	}
	if !hasCapability {
		t.Fatal("expected HOST_LEADER capability to be granted on approval")
	}

	var maxAdmins int
	if err := env.pool.QueryRow(bg,
		"SELECT max_admins FROM host_admin_limits WHERE host_organization_id = $1", env.orgID).Scan(&maxAdmins); err != nil {
		t.Fatalf("expected host_admin_limits row after approval: %v", err)
	}
	if maxAdmins != 5 {
		t.Fatalf("expected default admin quota 5, got %d", maxAdmins)
	}

	// The approval is a security mutation and must leave an audit trail naming the reviewer.
	var auditCount int
	if err := env.pool.QueryRow(bg, `
		SELECT COUNT(*) FROM audit_logs
		WHERE action_code = 'HOST_APPLICATION_APPROVED' AND target_id = $1 AND actor_user_id = $2`,
		applicationID, env.adminID).Scan(&auditCount); err != nil {
		t.Fatalf("failed to query approval audit log: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected exactly 1 approval audit entry, got %d", auditCount)
	}
}

func TestHostMembership_SecondActiveLeaderRejectedByConstraint(t *testing.T) {
	env := newMembershipTestEnv(t)
	defer env.cleanup()

	applicationID := env.applicationIDFor(t, env.orgID)
	if rec, _ := env.do(t, http.MethodPost, "/api/v1/admin/hosts/applications/"+applicationID+"/approve", env.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("approval failed: %d", rec.Code)
	}

	// A concurrent second LEADER insert must be refused by uq_active_host_leader.
	otherUser, _ := env.seedExtraUser(t, "second_leader")
	_, err := env.pool.Exec(context.Background(), `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE')`, env.orgID, otherUser)
	if err == nil {
		t.Fatal("expected unique constraint violation for a second active LEADER, got nil")
	}
}

func TestHostMembership_ReapprovalIsRejectedAndAudited(t *testing.T) {
	env := newMembershipTestEnv(t)
	defer env.cleanup()

	applicationID := env.applicationIDFor(t, env.orgID)
	if rec, _ := env.do(t, http.MethodPost, "/api/v1/admin/hosts/applications/"+applicationID+"/approve", env.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("first approval failed: %d", rec.Code)
	}

	// A second approval of the same application must be refused cleanly, not 500 and
	// not silently stack a second membership or capability row.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/admin/hosts/applications/"+applicationID+"/approve", env.admin, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on re-approval, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if code := errorCode(parsed); code != "APPLICATION_ALREADY_REVIEWED" {
		t.Fatalf("expected APPLICATION_ALREADY_REVIEWED, got %q", code)
	}

	bg := context.Background()
	var memberships, capabilities int
	if err := env.pool.QueryRow(bg,
		"SELECT COUNT(*) FROM host_memberships WHERE host_organization_id = $1 AND user_id = $2",
		env.orgID, env.leaderID).Scan(&memberships); err != nil {
		t.Fatalf("failed to count memberships: %v", err)
	}
	if memberships != 1 {
		t.Fatalf("expected exactly 1 membership after re-approval attempt, got %d", memberships)
	}
	if err := env.pool.QueryRow(bg, `
		SELECT COUNT(*) FROM user_capabilities uc
		JOIN capabilities c ON c.id = uc.capability_id
		WHERE uc.user_id = $1 AND c.code = 'HOST_LEADER'`, env.leaderID).Scan(&capabilities); err != nil {
		t.Fatalf("failed to count capabilities: %v", err)
	}
	if capabilities != 1 {
		t.Fatalf("expected exactly 1 HOST_LEADER capability row, got %d", capabilities)
	}
}

func TestHostMembership_AdminForbiddenOnLeaderOnlyOperations(t *testing.T) {
	env := newMembershipTestEnv(t)
	defer env.cleanup()

	applicationID := env.applicationIDFor(t, env.orgID)
	if rec, _ := env.do(t, http.MethodPost, "/api/v1/admin/hosts/applications/"+applicationID+"/approve", env.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("approval failed: %d", rec.Code)
	}

	// Promote a second user to ADMIN membership directly (invitations arrive in Task 2.8).
	adminMemberID, adminMemberToken := env.seedExtraUser(t, "org_admin")
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'ADMIN', 'ACTIVE')`, env.orgID, adminMemberID); err != nil {
		t.Fatalf("failed to seed ADMIN membership: %v", err)
	}

	// Leader can read the organization and sees quota counters.
	rec, parsed := env.do(t, http.MethodGet, "/api/v1/hosts/organization", env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for leader organization read, got %d", rec.Code)
	}
	orgData := successData(parsed)
	if got, _ := orgData["max_admin_count"].(float64); got != 5 {
		t.Fatalf("expected max_admin_count 5, got %v", orgData["max_admin_count"])
	}
	if got, _ := orgData["current_admin_count"].(float64); got != 1 {
		t.Fatalf("expected current_admin_count 1, got %v", orgData["current_admin_count"])
	}

	// Admin can read members but cannot mutate legal organization details.
	rec, _ = env.do(t, http.MethodGet, "/api/v1/hosts/members", adminMemberToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin member listing, got %d", rec.Code)
	}

	rec, _ = env.do(t, http.MethodPatch, "/api/v1/hosts/organization", adminMemberToken, map[string]string{
		"legal_name": "PT Diubah Admin",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin patching legal details, got %d", rec.Code)
	}

	// Admin cannot remove other members.
	rec, _ = env.do(t, http.MethodDelete, "/api/v1/hosts/members/"+env.leaderID.String(), adminMemberToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin deleting a member, got %d", rec.Code)
	}

	// Leader can patch legal details.
	rec, _ = env.do(t, http.MethodPatch, "/api/v1/hosts/organization", env.leader, map[string]string{
		"legal_name": "PT Uji Keanggotaan Baru",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for leader patch, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func (e *membershipTestEnv) applicationIDFor(t *testing.T, orgID uuid.UUID) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id FROM host_applications WHERE host_organization_id = $1", orgID).Scan(&id); err != nil {
		t.Fatalf("failed to locate fixture application: %v", err)
	}
	return id
}

func (e *membershipTestEnv) seedExtraUser(t *testing.T, prefix string) (uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	userRepo := repository.NewPostgresUserRepository(e.pool)
	email := fmt.Sprintf("%s_%s@galeriku.com", prefix, uuid.NewString()[:8])
	user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: prefix, Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "BUYER"); err != nil {
		t.Fatalf("failed to seed %s: %v", prefix, err)
	}
	tok, err := security.GenerateAccessToken(e.secret, user.ID.String(), email, []string{"BUYER"}, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to mint %s token: %v", prefix, err)
	}
	return user.ID, tok
}
