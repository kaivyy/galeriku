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
	// ErrUploadItemNotFound is returned when no upload item matches the batch and file token.
	ErrUploadItemNotFound = errors.New("upload item not found in this batch")

	// ErrMultipartSessionExists is returned when an item already has an active multipart session.
	ErrMultipartSessionExists = errors.New("an active multipart session already exists for this item")

	// ErrMultipartSessionNotFound is returned when an item has no multipart session to complete.
	ErrMultipartSessionNotFound = errors.New("no active multipart session for this item")
)

// MultipartRepository defines data access for multipart upload sessions.
type MultipartRepository interface {
	GetUploadItemByToken(ctx context.Context, batchID uuid.UUID, clientFileToken string) (*domain.UploadItem, error)
	GetBatchIDByItem(ctx context.Context, itemID uuid.UUID, out *uuid.UUID) error
	CreateMultipartSession(ctx context.Context, itemID uuid.UUID, providerUploadID, objectKey string, partCount int, expiresAt time.Time) (uuid.UUID, error)
	GetActiveSessionByItem(ctx context.Context, itemID uuid.UUID) (providerUploadID string, objectKey string, err error)
	CompleteMultipartSession(ctx context.Context, itemID uuid.UUID, actualBytes int64) error
}

// GetBatchIDByItem resolves the batch that owns one upload item.
func (r *PostgresMultipartRepository) GetBatchIDByItem(ctx context.Context, itemID uuid.UUID, out *uuid.UUID) error {
	err := r.pool.QueryRow(ctx, "SELECT upload_batch_id FROM upload_items WHERE id = $1", itemID).Scan(out)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUploadItemNotFound
		}
		return fmt.Errorf("failed to resolve item batch: %w", err)
	}
	return nil
}

// PostgresMultipartRepository implements MultipartRepository backed by PostgreSQL 16.
type PostgresMultipartRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresMultipartRepository constructs a new PostgresMultipartRepository.
func NewPostgresMultipartRepository(pool *pgxpool.Pool) *PostgresMultipartRepository {
	return &PostgresMultipartRepository{pool: pool}
}

// GetUploadItemByToken loads one file slot by the batch and the client's own token.
func (r *PostgresMultipartRepository) GetUploadItemByToken(ctx context.Context, batchID uuid.UUID, clientFileToken string) (*domain.UploadItem, error) {
	var item domain.UploadItem
	err := r.pool.QueryRow(ctx, `
		SELECT id, upload_batch_id, client_file_token, expected_bytes, object_key, status
		FROM upload_items
		WHERE upload_batch_id = $1 AND client_file_token = $2
	`, batchID, clientFileToken).Scan(&item.ID, &item.UploadBatchID, &item.ClientFileToken, &item.ExpectedBytes, &item.ObjectKey, &item.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUploadItemNotFound
		}
		return nil, fmt.Errorf("failed to query upload item: %w", err)
	}
	return &item, nil
}

// CreateMultipartSession records one open multipart session, refusing a second active one.
func (r *PostgresMultipartRepository) CreateMultipartSession(ctx context.Context, itemID uuid.UUID, providerUploadID, objectKey string, partCount int, expiresAt time.Time) (uuid.UUID, error) {
	var existing int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM multipart_upload_sessions
		WHERE upload_item_id = $1 AND status = 'ACTIVE'
	`, itemID).Scan(&existing); err != nil {
		return uuid.Nil, fmt.Errorf("failed to check existing multipart session: %w", err)
	}
	if existing > 0 {
		return uuid.Nil, ErrMultipartSessionExists
	}

	var sessionID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO multipart_upload_sessions (upload_item_id, provider_upload_id, status, part_count, expires_at)
		VALUES ($1, $2, 'ACTIVE', $3, $4)
		RETURNING id
	`, itemID, providerUploadID, partCount, expiresAt).Scan(&sessionID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert multipart session: %w", err)
	}

	if _, err := r.pool.Exec(ctx, `
		UPDATE upload_items SET object_key = $2 WHERE id = $1
	`, itemID, objectKey); err != nil {
		return uuid.Nil, fmt.Errorf("failed to record object key: %w", err)
	}
	return sessionID, nil
}

// GetActiveSessionByItem loads the open session for one item, if any.
func (r *PostgresMultipartRepository) GetActiveSessionByItem(ctx context.Context, itemID uuid.UUID) (string, string, error) {
	var providerUploadID string
	err := r.pool.QueryRow(ctx, `
		SELECT provider_upload_id FROM multipart_upload_sessions
		WHERE upload_item_id = $1 AND status = 'ACTIVE'
	`, itemID).Scan(&providerUploadID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// S3 accepts a UUID-shaped upload id, so the session identifier is stored as one.
			return "", "", ErrMultipartSessionNotFound
		}
		return "", "", fmt.Errorf("failed to query multipart session: %w", err)
	}

	var objectKey *string
	if err := r.pool.QueryRow(ctx, "SELECT object_key FROM upload_items WHERE id = $1", itemID).Scan(&objectKey); err != nil {
		return "", "", fmt.Errorf("failed to query item object key: %w", err)
	}
	key := ""
	if objectKey != nil {
		key = *objectKey
	}
	return providerUploadID, key, nil
}

// CompleteMultipartSession marks one item uploaded with its measured size and closes the session.
func (r *PostgresMultipartRepository) CompleteMultipartSession(ctx context.Context, itemID uuid.UUID, actualBytes int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin completion transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE multipart_upload_sessions
		SET status = 'COMPLETED', completed_at = CURRENT_TIMESTAMP
		WHERE upload_item_id = $1 AND status = 'ACTIVE'
	`, itemID)
	if err != nil {
		return fmt.Errorf("failed to close multipart session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMultipartSessionNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE upload_items SET status = $2, actual_bytes = $3 WHERE id = $1
	`, itemID, domain.UploadItemStatusUploaded, actualBytes); err != nil {
		return fmt.Errorf("failed to mark upload item uploaded: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit completion: %w", err)
	}
	return nil
}
