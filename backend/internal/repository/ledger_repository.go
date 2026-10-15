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
	// ErrLedgerImbalance is returned when a posting's debits do not equal credits.
	ErrLedgerImbalance = errors.New("ledger posting is unbalanced: SUM(debit) != SUM(credit)")

	// ErrLedgerAccountNotFound is returned when an account cannot be resolved.
	ErrLedgerAccountNotFound = errors.New("ledger account not found")

	// ErrSettlementNotFound is returned when no settlement matches the requested identifier.
	ErrSettlementNotFound = errors.New("settlement not found")

	// ErrSettlementNotReleased is returned when a settlement is still in hold.
	ErrSettlementNotReleased = errors.New("settlement is still in hold period")

	// ErrPayoutNotFound is returned when no payout matches the requested identifier.
	ErrPayoutNotFound = errors.New("payout not found")
)

// LedgerRepository defines data access for the double-entry ledger and settlements.
type LedgerRepository interface {
	EnsureAccount(ctx context.Context, ownerType string, ownerID *uuid.UUID, accountCode, currency string) (uuid.UUID, error)
	PostTransaction(ctx context.Context, req domain.LedgerPostingRequest) (*domain.LedgerTransaction, error)
	GetAccountBalance(ctx context.Context, accountID uuid.UUID) (int64, error)
	CreateSettlement(ctx context.Context, s *domain.Settlement) error
	ReleaseDueSettlements(ctx context.Context, now time.Time) (int, error)
	GetSettlementByID(ctx context.Context, id uuid.UUID) (*domain.Settlement, error)
}

// PostgresLedgerRepository implements LedgerRepository backed by PostgreSQL 16.
type PostgresLedgerRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresLedgerRepository constructs a new PostgresLedgerRepository.
func NewPostgresLedgerRepository(pool *pgxpool.Pool) *PostgresLedgerRepository {
	return &PostgresLedgerRepository{pool: pool}
}

func (r *PostgresLedgerRepository) EnsureAccount(ctx context.Context, ownerType string, ownerID *uuid.UUID, accountCode, currency string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM ledger_accounts
		WHERE owner_type = $1
		  AND COALESCE(owner_id::text, '') = COALESCE($2::text, '')
		  AND account_code = $3
		  AND currency_code = $4
	`, ownerType, ownerID, accountCode, currency).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("failed to query ledger account: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO ledger_accounts (owner_type, owner_id, account_code, currency_code, status)
		VALUES ($1, $2, $3, $4, 'ACTIVE')
		RETURNING id
	`, ownerType, ownerID, accountCode, currency).Scan(&id)
	if err != nil {
		err = r.pool.QueryRow(ctx, `
			SELECT id FROM ledger_accounts
			WHERE owner_type = $1
			  AND COALESCE(owner_id::text, '') = COALESCE($2::text, '')
			  AND account_code = $3
			  AND currency_code = $4
		`, ownerType, ownerID, accountCode, currency).Scan(&id)
		if err != nil {
			return uuid.Nil, ErrLedgerAccountNotFound
		}
	}
	return id, nil
}

// PostTransaction posts one balanced double-entry transaction.
// It rejects any request where the total debit does not equal the total credit.
func (r *PostgresLedgerRepository) PostTransaction(ctx context.Context, req domain.LedgerPostingRequest) (*domain.LedgerTransaction, error) {
	var debitSum, creditSum int64
	for _, l := range req.Debits {
		debitSum += l.AmountIDR
	}
	for _, l := range req.Credits {
		creditSum += l.AmountIDR
	}
	if debitSum != creditSum || debitSum == 0 {
		return nil, ErrLedgerImbalance
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin ledger transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var txn domain.LedgerTransaction
	err = tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (transaction_type, source_type, source_id, idempotency_key, status, posted_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		RETURNING id, created_at
	`, req.TransactionType, req.SourceType, req.SourceID, req.IdempotencyKey, domain.LedgerTxStatusPosted,
	).Scan(&txn.ID, &txn.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("ledger transaction with idempotency key %q already exists", req.IdempotencyKey)
		}
		return nil, fmt.Errorf("failed to insert ledger transaction: %w", err)
	}

	for _, line := range req.Debits {
		accountID, err := r.ensureAccountTx(ctx, tx, line)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (ledger_transaction_id, ledger_account_id, direction, amount_idr)
			VALUES ($1, $2, $3, $4)
		`, txn.ID, accountID, domain.LedgerDirectionDebit, line.AmountIDR); err != nil {
			return nil, fmt.Errorf("failed to insert debit entry: %w", err)
		}
	}
	for _, line := range req.Credits {
		accountID, err := r.ensureAccountTx(ctx, tx, line)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (ledger_transaction_id, ledger_account_id, direction, amount_idr)
			VALUES ($1, $2, $3, $4)
		`, txn.ID, accountID, domain.LedgerDirectionCredit, line.AmountIDR); err != nil {
			return nil, fmt.Errorf("failed to insert credit entry: %w", err)
		}
	}

	// Enforce the invariant at the database level: SUM(debit) == SUM(credit) for this transaction.
	var debitTotal, creditTotal int64
	err = tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount_idr ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN direction = 'CREDIT' THEN amount_idr ELSE 0 END), 0)
		FROM ledger_entries WHERE ledger_transaction_id = $1
	`, txn.ID).Scan(&debitTotal, &creditTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ledger balance: %w", err)
	}
	if debitTotal != creditTotal {
		return nil, fmt.Errorf("database-level invariant violation: debit=%d credit=%d: %w", debitTotal, creditTotal, ErrLedgerImbalance)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit ledger transaction: %w", err)
	}
	txn.TransactionType = req.TransactionType
	txn.SourceType = req.SourceType
	txn.SourceID = req.SourceID
	txn.IdempotencyKey = req.IdempotencyKey
	txn.Status = domain.LedgerTxStatusPosted
	return &txn, nil
}

