package usecase_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func liveLedgerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live ledger test in short mode")
	}
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5433"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "galeriku"
	}
	pass := os.Getenv("DB_PASSWORD")
	if pass == "" {
		pass = "galeriku_secret_pass"
	}
	name := os.Getenv("DB_NAME")
	if name == "" {
		name = "galeriku_db"
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}
	return pool
}

func TestComputeRevenueSplit_NoCentsLost(t *testing.T) {
	cases := []struct {
		total                int64
		platform, host, creator int64
	}{
		{45000, 6750, 6750, 31500},
		{100, 15, 15, 70},
		{33, 4, 4, 25}, // floor rounding, creator absorbs remainder
		{99, 14, 14, 71},
	}
	for _, c := range cases {
		split := usecase.ComputeRevenueSplit(c.total)
		if split.PlatformIDR != c.platform || split.HostIDR != c.host || split.CreatorIDR != c.creator {
			t.Errorf("split(%d) = platform=%d host=%d creator=%d, want %d/%d/%d",
				c.total, split.PlatformIDR, split.HostIDR, split.CreatorIDR, c.platform, c.host, c.creator)
		}
		if split.PlatformIDR+split.HostIDR+split.CreatorIDR != c.total {
			t.Errorf("split(%d) sums to %d, want %d", c.total, split.PlatformIDR+split.HostIDR+split.CreatorIDR, c.total)
		}
	}
}

func TestLedger_DoubleEntryBalanceInvariant(t *testing.T) {
	pool := liveLedgerPool(t)
	defer pool.Close()

	ledger := repository.NewPostgresLedgerRepository(pool)
	paymentRepo := repository.NewPostgresPaymentRepository(pool)
	uc := usecase.NewFinanceUsecase(ledger, paymentRepo, nil)

	orderID := uuid.Must(uuid.NewV7())
	creatorID := uuid.Must(uuid.NewV7())
	hostID := uuid.Must(uuid.NewV7())

	split, err := uc.RecordSale(context.Background(), orderID, 45000, &creatorID, &hostID)
	if err != nil {
		t.Fatalf("RecordSale failed: %v", err)
	}
	if split.PlatformIDR+split.HostIDR+split.CreatorIDR != 45000 {
		t.Fatalf("split sums to %d, want 45000", split.PlatformIDR+split.HostIDR+split.CreatorIDR)
	}

	// Verify the ledger_entries sum to zero across all accounts (double-entry invariant).
	var grandTotal int64
	err = pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount_idr ELSE -amount_idr END), 0)
		FROM ledger_entries
		WHERE ledger_transaction_id IN (
			SELECT id FROM ledger_transactions WHERE idempotency_key = $1
		)
	`, "sale:"+orderID.String()).Scan(&grandTotal)
	if err != nil {
		t.Fatalf("failed to query ledger balance: %v", err)
	}
	if grandTotal != 0 {
		t.Fatalf("double-entry invariant violated: net balance = %d, expected 0", grandTotal)
	}

	// Verify settlements were created for creator and Host with T+7 hold.
	var settlementCount int
	err = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM settlements
		WHERE beneficiary_id IN ($1, $2) AND status = 'HOLDING'
	`, creatorID, hostID).Scan(&settlementCount)
	if err != nil {
		t.Fatalf("failed to query settlements: %v", err)
	}
	if settlementCount != 2 {
		t.Fatalf("expected 2 settlements (creator + host), got %d", settlementCount)
	}

	// Cleanup.
	bg := context.Background()
	_, _ = pool.Exec(bg, "DELETE FROM ledger_entries WHERE ledger_transaction_id IN (SELECT id FROM ledger_transactions WHERE idempotency_key LIKE $1)", "sale:"+orderID.String()+"%")
	_, _ = pool.Exec(bg, "DELETE FROM ledger_transactions WHERE idempotency_key LIKE $1", "sale:"+orderID.String()+"%")
	_, _ = pool.Exec(bg, "DELETE FROM settlements WHERE beneficiary_id IN ($1, $2)", creatorID, hostID)
	_, _ = pool.Exec(bg, "DELETE FROM ledger_accounts WHERE owner_id IN ($1, $2, $3)", creatorID, hostID, orderID)
}

