package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
)

// BankAccountUsecase orchestrates Host payout account registration and retrieval.
type BankAccountUsecase interface {
	RegisterAccount(ctx context.Context, actorUserID, orgID uuid.UUID, req RegisterBankAccountRequest) (*domain.HostBankAccount, error)
	ListAccounts(ctx context.Context, orgID uuid.UUID) ([]domain.HostBankAccount, error)
}

// RegisterBankAccountRequest carries the validated registration input.
type RegisterBankAccountRequest struct {
	BankCode          string
	AccountNumber     string
	AccountHolderName string
}

type bankAccountUsecase struct {
	repo   repository.BankAccountRepository
	cipher *security.BankAccountCipher
}

// NewBankAccountUsecase constructs the Host bank account usecase.
func NewBankAccountUsecase(repo repository.BankAccountRepository, cipher *security.BankAccountCipher) BankAccountUsecase {
	return &bankAccountUsecase{repo: repo, cipher: cipher}
}

func (u *bankAccountUsecase) RegisterAccount(ctx context.Context, actorUserID, orgID uuid.UUID, req RegisterBankAccountRequest) (*domain.HostBankAccount, error) {
	req.BankCode = strings.ToUpper(strings.TrimSpace(req.BankCode))
	req.AccountNumber = strings.TrimSpace(req.AccountNumber)
	req.AccountHolderName = strings.TrimSpace(req.AccountHolderName)

	if req.BankCode == "" || req.AccountHolderName == "" {
		return nil, fmt.Errorf("%w: bank_code and account_holder_name are required", ErrInvalidInput)
	}
	if len(req.AccountNumber) < 8 || len(req.AccountNumber) > 34 {
		return nil, fmt.Errorf("%w: account_number must be between 8 and 34 characters", ErrInvalidInput)
	}
	for _, r := range req.AccountNumber {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("%w: account_number must contain digits only", ErrInvalidInput)
		}
	}
	if len(req.BankCode) > 32 || len(req.AccountHolderName) > 128 {
		return nil, fmt.Errorf("%w: bank_code or account_holder_name exceeds the allowed length", ErrInvalidInput)
	}

	encrypted, err := u.cipher.Encrypt(req.AccountNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt account number: %w", err)
	}

	account := &domain.HostBankAccount{
		HostOrganizationID:     orgID,
		BankCode:               req.BankCode,
		AccountNumberEncrypted: encrypted,
		AccountNumberMasked:    security.MaskAccountNumber(req.AccountNumber),
		AccountHolderName:      req.AccountHolderName,
		VerificationStatus:     domain.BankAccountVerificationPending,
	}
	if err := u.repo.CreateBankAccount(ctx, account); err != nil {
		return nil, err
	}

	// Financial mutations are always recorded. A failure here is reported so the
	// caller can retry rather than silently losing the audit trail.
	metadata := fmt.Sprintf(`{"bank_code":%q,"masked":%q}`, account.BankCode, account.AccountNumberMasked)
	if err := u.repo.RecordAuditLog(ctx, actorUserID, "HOST_BANK_ACCOUNT_REGISTERED", "host_bank_account", account.ID, metadata); err != nil {
		return nil, err
	}

	return account, nil
}

func (u *bankAccountUsecase) ListAccounts(ctx context.Context, orgID uuid.UUID) ([]domain.HostBankAccount, error) {
	return u.repo.ListBankAccounts(ctx, orgID)
}
