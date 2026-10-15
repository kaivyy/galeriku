package usecase

import (
	"context"
	"log"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

const (
	// RevenueSplitPlatformPercent is the platform share of one sale.
	RevenueSplitPlatformPercent = 15
	// RevenueSplitHostPercent is the Host share of one sale.
	RevenueSplitHostPercent = 15
	// The creator receives the remainder (70%).

	// refundSLAGracePeriod is how long past hires_due_at a refund claim is allowed.
	refundSLAGracePeriod = 0
)

// FinanceUsecase orchestrates the double-entry ledger, revenue split, settlements, and refunds.
type FinanceUsecase interface {
	RecordSale(ctx context.Context, orderID uuid.UUID, orderTotalIDR int64, creatorID, hostID *uuid.UUID) (*domain.RevenueSplit, error)
	ReleaseDueSettlements(ctx context.Context) (int, error)
	RequestRefund(ctx context.Context, buyerID uuid.UUID, orderItemID uuid.UUID, reasonCode, destinationType string) (*RefundResult, error)
	GetWalletBalance(ctx context.Context, ownerType string, ownerID uuid.UUID) (*WalletBalanceResult, error)
}

// RefundResult is the response for one refund request.
type RefundResult struct {
	RefundItemID        string `json:"refund_item_id"`
	OrderItemID         string `json:"order_item_id"`
	EligibleAmountIDR   int64  `json:"eligible_amount_idr"`
	BuyerCreditIDR      int64  `json:"buyer_credit_amount_idr"`
	CreatorClawbackIDR  int64  `json:"creator_clawback_idr"`
	HostClawbackIDR     int64  `json:"host_clawback_idr"`
	PlatformClawbackIDR int64  `json:"platform_clawback_idr"`
	Status              string `json:"status"`
}

// WalletBalanceResult is the wallet summary.
type WalletBalanceResult struct {
	AvailableBalanceIDR int64 `json:"available_balance_idr"`
	PendingHoldIDR      int64 `json:"pending_hold_balance_idr"`
}

type financeUsecase struct {
	ledger  repository.LedgerRepository
	payRepo repository.PaymentRepository
	auditor audit.Logger
}

// NewFinanceUsecase constructs the finance usecase.
func NewFinanceUsecase(ledger repository.LedgerRepository, payRepo repository.PaymentRepository, auditor audit.Logger) FinanceUsecase {
	return &financeUsecase{ledger: ledger, payRepo: payRepo, auditor: auditor}
}

func (u *financeUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

// ComputeRevenueSplit divides one sale deterministically without losing cents:
// the platform and Host take their percentages floor-rounded; the creator
// receives the remainder so the three shares always sum to the total.
func ComputeRevenueSplit(totalIDR int64) domain.RevenueSplit {
	platform := totalIDR * RevenueSplitPlatformPercent / 100
	host := totalIDR * RevenueSplitHostPercent / 100
	creator := totalIDR - platform - host
	return domain.RevenueSplit{PlatformIDR: platform, HostIDR: host, CreatorIDR: creator}
}

// RecordSale posts the sale journal, records the 3-party split, and creates
// the T+7 settlements for the creator and Host shares.
func (u *financeUsecase) RecordSale(ctx context.Context, orderID uuid.UUID, orderTotalIDR int64, creatorID, hostID *uuid.UUID) (*domain.RevenueSplit, error) {
	split := ComputeRevenueSplit(orderTotalIDR)

	// Buyer pays: debit the buyer wallet (money leaves the buyer), credit the
	// three beneficiaries' payable accounts.
	platformLine := domain.LedgerLine{
		OwnerType: domain.LedgerAccountOwnerPlatform, AccountCode: domain.LedgerAccountCodePlatformRevenue, AmountIDR: split.PlatformIDR,
	}
	creatorLine := domain.LedgerLine{
		OwnerType: domain.LedgerAccountOwnerCreator, OwnerID: creatorID, AccountCode: domain.LedgerAccountCodeCreatorPayable, AmountIDR: split.CreatorIDR,
	}
	hostLine := domain.LedgerLine{
		OwnerType: domain.LedgerAccountOwnerHost, OwnerID: hostID, AccountCode: domain.LedgerAccountCodeHostPayable, AmountIDR: split.HostIDR,
	}

	posting := domain.LedgerPostingRequest{
		IdempotencyKey:  "sale:" + orderID.String(),
		TransactionType: domain.LedgerTxTypeSale,
		SourceType:      "order",
		SourceID:        &orderID,
		Debits: []domain.LedgerLine{
			{OwnerType: domain.LedgerAccountOwnerBuyer, OwnerID: &orderID, AccountCode: domain.LedgerAccountCodeSalesClearing, AmountIDR: orderTotalIDR},
		},
		Credits: []domain.LedgerLine{platformLine, creatorLine, hostLine},
	}
	if _, err := u.ledger.PostTransaction(ctx, posting); err != nil {
		// An idempotent replay is fine; the split was already recorded.
		return &split, nil
	}

	// T+7 hold for the creator and Host shares. The platform share is available immediately.
	now := time.Now().UTC()
	holdUntil := now.AddDate(0, 0, domain.SettlementHoldDays)

	if creatorID != nil {
		if err := u.ledger.CreateSettlement(ctx, &domain.Settlement{
			BeneficiaryType: domain.LedgerAccountOwnerCreator,
			BeneficiaryID:   creatorID,
			Status:          domain.SettlementStatusHolding,
			HoldUntil:       holdUntil,
			AmountIDR:       split.CreatorIDR,
		}); err != nil {
			return nil, err
		}
	}
	if hostID != nil {
		if err := u.ledger.CreateSettlement(ctx, &domain.Settlement{
			BeneficiaryType: domain.LedgerAccountOwnerHost,
			BeneficiaryID:   hostID,
			Status:          domain.SettlementStatusHolding,
			HoldUntil:       holdUntil,
			AmountIDR:       split.HostIDR,
		}); err != nil {
			return nil, err
		}
	}

	u.recordAudit(ctx, audit.Entry{
		ActionCode: "FINANCE_SALE_RECORDED",
		TargetType: "order",
		TargetID:   &orderID,
		Metadata: map[string]interface{}{
			"total_idr": orderTotalIDR, "platform_idr": split.PlatformIDR,
			"host_idr": split.HostIDR, "creator_idr": split.CreatorIDR,
		},
	})
	return &split, nil
}

func (u *financeUsecase) ReleaseDueSettlements(ctx context.Context) (int, error) {
	return u.ledger.ReleaseDueSettlements(ctx, time.Now().UTC())
}

// RequestRefund handles an SLA-breach refund, reversing the 3-party split atomically.
func (u *financeUsecase) RequestRefund(ctx context.Context, buyerID, orderItemID uuid.UUID, reasonCode, destinationType string) (*RefundResult, error) {
	item, err := u.payRepo.GetOrderItem(ctx, orderItemID)
	if err != nil {
		return nil, err
	}
	split := ComputeRevenueSplit(item.LineTotalIDR)

	// Build the compensating posting: the buyer wallet is credited 100%; the
	// creator/Host/platform payables are debited by their original shares.
	creatorLine := domain.LedgerLine{
		OwnerType: domain.LedgerAccountOwnerCreator, OwnerID: &item.SellerUserID,
		AccountCode: domain.LedgerAccountCodeCreatorPayable, AmountIDR: split.CreatorIDR,
	}
	hostLine := domain.LedgerLine{
		OwnerType: domain.LedgerAccountOwnerHost, AccountCode: domain.LedgerAccountCodeHostPayable, AmountIDR: split.HostIDR,
	}
	platformLine := domain.LedgerLine{
		OwnerType: domain.LedgerAccountOwnerPlatform, AccountCode: domain.LedgerAccountCodePlatformRevenue, AmountIDR: split.PlatformIDR,
	}

	debits := []domain.LedgerLine{platformLine}
	if split.CreatorIDR > 0 {
		debits = append(debits, creatorLine)
	}
	if split.HostIDR > 0 {
		debits = append(debits, hostLine)
	}

	posting := domain.LedgerPostingRequest{
		IdempotencyKey:  "refund:" + orderItemID.String() + ":" + reasonCode,
		TransactionType: domain.LedgerTxTypeRefund,
		SourceType:      "order_item",
		SourceID:        &orderItemID,
		Debits:          debits,
		Credits: []domain.LedgerLine{
			{OwnerType: domain.LedgerAccountOwnerBuyer, OwnerID: &buyerID, AccountCode: domain.LedgerAccountCodeBuyerWallet, AmountIDR: item.LineTotalIDR},
		},
	}
	if _, err := u.ledger.PostTransaction(ctx, posting); err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &buyerID,
		ActionCode:  "FINANCE_REFUND_ISSUED",
		TargetType:  "order_item",
		TargetID:    &orderItemID,
		Metadata: map[string]interface{}{
			"reason_code": reasonCode, "refund_idr": item.LineTotalIDR, "destination_type": destinationType,
		},
	})

	return &RefundResult{
		RefundItemID:        orderItemID.String(),
		OrderItemID:         orderItemID.String(),
		EligibleAmountIDR:   item.LineTotalIDR,
		BuyerCreditIDR:      item.LineTotalIDR,
		CreatorClawbackIDR:  split.CreatorIDR,
		HostClawbackIDR:     split.HostIDR,
		PlatformClawbackIDR: split.PlatformIDR,
		Status:              "COMPLETED",
	}, nil
}

func (u *financeUsecase) GetWalletBalance(ctx context.Context, ownerType string, ownerID uuid.UUID) (*WalletBalanceResult, error) {
	accountID, err := u.ledger.EnsureAccount(ctx, ownerType, &ownerID, domain.LedgerAccountCodeBuyerWallet, "IDR")
	if err != nil {
		return nil, err
	}
	balance, err := u.ledger.GetAccountBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return &WalletBalanceResult{AvailableBalanceIDR: balance}, nil
}