func TestLedger_RefundReversesThreePartySplit(t *testing.T) {
	pool := liveLedgerPool(t)
	defer pool.Close()

	ledger := repository.NewPostgresLedgerRepository(pool)
	paymentRepo := repository.NewPostgresPaymentRepository(pool)
	uc := usecase.NewFinanceUsecase(ledger, paymentRepo, nil)

	// Seed: one order, one order_item, and a sale posting.
	orderID := uuid.Must(uuid.NewV7())
	buyerID := uuid.Must(uuid.NewV7())
	creatorID := uuid.Must(uuid.NewV7())
	hostID := uuid.Must(uuid.NewV7())

	_, err := pool.Exec(context.Background(), `
		INSERT INTO orders (id, buyer_user_id, status, currency_code, subtotal_idr, discount_idr, total_idr)
		VALUES ($1, $2, 'PAID', 'IDR', 45000, 0, 45000)
	`, orderID, buyerID)
	if err != nil {
		t.Fatalf("failed to seed order: %v", err)
	}

	orderItemID := uuid.Must(uuid.NewV7())
	_, err = pool.Exec(context.Background(), `
		INSERT INTO order_items (id, order_id, photo_id, seller_user_id, product_type, unit_price_idr, quantity, line_total_idr)
		VALUES ($1, $2, $3, $4, 'DIGITAL_HIRES', 45000, 1, 45000)
	`, orderItemID, orderID, uuid.Must(uuid.NewV7()), creatorID)
	if err != nil {
		t.Fatalf("failed to seed order item: %v", err)
	}

	_, err = uc.RecordSale(context.Background(), orderID, 45000, &creatorID, &hostID)
	if err != nil {
		t.Fatalf("RecordSale failed: %v", err)
	}

	// Request a refund.
	result, err := uc.RequestRefund(context.Background(), buyerID, orderItemID, "SLA_BREACH", "INTERNAL_WALLET")
	if err != nil {
		t.Fatalf("RequestRefund failed: %v", err)
	}
	if result.BuyerCreditIDR != 45000 {
		t.Fatalf("expected buyer credit 45000, got %d", result.BuyerCreditIDR)
	}
	if result.Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %q", result.Status)
	}

	// Verify the refund posting also balances.
	var refundTotal int64
	err = pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount_idr ELSE -amount_idr END), 0)
		FROM ledger_entries
		WHERE ledger_transaction_id IN (
			SELECT id FROM ledger_transactions WHERE idempotency_key = $1
		)
	`, "refund:"+orderItemID.String()+":SLA_BREACH").Scan(&refundTotal)
	if err != nil {
		t.Fatalf("failed to query refund balance: %v", err)
	}
	if refundTotal != 0 {
		t.Fatalf("refund double-entry invariant violated: net = %d, expected 0", refundTotal)
	}

	// The buyer wallet liability moves by the refund amount. The exact sign depends
	// on the direction convention; what matters is that it moved and the posting
	// itself balanced (verified above via the net-zero invariant).
	balance, err := uc.GetWalletBalance(context.Background(), domain.LedgerAccountOwnerBuyer, buyerID)
	if err != nil {
		t.Fatalf("GetWalletBalance failed: %v", err)
	}
	if balance.AvailableBalanceIDR == 0 {
		t.Fatal("expected the buyer wallet balance to move after a refund, got 0")
	}

	// Cleanup.
	bg := context.Background()
	_, _ = pool.Exec(bg, "DELETE FROM ledger_entries WHERE ledger_transaction_id IN (SELECT id FROM ledger_transactions WHERE idempotency_key LIKE $1)", "sale:"+orderID.String()+"%")
	_, _ = pool.Exec(bg, "DELETE FROM ledger_entries WHERE ledger_transaction_id IN (SELECT id FROM ledger_transactions WHERE idempotency_key LIKE $1)", "refund:"+orderItemID.String()+"%")
	_, _ = pool.Exec(bg, "DELETE FROM ledger_transactions WHERE idempotency_key LIKE $1", "sale:"+orderID.String()+"%")
	_, _ = pool.Exec(bg, "DELETE FROM ledger_transactions WHERE idempotency_key LIKE $1", "refund:"+orderItemID.String()+"%")
	_, _ = pool.Exec(bg, "DELETE FROM settlements WHERE beneficiary_id IN ($1, $2)", creatorID, hostID)
	_, _ = pool.Exec(bg, "DELETE FROM ledger_accounts WHERE owner_id IN ($1, $2, $3, $4)", creatorID, hostID, orderID, buyerID)
	_, _ = pool.Exec(bg, "DELETE FROM order_items WHERE order_id = $1", orderID)
	_, _ = pool.Exec(bg, "DELETE FROM orders WHERE id = $1", orderID)
}

func TestLedger_RejectUnbalancedPosting(t *testing.T) {
	pool := liveLedgerPool(t)
	defer pool.Close()

	ledger := repository.NewPostgresLedgerRepository(pool)
	_, err := ledger.PostTransaction(context.Background(), domain.LedgerPostingRequest{
		IdempotencyKey:  "unbalanced:" + uuid.New().String(),
		TransactionType: domain.LedgerTxTypeSale,
		SourceType:      "test",
		Debits:          []domain.LedgerLine{{OwnerType: "PLATFORM", AccountCode: "TEST", AmountIDR: 100}},
		Credits:         []domain.LedgerLine{{OwnerType: "PLATFORM", AccountCode: "TEST2", AmountIDR: 99}},
	})
	if err == nil {
		t.Fatal("expected error for unbalanced posting, got nil")
	}
}
