package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

func TestTenantRepository_HostMembershipAndEvents(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	userRepo := repository.NewPostgresUserRepository(pool)
	tenantRepo := repository.NewPostgresTenantRepository(pool)

	// 1. Create test user
	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("tenant_user_%s@galeriku.com", uniqueID)
	user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: "Tenant Tester", Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "HOST_LEADER"); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// 2. Create test host organizations A and B
	orgAID, err := uuid.NewV7()
	if err != nil {
		orgAID = uuid.New()
	}
	orgBID, err := uuid.NewV7()
	if err != nil {
		orgBID = uuid.New()
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO host_organizations (id, legal_name, display_name, business_field, verification_status)
		VALUES ($1, 'PT Marathon A', 'Marathon Organizer A', 'Sports EO', 'VERIFIED'),
		       ($2, 'PT Marathon B', 'Marathon Organizer B', 'Music EO', 'VERIFIED')
	`, orgAID, orgBID)
	if err != nil {
		t.Fatalf("failed to insert test host organizations: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM host_memberships WHERE host_organization_id IN ($1, $2)", orgAID, orgBID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM events WHERE host_organization_id IN ($1, $2)", orgAID, orgBID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM host_organizations WHERE id IN ($1, $2)", orgAID, orgBID)
	}()

	// 3. Insert active membership for user in Org A as LEADER
	_, err = pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE')
	`, orgAID, user.ID)
	if err != nil {
		t.Fatalf("failed to insert active membership for Org A: %v", err)
	}

	// 4. Test GetActiveHostMembership for Org A (Expected: Success)
	hmA, hoA, err := tenantRepo.GetActiveHostMembership(ctx, user.ID, orgAID)
	if err != nil {
		t.Fatalf("expected successful active membership retrieval for Org A, got error: %v", err)
	}
	if hmA.MembershipRole != domain.HostRoleLeader {
		t.Errorf("expected role LEADER, got: %s", hmA.MembershipRole)
	}
	if hoA.DisplayName != "Marathon Organizer A" {
		t.Errorf("expected display name 'Marathon Organizer A', got: %s", hoA.DisplayName)
	}

	// 5. Test GetActiveHostMembership for Org B (User is not a member -> Expected: ErrMembershipNotFound)
	_, _, err = tenantRepo.GetActiveHostMembership(ctx, user.ID, orgBID)
	if err != repository.ErrMembershipNotFound {
		t.Errorf("expected ErrMembershipNotFound for Org B, got: %v", err)
	}

	// 6. Test Inactive/Revoked Membership
	user2Email := fmt.Sprintf("inactive_user_%s@galeriku.com", uniqueID)
	user2 := &domain.User{EmailNormalized: &user2Email, AccountStatus: "ACTIVE"}
	profile2 := &domain.UserProfile{DisplayName: "Inactive Member", Locale: "id-ID"}
	creds2 := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user2, profile2, creds2, "BUYER"); err != nil {
		t.Fatalf("failed to create second test user: %v", err)
	}

	tNow := time.Now()
	_, err = pool.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status, revoked_at)
		VALUES ($1, $2, 'ADMIN', 'REVOKED', $3)
	`, orgAID, user2.ID, tNow)
	if err != nil {
		t.Fatalf("failed to insert revoked membership: %v", err)
	}

	_, _, err = tenantRepo.GetActiveHostMembership(ctx, user2.ID, orgAID)
	if err != repository.ErrMembershipInactive {
		t.Errorf("expected ErrMembershipInactive for revoked member, got: %v", err)
	}

	// 7. Test GetEventHostOrganizationID
	eventAID, err := uuid.NewV7()
	if err != nil {
		eventAID = uuid.New()
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO events (id, host_organization_id, slug, name, status, created_by)
		VALUES ($1, $2, 'jakarta-marathon-2026', 'Jakarta Marathon 2026', 'PUBLISHED', $3)
	`, eventAID, orgAID, user.ID)
	if err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}

	resolvedOrgID, err := tenantRepo.GetEventHostOrganizationID(ctx, eventAID)
	if err != nil {
		t.Fatalf("expected successful event lookup, got: %v", err)
	}
	if resolvedOrgID != orgAID {
		t.Errorf("expected resolved org ID %s, got: %s", orgAID, resolvedOrgID)
	}

	// Non-existent event
	_, err = tenantRepo.GetEventHostOrganizationID(ctx, uuid.New())
	if err != repository.ErrEventNotFound {
		t.Errorf("expected ErrEventNotFound for non-existent event, got: %v", err)
	}

	// 8. Test GetUserActiveCapabilities
	caps, err := tenantRepo.GetUserActiveCapabilities(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get active capabilities: %v", err)
	}
	hasHostLeader := false
	for _, c := range caps {
		if c == "HOST_LEADER" {
			hasHostLeader = true
			break
		}
	}
	if !hasHostLeader {
		t.Errorf("expected user to possess HOST_LEADER capability, got: %v", caps)
	}
}
