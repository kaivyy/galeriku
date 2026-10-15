package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// CartStatusActive marks an open cart.
	CartStatusActive = "ACTIVE"

	// CartStatusConverted marks a cart that became an order.
	CartStatusConverted = "CONVERTED"

	// OrderStatusPendingPayment is the initial order state.
	OrderStatusPendingPayment = "PENDING_PAYMENT"

	// OrderStatusPaid marks a paid order awaiting fulfillment.
	OrderStatusPaid = "PAID"

	// OrderStatusCancelled is a cancelled order.
	OrderStatusCancelled = "CANCELLED"

	// ProductTypeDigitalHiRes is the one product Phase 5 sells.
	ProductTypeDigitalHiRes = "DIGITAL_HIRES"

	// CheckoutScopeBIBEvent ties a grant to one bib within one event.
	CheckoutScopeBIBEvent = "BIB_EVENT"
)

// PhotoSearchResult is one entry in a BIB or face search response.
type PhotoSearchResult struct {
	PhotoID                          uuid.UUID `json:"photo_id"`
	PriceIDR                         int64     `json:"price_idr"`
	BIBCheckoutRequiresVerification  bool      `json:"bib_checkout_requires_verification"`
}

// Cart is one buyer's open shopping cart.
type Cart struct {
	ID           uuid.UUID `json:"id"`
	BuyerUserID  uuid.UUID `json:"buyer_user_id"`
	Status       string    `json:"status"`
	CurrencyCode string    `json:"currency_code"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CartItem is one photo in a cart with its price snapshot.
type CartItem struct {
	ID          uuid.UUID `json:"id"`
	CartID      uuid.UUID `json:"cart_id"`
	PhotoID     uuid.UUID `json:"photo_id"`
	ProductType string    `json:"product_type"`
	PriceIDR    int64     `json:"price_idr"`
	AddedAt     time.Time `json:"added_at"`
}

// CheckoutVerification tracks one BIB anti-fraud challenge.
type CheckoutVerification struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	EventID          *uuid.UUID `json:"event_id,omitempty"`
	VerificationType string     `json:"verification_type"`
	Status           string     `json:"status"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	AttemptCount     int        `json:"attempt_count"`
}

// CheckoutVerificationGrant is a one-shot proof that a BIB verification succeeded.
type CheckoutVerificationGrant struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	EventID        *uuid.UUID `json:"event_id,omitempty"`
	OrderID        *uuid.UUID `json:"order_id,omitempty"`
	VerificationID uuid.UUID  `json:"verification_id"`
	Scope          string     `json:"scope"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ConsumedAt     *time.Time `json:"consumed_at,omitempty"`
}

// Order, OrderStatus* and ProductType* live in entity.go; commerce.go extends them.

// OrderCurrencyIDR is the only currency Phase 5 settles in.
const OrderCurrencyIDR = "IDR"

// OrderItem is one line on an order with its price snapshot.
type OrderItem struct {
	ID           uuid.UUID  `json:"id"`
	OrderID      uuid.UUID  `json:"order_id"`
	PhotoID      uuid.UUID  `json:"photo_id"`
	SellerUserID uuid.UUID  `json:"seller_user_id"`
	EventID      *uuid.UUID `json:"event_id,omitempty"`
	ProductType  string     `json:"product_type"`
	UnitPriceIDR int64      `json:"unit_price_idr"`
	Quantity     int        `json:"quantity"`
	LineTotalIDR int64      `json:"line_total_idr"`
}
