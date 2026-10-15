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
	// ErrPhotoNotFound is returned when a photo row does not exist.
	ErrPhotoNotFound = errors.New("photo not found")

	// ErrPriceNotFound is returned when a photo has no price in the catalog yet.
	ErrPriceNotFound = errors.New("photo price not set in catalog")

	// ErrVerificationNotFound is returned when no verification challenge matches.
	ErrVerificationNotFound = errors.New("checkout verification not found")

	// ErrVerificationExpired is returned when the challenge has passed its expiry.
	ErrVerificationExpired = errors.New("checkout verification code has expired")

	// ErrVerificationTooManyAttempts is returned when the attempt limit is reached.
	ErrVerificationTooManyAttempts = errors.New("too many verification attempts")

	// ErrGrantNotFound is returned when no uncommitted grant matches the token.
	ErrGrantNotFound = errors.New("checkout verification grant not found or already consumed")

	// ErrIdempotencyKeyConflict is returned when an idempotency key is reused with a different buyer.
	ErrIdempotencyKeyConflict = errors.New("idempotency key already used by a different buyer")
)

// DiscoveryRepository defines data access for the Phase 5 discovery, cart, BIB
// verification, and order flows.
type DiscoveryRepository interface {
	// BIB search
	SearchPhotosByBIB(ctx context.Context, bibNormalized string, eventID *uuid.UUID) ([]domain.PhotoSearchResult, error)

	// Photo prices
	GetPhotoPrice(ctx context.Context, photoID uuid.UUID) (int64, error)
	UpsertPhotoPrice(ctx context.Context, photoID uuid.UUID, priceIDR int64) error

	// Cart
	GetOrCreateActiveCart(ctx context.Context, buyerUserID uuid.UUID) (*domain.Cart, error)
	ListCartItems(ctx context.Context, cartID uuid.UUID) ([]domain.CartItem, error)
	AddCartItem(ctx context.Context, cartID, photoID uuid.UUID, productType string) (uuid.UUID, error)
	RemoveCartItem(ctx context.Context, cartID, itemID uuid.UUID) error

	// BIB checkout verification
	CreateVerification(ctx context.Context, verification *domain.CheckoutVerification, challengeHash string, expiresAt time.Time) error
	UpdateChallengeDigest(ctx context.Context, verificationID uuid.UUID, digest string, expiresAt time.Time) error
	GetVerification(ctx context.Context, verificationID uuid.UUID) (*domain.CheckoutVerification, string, time.Time, error)
	IncrementVerificationAttempts(ctx context.Context, verificationID uuid.UUID) error
	IssueGrant(ctx context.Context, grant *domain.CheckoutVerificationGrant) error
	ConsumeGrant(ctx context.Context, grantToken string, orderID uuid.UUID) error

	// Orders
	CreateOrder(ctx context.Context, order *domain.Order, items []domain.OrderItem) error
	GetOrderByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, []domain.OrderItem, error)
	FindOrderByIdempotencyKey(ctx context.Context, key string, buyerID uuid.UUID) (uuid.UUID, error)
	SaveIdempotencyKey(ctx context.Context, key string, buyerID, orderID uuid.UUID) error
}

// PostgresDiscoveryRepository implements DiscoveryRepository backed by PostgreSQL 16.
type PostgresDiscoveryRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresDiscoveryRepository constructs a new PostgresDiscoveryRepository.
func NewPostgresDiscoveryRepository(pool *pgxpool.Pool) *PostgresDiscoveryRepository {
	return &PostgresDiscoveryRepository{pool: pool}
}

