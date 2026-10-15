package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/usecase"
)

// BankAccountHandler serves the Task 2.7 Host payout account endpoints.
// Both routes are mounted behind RequireHostLeader, so an ADMIN never reaches these methods.
type BankAccountHandler struct {
	usecase usecase.BankAccountUsecase
}

// NewBankAccountHandler constructs the Host bank account handler.
func NewBankAccountHandler(uc usecase.BankAccountUsecase) *BankAccountHandler {
	return &BankAccountHandler{usecase: uc}
}

func (h *BankAccountHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
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

func (h *BankAccountHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
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

func (h *BankAccountHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, usecase.ErrInvalidInput) {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
}

// ListAccounts handles GET /api/v1/hosts/bank-accounts for active Leaders.
func (h *BankAccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}
	accounts, err := h.usecase.ListAccounts(r.Context(), tc.OrganizationID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, map[string]interface{}{"bank_accounts": accounts})
}

// RegisterAccount handles POST /api/v1/hosts/bank-accounts for active Leaders.
func (h *BankAccountHandler) RegisterAccount(w http.ResponseWriter, r *http.Request) {
	tc, ok := middleware.GetTenantContext(r.Context())
	if !ok || tc == nil {
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: verified tenant context required")
		return
	}

	var body struct {
		BankCode          string `json:"bank_code"`
		AccountNumber     string `json:"account_number"`
		AccountHolderName string `json:"account_holder_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	account, err := h.usecase.RegisterAccount(r.Context(), tc.UserID, tc.OrganizationID, usecase.RegisterBankAccountRequest{
		BankCode:          body.BankCode,
		AccountNumber:     body.AccountNumber,
		AccountHolderName: body.AccountHolderName,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	h.writeSuccess(w, r, http.StatusCreated, map[string]interface{}{
		"id":                    account.ID,
		"bank_code":             account.BankCode,
		"account_number_masked": account.AccountNumberMasked,
		"account_holder_name":   account.AccountHolderName,
		"verification_status":   account.VerificationStatus,
		"created_at":            account.CreatedAt,
	})
}
