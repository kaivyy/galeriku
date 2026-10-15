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
	// ErrInvitationNotFound is returned when no invitation matches the presented token or id.
	ErrInvitationNotFound = errors.New("host invitation not found")

	// ErrInvitationExpired is returned when an invitation is past its expiry time.
	ErrInvitationExpired = errors.New("host invitation has expired")

	// ErrInvitationConsumed is returned when an invitation was already accepted or revoked.
	ErrInvitationConsumed = errors.New("host invitation is no longer pending")

	// ErrAdminLimitExceeded is returned when accepting would exceed the organization's admin quota.
	ErrAdminLimitExceeded = errors.New("host admin quota exceeded")

	// ErrInviteeMismatch is returned when the accepting user is not the invited recipient.
	ErrInviteeMismatch = errors.New("invitation was issued to a different recipient")
)

// InvitationRepository defines data access for Host team invitations.
type InvitationRepository interface {
	CreateInvitation(ctx context.Context, inv *domain.HostInvitation) error
	GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*domain.HostInvitation, error)
	GetInvitationByID(ctx context.Context, id uuid.UUID) (*domain.HostInvitation, error)
	RevokeInvitation(ctx context.Context, id uuid.UUID) error
	AcceptInvitation(ctx context.Context, invitationID, userID uuid.UUID) error
}

// PostgresInvitationRepository implements InvitationRepository backed by PostgreSQL 16.
type PostgresInvitationRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresInvitationRepository constructs a new PostgresInvitationRepository.
func NewPostgresInvitationRepository(pool *pgxpool.Pool) *PostgresInvitationRepository {
	return &PostgresInvitationRepository{pool: pool}
}

// CreateInvitation persists one invitation with its token hash.
func (r *PostgresInvitationRepository) CreateInvitation(ctx context.Context, inv *domain.HostInvitation) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO host_invitations (host_organization_id, invited_by, invitee_user_id, invitee_email, role, token_hash, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, inv.HostOrganizationID, inv.InvitedBy, inv.InviteeUserID, inv.InviteeEmail, inv.Role, inv.TokenHash, inv.Status, inv.ExpiresAt).Scan(&inv.ID)
	if err != nil {
		return fmt.Errorf("failed to insert host invitation: %w", err)
	}
	return nil
}

// GetInvitationByTokenHash loads one invitation by the SHA-256 digest of its token.
func (r *PostgresInvitationRepository) GetInvitationByTokenHash(ctx context.Context, tokenHash string) (*domain.HostInvitation, error) {
	return r.scanInvitation(ctx, `
		SELECT id, host_organization_id, invited_by, invitee_user_id, invitee_email, role, token_hash, status, expires_at
		FROM host_invitations WHERE token_hash = $1
	`, tokenHash)
}

// GetInvitationByID loads one invitation by primary key.
func (r *PostgresInvitationRepository) GetInvitationByID(ctx context.Context, id uuid.UUID) (*domain.HostInvitation, error) {
	return r.scanInvitation(ctx, `
		SELECT id, host_organization_id, invited_by, invitee_user_id, invitee_email, role, token_hash, status, expires_at
		FROM host_invitations WHERE id = $1
	`, id)
}

func (r *PostgresInvitationRepository) scanInvitation(ctx context.Context, query string, arg interface{}) (*domain.HostInvitation, error) {
	var inv domain.HostInvitation
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&inv.ID, &inv.HostOrganizationID, &inv.InvitedBy, &inv.InviteeUserID, &inv.InviteeEmail,
		&inv.Role, &inv.TokenHash, &inv.Status, &inv.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		return nil, fmt.Errorf("failed to query host invitation: %w", err)
	}
	return &inv, nil
}