func (r *PostgresLedgerRepository) ensureAccountTx(ctx context.Context, tx pgx.Tx, line domain.LedgerLine) (uuid.UUID, error) {
	// Find-first: the unique expression index (migration 000005) prevents
	// duplicates, but ON CONFLICT DO NOTHING without an explicit target
	// does not fire on expression indexes. Find-then-insert is race-safe
	// inside the transaction because the index rejects the second insert.
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM ledger_accounts
		WHERE owner_type = $1
		  AND COALESCE(owner_id::text, '') = COALESCE($2::text, '')
		  AND account_code = $3
		  AND currency_code = 'IDR'
	`, line.OwnerType, line.OwnerID, line.AccountCode).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("failed to query ledger account: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO ledger_accounts (owner_type, owner_id, account_code, currency_code, status)
		VALUES ($1, $2, $3, 'IDR', 'ACTIVE')
		RETURNING id
	`, line.OwnerType, line.OwnerID, line.AccountCode).Scan(&id)
	if err != nil {
		// Race: another concurrent insert won; fetch it.
		err = tx.QueryRow(ctx, `
			SELECT id FROM ledger_accounts
			WHERE owner_type = $1
			  AND COALESCE(owner_id::text, '') = COALESCE($2::text, '')
			  AND account_code = $3
			  AND currency_code = 'IDR'
		`, line.OwnerType, line.OwnerID, line.AccountCode).Scan(&id)
		if err != nil {
			return uuid.Nil, ErrLedgerAccountNotFound
		}
	}
	return id, nil
}

func (r *PostgresLedgerRepository) GetAccountBalance(ctx context.Context, accountID uuid.UUID) (int64, error) {
	var balance int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN direction = 'DEBIT' THEN amount_idr ELSE -amount_idr END), 0)
		FROM ledger_entries WHERE ledger_account_id = $1
	`, accountID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("failed to query account balance: %w", err)
	}
	return balance, nil
}

func (r *PostgresLedgerRepository) CreateSettlement(ctx context.Context, s *domain.Settlement) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO settlements (settlement_batch_id, beneficiary_type, beneficiary_id, status, hold_until, amount_idr)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, s.SettlementBatchID, s.BeneficiaryType, s.BeneficiaryID, s.Status, s.HoldUntil, s.AmountIDR,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert settlement: %w", err)
	}
	return nil
}

// ReleaseDueSettlements moves all settlements past their hold_until to RELEASED status.
func (r *PostgresLedgerRepository) ReleaseDueSettlements(ctx context.Context, now time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE settlements
		SET status = $2, released_at = CURRENT_TIMESTAMP
		WHERE status = $3 AND hold_until < $1
	`, now, domain.SettlementStatusReleased, domain.SettlementStatusHolding)
	if err != nil {
		return 0, fmt.Errorf("failed to release settlements: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PostgresLedgerRepository) GetSettlementByID(ctx context.Context, id uuid.UUID) (*domain.Settlement, error) {
	var s domain.Settlement
	err := r.pool.QueryRow(ctx, `
		SELECT id, settlement_batch_id, beneficiary_type, beneficiary_id, status, hold_until, amount_idr, created_at, released_at
		FROM settlements WHERE id = $1
	`, id).Scan(&s.ID, &s.SettlementBatchID, &s.BeneficiaryType, &s.BeneficiaryID, &s.Status, &s.HoldUntil, &s.AmountIDR, &s.CreatedAt, &s.ReleasedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSettlementNotFound
		}
		return nil, fmt.Errorf("failed to query settlement: %w", err)
	}
	return &s, nil
}
