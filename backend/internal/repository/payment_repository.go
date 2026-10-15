package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrPaymentNotFound is returned when a payment row does not exist.
	ErrPaymentNotFound = errors.New("payment not found")

	// ErrOrderNotPayable is returned when an order is not in a payable state.
	ErrOrderNotPayable = errors.New("order is not awaiting payment")

	// ErrFulfillmentNotFound is returned when no fulfillment job matches an order item.
	ErrFulfillmentNotFound = errors.New("fulfillment job not found for this order item")

	// ErrFulfillmentAlreadyDone is returned when the SLA is already complete.
	ErrFulfillmentAlreadyDone = errors.New("fulfillment for this order item is already resolved")

	// ErrDownloadNotPermitted is returned when the caller cannot download this asset yet.
	ErrDownloadNotPermitted = errors.New("download not permitted for this order item")

	// ErrDownloadGrantNotFound is returned when a download token is unknown, expired, or revoked.
	ErrDownloadGrantNotFound = errors.New("download grant is invalid or expired")

	// ErrSLABreached is returned when a master upload arrives past the 48-hour deadline.
	ErrSLABreached = errors.New("the 48-hour SLA for this master upload has already been breached")
)

// PaymentRepository defines data access for payments, fulfillment, and downloads.
type PaymentRepository interface {
	GetOrderByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	CreatePayment(ctx context.Context, payment *domain.Payment) error
	GetPaymentByID(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, error)
	SettlePayment(ctx context.Context, paymentID uuid.UUID) error
	RecordPaymentEvent(ctx context.Context, paymentID *uuid.UUID, providerEventID string, eventType string, signatureValid bool) error
	MarkOrderPaid(ctx context.Context, orderID uuid.UUID) error
	CreateFulfillmentJobs(ctx context.Context, orderID uuid.UUID, dueAt time.Time) (int, error)
	GetFulfillmentByOrderItem(ctx context.Context, orderItemID uuid.UUID) (*domain.FulfillmentJob, error)
	CompleteFulfillment(ctx context.Context, orderItemID uuid.UUID, readyAt time.Time) error
	GetOrderItem(ctx context.Context, orderItemID uuid.UUID) (*domain.OrderItem, error)
	CreateDownloadGrant(ctx context.Context, grant *domain.DownloadGrant) error
	GetDownloadGrant(ctx context.Context, token string) (*domain.DownloadGrant, error)
	RecordDownloadEvent(ctx context.Context, grantID, userID, assetID uuid.UUID, eventType string) error
}

// PostgresPaymentRepository implements PaymentRepository backed by PostgreSQL 16.
type PostgresPaymentRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresPaymentRepository constructs a new PostgresPaymentRepository.
func NewPostgresPaymentRepository(pool *pgxpool.Pool) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{pool: pool}
}

func (r *PostgresPaymentRepository) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	var order domain.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, buyer_user_id, status, currency_code, total_idr, placed_at, created_at
		FROM orders WHERE id = $1
	`, orderID).Scan(&order.ID, &order.BuyerUserID, &order.Status, &order.Currency,
		&order.TotalAmountIDR, &order.CreatedAt, &order.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to query order: %w", err)
	}
	return &order, nil
}

func (r *PostgresPaymentRepository) CreatePayment(ctx context.Context, payment *domain.Payment) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO payments (order_id, provider, status, amount_idr, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, payment.OrderID, payment.Provider, payment.Status, payment.AmountIDR, payment.IdempotencyKey,
	).Scan(&payment.ID, &payment.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert payment: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) GetPaymentByID(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, error) {
	var p domain.Payment
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_id, provider, status, amount_idr, created_at, captured_at
		FROM payments WHERE id = $1
	`, paymentID).Scan(&p.ID, &p.OrderID, &p.Provider, &p.Status, &p.AmountIDR, &p.CreatedAt, &p.CapturedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to query payment: %w", err)
	}
	return &p, nil
}