// RevokeInvitation marks one pending invitation REVOKED.
func (r *PostgresInvitationRepository) RevokeInvitation(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE host_invitations SET status = $1
		WHERE id = $2 AND status = $3
	`, domain.InvitationStatusRevoked, id, domain.InvitationStatusPending)
	if err != nil {
		return fmt.Errorf("failed to revoke host invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvitationConsumed
	}
	return nil
}

// AcceptInvitation consumes one invitation atomically: it locks the quota row, re-checks the
// admin limit, creates the ACTIVE ADMIN membership, grants the HOST_ADMIN capability, and marks
// the invitation ACCEPTED. All of it commits together or not at all.
func (r *PostgresInvitationRepository) AcceptInvitation(ctx context.Context, invitationID, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin acceptance transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inv domain.HostInvitation
	err = tx.QueryRow(ctx, `
		SELECT id, host_organization_id, invited_by, invitee_user_id, invitee_email, role, token_hash, status, expires_at
		FROM host_invitations WHERE id = $1
		FOR UPDATE
	`, invitationID).Scan(
		&inv.ID, &inv.HostOrganizationID, &inv.InvitedBy, &inv.InviteeUserID, &inv.InviteeEmail,
		&inv.Role, &inv.TokenHash, &inv.Status, &inv.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvitationNotFound
		}
		return fmt.Errorf("failed to lock host invitation: %w", err)
	}

	if inv.Status != domain.InvitationStatusPending {
		return ErrInvitationConsumed
	}
	if inv.ExpiresAt.Before(time.Now().UTC()) {
		if _, err := tx.Exec(ctx, "UPDATE host_invitations SET status = $1 WHERE id = $2",
			domain.InvitationStatusExpired, inv.ID); err != nil {
			return fmt.Errorf("failed to mark invitation expired: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit expiry: %w", err)
		}
		return ErrInvitationExpired
	}

	// Lock the quota row so concurrent acceptances serialize on the admin count.
	var maxAdmins int
	err = tx.QueryRow(ctx,
		"SELECT max_admins FROM host_admin_limits WHERE host_organization_id = $1 FOR UPDATE",
		inv.HostOrganizationID).Scan(&maxAdmins)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to lock admin quota: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		maxAdmins = 5
	}

	var currentAdmins int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM host_memberships
		WHERE host_organization_id = $1 AND membership_role = $2 AND status = $3
	`, inv.HostOrganizationID, domain.HostRoleAdmin, domain.HostMembershipStatusActive).Scan(&currentAdmins); err != nil {
		return fmt.Errorf("failed to count active admins: %w", err)
	}
	if currentAdmins >= maxAdmins {
		return ErrAdminLimitExceeded
	}

	// One membership per user per organization: a second invitation to the same person
	// must not stack a duplicate ACTIVE ADMIN row that inflates the admin count.
	var existing int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM host_memberships
		WHERE host_organization_id = $1 AND user_id = $2
	`, inv.HostOrganizationID, userID).Scan(&existing); err != nil {
		return fmt.Errorf("failed to check existing membership: %w", err)
	}
	if existing > 0 {
		return ErrMembershipAlreadyExists
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status, invited_by)
		VALUES ($1, $2, $3, $4, $5)
	`, inv.HostOrganizationID, userID, domain.HostRoleAdmin, domain.HostMembershipStatusActive, inv.InvitedBy); err != nil {
		return fmt.Errorf("failed to create admin membership: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO capabilities (code, description)
		VALUES ($1, $2)
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
	`, "HOST_ADMIN", "Host organization admin capability"); err != nil {
		return fmt.Errorf("failed to ensure HOST_ADMIN capability: %w", err)
	}
	// Idempotent grant: user_capabilities has no unique constraint, so guard against stacking.
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_capabilities (user_id, capability_id, status, granted_by, granted_at)
		SELECT $1, c.id, 'ACTIVE', $2, CURRENT_TIMESTAMP
		FROM capabilities c
		WHERE c.code = 'HOST_ADMIN'
		  AND NOT EXISTS (
			SELECT 1 FROM user_capabilities uc
			JOIN capabilities c2 ON c2.id = uc.capability_id
			WHERE uc.user_id = $1 AND c2.code = 'HOST_ADMIN'
		  )
	`, userID, inv.InvitedBy); err != nil {
		return fmt.Errorf("failed to grant HOST_ADMIN capability: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE host_invitations SET status = $1, invitee_user_id = $2 WHERE id = $3
	`, domain.InvitationStatusAccepted, userID, inv.ID); err != nil {
		return fmt.Errorf("failed to mark invitation accepted: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit acceptance: %w", err)
	}
	return nil
}
