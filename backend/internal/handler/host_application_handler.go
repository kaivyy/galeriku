package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// HostApplicationHandler serves the Task 2.5 Host B2B application endpoints.
type HostApplicationHandler struct {
	usecase usecase.HostApplicationUsecase
}

// NewHostApplicationHandler constructs the Host application handler.
func NewHostApplicationHandler(uc usecase.HostApplicationUsecase) *HostApplicationHandler {
	return &HostApplicationHandler{usecase: uc}
}

func (h *HostApplicationHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
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

func (h *HostApplicationHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
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

func (h *HostApplicationHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrActiveApplicationExists):
		h.writeError(w, r, http.StatusConflict, "APPLICATION_ALREADY_EXISTS", err.Error())
	case errors.Is(err, repository.ErrApplicationNotFound), errors.Is(err, repository.ErrDocumentNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

// Apply handles POST /api/v1/hosts/apply.
func (h *HostApplicationHandler) Apply(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var body struct {
		LegalName     string `json:"legal_name"`
		DisplayName   string `json:"display_name"`
		BusinessField string `json:"business_field"`
		AddressText   string `json:"address_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.Apply(r.Context(), identity.UserID, usecase.ApplyHostRequest{
		LegalName:     body.LegalName,
		DisplayName:   body.DisplayName,
		BusinessField: body.BusinessField,
		AddressText:   body.AddressText,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, result)
}

// MyApplications handles GET /api/v1/hosts/applications/my.
func (h *HostApplicationHandler) MyApplications(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	result, err := h.usecase.GetMyApplication(r.Context(), identity.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// RequestDocumentUpload handles POST /api/v1/hosts/documents.
func (h *HostApplicationHandler) RequestDocumentUpload(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var body struct {
		ApplicationID string `json:"application_id"`
		DocumentType  string `json:"document_type"`
		FileName      string `json:"file_name"`
		SizeBytes     int64  `json:"size_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.RequestDocumentUpload(r.Context(), identity.UserID, usecase.DocumentUploadRequest{
		ApplicationID: body.ApplicationID,
		DocumentType:  body.DocumentType,
		FileName:      body.FileName,
		SizeBytes:     body.SizeBytes,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, result)
}

// ViewDocument handles GET /api/v1/hosts/documents/{id}/view.
func (h *HostApplicationHandler) ViewDocument(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	documentID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "document id must be a valid UUID")
		return
	}

	result, err := h.usecase.GetDocumentViewURL(r.Context(), identity.UserID, identity.Capabilities, documentID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}
