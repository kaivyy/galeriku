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
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fotoTreeTestEnv struct {
	pool     *pgxpool.Pool
	router   http.Handler
	orgID    uuid.UUID
	leader   string
	admin    string
	outsider string
	cleanup  func()
}

func newFotoTreeTestEnv(t *testing.T) *fotoTreeTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live FotoTree integration test in short mode")
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
	eventRepo := repository.NewPostgresEventRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)
	auditSvc := audit.NewService(pool)

	router := handler.SetupRoutesWithFotoTree(
		pool, nil, nil, nil, tenantRepo, nil, nil,
		handler.NewEventTreeHandler(usecase.NewEventTreeUsecase(eventRepo, auditSvc)),
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

	leaderID, leaderToken := seed("tree_leader", "HOST_LEADER")
	adminID, adminToken := seed("tree_admin", "BUYER")
	outsiderID, outsiderToken := seed("tree_outsider", "BUYER")

	orgID := uuid.Must(uuid.NewV7())
	otherOrgID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT FotoTree Uji', 'FotoTree Test EO', 'Sports', 'APPROVED'),
		       ($2, 'PT Lain', 'Other EO', 'Music', 'APPROVED')`, orgID, otherOrgID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed organizations: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE'), ($1, $3, 'ADMIN', 'ACTIVE')`, orgID, leaderID, adminID); err != nil {
		pool.Close()
		t.Fatalf("failed to seed memberships: %v", err)
	}

	env := &fotoTreeTestEnv{
		pool: pool, router: router, orgID: orgID,
		leader: leaderToken, admin: adminToken, outsider: outsiderToken,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM event_locations WHERE event_id IN (SELECT id FROM events WHERE host_organization_id = $1)", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM event_sessions WHERE event_id IN (SELECT id FROM events WHERE host_organization_id = $1)", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM audit_logs WHERE action_code LIKE 'EVENT_%' AND metadata->>'organization_id' = $1", orgID.String())
		_, _ = pool.Exec(bg, "DELETE FROM events WHERE host_organization_id IN ($1, $2)", orgID, otherOrgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id IN ($1, $2)", orgID, otherOrgID)
		for _, id := range []uuid.UUID{leaderID, adminID, outsiderID} {
			_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", id)
			_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", id)
		}
		pool.Close()
	}
	return env
}

