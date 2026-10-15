package database

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// TestMigration000003_ProviderUploadIDIsText verifies that migration 000003 widened
// multipart_upload_sessions.provider_upload_id to TEXT, so a real object storage
// provider identifier (opaque, longer than 36 characters) can be persisted.
func TestMigration000003_ProviderUploadIDIsText(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	migrationsDir := filepath.Join("..", "..", "..", "migrations")
	if _, err := RunPendingMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("RunPendingMigrations failed: %v", err)
	}

	var dataType string
	err := pool.QueryRow(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'multipart_upload_sessions'
		  AND column_name = 'provider_upload_id'
	`).Scan(&dataType)
	if err != nil {
		t.Fatalf("failed to read column type: %v", err)
	}
	if dataType != "text" {
		t.Fatalf("expected provider_upload_id to be text after migration 000003, got %q", dataType)
	}

	// A genuine provider identifier must fit; this is the value shape MinIO returns.
	providerID := "NWMzNzBhZTUtNmVjNS00ZWNiLWI3YmMtMWEzNzFhOWEzMTFjLjk2MmU4YmQ1LTUxMmEtNDEyZC1hY2UwLTY3ODJjZDNlNjgxMngxNzkyMDg0ODMwODI0NTEzMDc4"

	// The probe builds its own fixtures so it never depends on data left by another test.
	accountID := uuid.Must(uuid.NewV7())
	policyID := uuid.Must(uuid.NewV7())
	quotaID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO storage_accounts (id, owner_type, owner_id, status)
		VALUES ($1, 'HOST_ORGANIZATION', NULL, 'ACTIVE')`, accountID); err != nil {
		t.Fatalf("failed to seed storage account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quota_policy_versions (id, policy_code, version_number, billable_categories, limit_bytes, effective_from)
		VALUES ($1, 'MIGRATION_000003_PROBE', 1, '["RAW"]'::jsonb, 1024, CURRENT_TIMESTAMP)`, policyID); err != nil {
		t.Fatalf("failed to seed quota policy: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO storage_quotas (id, storage_account_id, policy_version_id, quota_limit_bytes, billable_bytes, reserved_bytes)
		VALUES ($1, $2, $3, 1024, 0, 0)`, quotaID, accountID, policyID); err != nil {
		t.Fatalf("failed to seed storage quota: %v", err)
	}

	batchID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO upload_batches (id, storage_account_id, owner_user_id, status, expected_file_count, expected_bytes, actual_bytes)
		VALUES ($1, $2, $3, 'RESERVED', 1, 1024, 0)`, batchID, accountID, uuid.Nil); err != nil {
		t.Fatalf("failed to seed upload batch: %v", err)
	}

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM multipart_upload_sessions WHERE upload_item_id IN (SELECT id FROM upload_items WHERE upload_batch_id = $1)", batchID)
		_, _ = pool.Exec(ctx, "DELETE FROM upload_items WHERE upload_batch_id = $1", batchID)
		_, _ = pool.Exec(ctx, "DELETE FROM upload_batches WHERE id = $1", batchID)
		_, _ = pool.Exec(ctx, "DELETE FROM storage_quotas WHERE storage_account_id = $1", accountID)
		_, _ = pool.Exec(ctx, "DELETE FROM quota_policy_versions WHERE id = $1", policyID)
		_, _ = pool.Exec(ctx, "DELETE FROM storage_accounts WHERE id = $1", accountID)
	}()

	var itemRowID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO upload_items (upload_batch_id, client_file_token, expected_bytes, status)
		VALUES ($1, 'migration_000003_probe', 1024, 'PENDING')
		RETURNING id::text
	`, batchID).Scan(&itemRowID); err != nil {
		t.Fatalf("failed to create upload item probe: %v", err)
	}

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO multipart_upload_sessions (upload_item_id, provider_upload_id, status, part_count, expires_at)
		VALUES ($1::uuid, $2, 'ACTIVE', 2, CURRENT_TIMESTAMP + INTERVAL '1 hour')
		RETURNING id::text
	`, itemRowID, providerID).Scan(&sessionID); err != nil {
		t.Fatalf("a real provider upload id must fit after migration 000003: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx,
		"SELECT provider_upload_id FROM multipart_upload_sessions WHERE id = $1::uuid", sessionID).Scan(&stored); err != nil {
		t.Fatalf("failed to read back provider id: %v", err)
	}
	if stored != providerID {
		t.Fatalf("provider_upload_id round trip mismatch: stored %q", stored)
	}
}
