package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// PaymentHandler serves the Phase 6 payment, webhook, download, and fulfillment endpoints.
type PaymentHandler struct {
	usecase usecase.PaymentUsecase
	s3      *storage.S3Service
}

// NewPaymentHandler constructs the payment handler.
func NewPaymentHandler(uc usecase.PaymentUsecase, s3 *storage.S3Service) *PaymentHandler {
	return &PaymentHandler{usecase: uc, s3: s3}
}

func (h *PaymentHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	reqID := middleware.GetRequestID(r.Context())
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{Success: true, Data: data, Meta: SuccessMeta{Timestamp: nowRFC3339(), RequestID: reqID}})
}

func (h *PaymentHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	reqID := middleware.GetRequestID(r.Context())
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Success: false,
		Error:   ErrorBody{Code: code, Message: message, RequestID: reqID},
		Meta:    ErrorMeta{Timestamp: nowRFC3339(), RequestID: reqID},
	})
}

func (h *PaymentHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrOrderNotPayable):
		h.writeError(w, r, http.StatusConflict, "ORDER_NOT_PAYABLE", err.Error())
	case errors.Is(err, repository.ErrDownloadNotPermitted):
		h.writeError(w, r, http.StatusForbidden, "DOWNLOAD_NOT_PERMITTED", err.Error())
	case errors.Is(err, repository.ErrDownloadGrantNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrFulfillmentNotFound):
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, repository.ErrFulfillmentAlreadyDone):
		h.writeError(w, r, http.StatusConflict, "FULFILLMENT_ALREADY_DONE", err.Error())
	case errors.Is(err, usecase.ErrForbiddenResource):
		h.writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not authorized for this resource")
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred")
	}
}

func (h *PaymentHandler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return false
	}
	return true
}

// InitiatePayment handles POST /api/v1/payments/initiate.
func (h *PaymentHandler) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	identity, _ := middleware.GetAuthIdentity(r.Context())

	var body struct {
		OrderID       string `json:"order_id"`
		PaymentMethod string `json:"payment_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}
	orderID, err := uuid.Parse(strings.TrimSpace(body.OrderID))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "order_id must be a valid UUID")
		return
	}

	result, err := h.usecase.InitiatePayment(r.Context(), identity.UserID, domain.PaymentInitiationRequest{
		OrderID:       orderID,
		PaymentMethod: body.PaymentMethod,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusCreated, result)
}

// Webhook handles POST /api/v1/webhooks/payments/midtrans. Public; signature-validated.
func (h *PaymentHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID           string `json:"order_id"`
		TransactionStatus string `json:"transaction_status"`
		GrossAmount       string `json:"gross_amount"`
		SignatureKey      string `json:"signature_key"`
		TransactionID     string `json:"transaction_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.HandleWebhook(r.Context(), domain.WebhookEvent{
		OrderID:           body.OrderID,
		TransactionStatus: body.TransactionStatus,
		GrossAmount:       body.GrossAmount,
		SignatureKey:      body.SignatureKey,
		TransactionID:     body.TransactionID,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// GrantDownload handles POST /api/v1/downloads/grant/{order_item_id}.
func (h *PaymentHandler) GrantDownload(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	identity, _ := middleware.GetAuthIdentity(r.Context())
	orderItemID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "order item id must be a valid UUID")
		return
	}
	var body struct {
		RequestedAsset string `json:"requested_asset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.GrantDownload(r.Context(), identity.UserID, orderItemID, body.RequestedAsset)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}

// ServeDownload handles GET /api/v1/downloads/file/{token}. Public; token in path.
func (h *PaymentHandler) ServeDownload(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	result, err := h.usecase.ServeDownload(r.Context(), token)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	// Stream the object from the vault through the response.
	object, err := h.s3.StreamObject(r.Context(), result.ObjectKey)
	if err != nil {
		h.writeError(w, r, http.StatusNotFound, "NOT_FOUND", "download object is not available")
		return
	}
	defer object.Close()

	w.Header().Set("Content-Type", "image/jpeg")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, object); err != nil {
		return
	}
}

// UploadMaster handles POST /api/v1/fulfillment/upload-master/{order_item_id}.
func (h *PaymentHandler) UploadMaster(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	identity, _ := middleware.GetAuthIdentity(r.Context())
	orderItemID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "order item id must be a valid UUID")
		return
	}
	var body struct {
		StorageObjectKey string `json:"storage_object_key"`
		SHA256Checksm    string `json:"sha256_checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "request body must be valid JSON")
		return
	}

	result, err := h.usecase.UploadMaster(r.Context(), identity.UserID, orderItemID, body.StorageObjectKey, body.SHA256Checksm)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.writeSuccess(w, r, http.StatusOK, result)
}
