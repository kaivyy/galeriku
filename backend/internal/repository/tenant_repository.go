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
	ErrMembershipNotFound   = errors.New("host membership not found")
	ErrMembershipInactive   = errors.New("host membership is not active")
	ErrEventNotFound        = errors.New("event not found")
	ErrOrganizationNotFound = errors.New("host organization not found")
)

// TenantRepository defines authoritative data access methods for tenant isolation and membership checks.
type TenantRepository interface {
	GetActiveHostMembership(ctx context.Context, userID, orgID uuid.UUID) (*domain.HostMembership, *domain.HostOrganization, error)
	GetEventHostOrganizationID(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error)
	GetUserActiveCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// PostgresTenantRepository implements TenantRepository backed by PostgreSQL 16.
type PostgresTenantRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresTenantRepository constructs a new PostgresTenantRepository.
func NewPostgresTenantRepository(pool *pgxpool.Pool) *PostgresTenantRepository {
	return &PostgresTenantRepository{pool: pool}
}

// GetActiveHostMembership loads the membership and organization record, ensuring status is ACTIVE.
func (r *PostgresTenantRepository) GetActiveHostMembership(ctx context.Context, userID, orgID uuid.UUID) (*domain.HostMembership, *domain.HostOrganization, error) {
	query := `
		SELECT 
			hm.id, hm.host_organization_id, hm.user_id, hm.membership_role, hm.status, hm.invited_by, hm.created_at, hm.revoked_at,
			ho.id, ho.legal_name, ho.display_name, ho.business_field, ho.verification_status, ho.created_at
		FROM host_memberships hm
		JOIN host_organizations ho ON ho.id = hm.host_organization_id
		WHERE hm.user_id = $1 AND hm.host_organization_id = $2
		ORDER BY hm.created_at DESC
		LIMIT 1
	`
	var hm domain.HostMembership
	var ho domain.HostOrganization

	err := r.pool.QueryRow(ctx, query, userID, orgID).Scan(
		&hm.ID,
		&hm.HostOrganizationID,
		&hm.UserID,
		&hm.MembershipRole,
		&hm.Status,
		&hm.InvitedBy,
		&hm.CreatedAt,
		&hm.RevokedAt,
		&ho.ID,
		&ho.LegalName,
		&ho.DisplayName,
		&ho.BusinessField,
		&ho.VerificationStatus,
		&ho.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrMembershipNotFound
		}
		return nil, nil, fmt.Errorf("failed to query host membership: %w", err)
	}

	if hm.Status != domain.HostMembershipStatusActive {
		return &hm, &ho, ErrMembershipInactive
	}

	return &hm, &ho, nil
}

// GetEventHostOrganizationID resolves the owning host organization for a given event ID.
func (r *PostgresTenantRepository) GetEventHostOrganizationID(ctx context.Context, eventID uuid.UUID) (uuid.UUID, error) {
	query := `
		SELECT host_organization_id
		FROM events
		WHERE id = $1
	`
	var orgID uuid.UUID
	err := r.pool.QueryRow(ctx, query, eventID).Scan(&orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrEventNotFound
		}
		return uuid.Nil, fmt.Errorf("failed to query event owner: %w", err)
	}
	return orgID, nil
}

// GetUserActiveCapabilities authoritatively queries active capabilities for a user from PostgreSQL.
func (r *PostgresTenantRepository) GetUserActiveCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error) {
	query := `
		SELECT c.code
		FROM user_capabilities uc
		JOIN capabilities c ON c.id = uc.capability_id
		WHERE uc.user_id = $1 AND uc.status = 'ACTIVE'
		ORDER BY c.code ASC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user active capabilities: %w", err)
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
