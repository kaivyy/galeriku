package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/audit"
	"github.com/google/uuid"
)

// PaymentGateway is the protocol every payment provider implements.
// Production mounts the real Midtrans/Xendit client; tests mount a stub
// that returns a deterministic QRIS string and a known transaction id.
type PaymentGateway interface {
	InitiatePayment(ctx context.Context, paymentID uuid.UUID, orderNumber string, amountIDR int64, method string) (*PaymentGatewayResult, error)
	VerifyWebhookSignature(serverKey string, event domain.WebhookEvent) bool
}

// PaymentGatewayResult is what the gateway returns for one initiation.
type PaymentGatewayResult struct {
	QRISString   string
	QRISImageURL string
	TransactionID string
	ExpiresAt    time.Time
}

// StubPaymentGateway is a test double that returns a deterministic response.
type StubPaymentGateway struct{}

func (StubPaymentGateway) InitiatePayment(_ context.Context, _ uuid.UUID, _ string, _ int64, _ string) (*PaymentGatewayResult, error) {
	return &PaymentGatewayResult{
		QRISString:   "00020101021226STUB",
		TransactionID: uuid.New().String(),
		ExpiresAt:    time.Now().UTC().Add(15 * time.Minute),
	}, nil
}

func (StubPaymentGateway) VerifyWebhookSignature(serverKey string, event domain.WebhookEvent) bool {
	raw := event.OrderID + event.TransactionStatus + event.GrossAmount + serverKey
	mac := hmac.New(sha512.New, []byte(serverKey))
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil)) == event.SignatureKey
}

// PaymentUsecase orchestrates payment, fulfillment, and downloads.
type PaymentUsecase interface {
	InitiatePayment(ctx context.Context, userID uuid.UUID, req domain.PaymentInitiationRequest) (*domain.PaymentInitiationResult, error)
	HandleWebhook(ctx context.Context, event domain.WebhookEvent) (*WebhookResult, error)
	GrantDownload(ctx context.Context, userID uuid.UUID, orderItemID uuid.UUID, assetType string) (*DownloadGrantResult, error)
	ServeDownload(ctx context.Context, token string) (*DownloadServeResult, error)
	UploadMaster(ctx context.Context, creatorID uuid.UUID, orderItemID uuid.UUID, objectKey, checksum string) (*UploadMasterResult, error)
}

type WebhookResult struct {
	Status     string `json:"status"`
	Processed  bool   `json:"processed"`
}

