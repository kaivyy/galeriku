package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTokenNotFound     = errors.New("refresh token not found")
	ErrChallengeNotFound = errors.New("OTP challenge not found")
	ErrTokenAlreadyRevoked = errors.New("refresh token is already revoked")
)

// AuthRepository defines the persistence contract for refresh tokens and OTP challenges.
type AuthRepository interface {
	CreateRefreshToken(ctx context.Context, token *domain.RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldTokenID uuid.UUID, newToken *domain.RefreshToken) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string) error
	RevokeToken(ctx context.Context, tokenID uuid.UUID, reason string) error

	CreateOTPChallenge(ctx context.Context, challenge *domain.OTPChallenge) error
	GetOTPChallengeByID(ctx context.Context, id uuid.UUID) (*domain.OTPChallenge, error)
	IncrementOTPAttempts(ctx context.Context, id uuid.UUID) (int, error)
	MarkOTPVerified(ctx context.Context, id uuid.UUID) error
	MarkOTPStatus(ctx context.Context, id uuid.UUID, status string) error
}

type PostgresAuthRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAuthRepository(pool *pgxpool.Pool) *PostgresAuthRepository {
	return &PostgresAuthRepository{pool: pool}
}

func (r *PostgresAuthRepository) CreateRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	query := `
		INSERT INTO auth_refresh_tokens (user_id, token_hash, family_id, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`
	err := r.pool.QueryRow(
		ctx,
		query,
		token.UserID,
		token.TokenHash,
		token.FamilyID,
		token.ExpiresAt,
		token.UserAgent,
		token.IPAddress,
	).Scan(&token.ID, &token.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert refresh token: %w", err)
	}
	return nil
}

func (r *PostgresAuthRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, family_id, is_revoked, revoked_reason, expires_at, created_at, user_agent, ip_address
		FROM auth_refresh_tokens
		WHERE token_hash = $1
	`
	var t domain.RefreshToken
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&t.ID,
		&t.UserID,
		&t.TokenHash,
		&t.FamilyID,
		&t.IsRevoked,
		&t.RevokedReason,
		&t.ExpiresAt,
		&t.CreatedAt,
		&t.UserAgent,
		&t.IPAddress,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("failed to query refresh token by hash: %w", err)
	}
	return &t, nil
}

// RotateRefreshToken performs atomic token rotation under a row-level lock.
func (r *PostgresAuthRepository) RotateRefreshToken(ctx context.Context, oldTokenID uuid.UUID, newToken *domain.RefreshToken) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin rotation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock and inspect the current token
	var isRevoked bool
	var familyID uuid.UUID
	lockQuery := `
		SELECT is_revoked, family_id
		FROM auth_refresh_tokens
		WHERE id = $1
		FOR UPDATE
	`
	err = tx.QueryRow(ctx, lockQuery, oldTokenID).Scan(&isRevoked, &familyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("failed to lock refresh token for rotation: %w", err)
	}

	if isRevoked {
		return ErrTokenAlreadyRevoked
	}

	// Mark old token revoked
	revokeQuery := `
		UPDATE auth_refresh_tokens
		SET is_revoked = TRUE, revoked_reason = 'ROTATED'
		WHERE id = $1
	`
	if _, err := tx.Exec(ctx, revokeQuery, oldTokenID); err != nil {
		return fmt.Errorf("failed to mark token rotated: %w", err)
	}

	// Insert successor token in the same family
	newToken.FamilyID = familyID
	insertQuery := `
		INSERT INTO auth_refresh_tokens (user_id, token_hash, family_id, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`
	err = tx.QueryRow(
		ctx,
		insertQuery,
		newToken.UserID,
		newToken.TokenHash,
		newToken.FamilyID,
		newToken.ExpiresAt,
		newToken.UserAgent,
		newToken.IPAddress,
	).Scan(&newToken.ID, &newToken.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert successor refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit token rotation transaction: %w", err)
	}

	return nil
}

func (r *PostgresAuthRepository) RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string) error {
	query := `
		UPDATE auth_refresh_tokens
		SET is_revoked = TRUE, revoked_reason = $2
		WHERE family_id = $1 AND is_revoked = FALSE
	`
	_, err := r.pool.Exec(ctx, query, familyID, reason)
	if err != nil {
		return fmt.Errorf("failed to revoke token family: %w", err)
	}
	return nil
}

func (r *PostgresAuthRepository) RevokeToken(ctx context.Context, tokenID uuid.UUID, reason string) error {
	query := `
		UPDATE auth_refresh_tokens
		SET is_revoked = TRUE, revoked_reason = $2
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, tokenID, reason)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	return nil
}

func (r *PostgresAuthRepository) CreateOTPChallenge(ctx context.Context, challenge *domain.OTPChallenge) error {
	query := `
		INSERT INTO auth_otp_challenges (phone_e164, email_normalized, purpose, code_hash, max_attempts, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	if challenge.MaxAttempts == 0 {
		challenge.MaxAttempts = 3
	}
	if challenge.Status == "" {
		challenge.Status = "PENDING"
	}

	err := r.pool.QueryRow(
		ctx,
		query,
		challenge.PhoneE164,
		challenge.EmailNormalized,
		challenge.Purpose,
		challenge.CodeHash,
		challenge.MaxAttempts,
		challenge.Status,
		challenge.ExpiresAt,
	).Scan(&challenge.ID, &challenge.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert OTP challenge: %w", err)
	}
	return nil
}

func (r *PostgresAuthRepository) GetOTPChallengeByID(ctx context.Context, id uuid.UUID) (*domain.OTPChallenge, error) {
	query := `
		SELECT id, phone_e164, email_normalized, purpose, code_hash, attempt_count, max_attempts, status, expires_at, verified_at, created_at
		FROM auth_otp_challenges
		WHERE id = $1
	`
	var c domain.OTPChallenge
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&c.ID,
		&c.PhoneE164,
		&c.EmailNormalized,
		&c.Purpose,
		&c.CodeHash,
		&c.AttemptCount,
		&c.MaxAttempts,
		&c.Status,
		&c.ExpiresAt,
		&c.VerifiedAt,
		&c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrChallengeNotFound
		}
		return nil, fmt.Errorf("failed to query OTP challenge by id: %w", err)
	}
	return &c, nil
}

func (r *PostgresAuthRepository) IncrementOTPAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	query := `
		UPDATE auth_otp_challenges
		SET attempt_count = attempt_count + 1
		WHERE id = $1
		RETURNING attempt_count
	`
	var newAttempts int
	err := r.pool.QueryRow(ctx, query, id).Scan(&newAttempts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrChallengeNotFound
		}
		return 0, fmt.Errorf("failed to increment OTP attempts: %w", err)
	}
	return newAttempts, nil
}

func (r *PostgresAuthRepository) MarkOTPVerified(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE auth_otp_challenges
		SET status = 'VERIFIED', verified_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'PENDING'
	`
	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark OTP verified: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrChallengeNotFound
	}
	return nil
}

func (r *PostgresAuthRepository) MarkOTPStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `
		UPDATE auth_otp_challenges
		SET status = $2
		WHERE id = $1
	`
	result, err := r.pool.Exec(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("failed to mark OTP status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrChallengeNotFound
	}
	return nil
}
