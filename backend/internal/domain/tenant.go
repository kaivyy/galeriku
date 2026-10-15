package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// HostRoleLeader has full legal, financial, and operational authority over the Host Organization.
	// Exactly one active Leader is permitted per organization (invariant OPEN-10).
	HostRoleLeader = "LEADER"

	// HostRoleAdmin has delegated operational authority (events, FotoTree sessions, photographer invites).
	// Strictly forbidden from viewing bank accounts, balances, or requesting payouts.
	HostRoleAdmin = "ADMIN"

	// HostMembershipStatusActive denotes an active membership in the organization.
	HostMembershipStatusActive = "ACTIVE"

	// HostMembershipStatusInactive denotes an inactive or suspended membership.
	HostMembershipStatusInactive = "INACTIVE"

	// HostMembershipStatusRevoked denotes an explicitly revoked membership.
	HostMembershipStatusRevoked = "REVOKED"
)

// HostOrganization represents a registered B2B Host entity in host_organizations.
type HostOrganization struct {
	ID                 uuid.UUID `json:"id"`
	LegalName          string    `json:"legal_name"`
	DisplayName        string    `json:"display_name"`
	BusinessField      string    `json:"business_field"`
	VerificationStatus string    `json:"verification_status"`
	CreatedAt          time.Time `json:"created_at"`
}

// HostMembership represents a user's role and status within a Host Organization.
type HostMembership struct {
	ID                 uuid.UUID  `json:"id"`
	HostOrganizationID uuid.UUID  `json:"host_organization_id"`
	UserID             uuid.UUID  `json:"user_id"`
	MembershipRole     string     `json:"membership_role"`
	Status             string     `json:"status"`
	InvitedBy          *uuid.UUID `json:"invited_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
}

// EventSummary represents tenant ownership metadata for an event.
type EventSummary struct {
	ID                 uuid.UUID  `json:"id"`
	HostOrganizationID uuid.UUID  `json:"host_organization_id"`
	Slug               string     `json:"slug"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	CreatedBy          uuid.UUID  `json:"created_by"`
	StartsAt           *time.Time `json:"starts_at,omitempty"`
	EndsAt             *time.Time `json:"ends_at,omitempty"`
}
