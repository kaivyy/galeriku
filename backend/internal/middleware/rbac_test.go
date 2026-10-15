package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
)

// --- Mock Implementations for Deterministic Middleware Testing ---

type mockCapabilityChecker struct {
	caps map[uuid.UUID][]string
}

func (m *mockCapabilityChecker) GetUserActiveCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if caps, ok := m.caps[userID]; ok {
		return caps, nil
	}
	return []string{}, nil
}

type mockTenantRepo struct {
	memberships map[string]*domain.HostMembership
	orgs        map[uuid.UUID]*domain.HostOrganization
	events      map[uuid.UUID]uuid.UUID // eventID -> orgID
}

func newMockTenantRepo() *mockTenantRepo {
	return &mockTenantRepo{
		memberships: make(map[string]*domain.HostMembership),
		orgs:        make(map[uuid.UUID]*domain.HostOrganization),
		events:      make(map[uuid.UUID]uuid.UUID),
	}
}

func (m *mockTenantRepo) makeKey(userID, orgID uuid.UUID) string {
	return userID.String() + ":" + orgID.String()
}

func (m *mockTenantRepo) GetActiveHostMembership(ctx context.Context, userID, orgID uuid.UUID) (*domain.HostMembership, *domain.HostOrganization, error) {
	key := m.makeKey(userID, orgID)
	hm, ok := m.memberships[key]
	if !ok {
		return nil, nil, repository.ErrMembershipNotFound
	}

	ho, ok := m.orgs[orgID]
	if !ok {
		return nil, nil, repository.ErrOrganizationNotFound
	}

	if hm.Status != domain.HostMembershipStatusActive {
		return hm, ho, repository.ErrMembershipInactive
	}

	return hm, ho, nil
}

func (m *mockTenantRepo) GetEventHostOrganizationID(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error) {
	orgID, ok := m.events[eventID]
	if !ok {
		return uuid.Nil, repository.ErrEventNotFound
	}
	return orgID, nil
}

func (m *mockTenantRepo) GetUserActiveCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return []string{}, nil
}

// --- Test Suites ---

