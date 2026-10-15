package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrStorageAccountNotFound is returned when the organization has no storage account yet.
	ErrStorageAccountNotFound = errors.New("storage account not found for this organization")

	// ErrQuotaExceeded is returned when a reservation would push usage past the quota limit.
	ErrQuotaExceeded = errors.New("insufficient storage quota for this upload batch")

	// ErrUploadBatchNotFound is returned when no batch matches the requested identifier.
	ErrUploadBatchNotFound = errors.New("upload batch not found")

	// ErrBatchNotFinalizable is returned when a batch is not in a state that can be finalized.
	ErrBatchNotFinalizable = errors.New("upload batch is not in a finalizable state")
)

// UploadRepository defines data access for storage quotas and mass upload batches.
type UploadRepository interface {
	GetQuotaByOrganization(ctx context.Context, orgID uuid.UUID) (*domain.StorageQuota, error)
	ReserveBatch(ctx context.Context, orgID, ownerUserID uuid.UUID, batch *domain.UploadBatch, items []domain.UploadItem) error
	GetBatchByID(ctx context.Context, batchID uuid.UUID) (*domain.UploadBatch, error)
	FinalizeBatch(ctx context.Context, batchID uuid.UUID, items []domain.UploadItem) (*domain.BatchFinalization, error)
	ListBatchItems(ctx context.Context, batchID uuid.UUID) ([]domain.UploadItem, error)
}

// PostgresUploadRepository implements UploadRepository backed by PostgreSQL 16.
type PostgresUploadRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUploadRepository constructs a new PostgresUploadRepository.
func NewPostgresUploadRepository(pool *pgxpool.Pool) *PostgresUploadRepository {
	return &PostgresUploadRepository{pool: pool}
}

// storageAccountIDForOrganization resolves the organization's storage account identifier.
func (r *PostgresUploadRepository) storageAccountIDForOrganization(ctx context.Context, q queryRower, orgID uuid.UUID) (uuid.UUID, error) {
	var accountID uuid.UUID
	err := q.QueryRow(ctx, `
		SELECT id FROM storage_accounts
		WHERE owner_type = $1 AND owner_id = $2 AND status = 'ACTIVE'
		LIMIT 1
	`, domain.StorageOwnerHostOrganization, orgID).Scan(&accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrStorageAccountNotFound
		}
		return uuid.Nil, fmt.Errorf("failed to resolve storage account: %w", err)
	}
	return accountID, nil
}

// queryRower is satisfied by both the pool and an open transaction.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

// GetQuotaByOrganization loads the quota summary for one organization's storage account.
func (r *PostgresUploadRepository) GetQuotaByOrganization(ctx context.Context, orgID uuid.UUID) (*domain.StorageQuota, error) {
	accountID, err := r.storageAccountIDForOrganization(ctx, r.pool, orgID)
	if err != nil {
		return nil, err
	}

	var q domain.StorageQuota
	err = r.pool.QueryRow(ctx, `
		SELECT id, storage_account_id, quota_limit_bytes, billable_bytes, reserved_bytes, updated_at
		FROM storage_quotas WHERE storage_account_id = $1
	`, accountID).Scan(&q.ID, &q.StorageAccountID, &q.QuotaLimitBytes, &q.BillableBytes, &q.ReservedBytes, &q.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrStorageAccountNotFound
		}
		return nil, fmt.Errorf("failed to query storage quota: %w", err)
	}
	return &q, nil
}

