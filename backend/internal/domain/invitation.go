package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// InvitationStatusPending marks an invitation awaiting acceptance.
	InvitationStatusPending = "PENDING"

	// InvitationStatusAccepted marks a consumed invitation.
	InvitationStatusAccepted = "ACCEPTED"

	// InvitationStatusRevoked marks an invitation cancelled by a Leader.
	InvitationStatusRevoked = "REVOKED"

	// InvitationStatusExpired marks an invitation past its expiry.
	InvitationStatusExpired = "EXPIRED"

	// InvitationTTL is how long an invitation stays valid.
	InvitationTTL = 72 * time.Hour
)

// HostInvitation is a time-bound team invitation. Only the token hash is persisted.
type HostInvitation struct {
	ID                 uuid.UUID  `json:"id"`
	HostOrganizationID uuid.UUID  `json:"host_organization_id"`
	InvitedBy          uuid.UUID  `json:"invited_by"`
	InviteeUserID      *uuid.UUID `json:"invitee_user_id,omitempty"`
	InviteeEmail       *string    `json:"invitee_email,omitempty"`
	Role               string     `json:"role"`
	TokenHash          string     `json:"-"`
	Status             string     `json:"status"`
	ExpiresAt          time.Time  `json:"expires_at"`
}
