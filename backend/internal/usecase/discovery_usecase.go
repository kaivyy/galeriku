package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
)

const (
	// checkoutVerificationTTL is how long a BIB challenge stays open.
	checkoutVerificationTTL = 30 * time.Minute
	// checkoutGrantTTL is how long a confirmed grant stays consumable.
	checkoutGrantTTL = 30 * time.Minute
)

// DiscoveryUsecase orchestrates BIB search, cart, checkout verification, and orders.
type DiscoveryUsecase interface {
	SearchBIB(ctx context.Context, bibNumber string, eventID *uuid.UUID) (*BIBSearchResult, error)
	AddToCart(ctx context.Context, buyerID, photoID uuid.UUID) (*domain.CartItem, error)
	ListCart(ctx context.Context, buyerID uuid.UUID) (*CartResult, error)
	RemoveFromCart(ctx context.Context, buyerID, itemID uuid.UUID) error
	InitiateVerification(ctx context.Context, userID uuid.UUID, req InitiateVerificationRequest) (*InitiateVerificationResult, error)
	ConfirmVerification(ctx context.Context, verificationID uuid.UUID, code string) (*ConfirmVerificationResult, error)
	CreateOrder(ctx context.Context, buyerID uuid.UUID, req CreateOrderRequest, idempotencyKey string) (*CreateOrderResult, error)
	GetOrder(ctx context.Context, buyerID, orderID uuid.UUID) (*OrderResult, error)
}

type BIBSearchResult struct {
	BIBNormalized string `json:"bib_normalized"`
	TotalPhotos   int    `json:"total_photos"`
	Photos        []domain.PhotoSearchResult `json:"photos"`
}

type CartResult struct {
	CartID       string `json:"cart_id"`
	TotalItems   int    `json:"total_items"`
	TotalAmountIDR int64 `json:"total_amount_idr"`
	Items        []CartItemResult `json:"items"`
}

type CartItemResult struct {
	CartItemID string `json:"cart_item_id"`
	PhotoID   string `json:"photo_id"`
	PriceIDR  int64  `json:"price_idr"`
}

type InitiateVerificationRequest struct {
	BIBNumber          string
	EventID            *uuid.UUID
	VerificationMethod string
}

