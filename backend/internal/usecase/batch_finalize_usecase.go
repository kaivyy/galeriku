package usecase

import (
	"context"
	"log"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/messaging"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/google/uuid"
)

// BatchFinalizeUsecase reconciles a completed mass upload batch and dispatches its
// objects to the asynchronous AI pipeline.
type BatchFinalizeUsecase interface {
	FinalizeBatch(ctx context.Context, actorUserID, orgID, batchID uuid.UUID) (*domain.BatchFinalization, error)
}

type batchFinalizeUsecase struct {
	repo      repository.UploadRepository
	s3        *storage.S3Service
	publisher messaging.Publisher
	auditor   audit.Logger
}

// NewBatchFinalizeUsecase constructs the batch finalization usecase.
// publisher and auditor may be nil, in which case dispatch or audit records are skipped.
func NewBatchFinalizeUsecase(repo repository.UploadRepository, s3 *storage.S3Service, publisher messaging.Publisher, auditor audit.Logger) BatchFinalizeUsecase {
	return &batchFinalizeUsecase{repo: repo, s3: s3, publisher: publisher, auditor: auditor}
}

func (u *batchFinalizeUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

func (u *batchFinalizeUsecase) FinalizeBatch(ctx context.Context, actorUserID, orgID, batchID uuid.UUID) (*domain.BatchFinalization, error) {
	// Ownership is established before any state changes: the batch must belong to the
	// caller's organization storage account.
	batch, err := u.repo.GetBatchByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	quota, err := u.repo.GetQuotaByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if batch.StorageAccountID != quota.StorageAccountID {
		return nil, ErrForbiddenResource
	}

	items, err := u.repo.ListBatchItems(ctx, batchID)
	if err != nil {
		return nil, err
	}

	result, err := u.repo.FinalizeBatch(ctx, batchID, items)
	if err != nil {
		return nil, err
	}

	// Dispatch happens after the transaction commits, so the AI worker can never observe
	// an object whose quota reconciliation is still uncommitted. A dispatch failure is
	// reported in the response rather than rolling back a committed reconciliation.
	published := true
	if u.publisher != nil {
		for _, item := range items {
			if item.Status != domain.UploadItemStatusUploaded || item.ObjectKey == nil {
				continue
			}
			event := messaging.PhotoIngestEvent{
				ObjectKey:        *item.ObjectKey,
				UploadBatchID:    batchID.String(),
				UploadItemID:     item.ID.String(),
				StorageAccountID: batch.StorageAccountID.String(),
				ActualBytes:      result.ActualBytes,
			}
			if err := u.publisher.PublishPhotoIngest(ctx, event); err != nil {
				log.Printf("[FINALIZE] failed to publish ingest event for item %s: %v", item.ID, err)
				published = false
			}
		}
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "UPLOAD_BATCH_FINALIZED",
		TargetType:  "upload_batch",
		TargetID:    &batchID,
		Metadata: map[string]interface{}{
			"storage_account_id":   batch.StorageAccountID.String(),
			"actual_bytes":         result.ActualBytes,
			"total_files_uploaded": result.TotalFilesUploaded,
			"event_published":      published,
		},
	})

	// The caller learns whether the async dispatch succeeded; the reconciliation itself
	// already stands and must not be retried on a dispatch failure alone.
	result.EventPublished = published
	return result, nil
}
