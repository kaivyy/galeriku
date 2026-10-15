package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// InvitationHandler serves the Task 2.8 Host team invitation endpoints.
type InvitationHandler struct {
	usecase usecase.InvitationUsecase
}

// NewInvitationHandler constructs the Host invitation handler.
func NewInvitationHandler(uc usecase.InvitationUsecase) *InvitationHandler {
	return &InvitationHandler{usecase: uc}
}

func (h *InvitationHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
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

func (h *InvitationHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
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

func (h *InvitationHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrInvitationExpired):
		h.writeError(w, r, http.StatusGone, "INVITATION_EXPIRED", err.Error())
	case errors.Is(err, repository.ErrAdminLimitExceeded):
		h.writeError(w, r, http.StatusUnprocessableEntity, "ADMIN_LIMIT_EXCEEDED", err.Error())
	case errors.Is(err, repository.ErrInvitationConsumed):
		h.writeError(w, r, http.StatusConflict, "INVITATION_CONSUMED", err.Error())
	case errors.Is(err, repository.ErrInvitationNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrMembershipAlreadyExists):
		h.writeError(w, r, http.StatusConflict, "MEMBERSHIP_ALREADY_EXISTS", err.Error())
	case errors.Is(err, repository.ErrInviteeMismatch):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: invitation was issued to a different recipient")
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

// CreateInvitation handles POST /api/v1/hosts/invitations for active Leaders.
func (h *InvitationHandler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}

	var body struct {
		InvitedEmail   string `json:"invited_email"`
		MembershipRole string `json:"membership_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.CreateInvitation(r.Context(), tc.UserID, tc.OrganizationID, usecase.CreateInvitationRequest{
		InvitedEmail:   body.InvitedEmail,
		MembershipRole: body.MembershipRole,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, result)
}

// AcceptInvitation handles POST /api/v1/hosts/invitations/accept for any authenticated user.
func (h *InvitationHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var body struct {
		InvitationToken string `json:"invitation_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.AcceptInvitation(r.Context(), identity.UserID, identity.Email, body.InvitationToken)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// RevokeInvitation handles DELETE /api/v1/hosts/invitations/{id} for active Leaders.
func (h *InvitationHandler) RevokeInvitation(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}

	invitationID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "invitation id must be a valid UUID")
		return
	}

	if err := h.usecase.RevokeInvitation(r.Context(), tc.OrganizationID, invitationID); err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"revoked": true})
}