func (e *fotoTreeTestEnv) do(t *testing.T, method, path, token string, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
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

func TestFotoTree_EventLifecycleWithSessionsAndLocations(t *testing.T) {
	env := newFotoTreeTestEnv(t)
	defer env.cleanup()

	// 1. Create an event as Leader.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/events", env.leader, map[string]interface{}{
		"name":       "Jakarta Marathon 2026",
		"slug":       "jakarta-marathon-2026-" + uuid.NewString()[:6],
		"event_type": "RUNNING_MARATHON",
		"starts_at":  "2026-10-25T05:00:00Z",
		"ends_at":    "2026-10-25T13:00:00Z",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for event create, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	eventID, _ := successData(parsed)["id"].(string)
	if eventID == "" {
		t.Fatalf("expected event id, got %v", successData(parsed))
	}
	if status, _ := successData(parsed)["status"].(string); status != "DRAFT" {
		t.Fatalf("expected DRAFT status, got %q", status)
	}

	// 2. Duplicate slug in the same organization is refused.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/events", env.leader, map[string]interface{}{
		"name": "Duplikat", "slug": successData(parsed)["slug"], "event_type": "RUNNING_MARATHON",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate slug, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "SLUG_ALREADY_EXISTS" {
		t.Fatalf("expected SLUG_ALREADY_EXISTS, got %q", code)
	}

	// 3. Publish the event.
	rec, _ = env.do(t, http.MethodPut, "/api/v1/events/"+eventID, env.leader, map[string]string{"status": "PUBLISHED"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for publish, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var status string
	if err := env.pool.QueryRow(context.Background(), "SELECT status FROM events WHERE id = $1", eventID).Scan(&status); err != nil {
		t.Fatalf("failed to read event status: %v", err)
	}
	if status != "PUBLISHED" {
		t.Fatalf("expected PUBLISHED in database, got %q", status)
	}

	// 4. Add a FotoTree session.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/events/"+eventID+"/sessions", env.admin, map[string]interface{}{
		"name": "KM 21 Halfway Point", "session_type": "TRACK_KM21",
		"starts_at": "2026-10-25T06:00:00Z", "ends_at": "2026-10-25T10:00:00Z",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for session create, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	sessionID, _ := successData(parsed)["id"].(string)

	// 5. Add a route location checkpoint.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/events/"+eventID+"/locations", env.admin, map[string]interface{}{
		"name": "Start Gate", "latitude": -6.2088, "longitude": 106.8456, "route_order": 1,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for location create, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// 6. Read the tree back.
	rec, parsed = env.do(t, http.MethodGet, "/api/v1/events/"+eventID, env.leader, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for tree read, got %d", rec.Code)
	}
	tree := successData(parsed)
	sessions, _ := tree["sessions"].([]interface{})
	locations, _ := tree["locations"].([]interface{})
	if len(sessions) != 1 || len(locations) != 1 {
		t.Fatalf("expected 1 session and 1 location, got %d and %d", len(sessions), len(locations))
	}

	// 7. Event creation is audited.
	var auditCount int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM audit_logs
		WHERE action_code = 'EVENT_CREATED' AND target_id = $1`, eventID).Scan(&auditCount); err != nil {
		t.Fatalf("failed to query audit log: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 event-created audit entry, got %d", auditCount)
	}

	_ = sessionID
}

func TestFotoTree_CrossTenantAndOutsiderDenied(t *testing.T) {
	env := newFotoTreeTestEnv(t)
	defer env.cleanup()

	rec, parsed := env.do(t, http.MethodPost, "/api/v1/events", env.leader, map[string]interface{}{
		"name": "Event Terlindungi", "slug": "protected-" + uuid.NewString()[:6], "event_type": "RUNNING_MARATHON",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup failed: %d", rec.Code)
	}
	eventID := successData(parsed)["id"].(string)

	// A non-member cannot read or modify the event tree.
	for _, tc := range []struct {
		method, path string
		body         interface{}
	}{
		{http.MethodGet, "/api/v1/events/" + eventID, nil},
		{http.MethodPut, "/api/v1/events/" + eventID, map[string]string{"status": "PUBLISHED"}},
		{http.MethodPost, "/api/v1/events/" + eventID + "/sessions", map[string]string{"name": "X", "session_type": "START"}},
	} {
		rec, _ := env.do(t, tc.method, tc.path, env.outsider, tc.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s: expected 403 for non-member, got %d", tc.method, tc.path, rec.Code)
		}
	}

	// An event belonging to another organization is invisible even with a valid org header.
	otherOrgID := uuid.Must(uuid.NewV7())
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Rahasia', 'Secret EO', 'Music', 'APPROVED')`, otherOrgID); err != nil {
		t.Fatalf("failed to seed other org: %v", err)
	}
	defer func() {
		_, _ = env.pool.Exec(context.Background(), "DELETE FROM host_organizations WHERE id = $1", otherOrgID)
	}()
	foreignEventID := uuid.Must(uuid.NewV7())
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO events (id, host_organization_id, slug, name, status, created_by)
		VALUES ($1, $2, 'foreign-event', 'Foreign Event', 'DRAFT', $3)`,
		foreignEventID, otherOrgID, uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("failed to seed foreign event: %v", err)
	}

	rec, _ = env.do(t, http.MethodGet, "/api/v1/events/"+foreignEventID.String(), env.leader, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for foreign-tenant event, got %d", rec.Code)
	}
}

func TestFotoTree_InvalidInputRejected(t *testing.T) {
	env := newFotoTreeTestEnv(t)
	defer env.cleanup()

	for name, body := range map[string]map[string]interface{}{
		"empty name": {"name": "", "slug": "valid-slug", "event_type": "RUNNING_MARATHON"},
		"empty slug": {"name": "Valid", "slug": "", "event_type": "RUNNING_MARATHON"},
		"bad slug":   {"name": "Valid", "slug": "Not A Slug!", "event_type": "RUNNING_MARATHON"},
		"ends before starts": {"name": "Valid", "slug": "ok-slug", "event_type": "RUNNING_MARATHON",
			"starts_at": "2026-10-25T13:00:00Z", "ends_at": "2026-10-25T05:00:00Z"},
	} {
		rec, parsed := env.do(t, http.MethodPost, "/api/v1/events", env.leader, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (body: %s)", name, rec.Code, rec.Body.String())
			continue
		}
		if code := errorCode(parsed); code != "INVALID_INPUT" {
			t.Errorf("%s: expected INVALID_INPUT, got %q", name, code)
		}
	}

	// A latitude outside the valid range must be refused.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/events", env.leader, map[string]interface{}{
		"name": "Geo Event", "slug": "geo-" + uuid.NewString()[:6], "event_type": "RUNNING_MARATHON",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup failed: %d", rec.Code)
	}
	eventID := successData(parsed)["id"].(string)

	rec, _ = env.do(t, http.MethodPost, "/api/v1/events/"+eventID+"/locations", env.leader, map[string]interface{}{
		"name": "Bad Point", "latitude": 120.0, "longitude": 106.8,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-range latitude, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "latitude") {
		t.Fatalf("expected the error to name the offending field, got %s", rec.Body.String())
	}
}
