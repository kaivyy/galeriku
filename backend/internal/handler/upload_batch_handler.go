package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/usecase"
)

// UploadBatchHandler serves the Phase 3 storage quota and mass upload reservation endpoints.
// Both routes require an authenticated active Host member; the organization is taken from the
// verified TenantContext so a caller can never reserve against another tenant's account.
type UploadBatchHandler struct {
	usecase usecase.UploadBatchUsecase
}

// NewUploadBatchHandler constructs the upload batch handler.
func NewUploadBatchHandler(uc usecase.UploadBatchUsecase) *UploadBatchHandler {
	return &UploadBatchHandler{usecase: uc}
}

func (h *UploadBatchHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
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

func (h *UploadBatchHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
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

func (h *UploadBatchHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrQuotaExceeded):
		h.writeError(w, r, http.StatusUnprocessableEntity, "QUOTA_EXCEEDED", err.Error())
	case errors.Is(err, repository.ErrStorageAccountNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

// GetQuota handles GET /api/v1/storage/quota.
func (h *UploadBatchHandler) GetQuota(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}
	quota, err := h.usecase.GetQuota(r.Context(), tc.OrganizationID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, quota)
}

// InitiateBatch handles POST /api/v1/storage/upload-batches/initiate.
func (h *UploadBatchHandler) InitiateBatch(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}

	var body struct {
		EventID    string `json:"event_id"`
		TotalFiles int    `json:"total_files"`
		TotalBytes int64  `json:"total_bytes"`
		Files      []struct {
			ClientFileToken  string `json:"client_file_token"`
			OriginalFilename string `json:"original_filename"`
			ExpectedBytes    int64  `json:"expected_bytes"`
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	req := usecase.InitiateBatchRequest{
		TotalFiles: body.TotalFiles,
		TotalBytes: body.TotalBytes,
		Files:      make([]domain.UploadBatchRequest, 0, len(body.Files)),
	}
	for _, f := range body.Files {
		req.Files = append(req.Files, domain.UploadBatchRequest{
			ClientFileToken:  f.ClientFileToken,
			OriginalFilename: f.OriginalFilename,
			ExpectedBytes:    f.ExpectedBytes,
		})
	}

	result, err := h.usecase.InitiateBatch(r.Context(), tc.UserID, tc.OrganizationID, req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, result)
}
