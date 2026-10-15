package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// DiscoveryHandler serves the Phase 5 discovery, cart, checkout verification, and order endpoints.
type DiscoveryHandler struct {
	usecase usecase.DiscoveryUsecase
}

// NewDiscoveryHandler constructs the discovery handler.
func NewDiscoveryHandler(uc usecase.DiscoveryUsecase) *DiscoveryHandler {
	return &DiscoveryHandler{usecase: uc}
}

func (h *DiscoveryHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	reqID := middleware.GetRequestID(r.Context())
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{Success: true, Data: data, Meta: SuccessMeta{Timestamp: nowRFC3339(), RequestID: reqID}})
}

func (h *DiscoveryHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	reqID := middleware.GetRequestID(r.Context())
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Success: false,
		Error: ErrorBody{Code: code, Message: message, RequestID: reqID},
		Meta: ErrorMeta{Timestamp: nowRFC3339(), RequestID: reqID},
	})
}

func (h *DiscoveryHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrVerificationTooManyAttempts):
		h.writeError(w, r, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", err.Error())
	case errors.Is(err, repository.ErrVerificationExpired):
		h.writeError(w, r, http.StatusGone, "VERIFICATION_EXPIRED", err.Error())
	case errors.Is(err, repository.ErrGrantNotFound):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", err.Error())
	case errors.Is(err, repository.ErrIdempotencyKeyConflict):
		h.writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", err.Error())
	case errors.Is(err, repository.ErrPriceNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrOrderNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, security.ErrCheckoutOTPInvalid):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_VERIFICATION_CODE", err.Error())
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

// SearchBIB handles POST /api/v1/search/bib. Public endpoint, no authorization.
func (h *DiscoveryHandler) SearchBIB(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BIBNumber string `json:"bib_number"`
		EventID   string `json:"event_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	var eventID *uuid.UUID
	if trimmed := strings.TrimSpace(body.EventID); trimmed != "" {
		id, err := uuid.Parse(trimmed)
		if err != nil {
			h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "event_id must be a valid UUID")
			return
		}
		eventID = &id
	}
	result, err := h.usecase.SearchBIB(r.Context(), body.BIBNumber, eventID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// GetCart handles GET /api/v1/cart.
func (h *DiscoveryHandler) GetCart(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	result, err := h.usecase.ListCart(r.Context(), identity.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// AddCartItem handles POST /api/v1/cart/items.
func (h *DiscoveryHandler) AddCartItem(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	var body struct {
		PhotoID string `json:"photo_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	photoID, err := uuid.Parse(strings.TrimSpace(body.PhotoID))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "photo_id must be a valid UUID")
		return
	}
	item, err := h.usecase.AddToCart(r.Context(), identity.UserID, photoID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, map[string]interface{}{
		"cart_item_id": item.ID.String(),
		"photo_id":     item.PhotoID.String(),
		"price_idr":    item.PriceIDR,
	})
}

// RemoveCartItem handles DELETE /api/v1/cart/items/{id}.
func (h *DiscoveryHandler) RemoveCartItem(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	itemID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "cart item id must be a valid UUID")
		return
	}
	if err := h.usecase.RemoveFromCart(r.Context(), identity.UserID, itemID); err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"deleted": true})
}

// InitiateVerification handles POST /api/v1/checkout/verify-bib/initiate.
func (h *DiscoveryHandler) InitiateVerification(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	var body struct {
		BIBNumber          string `json:"bib_number"`
		EventID            string `json:"event_id"`
		VerificationMethod string `json:"verification_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	var eventID *uuid.UUID
	if trimmed := strings.TrimSpace(body.EventID); trimmed != "" {
		id, err := uuid.Parse(trimmed)
		if err != nil {
			h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "event_id must be a valid UUID")
			return
		}
		eventID = &id
	}
	result, err := h.usecase.InitiateVerification(r.Context(), identity.UserID, usecase.InitiateVerificationRequest{
		BIBNumber:          body.BIBNumber,
		EventID:            eventID,
		VerificationMethod: body.VerificationMethod,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// ConfirmVerification handles POST /api/v1/checkout/verify-bib/confirm.
func (h *DiscoveryHandler) ConfirmVerification(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	var body struct {
		VerificationID string `json:"verification_id"`
		Code           string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	verificationID, err := uuid.Parse(strings.TrimSpace(body.VerificationID))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "verification_id must be a valid UUID")
		return
	}
	result, err := h.usecase.ConfirmVerification(r.Context(), verificationID, body.Code)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// CreateOrder handles POST /api/v1/orders.
func (h *DiscoveryHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	var body struct {
		PhotoIDs []string `json:"photo_ids"`
		BIBGrants []string `json:"bib_grants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	photoIDs := make([]uuid.UUID, 0, len(body.PhotoIDs))
	for _, raw := range body.PhotoIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "each photo_id must be a valid UUID")
			return
		}
		photoIDs = append(photoIDs, id)
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	result, err := h.usecase.CreateOrder(r.Context(), identity.UserID, usecase.CreateOrderRequest{
		PhotoIDs:  photoIDs,
		BIBGrants: body.BIBGrants,
	}, idempotencyKey)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, result)
}

// GetOrder handles GET /api/v1/orders/{id}.
func (h *DiscoveryHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	orderID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "order id must be a valid UUID")
		return
	}
	result, err := h.usecase.GetOrder(r.Context(), identity.UserID, orderID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}
