package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// HostMembershipHandler serves the Task 2.6 organization and membership endpoints.
type HostMembershipHandler struct {
	usecase usecase.HostMembershipUsecase
}

// NewHostMembershipHandler constructs the Host membership handler.
func NewHostMembershipHandler(uc usecase.HostMembershipUsecase) *HostMembershipHandler {
	return &HostMembershipHandler{usecase: uc}
}

func (h *HostMembershipHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	reqID := middleware.GetRequestID(r.Context())
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{
		Success: true,
		Data:    data,
		Meta: SuccessMeta{
			Timestamp: nowRFC3339(),
			RequestID: reqID,
		},
	})
}

func (h *HostMembershipHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	reqID := middleware.GetRequestID(r.Context())
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Success: false,
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			RequestID: reqID,
		},
		Meta: ErrorMeta{
			Timestamp: nowRFC3339(),
			RequestID: reqID,
		},
	})
}

func (h *HostMembershipHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrApplicationNotFound),
		errors.Is(err, repository.ErrOrganizationNotFound),
		errors.Is(err, repository.ErrMembershipNotFoundInOrg):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrLastLeaderRemoval):
		h.writeError(w, r, http.StatusConflict, "LAST_LEADER_PROTECTED", err.Error())
	case errors.Is(err, repository.ErrApplicationNotPending):
		h.writeError(w, r, http.StatusConflict, "APPLICATION_ALREADY_REVIEWED", err.Error())
	case errors.Is(err, repository.ErrMembershipAlreadyExists):
		h.writeError(w, r, http.StatusConflict, "MEMBERSHIP_ALREADY_EXISTS", err.Error())
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

// GetOrganization handles GET /api/v1/hosts/organization for any active member.
func (h *HostMembershipHandler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}
	result, err := h.usecase.GetOrganization(r.Context(), tc.OrganizationID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// UpdateOrganization handles PATCH /api/v1/hosts/organization for active Leaders only.
func (h *HostMembershipHandler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}
	if !tc.IsLeader {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "insufficient host membership role: operation restricted to "+domain.HostRoleLeader)
		return
	}

	var body struct {
		LegalName     string `json:"legal_name"`
		DisplayName   string `json:"display_name"`
		BusinessField string `json:"business_field"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	err := h.usecase.UpdateOrganization(r.Context(), tc.OrganizationID, usecase.UpdateOrganizationRequest{
		LegalName:     body.LegalName,
		DisplayName:   body.DisplayName,
		BusinessField: body.BusinessField,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"updated": true})
}

// ListMembers handles GET /api/v1/hosts/members for Leaders and Admins.
func (h *HostMembershipHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}
	members, err := h.usecase.ListMembers(r.Context(), tc.OrganizationID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"members": members})
}

// RemoveMember handles DELETE /api/v1/hosts/members/{id} for active Leaders only.
func (h *HostMembershipHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}
	if !tc.IsLeader {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "insufficient host membership role: operation restricted to "+domain.HostRoleLeader)
		return
	}

	membershipID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "membership id must be a valid UUID")
		return
	}

	if err := h.usecase.RemoveMember(r.Context(), tc.OrganizationID, membershipID, tc.UserID); err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"removed": true})
}

// ApproveApplication handles POST /api/v1/admin/hosts/applications/{id}/approve for Platform Admins.
func (h *HostMembershipHandler) ApproveApplication(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	applicationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "application id must be a valid UUID")
		return
	}
	result, err := h.usecase.ApproveApplication(r.Context(), identity.UserID, applicationID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// RejectApplication handles POST /api/v1/admin/hosts/applications/{id}/reject for Platform Admins.
func (h *HostMembershipHandler) RejectApplication(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	applicationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "application id must be a valid UUID")
		return
	}

	var body struct {
		ReasonCode string `json:"reason_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.RejectApplication(r.Context(), identity.UserID, applicationID, body.ReasonCode)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}