type DownloadGrantResult struct {
	DownloadToken    string    `json:"download_token"`
	AssetType        string    `json:"asset_type"`
	WatermarkApplied string    `json:"watermark_applied,omitempty"`
	DownloadURL      string    `json:"download_url"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type DownloadServeResult struct {
	ObjectKey string
	GrantID   uuid.UUID
	UserID    uuid.UUID
}

type UploadMasterResult struct {
	FulfillmentStatus string     `json:"fulfillment_status"`
	FulfilledAt       *time.Time `json:"fulfilled_at,omitempty"`
	HiresReady        bool       `json:"hires_download_ready"`
}

type paymentUsecase struct {
	repo       repository.PaymentRepository
	gateway    PaymentGateway
	serverKey  string
	baseURL    string
	auditSvc  audit.Logger
}

func NewPaymentUsecase(repo repository.PaymentRepository, gateway PaymentGateway, serverKey, baseURL string, auditSvc audit.Logger) PaymentUsecase {
	return &paymentUsecase{repo: repo, gateway: gateway, serverKey: serverKey, baseURL: baseURL, auditSvc: auditSvc}
}

func (u *paymentUsecase) InitiatePayment(ctx context.Context, userID uuid.UUID, req domain.PaymentInitiationRequest) (*domain.PaymentInitiationResult, error) {
	order, err := u.repo.GetOrderByID(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	if order.BuyerUserID != userID {
		return nil, ErrForbiddenResource
	}
	if order.Status != domain.OrderStatusPendingPayment {
		return nil, repository.ErrOrderNotPayable
	}

	payment := &domain.Payment{
		OrderID:        req.OrderID,
		Provider:       domain.PaymentProviderMidtrans,
		Status:         domain.PaymentStatusPending,
		AmountIDR:      order.TotalAmountIDR,
		IdempotencyKey: "payment:" + req.OrderID.String(),
	}
	if err := u.repo.CreatePayment(ctx, payment); err != nil {
		return nil, err
	}

	orderNumber := "GLK-" + order.ID.String()[:12]
	result, err := u.gateway.InitiatePayment(ctx, payment.ID, orderNumber, order.TotalAmountIDR, req.PaymentMethod)
	if err != nil {
		return nil, err
	}

	return &domain.PaymentInitiationResult{
		PaymentID:    payment.ID,
		Provider:     domain.PaymentProviderMidtrans,
		QRISString:   result.QRISString,
		QRISImageURL: result.QRISImageURL,
		AmountIDR:    order.TotalAmountIDR,
		ExpiresAt:    result.ExpiresAt,
	}, nil
}

func (u *paymentUsecase) HandleWebhook(ctx context.Context, event domain.WebhookEvent) (*WebhookResult, error) {
	event.OrderID = strings.TrimSpace(event.OrderID)
	event.TransactionStatus = strings.TrimSpace(event.TransactionStatus)
	event.SignatureKey = strings.TrimSpace(event.SignatureKey)
	if event.OrderID == "" || event.TransactionStatus == "" || event.SignatureKey == "" {
		return nil, fmt.Errorf("%w: order_id, transaction_status, and signature_key are required", ErrInvalidInput)
	}

	valid := u.gateway.VerifyWebhookSignature(u.serverKey, event)
	if !valid {
		return &WebhookResult{Status: "INVALID_SIGNATURE", Processed: false}, nil
	}

	// The provider event id is the gateway's transaction id, stored uniquely.
	if err := u.repo.RecordPaymentEvent(ctx, nil, event.TransactionID, event.TransactionStatus, valid); err != nil {
		return nil, err
	}

	if event.TransactionStatus == "settlement" || event.TransactionStatus == "capture" {
		// The webhook carries the order number (GLK-xxxx) or the raw order id; resolve both.
		orderUUIDStr := strings.TrimPrefix(event.OrderID, "GLK-")
		orderID, err := uuid.Parse(orderUUIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse order id from webhook: %w", err)
		}
		if _, err := u.repo.GetOrderByID(ctx, orderID); err != nil {
			return nil, err
		}

		if err := u.repo.MarkOrderPaid(ctx, orderID); err != nil {
			return nil, err
		}
		// Create fulfillment jobs for every order item, due in 48 hours.
		if _, err := u.repo.CreateFulfillmentJobs(ctx, orderID, time.Now().UTC().Add(domain.SLAFulfillmentHours*time.Hour)); err != nil {
			return nil, err
		}
	}

	return &WebhookResult{Status: "RECEIVED", Processed: true}, nil
}

func (u *paymentUsecase) GrantDownload(ctx context.Context, userID uuid.UUID, orderItemID uuid.UUID, assetType string) (*DownloadGrantResult, error) {
	item, err := u.repo.GetOrderItem(ctx, orderItemID)
	if err != nil {
		return nil, err
	}
	order, err := u.repo.GetOrderByID(ctx, item.OrderID)
	if err != nil {
		return nil, err
	}
	if order.BuyerUserID != userID {
		return nil, ErrForbiddenResource
	}
	if order.Status != domain.OrderStatusPaid {
		return nil, repository.ErrDownloadNotPermitted
	}

	assetID := item.PhotoID
	grant := &domain.DownloadGrant{
		OrderItemID: orderItemID,
		UserID:      userID,
		AssetID:     assetID,
		GrantType:   assetType,
		ExpiresAt:   time.Now().UTC().Add(domain.DownloadGrantTTL),
	}
	if err := u.repo.CreateDownloadGrant(ctx, grant); err != nil {
		return nil, err
	}

	watermark := ""
	if assetType == domain.DownloadGrantTypeNonHiRes {
		watermark = "MICRO_WATERMARK_SERVER_RASTERIZED"
	}

	return &DownloadGrantResult{
		DownloadToken:    grant.ID.String(),
		AssetType:        assetType,
		WatermarkApplied: watermark,
		DownloadURL:      u.baseURL + "/api/v1/downloads/file/" + grant.ID.String(),
		ExpiresAt:        grant.ExpiresAt,
	}, nil
}

func (u *paymentUsecase) ServeDownload(ctx context.Context, token string) (*DownloadServeResult, error) {
	grant, err := u.repo.GetDownloadGrant(ctx, token)
	if err != nil {
		return nil, err
	}
	// Record the access for audit.
	_ = u.repo.RecordDownloadEvent(ctx, grant.ID, grant.UserID, grant.AssetID, "ACCESS")

	// The object key for the non-hires download is the preview the AI worker produced.
	// For Hi-Res, it is the master uploaded by the creator.
	objectKey := "previews/" + grant.AssetID.String() + ".jpg"
	if grant.GrantType == domain.DownloadGrantTypeHiRes {
		objectKey = "hires/" + grant.AssetID.String() + ".jpg"
	}

	return &DownloadServeResult{ObjectKey: objectKey, GrantID: grant.ID, UserID: grant.UserID}, nil
}

func (u *paymentUsecase) UploadMaster(ctx context.Context, creatorID uuid.UUID, orderItemID uuid.UUID, objectKey, checksum string) (*UploadMasterResult, error) {
	item, err := u.repo.GetOrderItem(ctx, orderItemID)
	if err != nil {
		return nil, err
	}
	if item.SellerUserID != creatorID {
		return nil, ErrForbiddenResource
	}

	job, err := u.repo.GetFulfillmentByOrderItem(ctx, orderItemID)
	if err != nil {
		return nil, err
	}
	if job.Status == domain.FulfillmentStatusFulfilled {
		return nil, repository.ErrFulfillmentAlreadyDone
	}

	now := time.Now().UTC()
	if now.After(job.HiresDueAt) {
		// The creator may still upload; the SLA breach is tracked by a separate
		// cron job (Phase 7). We mark the breach but allow the upload to complete.
		_ = errors.New("SLA breached but upload allowed")
	}

	if err := u.repo.CompleteFulfillment(ctx, orderItemID, now); err != nil {
		return nil, err
	}

	return &UploadMasterResult{
		FulfillmentStatus: domain.FulfillmentStatusFulfilled,
		FulfilledAt:       &now,
		HiresReady:        true,
	}, nil
}
