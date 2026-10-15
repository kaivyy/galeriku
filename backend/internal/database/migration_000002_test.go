package database

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigration000002_Integration(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	// 1. Run pending migrations from migrations directory
	migrationsDir := filepath.Join("..", "..", "..", "migrations")
	applied, err := RunPendingMigrations(ctx, pool, migrationsDir)
	if err != nil {
		t.Fatalf("RunPendingMigrations failed: %v", err)
	}
	t.Logf("Applied %d pending migration(s)", applied)

	// 2. Verify UUIDv7 function runtime execution (both aliases)
	var uuid1, uuid2 string
	err = pool.QueryRow(ctx, "SELECT uuid_generate_v7()::text").Scan(&uuid1)
	if err != nil {
		t.Fatalf("uuid_generate_v7() execution failed: %v", err)
	}
	err = pool.QueryRow(ctx, "SELECT generate_uuid_v7()::text").Scan(&uuid2)
	if err != nil {
		t.Fatalf("generate_uuid_v7() execution failed: %v", err)
	}

	// Format: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx where y is 8, 9, a, or b
	uuid7Regex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuid7Regex.MatchString(uuid1) {
		t.Errorf("uuid_generate_v7() produced invalid UUIDv7 bit layout: %s", uuid1)
	}
	if !uuid7Regex.MatchString(uuid2) {
		t.Errorf("generate_uuid_v7() produced invalid UUIDv7 bit layout: %s", uuid2)
	}

	// 3. Verify total table count is now at least 80
	count, err := VerifySchemaCompleteness(ctx, pool)
	if err != nil {
		t.Fatalf("VerifySchemaCompleteness failed after migration 000002: %v", err)
	}
	if count < 80 {
		t.Fatalf("expected at least 80 tables after migration 000002, got %d", count)
	}
	t.Logf("Total verified base tables in PostgreSQL: %d", count)

	// 4. Verify user insertion with default UUIDv7
	testEmail := fmt.Sprintf("phase2_user_%d@galeriku.com", time.Now().UnixNano())
	var userID string
	insertUserQuery := `
		INSERT INTO users (email_normalized, account_status) 
		VALUES ($1, 'ACTIVE') 
		RETURNING id::text;
	`
	err = pool.QueryRow(ctx, insertUserQuery, testEmail).Scan(&userID)
	if err != nil {
		t.Fatalf("INSERT into users using default UUIDv7 failed: %v", err)
	}
	if !uuid7Regex.MatchString(userID) {
		t.Errorf("inserted user ID has invalid UUIDv7: %s", userID)
	}

	// 5. Verify user_credentials insertion & foreign key cascade
	fakeHash := "$argon2id$v=19$m=65536,t=3,p=4$fakeSaltString$fakeHashDigestString"
	insertCredQuery := `
		INSERT INTO user_credentials (user_id, password_hash, password_algo)
		VALUES ($1, $2, 'argon2id')
		RETURNING id::text;
	`
	var credID string
	err = pool.QueryRow(ctx, insertCredQuery, userID, fakeHash).Scan(&credID)
	if err != nil {
		t.Fatalf("INSERT into user_credentials failed: %v", err)
	}

	// Verify foreign key constraint fk_user_credentials_user
	invalidUserID := uuid.New().String()
	err = pool.QueryRow(ctx, insertCredQuery, invalidUserID, fakeHash).Scan(&credID)
	if err == nil {
		t.Fatal("expected foreign key violation when inserting user_credentials with non-existent user_id, got nil")
	}

	// 6. Verify auth_refresh_tokens table & family ID rotation
	fakeTokenHash := fmt.Sprintf("sha256_hash_%d", time.Now().UnixNano())
	familyID := uuid.New().String()
	insertTokenQuery := `
		INSERT INTO auth_refresh_tokens (user_id, token_hash, family_id, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text;
	`
	var tokenID string
	err = pool.QueryRow(ctx, insertTokenQuery, userID, fakeTokenHash, familyID, time.Now().Add(30*24*time.Hour)).Scan(&tokenID)
	if err != nil {
		t.Fatalf("INSERT into auth_refresh_tokens failed: %v", err)
	}

	// 7. Verify auth_otp_challenges check constraint (both phone and email cannot be null)
	invalidOTPQuery := `
		INSERT INTO auth_otp_challenges (purpose, code_hash, expires_at)
		VALUES ('REGISTRATION', 'dummy_hash', $1);
	`
	_, err = pool.Exec(ctx, invalidOTPQuery, time.Now().Add(3*time.Minute))
	if err == nil {
		t.Fatal("expected check constraint violation on auth_otp_challenges when phone and email are null, got nil")
	}

	// 8. Verify host_bank_accounts & single active Host Leader invariant (OPEN-10)
	testLeaderOrg(t, ctx, pool, userID)

	// Clean up user (cascades to user_credentials and auth_refresh_tokens)
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
}

func testLeaderOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool, leaderUserID string) {
	// Create test host organization
	var orgID string
	insertOrgQuery := `
		INSERT INTO host_organizations (legal_name, display_name, business_field, verification_status)
		VALUES ('PT Tes Marathon Indonesia', 'Tes Marathon EO', 'Sport Events', 'APPROVED')
		RETURNING id::text;
	`
	err := pool.QueryRow(ctx, insertOrgQuery).Scan(&orgID)
	if err != nil {
		t.Fatalf("INSERT into host_organizations failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM host_memberships WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(ctx, "DELETE FROM host_bank_accounts WHERE host_organization_id = $1", orgID)
		_, _ = pool.Exec(ctx, "DELETE FROM host_organizations WHERE id = $1", orgID)
	}()

	// Create test host bank account
	var bankAccID string
	insertBankQuery := `
		INSERT INTO host_bank_accounts (host_organization_id, bank_code, account_number_encrypted, account_number_masked, account_holder_name)
		VALUES ($1, 'BCA', 'enc_blob_12345', '1234xxxx5678', 'PT Tes Marathon Indonesia')
		RETURNING id::text;
	`
	err = pool.QueryRow(ctx, insertBankQuery, orgID).Scan(&bankAccID)
	if err != nil {
		t.Fatalf("INSERT into host_bank_accounts failed: %v", err)
	}

	// Add First Host Leader (role: 'LEADER', status: 'ACTIVE')
	var membership1ID string
	insertLeaderQuery := `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'ACTIVE')
		RETURNING id::text;
	`
	err = pool.QueryRow(ctx, insertLeaderQuery, orgID, leaderUserID).Scan(&membership1ID)
	if err != nil {
		t.Fatalf("INSERT into host_memberships for first Leader failed: %v", err)
	}

	// Positive Invariant Test: Inactive and Revoked leaders for the same organization MUST succeed
	inactiveUserID := uuid.New().String()
	_, err = pool.Exec(ctx, "INSERT INTO users (id, email_normalized, account_status) VALUES ($1, $2, 'ACTIVE')", inactiveUserID, fmt.Sprintf("user_inactive_%s@test.com", inactiveUserID[:8]))
	if err != nil {
		t.Fatalf("INSERT into users for inactive leader user failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", inactiveUserID)
	}()

	var membershipInactiveID string
	insertInactiveLeaderQuery := `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'INACTIVE')
		RETURNING id::text;
	`
	err = pool.QueryRow(ctx, insertInactiveLeaderQuery, orgID, inactiveUserID).Scan(&membershipInactiveID)
	if err != nil {
		t.Fatalf("expected inserting INACTIVE leader to succeed alongside active leader, failed: %v", err)
	}
	t.Logf("Positive Test PASS: INACTIVE leader successfully coexists with ACTIVE leader")

	var membershipRevokedID string
	insertRevokedLeaderQuery := `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status)
		VALUES ($1, $2, 'LEADER', 'REVOKED')
		RETURNING id::text;
	`
	err = pool.QueryRow(ctx, insertRevokedLeaderQuery, orgID, inactiveUserID).Scan(&membershipRevokedID)
	if err != nil {
		t.Fatalf("expected inserting REVOKED leader to succeed alongside active leader, failed: %v", err)
	}
	t.Logf("Positive Test PASS: REVOKED leader successfully coexists with ACTIVE leader")

	// Negative Test: Attempt to insert a Second Active Host Leader for the same org (violates uq_active_host_leader)
	anotherUserID := uuid.New().String()
	// Create another user
	_, err = pool.Exec(ctx, "INSERT INTO users (id, email_normalized, account_status) VALUES ($1, $2, 'ACTIVE')", anotherUserID, fmt.Sprintf("user2_%s@test.com", anotherUserID[:8]))
	if err != nil {
		t.Fatalf("INSERT into users for second user failed: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", anotherUserID)
	}()

	var membership2ID string
	err = pool.QueryRow(ctx, insertLeaderQuery, orgID, anotherUserID).Scan(&membership2ID)
	if err == nil {
		t.Fatal("expected unique constraint violation on uq_active_host_leader when inserting second active leader, got nil")
	}
	t.Logf("Negative Test PASS: uq_active_host_leader successfully rejected second active Leader (%v)", err)
}

func TestMigration000002_Rollback_And_Reapply(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	migrationsDir := filepath.Join("..", "..", "..", "migrations")

	// Ensure every migration is applied before testing rollback.
	if _, err := RunPendingMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("RunPendingMigrations failed: %v", err)
	}

	// Roll back migrations newer than 000002 first, so this test always exercises the
	// 000002 rollback regardless of how many later migrations exist.
	for {
		var latest string
		if err := pool.QueryRow(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&latest); err != nil {
			t.Fatalf("failed to read latest migration version: %v", err)
		}
		if latest == "000002" {
			break
		}
		if _, err := RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("failed to roll back migration %s: %v", latest, err)
		}
	}

	// Rollback migration 000002
	rolledBackVersion, err := RollbackLastMigration(ctx, pool, migrationsDir)
	if err != nil {
		t.Fatalf("RollbackLastMigration failed: %v", err)
	}
	if rolledBackVersion != "000002" {
		t.Fatalf("expected rolled back version to be 000002, got %s", rolledBackVersion)
	}
	t.Logf("Rollback PASS: successfully rolled back version %s", rolledBackVersion)

	// Verify user_credentials table is dropped
	var tableExists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'user_credentials'
		);
	`
	err = pool.QueryRow(ctx, checkQuery).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to query table existence: %v", err)
	}
	if tableExists {
		t.Fatal("expected user_credentials to be dropped after rollback, but it still exists")
	}
	t.Logf("Rollback Verification PASS: user_credentials table dropped cleanly")

	// Re-apply every migration from 000002 onward. Later migrations are re-applied too,
	// so the expected count is whichever versions are currently pending.
	reappliedCount, err := RunPendingMigrations(ctx, pool, migrationsDir)
	if err != nil {
		t.Fatalf("re-applying RunPendingMigrations failed: %v", err)
	}
	if reappliedCount < 1 {
		t.Fatalf("expected at least 1 migration re-applied (000002), got %d", reappliedCount)
	}
	t.Logf("Re-applied %d migration(s)", reappliedCount)

	// Verify table count is back to 81
	count, err := VerifySchemaCompleteness(ctx, pool)
	if err != nil {
		t.Fatalf("VerifySchemaCompleteness failed after re-applying 000002: %v", err)
	}
	// The table count depends on how many migrations exist beyond 000002; later
	// migrations add tables too. 81 is the floor after 000002, not the ceiling.
	if count < 81 {
		t.Fatalf("expected at least 81 tables after re-applying migration 000002, got %d", count)
	}
	t.Logf("Re-apply Verification PASS: successfully restored 81 tables and verified schema completeness")
}
