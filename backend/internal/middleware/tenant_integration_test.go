package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getLiveTestPool(t *testing.T) (*pgxpool.Pool, context.Context, context.CancelFunc) {
	if testing.Short() {
		t.Skip("skipping live database test in short mode")
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5433"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = "galeriku"
		}
		pass := os.Getenv("DB_PASSWORD")
		if pass == "" {
			pass = "galeriku_secret_pass"
		}
		dbname := os.Getenv("DB_NAME")
		if dbname == "" {
			dbname = "galeriku_db"
		}
		connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, dbname)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		cancel()
		t.Skipf("skipping: cannot connect to postgres at %s: %v", connStr, err)
		return nil, nil, nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cancel()
		t.Skipf("skipping: ping failed to postgres at %s: %v", connStr, err)
		return nil, nil, nil
	}

	return pool, ctx, cancel
}

func TestLiveTenantIsolationMiddleware(t *testing.T) {
	pool, ctx, cancel := getLiveTestPool(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	defer cancel()

	userRepo := repository.NewPostgresUserRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)
	jwtSecret := []byte("test_super_secret_jwt_key_32_bytes_long_12345")

	// 1. Create two test users: Leader and Admin
	uniqueID := uuid.New().String()[:8]
	leaderEmail := fmt.Sprintf("live_leader_%s@galeriku.com", uniqueID)
	leaderUser := &domain.User{EmailNormalized: &leaderEmail, AccountStatus: "ACTIVE"}
	leaderProfile := &domain.UserProfile{DisplayName: "Live Leader", Locale: "id-ID"}
	leaderCreds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, leaderUser, leaderProfile, leaderCreds, "HOST_LEADER"); err != nil {
		t.Fatalf("failed to create leader user: %v", err)
	}

	adminEmail := fmt.Sprintf("live_admin_%s@galeriku.com", uniqueID)
	adminUser := &domain.User{EmailNormalized: &adminEmail, AccountStatus: "ACTIVE"}
	adminProfile := &domain.UserProfile{DisplayName: "Live Admin", Locale: "id-ID"}
	adminCreds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, adminUser, adminProfile, adminCreds, "BUYER"); err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}

	// 2. Create organizations A and B
	orgAID, _ := uuid.NewV7()
	orgBID, _ := uuid.NewV7()

	_, err := pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Live Org A', 'Live Org A', 'Sports', 'VERIFIED'),
		       ($2, 'PT Live Org B', 'Live Org B', 'Music', 'VERIFIED')
	`, orgAID, orgBID)
	if err != nil {
		t.Fatalf("failed to insert live test organizations: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM host_memberships WHERE host_organization_id IN ($1, $2)", orgAID, orgBID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM events WHERE host_organization_id IN ($1, $2)", orgAID, orgBID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM host_organizations WHERE id IN ($1, $2)", orgAID, orgBID)
	}()

	// 3. Memberships: Leader in Org A, Admin in Org A
	_, err = pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE'),
		       ($1, $3, 'ADMIN', 'ACTIVE')
	`, orgAID, leaderUser.ID, adminUser.ID)
	if err != nil {
		t.Fatalf("failed to insert live test memberships: %v", err)
	}

	// 4. Create an event in Org A
	eventAID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO events (id, host_organization_id, slug, name, status, created_by)
		VALUES ($1, $2, 'live-event-a', 'Live Event A', 'PUBLISHED', $3)
	`, eventAID, orgAID, leaderUser.ID)
	if err != nil {
		t.Fatalf("failed to insert live test event: %v", err)
	}

	// 5. Create an event in Org B
	eventBID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO events (id, host_organization_id, slug, name, status, created_by)
		VALUES ($1, $2, 'live-event-b', 'Live Event B', 'PUBLISHED', $3)
	`, eventBID, orgBID, leaderUser.ID)
	if err != nil {
		t.Fatalf("failed to insert live test event B: %v", err)
	}

	// Tokens
	leaderToken, _ := security.GenerateAccessToken(jwtSecret, leaderUser.ID.String(), leaderEmail, []string{"BUYER", "HOST_LEADER"}, 15*time.Minute)
	adminToken, _ := security.GenerateAccessToken(jwtSecret, adminUser.ID.String(), adminEmail, []string{"BUYER"}, 15*time.Minute)

	// Setup protected pipeline
	authMw := middleware.Authenticate(jwtSecret)
	membershipMw := middleware.RequireHostMembership(tenantRepo)
	leaderOnlyMw := middleware.RequireHostLeader(tenantRepo)

	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, _ := middleware.GetTenantContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tc)
	})

	protectedTenantHandler := authMw(membershipMw(endpoint))
	protectedLeaderHandler := authMw(leaderOnlyMw(endpoint))

	// Case 1: Active Leader accessing Org A -> 200 OK
	req1 := httptest.NewRequest(http.MethodGet, "/hosts/dashboard", nil)
	req1.Header.Set("Authorization", "Bearer "+leaderToken)
	req1.Header.Set("X-Organization-ID", orgAID.String())
	rec1 := httptest.NewRecorder()
	protectedTenantHandler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Leader on Org A, got %d (body: %s)", rec1.Code, rec1.Body.String())
	}
	var tc1 middleware.TenantContext
	_ = json.Unmarshal(rec1.Body.Bytes(), &tc1)
	if tc1.OrganizationID != orgAID || !tc1.IsLeader {
		t.Errorf("unexpected tenant context: %+v", tc1)
	}

	// Case 2: Active Admin accessing Org A on general membership route -> 200 OK
	req2 := httptest.NewRequest(http.MethodGet, "/hosts/events", nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	req2.Header.Set("X-Organization-ID", orgAID.String())
	rec2 := httptest.NewRecorder()
	protectedTenantHandler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Admin on Org A, got %d", rec2.Code)
	}

	// Case 3: Active Admin accessing Org A on Leader-only route -> 403 Forbidden
	req3 := httptest.NewRequest(http.MethodPost, "/hosts/payouts", nil)
	req3.Header.Set("Authorization", "Bearer "+adminToken)
	req3.Header.Set("X-Organization-ID", orgAID.String())
	rec3 := httptest.NewRecorder()
	protectedLeaderHandler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Admin on Leader route, got %d", rec3.Code)
	}

	// Case 4: Leader of Org A accessing Org B -> 403 Forbidden
	req4 := httptest.NewRequest(http.MethodGet, "/hosts/dashboard", nil)
	req4.Header.Set("Authorization", "Bearer "+leaderToken)
	req4.Header.Set("X-Organization-ID", orgBID.String())
	rec4 := httptest.NewRecorder()
	protectedTenantHandler.ServeHTTP(rec4, req4)

	if rec4.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-tenant access, got %d", rec4.Code)
	}

	// Case 5: Event-scoped access (Event A belongs to Org A; Admin is member of Org A) -> 200 OK
	req5 := httptest.NewRequest(http.MethodGet, "/events/summary?event_id="+eventAID.String(), nil)
	req5.Header.Set("Authorization", "Bearer "+adminToken)
	rec5 := httptest.NewRecorder()
	protectedTenantHandler.ServeHTTP(rec5, req5)

	if rec5.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for event-scoped access, got %d (body: %s)", rec5.Code, rec5.Body.String())
	}

	// Case 6: Cross-tenant event access (Event B belongs to Org B; Admin is NOT member of Org B) -> 403 Forbidden
	req6 := httptest.NewRequest(http.MethodGet, "/events/summary?event_id="+eventBID.String(), nil)
	req6.Header.Set("Authorization", "Bearer "+adminToken)
	rec6 := httptest.NewRecorder()
	protectedTenantHandler.ServeHTTP(rec6, req6)

	if rec6.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-tenant event access, got %d", rec6.Code)
	}

	// Case 7: Cross-tenant resource mismatch tampering (header specifies Org A, but event belongs to Org B) -> 403 Forbidden
	req7 := httptest.NewRequest(http.MethodGet, "/events/summary?event_id="+eventBID.String(), nil)
	req7.Header.Set("Authorization", "Bearer "+leaderToken)
	req7.Header.Set("X-Organization-ID", orgAID.String())
	rec7 := httptest.NewRecorder()
	protectedTenantHandler.ServeHTTP(rec7, req7)

	if rec7.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-tenant mismatch tampering, got %d", rec7.Code)
	}
}
