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
	// ErrActiveApplicationExists is returned when the caller already has a pending application.
	ErrActiveApplicationExists = errors.New("an active host application already exists for this user")

	// ErrApplicationNotFound is returned when no application matches the requested identifier.
	ErrApplicationNotFound = errors.New("host application not found")

	// ErrDocumentNotFound is returned when no legal document matches the requested identifier.
	ErrDocumentNotFound = errors.New("host legal document not found")

	// ErrMembershipNotFoundInOrg is returned when a membership row does not exist in an organization.
	ErrMembershipNotFoundInOrg = errors.New("host membership not found in this organization")

	// ErrLastLeaderRemoval is returned when a removal would leave the organization without a Leader.
	ErrLastLeaderRemoval = errors.New("organization must retain exactly one active leader")

	// ErrApplicationNotPending is returned when a review targets an application that is not SUBMITTED.
	ErrApplicationNotPending = errors.New("host application has already been reviewed")

	// ErrMembershipAlreadyExists is returned when the applicant already belongs to the organization.
	ErrMembershipAlreadyExists = errors.New("user already has a membership in this organization")
)

// AdminQuota is the configured maximum number of admins for one organization.
type AdminQuota struct {
	MaxAdmins     int
	PolicyVersion string
}

// HostRepository defines data access for Host applications, documents, and memberships.
type HostRepository interface {
	CreateApplication(ctx context.Context, org *domain.HostOrganization, app *domain.HostApplication) error
	GetActiveApplicationByUser(ctx context.Context, userID uuid.UUID) (*domain.HostApplication, *domain.HostOrganization, error)
	GetApplicationByID(ctx context.Context, applicationID uuid.UUID) (*domain.HostApplication, *domain.HostOrganization, error)
	CountDocumentsByApplication(ctx context.Context, applicationID uuid.UUID) (int, error)
	CreateLegalDocument(ctx context.Context, doc *domain.HostLegalDocument) error
	GetLegalDocumentByID(ctx context.Context, documentID uuid.UUID) (*domain.HostLegalDocument, *domain.HostApplication, error)

	// Task 2.6 membership surface.
	ApproveApplication(ctx context.Context, applicationID, reviewerID uuid.UUID) (*domain.HostApplication, *domain.HostOrganization, error)
	RejectApplication(ctx context.Context, applicationID, reviewerID uuid.UUID, reasonCode string) (*domain.HostApplication, *domain.HostOrganization, error)
	GetOrganizationByID(ctx context.Context, orgID uuid.UUID) (*domain.HostOrganization, error)
	UpdateOrganizationDetails(ctx context.Context, orgID uuid.UUID, legalName, displayName, businessField string) error
	GetAdminQuota(ctx context.Context, orgID uuid.UUID) (*AdminQuota, error)
	CountActiveAdmins(ctx context.Context, orgID uuid.UUID) (int, error)
	ListMembers(ctx context.Context, orgID uuid.UUID) ([]domain.HostMemberSummary, error)
	GetMembershipByID(ctx context.Context, membershipID uuid.UUID) (*domain.HostMembership, error)
	RevokeMembership(ctx context.Context, membershipID uuid.UUID) error
}

// PostgresHostRepository implements HostRepository backed by PostgreSQL 16.
type PostgresHostRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresHostRepository constructs a new PostgresHostRepository.
func NewPostgresHostRepository(pool *pgxpool.Pool) *PostgresHostRepository {
	return &PostgresHostRepository{pool: pool}
}

