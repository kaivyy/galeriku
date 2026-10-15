package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicateAccount = errors.New("account with this email or phone number already exists")
	ErrUserNotFound     = errors.New("user not found")
)

// UserRepository defines the persistence contract for users, profiles, and credentials.
type UserRepository interface {
	CreateUserWithProfileAndCredentials(ctx context.Context, user *domain.User, profile *domain.UserProfile, creds *domain.UserCredential, capabilityCode string) error
	GetByIdentifier(ctx context.Context, identifier string) (*domain.User, *domain.UserCredential, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetUserProfile(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error)
	GetUserCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error)
	GetUserHostMemberships(ctx context.Context, userID uuid.UUID) ([]domain.HostMembershipSummary, error)
	UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, failedAttempts int, lockedUntil *time.Time) error
	ResetFailedAttempts(ctx context.Context, userID uuid.UUID) error
}

type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

func (r *PostgresUserRepository) CreateUserWithProfileAndCredentials(
	ctx context.Context,
	user *domain.User,
	profile *domain.UserProfile,
	creds *domain.UserCredential,
	capabilityCode string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Ensure capability exists in capabilities table
	var capabilityID uuid.UUID
	capQuery := `
		INSERT INTO capabilities (id, code, description)
		VALUES (uuid_generate_v7(), $1, $2)
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
		RETURNING id
	`
	capDesc := fmt.Sprintf("Standard %s capability", capabilityCode)
	if err := tx.QueryRow(ctx, capQuery, capabilityCode, capDesc).Scan(&capabilityID); err != nil {
		return fmt.Errorf("failed to ensure capability %q: %w", capabilityCode, err)
	}

	// 2. Insert user
	userQuery := `
		INSERT INTO users (email_normalized, phone_e164, account_status)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRow(ctx, userQuery, user.EmailNormalized, user.PhoneE164, user.AccountStatus).
		Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateAccount
		}
		return fmt.Errorf("failed to insert user: %w", err)
	}

	// 3. Insert profile
	profileQuery := `
		INSERT INTO user_profiles (user_id, display_name, locale)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	if profile.Locale == "" {
		profile.Locale = "id-ID"
	}
	err = tx.QueryRow(ctx, profileQuery, user.ID, profile.DisplayName, profile.Locale).
		Scan(&profile.ID, &profile.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert user profile: %w", err)
	}
	profile.UserID = user.ID

	// 4. Insert credentials
	credsQuery := `
		INSERT INTO user_credentials (user_id, password_hash, password_algo)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	algo := creds.PasswordAlgo
	if algo == "" {
		algo = "argon2id"
	}
	err = tx.QueryRow(ctx, credsQuery, user.ID, creds.PasswordHash, algo).
		Scan(&creds.ID, &creds.CreatedAt, &creds.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert user credentials: %w", err)
	}
	creds.UserID = user.ID

	// 5. Insert initial capability
	userCapQuery := `
		INSERT INTO user_capabilities (user_id, capability_id, status, granted_at)
		VALUES ($1, $2, 'ACTIVE', CURRENT_TIMESTAMP)
	`
	if _, err := tx.Exec(ctx, userCapQuery, user.ID, capabilityID); err != nil {
		return fmt.Errorf("failed to assign initial capability: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit user registration transaction: %w", err)
	}

	return nil
}

func (r *PostgresUserRepository) GetByIdentifier(ctx context.Context, identifier string) (*domain.User, *domain.UserCredential, error) {
	norm := strings.TrimSpace(identifier)
	lowerNorm := strings.ToLower(norm)

	query := `
		SELECT 
			u.id, u.email_normalized, u.phone_e164, u.account_status, u.created_at, u.updated_at,
			c.id, c.password_hash, c.password_algo, c.failed_attempts, c.locked_until, c.created_at, c.updated_at
		FROM users u
		JOIN user_credentials c ON c.user_id = u.id
		WHERE (u.email_normalized = $1 OR u.phone_e164 = $2)
	`

	var user domain.User
	var creds domain.UserCredential

	err := r.pool.QueryRow(ctx, query, lowerNorm, norm).Scan(
		&user.ID,
		&user.EmailNormalized,
		&user.PhoneE164,
		&user.AccountStatus,
		&user.CreatedAt,
		&user.UpdatedAt,
		&creds.ID,
		&creds.PasswordHash,
		&creds.PasswordAlgo,
		&creds.FailedAttempts,
		&creds.LockedUntil,
		&creds.CreatedAt,
		&creds.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, fmt.Errorf("failed to query user by identifier: %w", err)
	}

	creds.UserID = user.ID
	return &user, &creds, nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, email_normalized, phone_e164, account_status, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	var user domain.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.EmailNormalized,
		&user.PhoneE164,
		&user.AccountStatus,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by id: %w", err)
	}
	return &user, nil
}

func (r *PostgresUserRepository) GetUserProfile(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	query := `
		SELECT id, user_id, display_name, avatar_asset_key, locale, created_at
		FROM user_profiles
		WHERE user_id = $1
	`
	var profile domain.UserProfile
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.UserID,
		&profile.DisplayName,
		&profile.AvatarAssetKey,
		&profile.Locale,
		&profile.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user profile: %w", err)
	}
	return &profile, nil
}

func (r *PostgresUserRepository) GetUserCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error) {
	query := `
		SELECT c.code
		FROM user_capabilities uc
		JOIN capabilities c ON c.id = uc.capability_id
		WHERE uc.user_id = $1 AND uc.status = 'ACTIVE'
		ORDER BY c.code ASC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user capabilities: %w", err)
	}
	defer rows.Close()

	capabilities := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("failed to scan capability code: %w", err)
		}
		capabilities = append(capabilities, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading capability rows: %w", err)
	}

	return capabilities, nil
}

func (r *PostgresUserRepository) GetUserHostMemberships(ctx context.Context, userID uuid.UUID) ([]domain.HostMembershipSummary, error) {
	query := `
		SELECT hm.host_organization_id::text, ho.display_name, hm.membership_role
		FROM host_memberships hm
		JOIN host_organizations ho ON ho.id = hm.host_organization_id
		WHERE hm.user_id = $1 AND hm.status = 'ACTIVE'
		ORDER BY ho.display_name ASC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user host memberships: %w", err)
	}
	defer rows.Close()

	memberships := make([]domain.HostMembershipSummary, 0)
	for rows.Next() {
		var m domain.HostMembershipSummary
		if err := rows.Scan(&m.OrganizationID, &m.OrganizationName, &m.Role); err != nil {
			return nil, fmt.Errorf("failed to scan membership row: %w", err)
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading membership rows: %w", err)
	}

	return memberships, nil
}

func (r *PostgresUserRepository) UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, failedAttempts int, lockedUntil *time.Time) error {
	query := `
		UPDATE user_credentials
		SET failed_attempts = $2, locked_until = $3, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1
	`
	_, err := r.pool.Exec(ctx, query, userID, failedAttempts, lockedUntil)
	if err != nil {
		return fmt.Errorf("failed to update failed attempts: %w", err)
	}
	return nil
}

func (r *PostgresUserRepository) ResetFailedAttempts(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE user_credentials
		SET failed_attempts = 0, locked_until = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1
	`
	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to reset failed attempts: %w", err)
	}
	return nil
}
