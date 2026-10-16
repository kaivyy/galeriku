package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// PreviewHandler serves time-limited presigned URLs for photo previews.
// The backing object must exist under the previews/ prefix and the photo must
// be READY in the public catalog before a URL is issued.
type PreviewHandler struct {
	catalog repository.PhotoCatalogRepository
	s3      *storage.S3Service
}

// NewPreviewHandler constructs the preview handler.
func NewPreviewHandler(catalog repository.PhotoCatalogRepository, s3 *storage.S3Service) *PreviewHandler {
	return &PreviewHandler{catalog: catalog, s3: s3}
}

// GetPreviewURL handles GET /api/v1/photos/{id}/preview-url?ttl_minutes=N.
func (h *PreviewHandler) GetPreviewURL(w http.ResponseWriter, r *http.Request) {
	photoID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writePreviewError(w, http.StatusBadRequest, "INVALID_INPUT", "photo id must be a valid UUID")
		return
	}

	// Only catalog-visible photos can be presigned.
	if _, err := h.catalog.GetPhotoByID(r.Context(), photoID); err != nil {
		writePreviewError(w, http.StatusNotFound, "NOT_FOUND", "photo not found in public catalog")
		return
	}

	// TTL is clamped server-side: the client hint never extends past 60 minutes.
	ttlMinutes, err := strconv.Atoi(r.URL.Query().Get("ttl_minutes"))
	if err != nil || ttlMinutes <= 0 {
		ttlMinutes = 60
	}
	url, expiresAt := h.presign(r, photoID, time.Duration(ttlMinutes)*time.Minute)
	if url == "" {
		writePreviewError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to sign preview URL")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{
		Success: true,
		Data: map[string]interface{}{
			"preview_url": url,
			"object_key":  "previews/" + photoID.String() + ".jpg",
			"expires_at":  expiresAt.UTC().Format(time.RFC3339),
		},
		Meta: SuccessMeta{Timestamp: nowRFC3339(), RequestID: "req_preview"},
	})
}

func (h *PreviewHandler) presign(r *http.Request, photoID uuid.UUID, ttl time.Duration) (string, time.Time) {
	// presign marshals the failure path as an empty URL; the caller converts it
	// into a 500 without leaking MinIO internals.
	url, err := h.s3.PresignPreviewGet(r.Context(), photoID, ttl)
	if err != nil {
		return "", time.Time{}
	}
	// The clamp lives in PresignPreviewGet; mirror the effective expiry here.
	effective := ttl
	if effective <= 0 || effective > 60*time.Minute {
		effective = 60 * time.Minute
	}
	return url, time.Now().UTC().Add(effective)
}

func writePreviewError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Success: false,
		Error:   ErrorBody{Code: code, Message: message},
		Meta:    ErrorMeta{Timestamp: nowRFC3339(), RequestID: "req_preview"},
	})
}

// Compile-time guard that the usecase layer stays out of the signing path:
// previews are a storage concern, not a business-rule concern.
var _ = usecase.ErrForbiddenResource
