package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// defaultMaxAdmins is the admin quota initialized for a newly approved organization.
	defaultMaxAdmins = 5

	// adminPolicyVersion labels the quota policy applied at approval time.
	adminPolicyVersion = "v1-default-5"
)

// getMembershipTx reports whether a membership row already exists for one user in one
// organization, inside an open transaction.
func (r *PostgresHostRepository) getMembershipTx(ctx context.Context, tx pgx.Tx, orgID, userID uuid.UUID) (*domain.HostMembership, error) {
	var hm domain.HostMembership
	err := tx.QueryRow(ctx, `
		SELECT id, host_organization_id, user_id, membership_role, status, invited_by, created_at, revoked_at
		FROM host_memberships
		WHERE host_organization_id = $1 AND user_id = $2
		LIMIT 1
	`, orgID, userID).Scan(&hm.ID, &hm.HostOrganizationID, &hm.UserID, &hm.MembershipRole, &hm.Status, &hm.InvitedBy, &hm.CreatedAt, &hm.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to check existing membership: %w", err)
	}
	return &hm, nil
}

// ApproveApplication atomically approves one application: organization status, application status,
// the applicant's ACTIVE LEADER membership, the global HOST_LEADER capability, and the admin quota row.
func (r *PostgresHostRepository) ApproveApplication(ctx context.Context, applicationID, reviewerID uuid.UUID) (*domain.HostApplication, *domain.HostOrganization, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin approval transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var app domain.HostApplication
	var org domain.HostOrganization
	err = tx.QueryRow(ctx, `
		SELECT ha.id, ha.host_organization_id, ha.submitted_by, ha.status, ha.reviewed_by, ha.review_reason_code, ha.submitted_at, ha.reviewed_at,
		       ho.id, ho.legal_name, ho.display_name, ho.business_field, ho.verification_status, ho.created_at
		FROM host_applications ha
		JOIN host_organizations ho ON ho.id = ha.host_organization_id
		WHERE ha.id = $1
		FOR UPDATE OF ha
	`, applicationID).Scan(
		&app.ID, &app.HostOrganizationID, &app.SubmittedBy, &app.Status, &app.ReviewedBy, &app.ReviewReasonCode, &app.SubmittedAt, &app.ReviewedAt,
		&org.ID, &org.LegalName, &org.DisplayName, &org.BusinessField, &org.VerificationStatus, &org.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrApplicationNotFound
		}
		return nil, nil, fmt.Errorf("failed to lock host application: %w", err)
	}

	// Only a pending application may be approved. Re-approving would flip a rejected
	// organization back to APPROVED and stack a second membership and capability row.
	if app.Status != domain.HostApplicationStatusSubmitted {
		return nil, nil, ErrApplicationNotPending
	}
	if existing, err := r.getMembershipTx(ctx, tx, org.ID, app.SubmittedBy); err != nil {
		return nil, nil, err
	} else if existing != nil {
		return nil, nil, ErrMembershipAlreadyExists
	}

	if _, err := tx.Exec(ctx, `
		UPDATE host_organizations SET verification_status = $1 WHERE id = $2
	`, domain.HostApplicationStatusApproved, org.ID); err != nil {
		return nil, nil, fmt.Errorf("failed to update organization status: %w", err)
	}
	org.VerificationStatus = domain.HostApplicationStatusApproved

	if _, err := tx.Exec(ctx, `
		UPDATE host_applications
		SET status = $1, reviewed_by = $2, reviewed_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, domain.HostApplicationStatusApproved, reviewerID, app.ID); err != nil {
		return nil, nil, fmt.Errorf("failed to update application status: %w", err)
	}
	app.Status = domain.HostApplicationStatusApproved
	app.ReviewedBy = &reviewerID

	if _, err := tx.Exec(ctx, `
		INSERT INTO host_memberships (host_organization_id, user_id, membership_role, status, invited_by)
		VALUES ($1, $2, $3, $4, $5)
	`, org.ID, app.SubmittedBy, domain.HostRoleLeader, domain.HostMembershipStatusActive, reviewerID); err != nil {
		return nil, nil, fmt.Errorf("failed to create leader membership: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO capabilities (code, description)
		VALUES ($1, $2)
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
	`, "HOST_LEADER", "Host organization leader capability"); err != nil {
		return nil, nil, fmt.Errorf("failed to ensure HOST_LEADER capability: %w", err)
	}

	// The capability grant is idempotent: a re-grant updates the existing row instead of
	// stacking a duplicate, because user_capabilities has no unique constraint.
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_capabilities (user_id, capability_id, status, granted_by, granted_at)
		SELECT $1, c.id, 'ACTIVE', $2, CURRENT_TIMESTAMP
		FROM capabilities c
		WHERE c.code = 'HOST_LEADER'
		  AND NOT EXISTS (
			SELECT 1 FROM user_capabilities uc
			JOIN capabilities c2 ON c2.id = uc.capability_id
			WHERE uc.user_id = $1 AND c2.code = 'HOST_LEADER'
		  )
	`, app.SubmittedBy, reviewerID); err != nil {
		return nil, nil, fmt.Errorf("failed to grant HOST_LEADER capability: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO host_admin_limits (host_organization_id, max_admins, policy_version, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (host_organization_id) DO NOTHING
	`, org.ID, defaultMaxAdmins, adminPolicyVersion, reviewerID); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize admin quota: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to commit approval transaction: %w", err)
	}
	return &app, &org, nil
}

