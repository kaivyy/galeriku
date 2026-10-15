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

// EventTreeHandler serves the Phase 3 FotoTree hierarchy endpoints.
// Every route is mounted behind Authenticate plus a Host membership middleware, so the
// organization always comes from the verified TenantContext and never from the request body.
type EventTreeHandler struct {
	usecase usecase.EventTreeUsecase
}

// NewEventTreeHandler constructs the FotoTree handler.
func NewEventTreeHandler(uc usecase.EventTreeUsecase) *EventTreeHandler {
	return &EventTreeHandler{usecase: uc}
}

func (h *EventTreeHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
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

func (h *EventTreeHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
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

func (h *EventTreeHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrEventSlugExists):
		h.writeError(w, r, http.StatusConflict, "SLUG_ALREADY_EXISTS", err.Error())
	case errors.Is(err, repository.ErrEventNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

// tenantOf extracts the verified tenant context, writing a 403 when it is absent.
func (h *EventTreeHandler) tenantOf(w http.ResponseWriter, r *http.Request) (*middleware.TenantContext, bool) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return nil, false
	}
	return tc, true
}

// parseOptionalTime accepts an RFC 3339 string and reports a validation error otherwise.
func parseOptionalTime(raw string, field string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, errors.New(field + " must be an RFC 3339 timestamp")
	}
	return &parsed, nil
}

// CreateEvent handles POST /api/v1/events for active Leaders and Admins.
func (h *EventTreeHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}

	var body struct {
		Name      string `json:"name"`
		Slug      string `json:"slug"`
		EventType string `json:"event_type"`
		StartsAt  string `json:"starts_at"`
		EndsAt    string `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	startsAt, err := parseOptionalTime(body.StartsAt, "starts_at")
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	endsAt, err := parseOptionalTime(body.EndsAt, "ends_at")
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	event, err := h.usecase.CreateEvent(r.Context(), tc.UserID, tc.OrganizationID, usecase.CreateEventRequest{
		Name:      body.Name,
		Slug:      body.Slug,
		EventType: body.EventType,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, event)
}

// ListEvents handles GET /api/v1/events for active Leaders and Admins.
func (h *EventTreeHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}
	events, err := h.usecase.ListEvents(r.Context(), tc.OrganizationID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"events": events})
}

// GetEventTree handles GET /api/v1/events/{id} and returns the full FotoTree.
func (h *EventTreeHandler) GetEventTree(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}
	eventID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "event id must be a valid UUID")
		return
	}

	tree, err := h.usecase.GetEventTree(r.Context(), eventID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if tree.Event.HostOrganizationID != tc.OrganizationID {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: event belongs to another organization")
		return
	}
	h.writeSuccess(w, r, http.StatusOK, tree)
}

// UpdateEvent handles PUT /api/v1/events/{id} for active Leaders and Admins.
func (h *EventTreeHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}
	eventID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "event id must be a valid UUID")
		return
	}

	var body struct {
		Name     string `json:"name"`
		Status   string `json:"status"`
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	startsAt, err := parseOptionalTime(body.StartsAt, "starts_at")
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	endsAt, err := parseOptionalTime(body.EndsAt, "ends_at")
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	event, err := h.usecase.UpdateEvent(r.Context(), tc.UserID, eventID, usecase.UpdateEventRequest{
		Name:     body.Name,
		Status:   body.Status,
		StartsAt: startsAt,
		EndsAt:   endsAt,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if event.HostOrganizationID != tc.OrganizationID {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: event belongs to another organization")
		return
	}
	h.writeSuccess(w, r, http.StatusOK, event)
}

// CreateSession handles POST /api/v1/events/{id}/sessions for active Leaders and Admins.
func (h *EventTreeHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}
	eventID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "event id must be a valid UUID")
		return
	}

	event, err := h.usecase.GetEventTree(r.Context(), eventID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if event.Event.HostOrganizationID != tc.OrganizationID {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: event belongs to another organization")
		return
	}

	var body struct {
		Name        string `json:"name"`
		SessionType string `json:"session_type"`
		StartsAt    string `json:"starts_at"`
		EndsAt      string `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	startsAt, err := parseOptionalTime(body.StartsAt, "starts_at")
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	endsAt, err := parseOptionalTime(body.EndsAt, "ends_at")
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}

	session, err := h.usecase.CreateSession(r.Context(), tc.UserID, eventID, usecase.CreateSessionRequest{
		Name:        body.Name,
		SessionType: body.SessionType,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, session)
}

// CreateLocation handles POST /api/v1/events/{id}/locations for active Leaders and Admins.
func (h *EventTreeHandler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	tc, ok := h.tenantOf(w, r)
	if !ok {
		return
	}
	eventID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "event id must be a valid UUID")
		return
	}

	event, err := h.usecase.GetEventTree(r.Context(), eventID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	if event.Event.HostOrganizationID != tc.OrganizationID {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: event belongs to another organization")
		return
	}

	var body struct {
		Name       string   `json:"name"`
		Latitude   *float64 `json:"latitude"`
		Longitude  *float64 `json:"longitude"`
		RouteOrder *int     `json:"route_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	if body.Latitude == nil || body.Longitude == nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "latitude and longitude are required")
		return
	}

	location, err := h.usecase.CreateLocation(r.Context(), tc.UserID, eventID, usecase.CreateLocationRequest{
		Name:       body.Name,
		Latitude:   *body.Latitude,
		Longitude:  *body.Longitude,
		RouteOrder: body.RouteOrder,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, location)
}
