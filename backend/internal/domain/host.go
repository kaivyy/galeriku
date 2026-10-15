package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// HostApplicationStatusSubmitted marks an application awaiting Super Admin review.
	HostApplicationStatusSubmitted = "SUBMITTED"

	// HostApplicationStatusApproved marks an application accepted by Super Admin (set in Task 2.6).
	HostApplicationStatusApproved = "APPROVED"

	// HostApplicationStatusRejected marks an application refused by Super Admin (set in Task 2.6).
	HostApplicationStatusRejected = "REJECTED"

	// HostDocumentTypeKTP is a director identity card.
	HostDocumentTypeKTP = "KTP_DIREKSI"

	// HostDocumentTypeNPWP is a corporate tax registration.
	HostDocumentTypeNPWP = "NPWP_BADAN"

	// HostDocumentTypeNIB is an OSS business identification number.
	HostDocumentTypeNIB = "NIB_OSS"

	// HostDocumentTypeAKTA is a deed of establishment.
	HostDocumentTypeAKTA = "AKTA_PENDIRIAN"

	// HostDocumentReviewStatusPending marks an uploaded document awaiting review.
	HostDocumentReviewStatusPending = "PENDING"
)

// HostApplication represents one Host registration request in host_applications.
type HostApplication struct {
	ID                 uuid.UUID  `json:"id"`
	HostOrganizationID uuid.UUID  `json:"host_organization_id"`
	SubmittedBy        uuid.UUID  `json:"submitted_by"`
	Status             string     `json:"status"`
	ReviewedBy         *uuid.UUID `json:"reviewed_by,omitempty"`
	ReviewReasonCode   *string    `json:"review_reason_code,omitempty"`
	SubmittedAt        *time.Time `json:"submitted_at,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
}

// HostMemberSummary is the read model for GET /hosts/members.
type HostMemberSummary struct {
	MembershipID uuid.UUID  `json:"membership_id"`
	UserID       uuid.UUID  `json:"user_id"`
	DisplayName  string     `json:"display_name"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

// HostLegalDocument is metadata for a private legal document held in the master vault.
type HostLegalDocument struct {
	ID                uuid.UUID `json:"id"`
	HostApplicationID uuid.UUID `json:"host_application_id"`
	ObjectKey         string    `json:"object_key"`
	DocumentType      string    `json:"document_type"`
	MimeType          string    `json:"mime_type"`
	SizeBytes         int64     `json:"size_bytes"`
	ReviewStatus      string    `json:"review_status"`
	CreatedAt         time.Time `json:"created_at"`
}
