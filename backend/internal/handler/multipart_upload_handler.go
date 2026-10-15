package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
)

// MultipartUploadHandler serves the Phase 3 direct-to-S3 multipart upload endpoints.
// Every route requires an active Host member, and the item's batch must belong to the
// caller's organization storage account before any presigned URL is issued.
type MultipartUploadHandler struct {
	usecase usecase.MultipartUploadUsecase
}

// NewMultipartUploadHandler constructs the multipart upload handler.
func NewMultipartUploadHandler(uc usecase.MultipartUploadUsecase) *MultipartUploadHandler {
	return &MultipartUploadHandler{usecase: uc}
}

func (h *MultipartUploadHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
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

func (h *MultipartUploadHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
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

func (h *MultipartUploadHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrMultipartSessionExists):
		h.writeError(w, r, http.StatusConflict, "MULTIPART_SESSION_EXISTS", err.Error())
	case errors.Is(err, repository.ErrMultipartSessionNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrUploadItemNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrUploadBatchNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

func (h *MultipartUploadHandler) tenantOf(w http.ResponseWriter, r *http.Request) (*middleware.TenantContext, bool) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return nil, false
	}
	return tc, true
}

// Initiate handles POST /api/v1/storage/multipart/initiate.
func (h *MultipartUploadHandler) Initiate(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}

	var body struct {
		UploadBatchID   string `json:"upload_batch_id"`
		ClientFileToken string `json:"client_file_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.InitiateMultipart(r.Context(), tc.UserID, tc.OrganizationID, usecase.InitiateMultipartRequest{
		UploadBatchID:   body.UploadBatchID,
		ClientFileToken: body.ClientFileToken,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// SignPart handles POST /api/v1/storage/multipart/sign-part.
func (h *MultipartUploadHandler) SignPart(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}

	var body struct {
		UploadItemID string `json:"upload_item_id"`
		PartNumber   int    `json:"part_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.SignPart(r.Context(), tc.OrganizationID, usecase.SignPartRequest{
		UploadItemID: body.UploadItemID,
		PartNumber:   body.PartNumber,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// Complete handles POST /api/v1/storage/multipart/complete.
func (h *MultipartUploadHandler) Complete(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}

	var body struct {
		UploadItemID string `json:"upload_item_id"`
		Parts        []struct {
			PartNumber int    `json:"part_number"`
			ETag       string `json:"etag"`
		} `json:"parts"`
		SHA256Checksum string `json:"sha256_checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	parts := make([]storage.MultipartPart, 0, len(body.Parts))
	for _, p := range body.Parts {
		parts = append(parts, storage.MultipartPart{PartNumber: p.PartNumber, ETag: strings.TrimSpace(p.ETag)})
	}

	result, err := h.usecase.CompleteMultipart(r.Context(), tc.UserID, tc.OrganizationID, usecase.CompleteMultipartRequest{
		UploadItemID:   body.UploadItemID,
		Parts:          parts,
		SHA256Checksum: body.SHA256Checksum,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}
