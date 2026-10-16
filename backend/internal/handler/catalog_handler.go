package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

// CatalogHandler serves the public photo catalog endpoints (W-01/W-02).
// No authentication: the storefront browses previews without an account.
type CatalogHandler struct {
	repo repository.PhotoCatalogRepository
}

// NewCatalogHandler constructs the catalog handler.
func NewCatalogHandler(repo repository.PhotoCatalogRepository) *CatalogHandler {
	return &CatalogHandler{repo: repo}
}

func (h *CatalogHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{
		Success: true,
		Data:    data,
		Meta:    SuccessMeta{Timestamp: nowRFC3339(), RequestID: "req_catalog"},
	})
}

// ListPhotos handles GET /api/v1/photos?event_id=&limit=.
func (h *CatalogHandler) ListPhotos(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var photos interface{}
	if raw := r.URL.Query().Get("event_id"); raw != "" {
		eventID, err := uuid.Parse(raw)
		if err != nil {
			h.writeJSON(w, http.StatusBadRequest, map[string]string{"message": "event_id must be a valid UUID"})
			return
		}
		results, err := h.repo.ListPhotosByEvent(r.Context(), eventID)
		if err != nil {
			h.writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list photos"})
			return
		}
		photos = results
	} else {
		results, err := h.repo.ListPublicPhotos(r.Context(), limit)
		if err != nil {
			h.writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list photos"})
			return
		}
		photos = results
	}
	h.writeJSON(w, http.StatusOK, map[string]interface{}{"photos": photos})
}

// GetPhoto handles GET /api/v1/photos/{id}.
func (h *CatalogHandler) GetPhoto(w http.ResponseWriter, r *http.Request) {
	photoID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, map[string]string{"message": "photo id must be a valid UUID"})
		return
	}
	photo, err := h.repo.GetPhotoByID(r.Context(), photoID)
	if err != nil {
		h.writeJSON(w, http.StatusNotFound, map[string]string{"message": "photo not found"})
		return
	}
	h.writeJSON(w, http.StatusOK, photo)
}