// SettlePayment marks one payment captured and moves its order to PAID in one transaction.
func (r *PostgresPaymentRepository) SettlePayment(ctx context.Context, paymentID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin settle transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orderID uuid.UUID
	var currentStatus string
	err = tx.QueryRow(ctx, `
		UPDATE payments SET status = $2, captured_at = CURRENT_TIMESTAMP WHERE id = $1
		RETURNING order_id, status
	`, paymentID, domain.PaymentStatusSettled).Scan(&orderID, &currentStatus)
	if err != nil {
		return fmt.Errorf("failed to settle payment: %w", err)
	}

	if _, err := tx.Exec(ctx,
		"UPDATE orders SET status = $2 WHERE id = $1", orderID, domain.OrderStatusPaid); err != nil {
		return fmt.Errorf("failed to mark order paid: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO order_status_events (order_id, from_status, to_status, reason_code)
		SELECT $1, status, $2, 'PAYMENT_SETTLED' FROM orders WHERE id = $1
	`, orderID, domain.OrderStatusPaid); err != nil {
		return fmt.Errorf("failed to record status event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit settle: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) RecordPaymentEvent(ctx context.Context, paymentID *uuid.UUID, providerEventID string, eventType string, signatureValid bool) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payment_events (payment_id, provider_event_id, event_type, signature_valid, received_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
	`, paymentID, providerEventID, eventType, signatureValid)
	if err != nil {
		return fmt.Errorf("failed to record payment event: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) MarkOrderPaid(ctx context.Context, orderID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		"UPDATE orders SET status = $2 WHERE id = $1", orderID, domain.OrderStatusPaid)
	if err != nil {
		return fmt.Errorf("failed to mark order paid: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// CreateFulfillmentJobs inserts one SLA-tracked job per order item, due in 48 hours.
func (r *PostgresPaymentRepository) CreateFulfillmentJobs(ctx context.Context, orderID uuid.UUID, dueAt time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO fulfillment_jobs (order_item_id, status, hires_due_at, attempt_count)
		SELECT id, $2, $3, 0 FROM order_items WHERE order_id = $1
	`, orderID, domain.FulfillmentStatusWaitingMaster, dueAt)
	if err != nil {
		return 0, fmt.Errorf("failed to create fulfillment jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PostgresPaymentRepository) GetFulfillmentByOrderItem(ctx context.Context, orderItemID uuid.UUID) (*domain.FulfillmentJob, error) {
	var j domain.FulfillmentJob
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_item_id, status, hires_due_at, hires_ready_at, failure_code, attempt_count, created_at
		FROM fulfillment_jobs WHERE order_item_id = $1
	`, orderItemID).Scan(&j.ID, &j.OrderItemID, &j.Status, &j.HiresDueAt, &j.HiresReadyAt, &j.FailureCode, &j.AttemptCount, &j.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFulfillmentNotFound
		}
		return nil, fmt.Errorf("failed to query fulfillment job: %w", err)
	}
	return &j, nil
}

// CompleteFulfillment marks one fulfillment job FULFILLED with its ready time.
func (r *PostgresPaymentRepository) CompleteFulfillment(ctx context.Context, orderItemID uuid.UUID, readyAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE fulfillment_jobs
		SET status = $2, hires_ready_at = $3
		WHERE order_item_id = $1 AND status = $4
	`, orderItemID, domain.FulfillmentStatusFulfilled, readyAt, domain.FulfillmentStatusWaitingMaster)
	if err != nil {
		return fmt.Errorf("failed to complete fulfillment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrFulfillmentAlreadyDone
	}
	return nil
}

func (r *PostgresPaymentRepository) GetOrderItem(ctx context.Context, orderItemID uuid.UUID) (*domain.OrderItem, error) {
	var item domain.OrderItem
	err := r.pool.QueryRow(ctx, `
		SELECT id, order_id, photo_id, seller_user_id, product_type, unit_price_idr, quantity, line_total_idr
		FROM order_items WHERE id = $1
	`, orderItemID).Scan(&item.ID, &item.OrderID, &item.PhotoID, &item.SellerUserID,
		&item.ProductType, &item.UnitPriceIDR, &item.Quantity, &item.LineTotalIDR)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to query order item: %w", err)
	}
	return &item, nil
}

func (r *PostgresPaymentRepository) CreateDownloadGrant(ctx context.Context, grant *domain.DownloadGrant) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO download_grants (order_item_id, user_id, asset_id, grant_type, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, grant.OrderItemID, grant.UserID, grant.AssetID, grant.GrantType, grant.ExpiresAt,
	).Scan(&grant.ID, &grant.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert download grant: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) GetDownloadGrant(ctx context.Context, token string) (*domain.DownloadGrant, error) {
	grantID, err := uuid.Parse(token)
	if err != nil {
		return nil, ErrDownloadGrantNotFound
	}
	var g domain.DownloadGrant
	err = r.pool.QueryRow(ctx, `
		SELECT id, order_item_id, user_id, asset_id, grant_type, expires_at, created_at
		FROM download_grants
		WHERE id = $1 AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP
	`, grantID).Scan(&g.ID, &g.OrderItemID, &g.UserID, &g.AssetID, &g.GrantType, &g.ExpiresAt, &g.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDownloadGrantNotFound
		}
		return nil, fmt.Errorf("failed to query download grant: %w", err)
	}
	return &g, nil
}

func (r *PostgresPaymentRepository) RecordDownloadEvent(ctx context.Context, grantID, userID, assetID uuid.UUID, eventType string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO download_events (download_grant_id, user_id, asset_id, event_type)
		VALUES ($1, $2, $3, $4)
	`, grantID, userID, assetID, eventType)
	if err != nil {
		return fmt.Errorf("failed to record download event: %w", err)
	}
	return nil
}