// PhotoSearchResult matches the photos catalog for BIB and face search.
func (r *PostgresDiscoveryRepository) SearchPhotosByBIB(ctx context.Context, bibNormalized string, eventID *uuid.UUID) ([]domain.PhotoSearchResult, error) {
	query := `
		SELECT p.id, COALESCE(pp.price_idr, 0),
		       (EXISTS (SELECT 1 FROM photo_bib_tags t WHERE t.photo_id = p.id AND t.bib_normalized = $1 AND t.review_status = 'CONFIRMED')) AS requires_verification
		FROM photos p
		JOIN photo_bib_tags t ON t.photo_id = p.id
		LEFT JOIN photo_prices pp ON pp.photo_id = p.id
		WHERE t.bib_normalized = $1
		ORDER BY p.created_at DESC
	`
	var rows pgx.Rows
	var err error
	if eventID != nil {
		query = `
			SELECT p.id, COALESCE(pp.price_idr, 0),
			       (EXISTS (SELECT 1 FROM photo_bib_tags t WHERE t.photo_id = p.id AND t.bib_normalized = $1 AND t.review_status = 'CONFIRMED')) AS requires_verification
			FROM photos p
			JOIN photo_bib_tags t ON t.photo_id = p.id
			JOIN photo_event_links pe ON pe.photo_id = p.id
			LEFT JOIN photo_prices pp ON pp.photo_id = p.id
			WHERE t.bib_normalized = $1 AND pe.event_id = $2
			ORDER BY p.created_at DESC
		`
		rows, err = r.pool.Query(ctx, query, bibNormalized, *eventID)
	} else {
		rows, err = r.pool.Query(ctx, query, bibNormalized)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search photos by bib: %w", err)
	}
	defer rows.Close()

	results := make([]domain.PhotoSearchResult, 0)
	for rows.Next() {
		var psr domain.PhotoSearchResult
		var requires bool
		if err := rows.Scan(&psr.PhotoID, &psr.PriceIDR, &requires); err != nil {
			return nil, fmt.Errorf("failed to scan photo search result: %w", err)
		}
		psr.BIBCheckoutRequiresVerification = requires
		results = append(results, psr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading photo search results: %w", err)
	}
	return results, nil
}

func (r *PostgresDiscoveryRepository) GetPhotoPrice(ctx context.Context, photoID uuid.UUID) (int64, error) {
	var price int64
	err := r.pool.QueryRow(ctx, "SELECT price_idr FROM photo_prices WHERE photo_id = $1", photoID).Scan(&price)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrPriceNotFound
		}
		return 0, fmt.Errorf("failed to query photo price: %w", err)
	}
	return price, nil
}

