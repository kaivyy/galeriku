package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	LedgerAccountOwnerPlatform = "PLATFORM"
	LedgerAccountOwnerHost     = "HOST"
	LedgerAccountOwnerCreator  = "CREATOR"
	LedgerAccountOwnerBuyer    = "BUYER"

	LedgerAccountCodeSalesClearing = "SALES_CLEARING"
	LedgerAccountCodeCreatorPayable = "CREATOR_PAYABLE"
	LedgerAccountCodeHostPayable    = "HOST_PAYABLE"
	LedgerAccountCodePlatformRevenue = "PLATFORM_REVENUE"
	LedgerAccountCodeBuyerWallet    = "BUYER_WALLET"

	LedgerDirectionDebit  = "DEBIT"
	LedgerDirectionCredit = "CREDIT"

	LedgerTxTypeSale            = "SALE"
	LedgerTxTypeRefund          = "REFUND"
	LedgerTxTypePayout          = "PAYOUT"
	LedgerTxTypeWalletCashOut   = "WALLET_CASH_OUT"

	LedgerTxStatusPosted = "POSTED"

	SettlementStatusHolding  = "HOLDING"
	SettlementStatusReleased = "RELEASED"

	SettlementHoldDays = 7
)

// LedgerTransaction is one double-entry journal transaction.
type LedgerTransaction struct {
	ID              uuid.UUID  `json:"id"`
	TransactionType string     `json:"transaction_type"`
	SourceType      string     `json:"source_type"`
	SourceID        *uuid.UUID `json:"source_id,omitempty"`
	IdempotencyKey  string     `json:"-"`
	Status          string     `json:"status"`
	PostedAt        *time.Time `json:"posted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// LedgerEntry is one side (debit or credit) of a journal transaction.
type LedgerEntry struct {
	ID          uuid.UUID `json:"id"`
	TransactionID uuid.UUID `json:"ledger_transaction_id"`
	AccountID   uuid.UUID `json:"ledger_account_id"`
	Direction   string    `json:"direction"`
	AmountIDR   int64     `json:"amount_idr"`
}

// LedgerPostingRequest is one balanced journal request.
type LedgerPostingRequest struct {
	IdempotencyKey string
	TransactionType string
	SourceType     string
	SourceID       *uuid.UUID
	// Debits and Credits must sum to the same total.
	Debits  []LedgerLine
	Credits []LedgerLine
}

// LedgerLine is one side of a posting.
type LedgerLine struct {
	OwnerType string
	OwnerID   *uuid.UUID
	AccountCode string
	AmountIDR int64
}

// Settlement is one hold-period tracked seller share.
type Settlement struct {
	ID              uuid.UUID  `json:"id"`
	SettlementBatchID *uuid.UUID `json:"settlement_batch_id,omitempty"`
	BeneficiaryType string     `json:"beneficiary_type"`
	BeneficiaryID   *uuid.UUID `json:"beneficiary_id,omitempty"`
	Status          string     `json:"status"`
	HoldUntil       time.Time  `json:"hold_until"`
	AmountIDR       int64      `json:"amount_idr"`
	CreatedAt       time.Time  `json:"created_at"`
	ReleasedAt      *time.Time `json:"released_at,omitempty"`
}

// RevenueSplit is the deterministic 3-party share of one sale.
type RevenueSplit struct {
	PlatformIDR int64
	HostIDR     int64
	CreatorIDR  int64
}