// RejectApplication marks one application and its organization as rejected.
func (r *PostgresHostRepository) RejectApplication(ctx context.Context, applicationID, reviewerID uuid.UUID, reasonCode string) (*domain.HostApplication, *domain.HostOrganization, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin rejection transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var app domain.HostApplication
	var org domain.HostOrganization
	err = tx.QueryRow(ctx, `
		SELECT ha.id, ha.host_organization_id, ha.submitted_by, ha.status, ha.reviewed_by, ha.review_reason_code, ha.submitted_at, ha.reviewed_at,
		       ho.id, ho.legal_name, ho.display_name, ho.business_field, ho.verification_status, ho.created_at
		FROM host_applications ha
		JOIN host_organizations ho ON ho.id = ha.host_organization_id
		WHERE ha.id = $1
		FOR UPDATE OF ha
	`, applicationID).Scan(
		&app.ID, &app.HostOrganizationID, &app.SubmittedBy, &app.Status, &app.ReviewedBy, &app.ReviewReasonCode, &app.SubmittedAt, &app.ReviewedAt,
		&org.ID, &org.LegalName, &org.DisplayName, &org.BusinessField, &org.VerificationStatus, &org.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrApplicationNotFound
		}
		return nil, nil, fmt.Errorf("failed to lock host application: %w", err)
	}

	if app.Status != domain.HostApplicationStatusSubmitted {
		return nil, nil, ErrApplicationNotPending
	}

	if _, err := tx.Exec(ctx,
		"UPDATE host_organizations SET verification_status = $1 WHERE id = $2",
		domain.HostApplicationStatusRejected, org.ID); err != nil {
		return nil, nil, fmt.Errorf("failed to update organization status: %w", err)
	}
	org.VerificationStatus = domain.HostApplicationStatusRejected

	if _, err := tx.Exec(ctx, `
		UPDATE host_applications
		SET status = $1, reviewed_by = $2, review_reason_code = $3, reviewed_at = CURRENT_TIMESTAMP
		WHERE id = $4
	`, domain.HostApplicationStatusRejected, reviewerID, reasonCode, app.ID); err != nil {
		return nil, nil, fmt.Errorf("failed to update application status: %w", err)
	}
	app.Status = domain.HostApplicationStatusRejected
	app.ReviewedBy = &reviewerID
	app.ReviewReasonCode = &reasonCode

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to commit rejection transaction: %w", err)
	}
	return &app, &org, nil
}

// GetOrganizationByID loads one host organization.
func (r *PostgresHostRepository) GetOrganizationByID(ctx context.Context, orgID uuid.UUID) (*domain.HostOrganization, error) {
	var org domain.HostOrganization
	err := r.pool.QueryRow(ctx, `
		SELECT id, legal_name, display_name, business_field, verification_status, created_at
		FROM host_organizations WHERE id = $1
	`, orgID).Scan(&org.ID, &org.LegalName, &org.DisplayName, &org.BusinessField, &org.VerificationStatus, &org.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to query host organization: %w", err)
	}
	return &org, nil
}

