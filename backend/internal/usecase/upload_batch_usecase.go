package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

// UploadBatchUsecase orchestrates quota reads and atomic mass upload reservations.
type UploadBatchUsecase interface {
	GetQuota(ctx context.Context, orgID uuid.UUID) (*QuotaResult, error)
	InitiateBatch(ctx context.Context, actorUserID, orgID uuid.UUID, req InitiateBatchRequest) (*InitiateBatchResult, error)
}

// QuotaResult is the response payload for GET /storage/quota.
type QuotaResult struct {
	QuotaLimitBytes int64 `json:"quota_limit_bytes"`
	BillableBytes   int64 `json:"billable_bytes"`
	ReservedBytes   int64 `json:"reserved_bytes"`
	RemainingBytes  int64 `json:"remaining_bytes"`
}

// InitiateBatchRequest is the validated input for POST /storage/upload-batches/initiate.
type InitiateBatchRequest struct {
	TotalFiles int
	TotalBytes int64
	Files      []domain.UploadBatchRequest
}

// InitiateBatchResult is the response payload for a successful reservation.
type InitiateBatchResult struct {
	UploadBatchID        string `json:"upload_batch_id"`
	Status               string `json:"status"`
	ReservedBytes        int64  `json:"reserved_bytes"`
	ReservationExpiresAt string `json:"reservation_expires_at,omitempty"`
}

type uploadBatchUsecase struct {
	repo    repository.UploadRepository
	auditor audit.Logger
}

// NewUploadBatchUsecase constructs the upload batch usecase.
// auditor may be nil, in which case reservations proceed without audit records.
func NewUploadBatchUsecase(repo repository.UploadRepository, auditor audit.Logger) UploadBatchUsecase {
	return &uploadBatchUsecase{repo: repo, auditor: auditor}
}

func (u *uploadBatchUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

func (u *uploadBatchUsecase) GetQuota(ctx context.Context, orgID uuid.UUID) (*QuotaResult, error) {
	quota, err := u.repo.GetQuotaByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &QuotaResult{
		QuotaLimitBytes: quota.QuotaLimitBytes,
		BillableBytes:   quota.BillableBytes,
		ReservedBytes:   quota.ReservedBytes,
		RemainingBytes:  quota.RemainingBytes(),
	}, nil
}

func (u *uploadBatchUsecase) InitiateBatch(ctx context.Context, actorUserID, orgID uuid.UUID, req InitiateBatchRequest) (*InitiateBatchResult, error) {
	if req.TotalFiles < 1 {
		return nil, fmt.Errorf("%w: total_files must be at least 1", ErrInvalidInput)
	}
	if req.TotalFiles > domain.MaxFilesPerBatch {
		return nil, fmt.Errorf("%w: total_files must not exceed %d", ErrInvalidInput, domain.MaxFilesPerBatch)
	}
	if req.TotalBytes <= 0 {
		return nil, fmt.Errorf("%w: total_bytes must be positive", ErrInvalidInput)
	}
	if len(req.Files) != req.TotalFiles {
		return nil, fmt.Errorf("%w: files must contain exactly total_files entries", ErrInvalidInput)
	}

	// Each file slot carries the client's own idempotency token, and the expected sizes
	// must add up to the declared total so the reservation matches what will be uploaded.
	var sumBytes int64
	seenTokens := make(map[string]bool, len(req.Files))
	for i, f := range req.Files {
		f.ClientFileToken = strings.TrimSpace(f.ClientFileToken)
		if f.ClientFileToken == "" {
			return nil, fmt.Errorf("%w: files[%d].client_file_token is required", ErrInvalidInput, i)
		}
		if seenTokens[f.ClientFileToken] {
			return nil, fmt.Errorf("%w: duplicate client_file_token %q", ErrInvalidInput, f.ClientFileToken)
		}
		seenTokens[f.ClientFileToken] = true
		if f.ExpectedBytes <= 0 {
			return nil, fmt.Errorf("%w: files[%d].expected_bytes must be positive", ErrInvalidInput, i)
		}
		sumBytes += f.ExpectedBytes
	}
	if sumBytes != req.TotalBytes {
		return nil, fmt.Errorf("%w: total_bytes must equal the sum of expected_bytes", ErrInvalidInput)
	}

	batch := &domain.UploadBatch{
		Status:            domain.UploadBatchStatusReserved,
		ExpectedFileCount: req.TotalFiles,
		ExpectedBytes:     req.TotalBytes,
	}
	items := make([]domain.UploadItem, 0, len(req.Files))
	for _, f := range req.Files {
		items = append(items, domain.UploadItem{
			ClientFileToken: f.ClientFileToken,
			ExpectedBytes:   f.ExpectedBytes,
			Status:          domain.UploadItemStatusPending,
		})
	}

	if err := u.repo.ReserveBatch(ctx, orgID, actorUserID, batch, items); err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "UPLOAD_BATCH_RESERVED",
		TargetType:  "upload_batch",
		TargetID:    &batch.ID,
		Metadata: map[string]interface{}{
			"storage_account_id": batch.StorageAccountID.String(),
			"expected_bytes":     batch.ExpectedBytes,
			"expected_files":     batch.ExpectedFileCount,
		},
	})

	result := &InitiateBatchResult{
		UploadBatchID: batch.ID.String(),
		Status:        batch.Status,
		ReservedBytes: batch.ExpectedBytes,
	}
	if batch.ExpiresAt != nil {
		result.ReservationExpiresAt = batch.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return result, nil
}