// ReserveBatch atomically reserves quota and records the batch with its items.
// The quota row is locked with SELECT ... FOR UPDATE before the balance is checked, so
// concurrent initiations serialize and can never oversubscribe the account.
func (r *PostgresUploadRepository) ReserveBatch(ctx context.Context, orgID, ownerUserID uuid.UUID, batch *domain.UploadBatch, items []domain.UploadItem) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin reservation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	accountID, err := r.storageAccountIDForOrganization(ctx, tx, orgID)
	if err != nil {
		return err
	}

	var quotaID uuid.UUID
	var limitBytes, billableBytes, reservedBytes int64
	err = tx.QueryRow(ctx, `
		SELECT id, quota_limit_bytes, billable_bytes, reserved_bytes
		FROM storage_quotas
		WHERE storage_account_id = $1
		FOR UPDATE
	`, accountID).Scan(&quotaID, &limitBytes, &billableBytes, &reservedBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStorageAccountNotFound
		}
		return fmt.Errorf("failed to lock storage quota: %w", err)
	}

	// The balance check runs under the row lock, so it cannot race another reservation.
	if billableBytes+reservedBytes+batch.ExpectedBytes > limitBytes {
		return ErrQuotaExceeded
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO upload_batches (storage_account_id, owner_user_id, status, expected_file_count, expected_bytes, actual_bytes, expires_at)
		VALUES ($1, $2, $3, $4, $5, 0, $6)
		RETURNING id, created_at
	`, accountID, ownerUserID, batch.Status, batch.ExpectedFileCount, batch.ExpectedBytes,
		time.Now().UTC().Add(domain.ReservationTTL)).Scan(&batch.ID, &batch.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert upload batch: %w", err)
	}
	batch.StorageAccountID = accountID
	batch.OwnerUserID = ownerUserID

	for i := range items {
		items[i].UploadBatchID = batch.ID
		if err := tx.QueryRow(ctx, `
			INSERT INTO upload_items (upload_batch_id, client_file_token, expected_bytes, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, batch.ID, items[i].ClientFileToken, items[i].ExpectedBytes, items[i].Status).Scan(&items[i].ID); err != nil {
			return fmt.Errorf("failed to insert upload item: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE storage_quotas
		SET reserved_bytes = reserved_bytes + $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, quotaID, batch.ExpectedBytes); err != nil {
		return fmt.Errorf("failed to update reserved quota: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO quota_reservations (storage_quota_id, upload_batch_id, reserved_bytes, committed_bytes, status, expires_at)
		VALUES ($1, $2, $3, 0, $4, $5)
	`, quotaID, batch.ID, batch.ExpectedBytes, domain.ReservationStatusActive,
		time.Now().UTC().Add(domain.ReservationTTL)); err != nil {
		return fmt.Errorf("failed to insert quota reservation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit reservation: %w", err)
	}
	return nil
}

// ListBatchItems returns every file slot of one batch.
func (r *PostgresUploadRepository) ListBatchItems(ctx context.Context, batchID uuid.UUID) ([]domain.UploadItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, upload_batch_id, client_file_token, expected_bytes, object_key, status
		FROM upload_items WHERE upload_batch_id = $1
		ORDER BY created_at ASC
	`, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to list upload items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.UploadItem, 0)
	for rows.Next() {
		var it domain.UploadItem
		if err := rows.Scan(&it.ID, &it.UploadBatchID, &it.ClientFileToken, &it.ExpectedBytes, &it.ObjectKey, &it.Status); err != nil {
			return nil, fmt.Errorf("failed to scan upload item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading upload item rows: %w", err)
	}
	return items, nil
}

// FinalizeBatch reconciles one fully uploaded batch in a single transaction: it locks the
// batch and its quota row, converts the reservation into billable usage, appends a usage
// entry, and queues one pending AI job per uploaded object.
//
// The quota is moved by the bytes that actually landed in storage, not by the amount that
// was originally reserved, so a batch that uploads less than expected releases the remainder.
func (r *PostgresUploadRepository) FinalizeBatch(ctx context.Context, batchID uuid.UUID, items []domain.UploadItem) (*domain.BatchFinalization, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin finalize transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var accountID uuid.UUID
	var status string
	var expectedBytes int64
	err = tx.QueryRow(ctx, `
		SELECT storage_account_id, status, expected_bytes
		FROM upload_batches WHERE id = $1
		FOR UPDATE
	`, batchID).Scan(&accountID, &status, &expectedBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadBatchNotFound
		}
		return nil, fmt.Errorf("failed to lock upload batch: %w", err)
	}
	if status != domain.UploadBatchStatusReserved {
		return nil, ErrBatchNotFinalizable
	}

	var quotaID uuid.UUID
	var reservedBytes int64
	err = tx.QueryRow(ctx, `
		SELECT id, reserved_bytes FROM storage_quotas
		WHERE storage_account_id = $1
		FOR UPDATE
	`, accountID).Scan(&quotaID, &reservedBytes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrStorageAccountNotFound
		}
		return nil, fmt.Errorf("failed to lock storage quota: %w", err)
	}

	// Measure the real usage from the item rows, which the completion step already filled in.
	var uploadedCount int
	var actualBytes int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(COALESCE(actual_bytes, 0)), 0)
		FROM upload_items
		WHERE upload_batch_id = $1 AND status = $2
	`, batchID, domain.UploadItemStatusUploaded).Scan(&uploadedCount, &actualBytes); err != nil {
		return nil, fmt.Errorf("failed to aggregate uploaded items: %w", err)
	}

	// Release the whole reservation, then bill the measured amount. The reservation row
	// caps the release so a concurrent finalize cannot drive reserved_bytes negative.
	release := expectedBytes
	if release > reservedBytes {
		release = reservedBytes
	}
	if _, err := tx.Exec(ctx, `
		UPDATE storage_quotas
		SET reserved_bytes = reserved_bytes - $2,
		    billable_bytes = billable_bytes + $3,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, quotaID, release, actualBytes); err != nil {
		return nil, fmt.Errorf("failed to reconcile quota: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE quota_reservations
		SET status = $2, committed_bytes = $3, released_at = CURRENT_TIMESTAMP
		WHERE upload_batch_id = $1
	`, batchID, domain.ReservationStatusCommitted, actualBytes); err != nil {
		return nil, fmt.Errorf("failed to commit reservation: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE upload_batches
		SET status = $2, actual_bytes = $3
		WHERE id = $1
	`, batchID, domain.UploadBatchStatusCompleted, actualBytes); err != nil {
		return nil, fmt.Errorf("failed to complete upload batch: %w", err)
	}

	// The usage entry is the append-only evidence that the bytes were billed.
	if _, err := tx.Exec(ctx, `
		INSERT INTO storage_usage_entries (storage_account_id, event_type, physical_delta_bytes, billable_delta_bytes, source_type, source_id)
		VALUES ($1, 'UPLOAD_COMMITTED', $2, $2, 'upload_batch', $3)
	`, accountID, actualBytes, batchID); err != nil {
		return nil, fmt.Errorf("failed to append usage entry: %w", err)
	}

	// One pending AI job per uploaded object, keyed so a retry cannot duplicate work.
	for _, item := range items {
		if item.Status != domain.UploadItemStatusUploaded {
			continue
		}
		key := "upload:" + batchID.String() + ":" + item.ID.String()
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_jobs (job_type, subject_id, status, idempotency_key, attempt_count)
			VALUES ('PHOTO_INGEST', $1, 'PENDING', $2, 0)
			ON CONFLICT (idempotency_key) DO NOTHING
		`, item.ID, key); err != nil {
			return nil, fmt.Errorf("failed to queue ai job: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit finalization: %w", err)
	}

	return &domain.BatchFinalization{
		BatchID:            batchID,
		Status:             domain.UploadBatchStatusCompleted,
		TotalFilesUploaded: uploadedCount,
		ActualBytes:        actualBytes,
		QuotaReconciled:    true,
		// Dispatch is the usecase's responsibility, so the repository reports it as not yet done.
		EventPublished: false,
	}, nil
}

// GetBatchByID loads one upload batch.
func (r *PostgresUploadRepository) GetBatchByID(ctx context.Context, batchID uuid.UUID) (*domain.UploadBatch, error) {
	var b domain.UploadBatch
	err := r.pool.QueryRow(ctx, `
		SELECT id, storage_account_id, owner_user_id, status, expected_file_count, expected_bytes, actual_bytes, expires_at, created_at
		FROM upload_batches WHERE id = $1
	`, batchID).Scan(&b.ID, &b.StorageAccountID, &b.OwnerUserID, &b.Status, &b.ExpectedFileCount,
		&b.ExpectedBytes, &b.ActualBytes, &b.ExpiresAt, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadBatchNotFound
		}
		return nil, fmt.Errorf("failed to query upload batch: %w", err)
	}
	return &b, nil
}
