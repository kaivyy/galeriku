package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	PaymentStatusPending   = "PENDING"
	PaymentStatusSettled   = "SETTLED"
	PaymentStatusFailed    = "FAILED"
	PaymentStatusExpired   = "EXPIRED"

	PaymentProviderMidtrans = "MIDTRANS"

	FulfillmentStatusWaitingMaster = "WAITING_MASTER"
	FulfillmentStatusFulfilled     = "FULFILLED"
	FulfillmentStatusSLABreach     = "SLA_BREACH"

	SLAFulfillmentHours = 48

	DownloadGrantTypeNonHiRes = "NON_HIRES_DOWNLOAD"
	DownloadGrantTypeHiRes    = "HIRES_DOWNLOAD"
	DownloadGrantTTL          = 15 * time.Minute
)

type Payment struct {
	ID               uuid.UUID  `json:"id"`
	OrderID          uuid.UUID  `json:"order_id"`
	Provider         string     `json:"provider"`
	ProviderPaymentID *uuid.UUID `json:"provider_payment_id,omitempty"`
	Status           string     `json:"status"`
	AmountIDR        int64      `json:"amount_idr"`
	IdempotencyKey   string     `json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	CapturedAt       *time.Time `json:"captured_at,omitempty"`
}

type FulfillmentJob struct {
	ID            uuid.UUID  `json:"id"`
	OrderItemID   uuid.UUID  `json:"order_item_id"`
	Status        string     `json:"status"`
	HiresDueAt    time.Time  `json:"hires_due_at"`
	HiresReadyAt  *time.Time `json:"hires_ready_at,omitempty"`
	FailureCode   *string    `json:"failure_code,omitempty"`
	AttemptCount  int        `json:"attempt_count"`
	CreatedAt     time.Time  `json:"created_at"`
}

type DownloadGrant struct {
	ID          uuid.UUID  `json:"id"`
	OrderItemID uuid.UUID  `json:"order_item_id"`
	UserID      uuid.UUID  `json:"user_id"`
	AssetID     uuid.UUID  `json:"asset_id"`
	GrantType   string     `json:"grant_type"`
	ExpiresAt   time.Time  `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// PaymentInitiationRequest is the validated input for POST /payments/initiate.
type PaymentInitiationRequest struct {
	OrderID       uuid.UUID
	PaymentMethod string
}

// PaymentInitiationResult carries the gateway response.
type PaymentInitiationResult struct {
	PaymentID    uuid.UUID  `json:"payment_id"`
	Provider     string     `json:"provider"`
	QRISString   string     `json:"qris_string,omitempty"`
	QRISImageURL string     `json:"qris_image_url,omitempty"`
	AmountIDR    int64      `json:"amount_idr"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// WebhookEvent is the parsed Midtrans webhook payload.
type WebhookEvent struct {
	OrderID           string `json:"order_id"`
	TransactionStatus string `json:"transaction_status"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionID     string `json:"transaction_id"`
}