// CreateApplication inserts the organization and its application in one transaction.
func (r *PostgresHostRepository) CreateApplication(ctx context.Context, org *domain.HostOrganization, app *domain.HostApplication) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin application transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO host_organizations (legal_name, display_name, business_field, verification_status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, org.LegalName, org.DisplayName, org.BusinessField, org.VerificationStatus).Scan(&org.ID, &org.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert host organization: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO host_applications (host_organization_id, submitted_by, status, submitted_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		RETURNING id, submitted_at
	`, org.ID, app.SubmittedBy, app.Status).Scan(&app.ID, &app.SubmittedAt)
	if err != nil {
		return fmt.Errorf("failed to insert host application: %w", err)
	}
	app.HostOrganizationID = org.ID

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit application transaction: %w", err)
	}
	return nil
}

// GetActiveApplicationByUser loads the caller's pending application, if any.
func (r *PostgresHostRepository) GetActiveApplicationByUser(ctx context.Context, userID uuid.UUID) (*domain.HostApplication, *domain.HostOrganization, error) {
	return r.scanApplication(ctx, `
		SELECT ha.id, ha.host_organization_id, ha.submitted_by, ha.status, ha.reviewed_by, ha.review_reason_code, ha.submitted_at, ha.reviewed_at,
		       ho.id, ho.legal_name, ho.display_name, ho.business_field, ho.verification_status, ho.created_at
		FROM host_applications ha
		JOIN host_organizations ho ON ho.id = ha.host_organization_id
		WHERE ha.submitted_by = $1 AND ha.status = $2
		ORDER BY ha.submitted_at DESC
		LIMIT 1
	`, userID, domain.HostApplicationStatusSubmitted)
}

// GetApplicationByID loads one application with its organization.
func (r *PostgresHostRepository) GetApplicationByID(ctx context.Context, applicationID uuid.UUID) (*domain.HostApplication, *domain.HostOrganization, error) {
	return r.scanApplication(ctx, `
		SELECT ha.id, ha.host_organization_id, ha.submitted_by, ha.status, ha.reviewed_by, ha.review_reason_code, ha.submitted_at, ha.reviewed_at,
		       ho.id, ho.legal_name, ho.display_name, ho.business_field, ho.verification_status, ho.created_at
		FROM host_applications ha
		JOIN host_organizations ho ON ho.id = ha.host_organization_id
		WHERE ha.id = $1
	`, applicationID)
}

func (r *PostgresHostRepository) scanApplication(ctx context.Context, query string, args ...interface{}) (*domain.HostApplication, *domain.HostOrganization, error) {
	var app domain.HostApplication
	var org domain.HostOrganization
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&app.ID, &app.HostOrganizationID, &app.SubmittedBy, &app.Status, &app.ReviewedBy, &app.ReviewReasonCode, &app.SubmittedAt, &app.ReviewedAt,
		&org.ID, &org.LegalName, &org.DisplayName, &org.BusinessField, &org.VerificationStatus, &org.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrApplicationNotFound
		}
		return nil, nil, fmt.Errorf("failed to query host application: %w", err)
	}
	return &app, &org, nil
}

// CountDocumentsByApplication returns how many legal documents were registered for an application.
func (r *PostgresHostRepository) CountDocumentsByApplication(ctx context.Context, applicationID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM host_legal_documents WHERE host_application_id = $1", applicationID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count legal documents: %w", err)
	}
	return count, nil
}

// CreateLegalDocument stores legal document metadata and returns the generated identifier.
func (r *PostgresHostRepository) CreateLegalDocument(ctx context.Context, doc *domain.HostLegalDocument) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO host_legal_documents (host_application_id, object_key, document_type, mime_type, size_bytes, review_status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, doc.HostApplicationID, doc.ObjectKey, doc.DocumentType, doc.MimeType, doc.SizeBytes, doc.ReviewStatus).Scan(&doc.ID, &doc.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert host legal document: %w", err)
	}
	return nil
}

// GetLegalDocumentByID loads one legal document with its owning application.
func (r *PostgresHostRepository) GetLegalDocumentByID(ctx context.Context, documentID uuid.UUID) (*domain.HostLegalDocument, *domain.HostApplication, error) {
	var doc domain.HostLegalDocument
	var app domain.HostApplication
	err := r.pool.QueryRow(ctx, `
		SELECT d.id, d.host_application_id, d.object_key, d.document_type, d.mime_type, d.size_bytes, d.review_status, d.created_at,
		       ha.id, ha.host_organization_id, ha.submitted_by, ha.status, ha.reviewed_by, ha.review_reason_code, ha.submitted_at, ha.reviewed_at
		FROM host_legal_documents d
		JOIN host_applications ha ON ha.id = d.host_application_id
		WHERE d.id = $1
	`, documentID).Scan(
		&doc.ID, &doc.HostApplicationID, &doc.ObjectKey, &doc.DocumentType, &doc.MimeType, &doc.SizeBytes, &doc.ReviewStatus, &doc.CreatedAt,
		&app.ID, &app.HostOrganizationID, &app.SubmittedBy, &app.Status, &app.ReviewedBy, &app.ReviewReasonCode, &app.SubmittedAt, &app.ReviewedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrDocumentNotFound
		}
		return nil, nil, fmt.Errorf("failed to query host legal document: %w", err)
	}
	return &doc, &app, nil
}