// UpdateOrganizationDetails updates the legal-facing organization fields.
func (r *PostgresHostRepository) UpdateOrganizationDetails(ctx context.Context, orgID uuid.UUID, legalName, displayName, businessField string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE host_organizations
		SET legal_name = COALESCE(NULLIF($2, ''), legal_name),
		    display_name = COALESCE(NULLIF($3, ''), display_name),
		    business_field = COALESCE(NULLIF($4, ''), business_field)
		WHERE id = $1
	`, orgID, legalName, displayName, businessField)
	if err != nil {
		return fmt.Errorf("failed to update host organization: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOrganizationNotFound
	}
	return nil
}

// GetAdminQuota loads the configured admin quota for one organization.
func (r *PostgresHostRepository) GetAdminQuota(ctx context.Context, orgID uuid.UUID) (*AdminQuota, error) {
	var quota AdminQuota
	err := r.pool.QueryRow(ctx,
		"SELECT max_admins, policy_version FROM host_admin_limits WHERE host_organization_id = $1", orgID,
	).Scan(&quota.MaxAdmins, &quota.PolicyVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &AdminQuota{MaxAdmins: defaultMaxAdmins, PolicyVersion: adminPolicyVersion}, nil
		}
		return nil, fmt.Errorf("failed to query admin quota: %w", err)
	}
	return &quota, nil
}

// CountActiveAdmins counts ACTIVE ADMIN memberships in one organization.
func (r *PostgresHostRepository) CountActiveAdmins(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM host_memberships
		WHERE host_organization_id = $1 AND membership_role = $2 AND status = $3
	`, orgID, domain.HostRoleAdmin, domain.HostMembershipStatusActive).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active admins: %w", err)
	}
	return count, nil
}

// ListMembers returns every membership of one organization with its user display name.
func (r *PostgresHostRepository) ListMembers(ctx context.Context, orgID uuid.UUID) ([]domain.HostMemberSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT hm.id, hm.user_id, COALESCE(up.display_name, ''), hm.membership_role, hm.status, hm.created_at, hm.revoked_at
		FROM host_memberships hm
		LEFT JOIN user_profiles up ON up.user_id = hm.user_id
		WHERE hm.host_organization_id = $1
		ORDER BY hm.created_at ASC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list host members: %w", err)
	}
	defer rows.Close()

	members := make([]domain.HostMemberSummary, 0)
	for rows.Next() {
		var m domain.HostMemberSummary
		if err := rows.Scan(&m.MembershipID, &m.UserID, &m.DisplayName, &m.Role, &m.Status, &m.CreatedAt, &m.RevokedAt); err != nil {
			return nil, fmt.Errorf("failed to scan host member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading host member rows: %w", err)
	}
	return members, nil
}

// GetMembershipByID loads one membership row.
func (r *PostgresHostRepository) GetMembershipByID(ctx context.Context, membershipID uuid.UUID) (*domain.HostMembership, error) {
	var hm domain.HostMembership
	err := r.pool.QueryRow(ctx, `
		SELECT id, host_organization_id, user_id, membership_role, status, invited_by, created_at, revoked_at
		FROM host_memberships WHERE id = $1
	`, membershipID).Scan(&hm.ID, &hm.HostOrganizationID, &hm.UserID, &hm.MembershipRole, &hm.Status, &hm.InvitedBy, &hm.CreatedAt, &hm.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMembershipNotFoundInOrg
		}
		return nil, fmt.Errorf("failed to query host membership: %w", err)
	}
	return &hm, nil
}

// RevokeMembership marks one membership REVOKED and stamps the revocation time.
func (r *PostgresHostRepository) RevokeMembership(ctx context.Context, membershipID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE host_memberships
		SET status = $1, revoked_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, domain.HostMembershipStatusRevoked, membershipID)
	if err != nil {
		return fmt.Errorf("failed to revoke host membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrMembershipNotFoundInOrg
	}
	return nil
}
