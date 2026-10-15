package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// BankAccountVerificationPending marks an account awaiting verification.
	BankAccountVerificationPending = "PENDING"

	// BankAccountVerificationVerified marks a verified account.
	BankAccountVerificationVerified = "VERIFIED"
)

// HostBankAccount is a payout destination for one Host organization.
// The account number is never exposed in this struct: only ciphertext and the masked form are stored.
type HostBankAccount struct {
	ID                     uuid.UUID  `json:"id"`
	HostOrganizationID     uuid.UUID  `json:"host_organization_id"`
	BankCode               string     `json:"bank_code"`
	AccountNumberEncrypted string     `json:"-"`
	AccountNumberMasked    string     `json:"account_number_masked"`
	AccountHolderName      string     `json:"account_holder_name"`
	VerificationStatus     string     `json:"verification_status"`
	VerifiedAt             *time.Time `json:"verified_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}
