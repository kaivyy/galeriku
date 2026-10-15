package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

// InvitationUsecase orchestrates Host team invitations and their acceptance.
type InvitationUsecase interface {
	CreateInvitation(ctx context.Context, actorUserID, orgID uuid.UUID, req CreateInvitationRequest) (*CreateInvitationResult, error)
	AcceptInvitation(ctx context.Context, userID uuid.UUID, userEmail, rawToken string) (*AcceptInvitationResult, error)
	RevokeInvitation(ctx context.Context, orgID, invitationID uuid.UUID) error
}

// CreateInvitationRequest carries the validated invitation input.
type CreateInvitationRequest struct {
	InvitedEmail   string
	MembershipRole string
}

// CreateInvitationResult returns the raw token exactly once, at issuance.
type CreateInvitationResult struct {
	InvitationID    string    `json:"invitation_id"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expires_at"`
	InvitationToken string    `json:"invitation_token"`
}

// AcceptInvitationResult confirms the new membership.
type AcceptInvitationResult struct {
	MembershipStatus string `json:"status"`
	Role             string `json:"role"`
	OrganizationID   string `json:"organization_id"`
}

type invitationUsecase struct {
	repo    repository.InvitationRepository
	auditor audit.Logger
}

// NewInvitationUsecase constructs the Host invitation usecase.
// auditor may be nil, in which case mutations proceed without audit records.
func NewInvitationUsecase(repo repository.InvitationRepository, auditor audit.Logger) InvitationUsecase {
	return &invitationUsecase{repo: repo, auditor: auditor}
}

// recordAudit writes one audit entry when a logger is configured. Audit failures never
// block the mutation: the state change already happened and is the more important record.
func (u *invitationUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

// HashInvitationToken returns the SHA-256 hex digest stored for one raw invitation token.
func HashInvitationToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func (u *invitationUsecase) CreateInvitation(ctx context.Context, actorUserID, orgID uuid.UUID, req CreateInvitationRequest) (*CreateInvitationResult, error) {
	req.InvitedEmail = strings.ToLower(strings.TrimSpace(req.InvitedEmail))
	if req.InvitedEmail == "" || !strings.Contains(req.InvitedEmail, "@") {
		return nil, fmt.Errorf("%w: a valid invited_email is required", ErrInvalidInput)
	}
	if req.MembershipRole == "" {
		req.MembershipRole = domain.HostRoleAdmin
	}
	if req.MembershipRole != domain.HostRoleAdmin {
		return nil, fmt.Errorf("%w: membership_role must be %s", ErrInvalidInput, domain.HostRoleAdmin)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("failed to generate invitation token: %w", err)
	}
	rawToken := "inv_" + base64.RawURLEncoding.EncodeToString(raw)

	email := req.InvitedEmail
	inv := &domain.HostInvitation{
		HostOrganizationID: orgID,
		InvitedBy:          actorUserID,
		InviteeEmail:       &email,
		Role:               req.MembershipRole,
		TokenHash:          HashInvitationToken(rawToken),
		Status:             domain.InvitationStatusPending,
		ExpiresAt:          time.Now().UTC().Add(domain.InvitationTTL),
	}
	if err := u.repo.CreateInvitation(ctx, inv); err != nil {
		return nil, err
	}

	return &CreateInvitationResult{
		InvitationID:    inv.ID.String(),
		Status:          inv.Status,
		ExpiresAt:       inv.ExpiresAt,
		InvitationToken: rawToken,
	}, nil
}

func (u *invitationUsecase) AcceptInvitation(ctx context.Context, userID uuid.UUID, userEmail, rawToken string) (*AcceptInvitationResult, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, fmt.Errorf("%w: invitation_token is required", ErrInvalidInput)
	}

	inv, err := u.repo.GetInvitationByTokenHash(ctx, HashInvitationToken(rawToken))
	if err != nil {
		return nil, err
	}
	if inv.InviteeEmail != nil && !strings.EqualFold(strings.TrimSpace(userEmail), strings.TrimSpace(*inv.InviteeEmail)) {
		return nil, repository.ErrInviteeMismatch
	}

	if err := u.repo.AcceptInvitation(ctx, inv.ID, userID); err != nil {
		return nil, err
	}
	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &userID,
		ActionCode:  "HOST_MEMBERSHIP_ROLE_GRANTED",
		TargetType:  "host_membership",
		TargetID:    &inv.ID,
		Metadata: map[string]interface{}{
			"organization_id": inv.HostOrganizationID.String(),
			"granted_role":    inv.Role,
			"source":          "invitation",
		},
	})
	return &AcceptInvitationResult{
		MembershipStatus: domain.HostMembershipStatusActive,
		Role:             inv.Role,
		OrganizationID:   inv.HostOrganizationID.String(),
	}, nil
}

func (u *invitationUsecase) RevokeInvitation(ctx context.Context, orgID, invitationID uuid.UUID) error {
	inv, err := u.repo.GetInvitationByID(ctx, invitationID)
	if err != nil {
		return err
	}
	if inv.HostOrganizationID != orgID {
		return ErrForbiddenResource
	}
	if err := u.repo.RevokeInvitation(ctx, invitationID); err != nil {
		return err
	}
	u.recordAudit(ctx, audit.Entry{
		ActionCode: "HOST_INVITATION_REVOKED",
		TargetType: "host_invitation",
		TargetID:   &invitationID,
		Metadata:   map[string]interface{}{"organization_id": orgID.String()},
	})
	return nil
}