func TestRequireCapability(t *testing.T) {
	jwtSecret := []byte("test_super_secret_jwt_key_32_bytes_long_12345")
	authMw := middleware.Authenticate(jwtSecret)

	handlerCalled := false
	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	t.Run("MissingAuthentication", func(t *testing.T) {
		handlerCalled = false
		capMw := middleware.RequireCapability("CREATOR")
		handler := capMw(protectedHandler)

		req := httptest.NewRequest(http.MethodGet, "/creators/dashboard", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
		if handlerCalled {
			t.Error("protected handler was executed unexpectedly")
		}
	})

	t.Run("AuthorizedCapability_TokenClaims", func(t *testing.T) {
		handlerCalled = false
		capMw := middleware.RequireCapability("CREATOR")
		handler := authMw(capMw(protectedHandler))

		userID := uuid.New().String()
		tok, _ := security.GenerateAccessToken(jwtSecret, userID, "creator@galeriku.com", []string{"BUYER", "CREATOR"}, 15*time.Minute)

		req := httptest.NewRequest(http.MethodGet, "/creators/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		if !handlerCalled {
			t.Error("expected protected handler to be executed")
		}
	})

	t.Run("DeniedCapability_BuyerAccessingCreator", func(t *testing.T) {
		handlerCalled = false
		capMw := middleware.RequireCapability("CREATOR")
		handler := authMw(capMw(protectedHandler))

		userID := uuid.New().String()
		tok, _ := security.GenerateAccessToken(jwtSecret, userID, "buyer@galeriku.com", []string{"BUYER"}, 15*time.Minute)

		req := httptest.NewRequest(http.MethodGet, "/creators/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
		if handlerCalled {
			t.Error("protected handler was executed unexpectedly")
		}
	})

	t.Run("LiveRevocationCheck_ConditionF04", func(t *testing.T) {
		handlerCalled = false
		userUUID := uuid.New()
		checker := &mockCapabilityChecker{
			caps: map[uuid.UUID][]string{
				userUUID: {"BUYER"}, // Database has only BUYER (HOST_LEADER was revoked)
			},
		}

		capMw := middleware.RequireCapability("HOST_LEADER", middleware.WithLiveCapabilityCheck(checker))
		handler := authMw(capMw(protectedHandler))

		// Client presents unexpired 15-minute token that still claims HOST_LEADER
		staleToken, _ := security.GenerateAccessToken(jwtSecret, userUUID.String(), "revoked@galeriku.com", []string{"BUYER", "HOST_LEADER"}, 15*time.Minute)

		req := httptest.NewRequest(http.MethodGet, "/hosts/finance", nil)
		req.Header.Set("Authorization", "Bearer "+staleToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Authoritative live check detects revocation and blocks request with 403
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden on revoked capability, got %d", rec.Code)
		}
		if handlerCalled {
			t.Error("protected handler was executed unexpectedly despite revocation in database")
		}
	})
}

func TestRequireHostMembership(t *testing.T) {
	jwtSecret := []byte("test_super_secret_jwt_key_32_bytes_long_12345")
	authMw := middleware.Authenticate(jwtSecret)

	mockRepo := newMockTenantRepo()

	orgAID := uuid.New()
	orgBID := uuid.New()

	mockRepo.orgs[orgAID] = &domain.HostOrganization{
		ID:          orgAID,
		LegalName:   "PT Organizer A",
		DisplayName: "Event Organizer A",
	}
	mockRepo.orgs[orgBID] = &domain.HostOrganization{
		ID:          orgBID,
		LegalName:   "PT Organizer B",
		DisplayName: "Event Organizer B",
	}

	leaderUserUUID := uuid.New()
	adminUserUUID := uuid.New()
	nonMemberUserUUID := uuid.New()
	revokedUserUUID := uuid.New()

	// Org A Leader
	mockRepo.memberships[mockRepo.makeKey(leaderUserUUID, orgAID)] = &domain.HostMembership{
		ID:                 uuid.New(),
		HostOrganizationID: orgAID,
		UserID:             leaderUserUUID,
		MembershipRole:     domain.HostRoleLeader,
		Status:             domain.HostMembershipStatusActive,
	}

	// Org A Admin
	mockRepo.memberships[mockRepo.makeKey(adminUserUUID, orgAID)] = &domain.HostMembership{
		ID:                 uuid.New(),
		HostOrganizationID: orgAID,
		UserID:             adminUserUUID,
		MembershipRole:     domain.HostRoleAdmin,
		Status:             domain.HostMembershipStatusActive,
	}

	// Org A Revoked Member
	mockRepo.memberships[mockRepo.makeKey(revokedUserUUID, orgAID)] = &domain.HostMembership{
		ID:                 uuid.New(),
		HostOrganizationID: orgAID,
		UserID:             revokedUserUUID,
		MembershipRole:     domain.HostRoleAdmin,
		Status:             domain.HostMembershipStatusRevoked,
	}

	// Event owned by Org A
	eventAID := uuid.New()
	mockRepo.events[eventAID] = orgAID

	// Event owned by Org B
	eventBID := uuid.New()
	mockRepo.events[eventBID] = orgBID

	leaderToken, _ := security.GenerateAccessToken(jwtSecret, leaderUserUUID.String(), "leader@a.com", []string{"BUYER", "HOST_LEADER"}, 15*time.Minute)
	adminToken, _ := security.GenerateAccessToken(jwtSecret, adminUserUUID.String(), "admin@a.com", []string{"BUYER"}, 15*time.Minute)
	nonMemberToken, _ := security.GenerateAccessToken(jwtSecret, nonMemberUserUUID.String(), "stranger@b.com", []string{"BUYER"}, 15*time.Minute)
	revokedToken, _ := security.GenerateAccessToken(jwtSecret, revokedUserUUID.String(), "revoked@a.com", []string{"BUYER"}, 15*time.Minute)

	var capturedTenantCtx *middleware.TenantContext
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.GetTenantContext(r.Context())
		if ok {
			capturedTenantCtx = tc
		}
		w.WriteHeader(http.StatusOK)
	})

	t.Run("ActiveMember_AccessSuccess_ContextInjected", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		req := httptest.NewRequest(http.MethodGet, "/hosts/events", nil)
		req.Header.Set("Authorization", "Bearer "+leaderToken)
		req.Header.Set("X-Organization-ID", orgAID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if capturedTenantCtx == nil {
			t.Fatal("expected TenantContext to be injected in context")
		}
		if capturedTenantCtx.OrganizationID != orgAID {
			t.Errorf("expected OrgID %s, got %s", orgAID, capturedTenantCtx.OrganizationID)
		}
		if !capturedTenantCtx.IsLeader || capturedTenantCtx.Role != domain.HostRoleLeader {
			t.Errorf("expected Role LEADER, got %s", capturedTenantCtx.Role)
		}
	})

	t.Run("NonMember_AccessDenied", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		req := httptest.NewRequest(http.MethodGet, "/hosts/events", nil)
		req.Header.Set("Authorization", "Bearer "+nonMemberToken)
		req.Header.Set("X-Organization-ID", orgAID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for non-member, got %d", rec.Code)
		}
		if capturedTenantCtx != nil {
			t.Error("TenantContext should NOT be injected on authorization failure")
		}
	})

	t.Run("RevokedMember_AccessDenied", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		req := httptest.NewRequest(http.MethodGet, "/hosts/events", nil)
		req.Header.Set("Authorization", "Bearer "+revokedToken)
		req.Header.Set("X-Organization-ID", orgAID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for revoked member, got %d", rec.Code)
		}
	})

	t.Run("CrossTenantIsolation_MemberOfACannotAccessB", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		// Leader of Org A attempts to access Org B
		req := httptest.NewRequest(http.MethodGet, "/hosts/events", nil)
		req.Header.Set("Authorization", "Bearer "+leaderToken)
		req.Header.Set("X-Organization-ID", orgBID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for cross-tenant access, got %d", rec.Code)
		}
	})

	t.Run("HostAdmin_ForbiddenOnLeaderExclusiveRoute", func(t *testing.T) {
		capturedTenantCtx = nil
		// Financial path requires HostRoleLeader exclusively
		leaderOnlyMw := middleware.RequireHostLeader(mockRepo)
		handler := authMw(leaderOnlyMw(testHandler))

		// Host Admin requests payout endpoint
		req := httptest.NewRequest(http.MethodPost, "/hosts/payouts/request", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("X-Organization-ID", orgAID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for Host Admin on Leader route, got %d", rec.Code)
		}
		if capturedTenantCtx != nil {
			t.Error("TenantContext should not be set on forbidden access")
		}
	})

	t.Run("HostLeader_AllowedOnLeaderExclusiveRoute", func(t *testing.T) {
		capturedTenantCtx = nil
		leaderOnlyMw := middleware.RequireHostLeader(mockRepo)
		handler := authMw(leaderOnlyMw(testHandler))

		req := httptest.NewRequest(http.MethodPost, "/hosts/payouts/request", nil)
		req.Header.Set("Authorization", "Bearer "+leaderToken)
		req.Header.Set("X-Organization-ID", orgAID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for Host Leader on Leader route, got %d", rec.Code)
		}
		if capturedTenantCtx == nil || !capturedTenantCtx.IsLeader {
			t.Error("expected valid Leader TenantContext")
		}
	})

	t.Run("EventScoped_TenantResolution_Success", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		// Request specifies event_id in query; middleware resolves Org A and permits Admin of Org A
		req := httptest.NewRequest(http.MethodGet, "/events/analytics?event_id="+eventAID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for event-scoped tenant access, got %d", rec.Code)
		}
		if capturedTenantCtx == nil || capturedTenantCtx.OrganizationID != orgAID {
			t.Errorf("expected resolved OrgID %s, got %v", orgAID, capturedTenantCtx)
		}
	})

	t.Run("CrossTenantEventAccess_Blocked", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		// User from Org A tries to access event owned by Org B
		req := httptest.NewRequest(http.MethodGet, "/events/analytics?event_id="+eventBID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden when accessing event belonging to another org, got %d", rec.Code)
		}
	})

	t.Run("CrossTenantResourceMismatch_TamperingDetected", func(t *testing.T) {
		capturedTenantCtx = nil
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		// Attacker supplies Org A in header, but event belonging to Org B
		req := httptest.NewRequest(http.MethodGet, "/events/analytics?event_id="+eventBID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+leaderToken)
		req.Header.Set("X-Organization-ID", orgAID.String())
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for cross-tenant resource mismatch, got %d", rec.Code)
		}
	})

	t.Run("MissingOrganizationIdentifier_Returns400", func(t *testing.T) {
		tenantMw := middleware.RequireHostMembership(mockRepo)
		handler := authMw(tenantMw(testHandler))

		req := httptest.NewRequest(http.MethodGet, "/hosts/events", nil)
		req.Header.Set("Authorization", "Bearer "+leaderToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for missing organization identifier, got %d", rec.Code)
		}
	})
}

