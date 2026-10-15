package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

// HostMembershipUsecase orchestrates Super Admin review and Host team membership operations.
type HostMembershipUsecase interface {
	ApproveApplication(ctx context.Context, reviewerID uuid.UUID, applicationID uuid.UUID) (*ReviewResult, error)
	RejectApplication(ctx context.Context, reviewerID uuid.UUID, applicationID uuid.UUID, reasonCode string) (*ReviewResult, error)
	GetOrganization(ctx context.Context, orgID uuid.UUID) (*OrganizationResult, error)
	UpdateOrganization(ctx context.Context, orgID uuid.UUID, req UpdateOrganizationRequest) error
	ListMembers(ctx context.Context, orgID uuid.UUID) ([]domain.HostMemberSummary, error)
	RemoveMember(ctx context.Context, orgID, membershipID, actorUserID uuid.UUID) error
}

// ReviewResult is the response payload for approve/reject.
type ReviewResult struct {
	ApplicationID string `json:"application_id"`
	Status        string `json:"status"`
	OrgStatus     string `json:"organization_status"`
}

// OrganizationResult is the response payload for GET /hosts/organization.
type OrganizationResult struct {
	ID                 string `json:"id"`
	LegalName          string `json:"legal_name"`
	DisplayName        string `json:"display_name"`
	BusinessField      string `json:"business_field"`
	VerificationStatus string `json:"verification_status"`
	MaxAdminCount      int    `json:"max_admin_count"`
	CurrentAdminCount  int    `json:"current_admin_count"`
}

// UpdateOrganizationRequest carries the mutable organization fields.
type UpdateOrganizationRequest struct {
	LegalName     string
	DisplayName   string
	BusinessField string
}

type hostMembershipUsecase struct {
	repo    repository.HostRepository
	auditor audit.Logger
}

// NewHostMembershipUsecase constructs the Host membership usecase.
// auditor may be nil, in which case mutations proceed without audit records.
func NewHostMembershipUsecase(repo repository.HostRepository, auditor audit.Logger) HostMembershipUsecase {
	return &hostMembershipUsecase{repo: repo, auditor: auditor}
}

// recordAudit writes one audit entry when a logger is configured. Audit failures never
// block the mutation: the state change already happened and is the more important record.
func (u *hostMembershipUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

func (u *hostMembershipUsecase) ApproveApplication(ctx context.Context, reviewerID, applicationID uuid.UUID) (*ReviewResult, error) {
	app, org, err := u.repo.ApproveApplication(ctx, applicationID, reviewerID)
	if err != nil {
		return nil, err
	}
	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &reviewerID,
		ActionCode:  "HOST_APPLICATION_APPROVED",
		TargetType:  "host_application",
		TargetID:    &app.ID,
		Metadata:    map[string]interface{}{"organization_id": org.ID.String(), "applicant_user_id": app.SubmittedBy.String()},
	})
	return &ReviewResult{
		ApplicationID: app.ID.String(),
		Status:        app.Status,
		OrgStatus:     org.VerificationStatus,
	}, nil
}

func (u *hostMembershipUsecase) RejectApplication(ctx context.Context, reviewerID, applicationID uuid.UUID, reasonCode string) (*ReviewResult, error) {
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		return nil, fmt.Errorf("%w: reason_code is required", ErrInvalidInput)
	}
	app, org, err := u.repo.RejectApplication(ctx, applicationID, reviewerID, reasonCode)
	if err != nil {
		return nil, err
	}
	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &reviewerID,
		ActionCode:  "HOST_APPLICATION_REJECTED",
		TargetType:  "host_application",
		TargetID:    &app.ID,
		ReasonCode:  &reasonCode,
		Metadata:    map[string]interface{}{"organization_id": org.ID.String()},
	})
	return &ReviewResult{
		ApplicationID: app.ID.String(),
		Status:        app.Status,
		OrgStatus:     org.VerificationStatus,
	}, nil
}

func (u *hostMembershipUsecase) GetOrganization(ctx context.Context, orgID uuid.UUID) (*OrganizationResult, error) {
	org, err := u.repo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	quota, err := u.repo.GetAdminQuota(ctx, orgID)
	if err != nil {
		return nil, err
	}
	current, err := u.repo.CountActiveAdmins(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &OrganizationResult{
		ID:                 org.ID.String(),
		LegalName:          org.LegalName,
		DisplayName:        org.DisplayName,
		BusinessField:      org.BusinessField,
		VerificationStatus: org.VerificationStatus,
		MaxAdminCount:      quota.MaxAdmins,
		CurrentAdminCount:  current,
	}, nil
}

func (u *hostMembershipUsecase) UpdateOrganization(ctx context.Context, orgID uuid.UUID, req UpdateOrganizationRequest) error {
	if strings.TrimSpace(req.LegalName) == "" && strings.TrimSpace(req.DisplayName) == "" && strings.TrimSpace(req.BusinessField) == "" {
		return fmt.Errorf("%w: at least one mutable field is required", ErrInvalidInput)
	}
	return u.repo.UpdateOrganizationDetails(ctx, orgID, strings.TrimSpace(req.LegalName), strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.BusinessField))
}

func (u *hostMembershipUsecase) ListMembers(ctx context.Context, orgID uuid.UUID) ([]domain.HostMemberSummary, error) {
	return u.repo.ListMembers(ctx, orgID)
}

func (u *hostMembershipUsecase) RemoveMember(ctx context.Context, orgID, membershipID, actorUserID uuid.UUID) error {
	hm, err := u.repo.GetMembershipByID(ctx, membershipID)
	if err != nil {
		return err
	}
	if hm.HostOrganizationID != orgID {
		return ErrForbiddenResource
	}
	if hm.UserID == actorUserID {
		return fmt.Errorf("%w: a leader cannot remove their own membership", ErrInvalidInput)
	}
	if hm.MembershipRole == domain.HostRoleLeader {
		return repository.ErrLastLeaderRemoval
	}
	if hm.Status == domain.HostMembershipStatusRevoked {
		return nil
	}
	if err := u.repo.RevokeMembership(ctx, membershipID); err != nil {
		return err
	}
	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "HOST_MEMBERSHIP_REVOKED",
		TargetType:  "host_membership",
		TargetID:    &membershipID,
		Metadata: map[string]interface{}{
			"organization_id": orgID.String(),
			"removed_user_id": hm.UserID.String(),
			"removed_role":    hm.MembershipRole,
		},
	})
	return nil
}