func (r *PostgresDiscoveryRepository) UpsertPhotoPrice(ctx context.Context, photoID uuid.UUID, priceIDR int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO photo_prices (photo_id, price_idr)
		VALUES ($1, $2)
		ON CONFLICT (photo_id) DO UPDATE SET price_idr = EXCLUDED.price_idr, updated_at = CURRENT_TIMESTAMP
	`, photoID, priceIDR)
	if err != nil {
		return fmt.Errorf("failed to upsert photo price: %w", err)
	}
	return nil
}

func (r *PostgresDiscoveryRepository) GetOrCreateActiveCart(ctx context.Context, buyerUserID uuid.UUID) (*domain.Cart, error) {
	// First try to fetch an existing active cart for this buyer.
	var cart domain.Cart
	err := r.pool.QueryRow(ctx, `
		SELECT id, buyer_user_id, status, currency_code, created_at, updated_at
		FROM carts WHERE buyer_user_id = $1 AND status = 'ACTIVE'
		ORDER BY created_at DESC LIMIT 1
	`, buyerUserID).Scan(&cart.ID, &cart.BuyerUserID, &cart.Status, &cart.CurrencyCode, &cart.CreatedAt, &cart.UpdatedAt)
	if err == nil {
		return &cart, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to query active cart: %w", err)
	}

	// No active cart: create one. If another worker races us, fetch theirs.
	err = r.pool.QueryRow(ctx, `
		INSERT INTO carts (buyer_user_id, status, currency_code)
		VALUES ($1, 'ACTIVE', 'IDR')
		RETURNING id, buyer_user_id, status, currency_code, created_at, updated_at
	`, buyerUserID).Scan(&cart.ID, &cart.BuyerUserID, &cart.Status, &cart.CurrencyCode, &cart.CreatedAt, &cart.UpdatedAt)
	if err != nil {
		// Race: another worker created it; fetch theirs.
		err = r.pool.QueryRow(ctx, `
			SELECT id, buyer_user_id, status, currency_code, created_at, updated_at
			FROM carts WHERE buyer_user_id = $1 AND status = 'ACTIVE'
			ORDER BY created_at DESC LIMIT 1
		`, buyerUserID).Scan(&cart.ID, &cart.BuyerUserID, &cart.Status, &cart.CurrencyCode, &cart.CreatedAt, &cart.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to create or fetch cart: %w", err)
		}
	}
	return &cart, nil
}

func (r *PostgresDiscoveryRepository) ListCartItems(ctx context.Context, cartID uuid.UUID) ([]domain.CartItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ci.id, ci.cart_id, ci.photo_id, ci.product_type, ci.added_at, COALESCE(pp.price_idr, 0)
		FROM cart_items ci
		LEFT JOIN photo_prices pp ON pp.photo_id = ci.photo_id
		WHERE ci.cart_id = $1
		ORDER BY ci.added_at ASC
	`, cartID)
	if err != nil {
		return nil, fmt.Errorf("failed to list cart items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.CartItem, 0)
	for rows.Next() {
		var ci domain.CartItem
		if err := rows.Scan(&ci.ID, &ci.CartID, &ci.PhotoID, &ci.ProductType, &ci.AddedAt, &ci.PriceIDR); err != nil {
			return nil, fmt.Errorf("failed to scan cart item: %w", err)
		}
		items = append(items, ci)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading cart items: %w", err)
	}
	return items, nil
}

func (r *PostgresDiscoveryRepository) AddCartItem(ctx context.Context, cartID, photoID uuid.UUID, productType string) (uuid.UUID, error) {
	// Check first: if this photo is already in the cart, return its id without
	// inserting again. The cart_items table has no unique constraint, so an
	// unconditional INSERT would stack duplicates.
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		"SELECT id FROM cart_items WHERE cart_id = $1 AND photo_id = $2", cartID, photoID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("failed to check existing cart item: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO cart_items (cart_id, photo_id, product_type)
		VALUES ($1, $2, $3)
		RETURNING id
	`, cartID, photoID, productType).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to add cart item: %w", err)
	}
	return id, nil
}

func (r *PostgresDiscoveryRepository) RemoveCartItem(ctx context.Context, cartID, itemID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		"DELETE FROM cart_items WHERE id = $1 AND cart_id = $2", itemID, cartID)
	if err != nil {
		return fmt.Errorf("failed to remove cart item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPhotoNotFound
	}
	return nil
}

func (r *PostgresDiscoveryRepository) CreateVerification(ctx context.Context, verification *domain.CheckoutVerification, challengeHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin verification transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO checkout_verifications (user_id, event_id, verification_type, status, expires_at, attempt_count)
		VALUES ($1, $2, $3, 'PENDING', $4, 0)
		RETURNING id
	`, verification.UserID, verification.EventID, verification.VerificationType, expiresAt).Scan(&verification.ID)
	if err != nil {
		return fmt.Errorf("failed to insert checkout verification: %w", err)
	}

	var challengeID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO verification_challenges (checkout_verification_id, code_hash, status, expires_at, attempt_count)
		VALUES ($1, $2, 'PENDING', $3, 0)
		RETURNING id
	`, verification.ID, challengeHash, expiresAt).Scan(&challengeID)
	if err != nil {
		return fmt.Errorf("failed to insert challenge: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit verification: %w", err)
	}
	return nil
}

// UpdateChallengeDigest writes the HMAC digest for the newest challenge of a
// verification, after the row exists. Called by the usecase once the row id,
// which is the HMAC entity, is known.
func (r *PostgresDiscoveryRepository) UpdateChallengeDigest(ctx context.Context, verificationID uuid.UUID, digest string, expiresAt time.Time) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE verification_challenges
		SET code_hash = $2, expires_at = $3
		WHERE id = (
			SELECT id FROM verification_challenges
			WHERE checkout_verification_id = $1
			ORDER BY created_at DESC LIMIT 1
		)
	`, verificationID, digest, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to update challenge digest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVerificationNotFound
	}
	return nil
}

func (r *PostgresDiscoveryRepository) GetVerification(ctx context.Context, verificationID uuid.UUID) (*domain.CheckoutVerification, string, time.Time, error) {
	var v domain.CheckoutVerification
	var codeHash string
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT v.id, v.user_id, v.event_id, v.verification_type, v.status, v.verified_at, v.expires_at, v.attempt_count,
		       c.code_hash, c.expires_at
		FROM checkout_verifications v
		JOIN verification_challenges c ON c.checkout_verification_id = v.id
		WHERE v.id = $1
		ORDER BY c.created_at DESC LIMIT 1
	`, verificationID).Scan(&v.ID, &v.UserID, &v.EventID, &v.VerificationType, &v.Status, &v.VerifiedAt, &v.ExpiresAt, &v.AttemptCount, &codeHash, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", time.Time{}, ErrVerificationNotFound
		}
		return nil, "", time.Time{}, fmt.Errorf("failed to query verification: %w", err)
	}
	return &v, codeHash, expiresAt, nil
}

func (r *PostgresDiscoveryRepository) IncrementVerificationAttempts(ctx context.Context, verificationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE checkout_verifications SET attempt_count = attempt_count + 1 WHERE id = $1", verificationID)
	if err != nil {
		return fmt.Errorf("failed to increment verification attempts: %w", err)
	}
	return nil
}

func (r *PostgresDiscoveryRepository) IssueGrant(ctx context.Context, grant *domain.CheckoutVerificationGrant) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO checkout_verification_grants (user_id, event_id, order_id, verification_id, scope, expires_at, consumed_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULL)
		RETURNING id
	`, grant.UserID, grant.EventID, grant.OrderID, grant.VerificationID, grant.Scope, grant.ExpiresAt).Scan(&grant.ID)
	if err != nil {
		return fmt.Errorf("failed to insert grant: %w", err)
	}
	return nil
}

