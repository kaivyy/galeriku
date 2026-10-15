package repository

import (
	"context"
	"fmt"

	"github.com/galeriku/core/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BankAccountRepository defines data access for Host payout accounts.
type BankAccountRepository interface {
	CreateBankAccount(ctx context.Context, account *domain.HostBankAccount) error
	ListBankAccounts(ctx context.Context, orgID uuid.UUID) ([]domain.HostBankAccount, error)
	RecordAuditLog(ctx context.Context, actorUserID uuid.UUID, actionCode, targetType string, targetID uuid.UUID, metadata string) error
}

// PostgresBankAccountRepository implements BankAccountRepository backed by PostgreSQL 16.
type PostgresBankAccountRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresBankAccountRepository constructs a new PostgresBankAccountRepository.
func NewPostgresBankAccountRepository(pool *pgxpool.Pool) *PostgresBankAccountRepository {
	return &PostgresBankAccountRepository{pool: pool}
}

// CreateBankAccount persists one encrypted payout account.
func (r *PostgresBankAccountRepository) CreateBankAccount(ctx context.Context, account *domain.HostBankAccount) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO host_bank_accounts (host_organization_id, bank_code, account_number_encrypted, account_number_masked, account_holder_name, verification_status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, account.HostOrganizationID, account.BankCode, account.AccountNumberEncrypted, account.AccountNumberMasked,
		account.AccountHolderName, account.VerificationStatus,
	).Scan(&account.ID, &account.CreatedAt, &account.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert host bank account: %w", err)
	}
	return nil
}

// ListBankAccounts returns the masked payout accounts of one organization, newest first.
func (r *PostgresBankAccountRepository) ListBankAccounts(ctx context.Context, orgID uuid.UUID) ([]domain.HostBankAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, host_organization_id, bank_code, account_number_masked, account_holder_name,
		       verification_status, verified_at, created_at, updated_at
		FROM host_bank_accounts
		WHERE host_organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list host bank accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]domain.HostBankAccount, 0)
	for rows.Next() {
		var a domain.HostBankAccount
		if err := rows.Scan(&a.ID, &a.HostOrganizationID, &a.BankCode, &a.AccountNumberMasked, &a.AccountHolderName,
			&a.VerificationStatus, &a.VerifiedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan host bank account: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading bank account rows: %w", err)
	}
	return accounts, nil
}

// RecordAuditLog writes one security audit entry for a financial mutation.
func (r *PostgresBankAccountRepository) RecordAuditLog(ctx context.Context, actorUserID uuid.UUID, actionCode, targetType string, targetID uuid.UUID, metadata string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_code, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, actorUserID, actionCode, targetType, targetID, metadata)
	if err != nil {
		return fmt.Errorf("failed to write audit log: %w", err)
	}
	return nil
}