func TestConcurrentContextIsolation(t *testing.T) {
	jwtSecret := []byte("test_super_secret_jwt_key_32_bytes_long_12345")
	authMw := middleware.Authenticate(jwtSecret)

	mockRepo := newMockTenantRepo()

	const workerCount = 20
	var wg sync.WaitGroup
	wg.Add(workerCount)

	type testWorker struct {
		userUUID uuid.UUID
		orgUUID  uuid.UUID
		token    string
	}

	workers := make([]testWorker, workerCount)
	for i := 0; i < workerCount; i++ {
		uID := uuid.New()
		oID := uuid.New()
		role := domain.HostRoleAdmin
		if i%2 == 0 {
			role = domain.HostRoleLeader
		}

		mockRepo.orgs[oID] = &domain.HostOrganization{
			ID:          oID,
			DisplayName: "Org " + uID.String()[:4],
		}
		mockRepo.memberships[mockRepo.makeKey(uID, oID)] = &domain.HostMembership{
			ID:                 uuid.New(),
			HostOrganizationID: oID,
			UserID:             uID,
			MembershipRole:     role,
			Status:             domain.HostMembershipStatusActive,
		}
		tok, _ := security.GenerateAccessToken(jwtSecret, uID.String(), "u@test.com", []string{"BUYER"}, 15*time.Minute)

		workers[i] = testWorker{
			userUUID: uID,
			orgUUID:  oID,
			token:    tok,
		}
	}

	tenantMw := middleware.RequireHostMembership(mockRepo)

	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := middleware.GetTenantContext(r.Context())
		if !ok || tc == nil {
			http.Error(w, "missing tenant context", http.StatusInternalServerError)
			return
		}
		id, ok := middleware.GetAuthIdentity(r.Context())
		if !ok || id == nil {
			http.Error(w, "missing auth identity", http.StatusInternalServerError)
			return
		}

		// Simulate slight variable latency
		time.Sleep(10 * time.Millisecond)

		// Context invariant check: UserID in TenantContext must match AuthIdentity
		if tc.UserID != id.UserID {
			http.Error(w, "tenant context mismatch with auth identity", http.StatusConflict)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"user_id": tc.UserID.String(),
			"org_id":  tc.OrganizationID.String(),
		})
	})

	handler := authMw(tenantMw(endpoint))

	errChan := make(chan error, workerCount)

	for i := 0; i < workerCount; i++ {
		go func(w testWorker) {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodGet, "/hosts/resource", nil)
			req.Header.Set("Authorization", "Bearer "+w.token)
			req.Header.Set("X-Organization-ID", w.orgUUID.String())
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				errChan <- errors.New("worker failed with non-200 code")
				return
			}

			var res map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				errChan <- err
				return
			}

			if res["user_id"] != w.userUUID.String() || res["org_id"] != w.orgUUID.String() {
				errChan <- errors.New("tenant context leaked across concurrent requests")
				return
			}
		}(workers[i])
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Fatalf("concurrency failure: %v", err)
	}
}