func (r *PostgresDiscoveryRepository) ConsumeGrant(ctx context.Context, grantToken string, orderID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE checkout_verification_grants
		SET consumed_at = CURRENT_TIMESTAMP, order_id = $2
		WHERE id = $1::uuid AND consumed_at IS NULL AND expires_at > CURRENT_TIMESTAMP
	`, grantToken, orderID)
	if err != nil {
		return fmt.Errorf("failed to consume grant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGrantNotFound
	}
	return nil
}

func (r *PostgresDiscoveryRepository) CreateOrder(ctx context.Context, order *domain.Order, items []domain.OrderItem) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin order transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO orders (buyer_user_id, status, currency_code, subtotal_idr, discount_idr, total_idr, placed_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		RETURNING id, created_at
	`, order.BuyerUserID, order.Status, order.Currency, order.TotalAmountIDR, int64(0), order.TotalAmountIDR,
	).Scan(&order.ID, &order.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}

	for i := range items {
		items[i].OrderID = order.ID
		_, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, photo_id, seller_user_id, event_id, product_type, unit_price_idr, quantity, line_total_idr)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, items[i].OrderID, items[i].PhotoID, items[i].SellerUserID, items[i].EventID,
			items[i].ProductType, items[i].UnitPriceIDR, items[i].Quantity, items[i].LineTotalIDR)
		if err != nil {
			return fmt.Errorf("failed to insert order item: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO order_status_events (order_id, from_status, to_status, reason_code, actor_user_id)
		VALUES ($1, NULL, $2, 'CREATED', $3)
	`, order.ID, order.Status, order.BuyerUserID); err != nil {
		return fmt.Errorf("failed to insert order status event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit order: %w", err)
	}
	return nil
}

func (r *PostgresDiscoveryRepository) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, []domain.OrderItem, error) {
	var order domain.Order
	err := r.pool.QueryRow(ctx, `
		SELECT id, buyer_user_id, status, currency_code, total_idr, placed_at, created_at
		FROM orders WHERE id = $1
	`, orderID).Scan(&order.ID, &order.BuyerUserID, &order.Status, &order.Currency,
		&order.TotalAmountIDR, &order.CreatedAt, &order.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, fmt.Errorf("failed to query order: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, order_id, photo_id, seller_user_id, event_id, product_type, unit_price_idr, quantity, line_total_idr
		FROM order_items WHERE order_id = $1 ORDER BY id ASC
	`, orderID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query order items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.OrderItem, 0)
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ID, &item.OrderID, &item.PhotoID, &item.SellerUserID, &item.EventID,
			&item.ProductType, &item.UnitPriceIDR, &item.Quantity, &item.LineTotalIDR); err != nil {
			return nil, nil, fmt.Errorf("failed to scan order item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading order items: %w", err)
	}
	return &order, items, nil
}

func (r *PostgresDiscoveryRepository) FindOrderByIdempotencyKey(ctx context.Context, key string, buyerID uuid.UUID) (uuid.UUID, error) {
	var orderID uuid.UUID
	var storedBuyerID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT order_id, buyer_user_id FROM order_idempotency_keys WHERE idempotency_key = $1
	`, key).Scan(&orderID, &storedBuyerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrOrderNotFound
		}
		return uuid.Nil, fmt.Errorf("failed to query idempotency key: %w", err)
	}
	if storedBuyerID != buyerID {
		return uuid.Nil, ErrIdempotencyKeyConflict
	}
	return orderID, nil
}

// ErrOrderNotFound is returned when no order matches the requested identifier.
var ErrOrderNotFound = errors.New("order not found")

// SaveIdempotencyKey records the mapping from one key to its order.
func (r *PostgresDiscoveryRepository) SaveIdempotencyKey(ctx context.Context, key string, buyerID, orderID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO order_idempotency_keys (idempotency_key, buyer_user_id, order_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, key, buyerID, orderID)
	if err != nil {
		return fmt.Errorf("failed to save idempotency key: %w", err)
	}
	return nil
}
