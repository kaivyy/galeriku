package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// FinanceHandler serves the Phase 7 wallet and refund endpoints.
type FinanceHandler struct {
	usecase usecase.FinanceUsecase
}

// NewFinanceHandler constructs the finance handler.
func NewFinanceHandler(uc usecase.FinanceUsecase) *FinanceHandler {
	return &FinanceHandler{usecase: uc}
}

func (h *FinanceHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{Success: true, Data: data, Meta: SuccessMeta{Timestamp: nowRFC3339(), RequestID: "req_finance"}})
}

func (h *FinanceHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Success: false,
		Error:   ErrorBody{Code: code, Message: message},
		Meta:    ErrorMeta{Timestamp: nowRFC3339(), RequestID: "req_finance"},
	})
}

// GetWallet handles GET /api/v1/finance/wallet.
func (h *FinanceHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	balance, err := h.usecase.GetWalletBalance(r.Context(), domain.LedgerAccountOwnerBuyer, identity.UserID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to read wallet")
		return
	}
	h.writeJSON(w, http.StatusOK, balance)
}

type refundRequest struct {
	OrderItemID     string `json:"order_item_id"`
	ReasonCode      string `json:"reason_code"`
	DestinationType string `json:"destination_type"`
}

// RequestRefund handles POST /api/v1/refunds/request.
func (h *FinanceHandler) RequestRefund(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	var body refundRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	orderItemID, err := uuid.Parse(body.OrderItemID)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "order_item_id must be a valid UUID")
		return
	}
	if body.ReasonCode == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_INPUT", "reason_code is required")
		return
	}
	result, err := h.usecase.RequestRefund(r.Context(), identity.UserID, orderItemID, body.ReasonCode, body.DestinationType)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrOrderNotFound):
			h.writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		case errors.Is(err, repository.ErrLedgerImbalance):
			h.writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "ledger invariant violated")
		default:
			h.writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "refund failed")
		}
		return
	}
	h.writeJSON(w, http.StatusOK, result)
}

// ReleaseSettlements handles POST /api/v1/finance/settlements/release (admin/cron).
func (h *FinanceHandler) ReleaseSettlements(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	// Only SUPER_ADMIN may trigger this manually; the cron path calls the usecase directly.
	if !identity.HasCapability("SUPER_ADMIN") {
		h.writeError(w, http.StatusForbidden, "AUTH_FORBIDDEN", "super admin required")
		return
	}
	count, err := h.usecase.ReleaseDueSettlements(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "release failed")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]int{"released": count})
}

// parseLimit is a helper for future paginated finance endpoints.
func parseLimit(raw string, def int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