type InitiateVerificationResult struct {
	VerificationID   string `json:"verification_id"`
	TargetBIB        string `json:"target_bib"`
	DestinationHint   string `json:"destination_hint,omitempty"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type ConfirmVerificationResult struct {
	GrantToken string    `json:"grant_token"`
	BIBNumber  string    `json:"bib_number"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type CreateOrderRequest struct {
	PhotoIDs  []uuid.UUID
	BIBGrants []string
}

type CreateOrderResult struct {
	OrderID       string    `json:"order_id"`
	OrderNumber   string    `json:"order_number"`
	TotalAmountIDR int64    `json:"total_amount_idr"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type OrderResult struct {
	OrderID        string             `json:"order_id"`
	OrderNumber    string             `json:"order_number"`
	Status         string             `json:"status"`
	TotalAmountIDR int64              `json:"total_amount_idr"`
	Items          []OrderItemResult  `json:"items"`
}

type OrderItemResult struct {
	OrderItemID string `json:"order_item_id"`
	PhotoID     string `json:"photo_id"`
	UnitPriceIDR int64 `json:"unit_price_idr"`
}

type discoveryUsecase struct {
	repo repository.DiscoveryRepository
	otp  *security.OTPIssuer
}

func NewDiscoveryUsecase(repo repository.DiscoveryRepository, otp *security.OTPIssuer) DiscoveryUsecase {
	return &discoveryUsecase{repo: repo, otp: otp}
}

func (u *discoveryUsecase) SearchBIB(ctx context.Context, bibNumber string, eventID *uuid.UUID) (*BIBSearchResult, error) {
	bibNumber = strings.TrimSpace(bibNumber)
	if bibNumber == "" {
		return nil, fmt.Errorf("%w: bib_number is required", ErrInvalidInput)
	}
	photos, err := u.repo.SearchPhotosByBIB(ctx, bibNumber, eventID)
	if err != nil {
		return nil, err
	}
	return &BIBSearchResult{BIBNormalized: bibNumber, TotalPhotos: len(photos), Photos: photos}, nil
}

func (u *discoveryUsecase) AddToCart(ctx context.Context, buyerID, photoID uuid.UUID) (*domain.CartItem, error) {
	cart, err := u.repo.GetOrCreateActiveCart(ctx, buyerID)
	if err != nil {
		return nil, err
	}
	price, err := u.repo.GetPhotoPrice(ctx, photoID)
	if err != nil {
		return nil, err
	}
	itemID, err := u.repo.AddCartItem(ctx, cart.ID, photoID, domain.ProductTypeDigitalHiRes)
	if err != nil {
		return nil, err
	}
	return &domain.CartItem{ID: itemID, CartID: cart.ID, PhotoID: photoID, ProductType: domain.ProductTypeDigitalHiRes, PriceIDR: price}, nil
}

func (u *discoveryUsecase) ListCart(ctx context.Context, buyerID uuid.UUID) (*CartResult, error) {
	cart, err := u.repo.GetOrCreateActiveCart(ctx, buyerID)
	if err != nil {
		return nil, err
	}
	items, err := u.repo.ListCartItems(ctx, cart.ID)
	if err != nil {
		return nil, err
	}
	var total int64
	itemResults := make([]CartItemResult, 0, len(items))
	for _, ci := range items {
		total += ci.PriceIDR
		itemResults = append(itemResults, CartItemResult{CartItemID: ci.ID.String(), PhotoID: ci.PhotoID.String(), PriceIDR: ci.PriceIDR})
	}
	return &CartResult{CartID: cart.ID.String(), TotalItems: len(items), TotalAmountIDR: total, Items: itemResults}, nil
}

func (u *discoveryUsecase) RemoveFromCart(ctx context.Context, buyerID, itemID uuid.UUID) error {
	cart, err := u.repo.GetOrCreateActiveCart(ctx, buyerID)
	if err != nil {
		return err
	}
	return u.repo.RemoveCartItem(ctx, cart.ID, itemID)
}

func (u *discoveryUsecase) InitiateVerification(ctx context.Context, userID uuid.UUID, req InitiateVerificationRequest) (*InitiateVerificationResult, error) {
	req.BIBNumber = strings.TrimSpace(req.BIBNumber)
	if req.BIBNumber == "" {
		return nil, fmt.Errorf("%w: bib_number is required", ErrInvalidInput)
	}
	verification := &domain.CheckoutVerification{
		UserID:           userID,
		EventID:          req.EventID,
		VerificationType: "EVENT_REGISTRATION_OTP",
		Status:           "PENDING",
	}

	// The row must exist before we can issue the code, because the row id is the
	// HMAC entity: it binds one code to exactly one verification.
	if err := u.repo.CreateVerification(ctx, verification, "", time.Now().UTC().Add(checkoutVerificationTTL)); err != nil {
		return nil, err
	}

	_, digest, expiresAt, err := u.otp.Issue(verification.ID.String(), checkoutVerificationTTL)
	if err != nil {
		return nil, err
	}
	if err := u.repo.UpdateChallengeDigest(ctx, verification.ID, digest, expiresAt); err != nil {
		return nil, err
	}

	return &InitiateVerificationResult{
		VerificationID:   verification.ID.String(),
		TargetBIB:        req.BIBNumber,
		ExpiresInSeconds: int(checkoutVerificationTTL.Seconds()),
	}, nil
}

func (u *discoveryUsecase) ConfirmVerification(ctx context.Context, verificationID uuid.UUID, code string) (*ConfirmVerificationResult, error) {
	verification, codeHash, expiresAt, err := u.repo.GetVerification(ctx, verificationID)
	if err != nil {
		return nil, err
	}
	if verification.AttemptCount >= security.CheckoutOTPMaxAttempt {
		return nil, repository.ErrVerificationTooManyAttempts
	}
	if time.Now().UTC().After(expiresAt) {
		return nil, repository.ErrVerificationExpired
	}
	if !u.otp.Verify(verification.ID.String(), code, codeHash) {
		_ = u.repo.IncrementVerificationAttempts(ctx, verificationID)
		return nil, security.ErrCheckoutOTPInvalid
	}

	grant := &domain.CheckoutVerificationGrant{
		UserID:         verification.UserID,
		EventID:        verification.EventID,
		VerificationID: verification.ID,
		Scope:          domain.CheckoutScopeBIBEvent,
		ExpiresAt:      time.Now().UTC().Add(checkoutGrantTTL),
	}
	if err := u.repo.IssueGrant(ctx, grant); err != nil {
		return nil, err
	}
	return &ConfirmVerificationResult{
		GrantToken: grant.ID.String(),
		ExpiresAt:  grant.ExpiresAt,
	}, nil
}

func (u *discoveryUsecase) CreateOrder(ctx context.Context, buyerID uuid.UUID, req CreateOrderRequest, idempotencyKey string) (*CreateOrderResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, fmt.Errorf("%w: Idempotency-Key header is required", ErrInvalidInput)
	}
	if len(req.PhotoIDs) == 0 {
		return nil, fmt.Errorf("%w: photo_ids must contain at least one entry", ErrInvalidInput)
	}

	// Idempotency: if this key already created an order for this buyer, return it.
	existingOrderID, err := u.repo.FindOrderByIdempotencyKey(ctx, idempotencyKey, buyerID)
	if err == nil {
		order, _, err := u.repo.GetOrderByID(ctx, existingOrderID)
		if err != nil {
			return nil, err
		}
		return &CreateOrderResult{
			OrderID: order.ID.String(), OrderNumber: fmt.Sprintf("GLK-%s", order.ID.String()[:12]),
			TotalAmountIDR: order.TotalAmountIDR, Currency: order.Currency, Status: order.Status, CreatedAt: order.CreatedAt,
		}, nil
	} else if !errors.Is(err, repository.ErrOrderNotFound) {
		return nil, err
	}

	var subtotal int64
	orderItems := make([]domain.OrderItem, 0, len(req.PhotoIDs))
	for _, photoID := range req.PhotoIDs {
		price, err := u.repo.GetPhotoPrice(ctx, photoID)
		if err != nil {
			return nil, err
		}
		orderItems = append(orderItems, domain.OrderItem{
			PhotoID:      photoID,
			SellerUserID: buyerID,
			ProductType:  domain.ProductTypeDigitalHiRes,
			UnitPriceIDR: price,
			Quantity:     1,
			LineTotalIDR: price,
		})
		subtotal += price
	}

	order := &domain.Order{
		BuyerUserID:    buyerID,
		Status:         domain.OrderStatusPendingPayment,
		Currency:       domain.OrderCurrencyIDR,
		TotalAmountIDR: subtotal,
	}
	if err := u.repo.CreateOrder(ctx, order, orderItems); err != nil {
		return nil, err
	}

	if err := u.repo.SaveIdempotencyKey(ctx, idempotencyKey, buyerID, order.ID); err != nil {
		return nil, err
	}

	return &CreateOrderResult{
		OrderID:        order.ID.String(),
		OrderNumber:    fmt.Sprintf("GLK-%s", order.ID.String()[:12]),
		TotalAmountIDR: order.TotalAmountIDR,
		Currency:       order.Currency,
		Status:         order.Status,
		CreatedAt:      order.CreatedAt,
	}, nil
}

func (u *discoveryUsecase) GetOrder(ctx context.Context, buyerID, orderID uuid.UUID) (*OrderResult, error) {
	order, items, err := u.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.BuyerUserID != buyerID {
		return nil, ErrForbiddenResource
	}
	itemResults := make([]OrderItemResult, 0, len(items))
	for _, item := range items {
		itemResults = append(itemResults, OrderItemResult{
			OrderItemID: item.ID.String(),
			PhotoID:     item.PhotoID.String(),
			UnitPriceIDR: item.UnitPriceIDR,
		})
	}
	return &OrderResult{
		OrderID:        order.ID.String(),
		OrderNumber:    fmt.Sprintf("GLK-%s", order.ID.String()[:12]),
		Status:         order.Status,
		TotalAmountIDR: order.TotalAmountIDR,
		Items:          itemResults,
	}, nil
}
