package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/google/uuid"
)

const (
	// multipartSessionTTL is how long an open multipart session stays usable.
	multipartSessionTTL = 24 * time.Hour

	// uploadPartTTL is how long a single presigned part URL stays valid.
	uploadPartTTL = 30 * time.Minute

	// maxPartNumber is the S3 ceiling for multipart part numbers.
	maxPartNumber = 10000
)

// MultipartUploadUsecase orchestrates direct-to-S3 multipart uploads for mass photo batches.
type MultipartUploadUsecase interface {
	InitiateMultipart(ctx context.Context, actorUserID, orgID uuid.UUID, req InitiateMultipartRequest) (*InitiateMultipartResult, error)
	SignPart(ctx context.Context, orgID uuid.UUID, req SignPartRequest) (*SignPartResult, error)
	CompleteMultipart(ctx context.Context, actorUserID, orgID uuid.UUID, req CompleteMultipartRequest) (*CompleteMultipartResult, error)
}

// InitiateMultipartRequest is the validated input for POST /storage/multipart/initiate.
type InitiateMultipartRequest struct {
	UploadBatchID   string
	ClientFileToken string
}

// InitiateMultipartResult carries the provider session identity and the target object key.
type InitiateMultipartResult struct {
	UploadItemID     string `json:"upload_item_id"`
	ProviderUploadID string `json:"provider_upload_id"`
	S3ObjectKey      string `json:"s3_object_key"`
}

// SignPartRequest is the validated input for POST /storage/multipart/sign-part.
type SignPartRequest struct {
	UploadItemID string
	PartNumber   int
}

// SignPartResult carries one presigned part URL.
type SignPartResult struct {
	PartNumber   int    `json:"part_number"`
	PresignedURL string `json:"presigned_url"`
}

// CompleteMultipartRequest is the validated input for POST /storage/multipart/complete.
type CompleteMultipartRequest struct {
	UploadItemID   string
	Parts          []storage.MultipartPart
	SHA256Checksum string
}

// CompleteMultipartResult confirms the closed session and the measured object size.
type CompleteMultipartResult struct {
	UploadItemID string `json:"upload_item_id"`
	Status       string `json:"status"`
	ObjectKey    string `json:"object_key"`
	ActualBytes  int64  `json:"actual_bytes"`
}

type multipartUploadUsecase struct {
	repo      repository.MultipartRepository
	batchRepo repository.UploadRepository
	s3        *storage.S3Service
	auditor   audit.Logger
}

// NewMultipartUploadUsecase constructs the multipart upload usecase.
// auditor may be nil, in which case mutations proceed without audit records.
func NewMultipartUploadUsecase(repo repository.MultipartRepository, batchRepo repository.UploadRepository, s3 *storage.S3Service, auditor audit.Logger) MultipartUploadUsecase {
	return &multipartUploadUsecase{repo: repo, batchRepo: batchRepo, s3: s3, auditor: auditor}
}

func (u *multipartUploadUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

// resolveItem loads the file slot and verifies the batch belongs to the caller's organization.
func (u *multipartUploadUsecase) resolveItem(ctx context.Context, orgID uuid.UUID, batchID uuid.UUID, clientFileToken string) (*domain.UploadItem, *domain.UploadBatch, error) {
	batch, err := u.batchRepo.GetBatchByID(ctx, batchID)
	if err != nil {
		return nil, nil, err
	}
	// The batch's storage account must belong to the caller's organization.
	quota, err := u.batchRepo.GetQuotaByOrganization(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	if quota.StorageAccountID != batch.StorageAccountID {
		return nil, nil, ErrForbiddenResource
	}

	item, err := u.repo.GetUploadItemByToken(ctx, batchID, clientFileToken)
	if err != nil {
		return nil, nil, err
	}
	return item, batch, nil
}

// batchIDForItem resolves which batch an upload item belongs to.
func (u *multipartUploadUsecase) batchIDForItem(ctx context.Context, itemID uuid.UUID, out *uuid.UUID) error {
	return u.repo.GetBatchIDByItem(ctx, itemID, out)
}

func (u *multipartUploadUsecase) InitiateMultipart(ctx context.Context, actorUserID, orgID uuid.UUID, req InitiateMultipartRequest) (*InitiateMultipartResult, error) {
	batchID, err := uuid.Parse(strings.TrimSpace(req.UploadBatchID))
	if err != nil {
		return nil, fmt.Errorf("%w: upload_batch_id must be a valid UUID", ErrInvalidInput)
	}
	req.ClientFileToken = strings.TrimSpace(req.ClientFileToken)
	if req.ClientFileToken == "" {
		return nil, fmt.Errorf("%w: client_file_token is required", ErrInvalidInput)
	}

	item, batch, err := u.resolveItem(ctx, orgID, batchID, req.ClientFileToken)
	if err != nil {
		return nil, err
	}
	if item.Status == domain.UploadItemStatusUploaded {
		return nil, fmt.Errorf("%w: this file slot is already uploaded", ErrInvalidInput)
	}

	// The object key is composed only from typed identifiers, so a caller can never
	// steer the upload outside raw-uploads/{org}/{batch}/.
	objectKey := storage.BuildRawUploadKey(orgID, batch.ID, item.ID, req.ClientFileToken+".jpg")

	providerUploadID, err := u.s3.CreateMultipartUpload(ctx, objectKey, "application/octet-stream")
	if err != nil {
		return nil, err
	}

	if _, err := u.repo.CreateMultipartSession(ctx, item.ID, providerUploadID, objectKey, maxPartNumber, time.Now().UTC().Add(multipartSessionTTL)); err != nil {
		// The provider session is useless without a database record, so discard it.
		_ = u.s3.AbortMultipartUpload(ctx, objectKey, providerUploadID)
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "UPLOAD_MULTIPART_INITIATED",
		TargetType:  "upload_item",
		TargetID:    &item.ID,
		Metadata:    map[string]interface{}{"object_key": objectKey, "batch_id": batch.ID.String()},
	})

	return &InitiateMultipartResult{
		UploadItemID:     item.ID.String(),
		ProviderUploadID: providerUploadID,
		S3ObjectKey:      objectKey,
	}, nil
}

func (u *multipartUploadUsecase) SignPart(ctx context.Context, orgID uuid.UUID, req SignPartRequest) (*SignPartResult, error) {
	itemID, err := uuid.Parse(strings.TrimSpace(req.UploadItemID))
	if err != nil {
		return nil, fmt.Errorf("%w: upload_item_id must be a valid UUID", ErrInvalidInput)
	}
	if req.PartNumber < 1 || req.PartNumber > maxPartNumber {
		return nil, fmt.Errorf("%w: part_number must be between 1 and %d", ErrInvalidInput, maxPartNumber)
	}

	providerUploadID, objectKey, err := u.repo.GetActiveSessionByItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if objectKey == "" {
		return nil, repository.ErrMultipartSessionNotFound
	}

	// Ownership is verified through the item's batch before any URL is issued.
	var batchID uuid.UUID
	if err := u.batchIDForItem(ctx, itemID, &batchID); err != nil {
		return nil, err
	}
	quota, err := u.batchRepo.GetQuotaByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	batch, err := u.batchRepo.GetBatchByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch.StorageAccountID != quota.StorageAccountID {
		return nil, ErrForbiddenResource
	}

	presignedURL, err := u.s3.PresignUploadPart(ctx, objectKey, providerUploadID, req.PartNumber, uploadPartTTL)
	if err != nil {
		return nil, err
	}
	return &SignPartResult{PartNumber: req.PartNumber, PresignedURL: presignedURL}, nil
}

func (u *multipartUploadUsecase) CompleteMultipart(ctx context.Context, actorUserID, orgID uuid.UUID, req CompleteMultipartRequest) (*CompleteMultipartResult, error) {
	itemID, err := uuid.Parse(strings.TrimSpace(req.UploadItemID))
	if err != nil {
		return nil, fmt.Errorf("%w: upload_item_id must be a valid UUID", ErrInvalidInput)
	}
	if len(req.Parts) == 0 {
		return nil, fmt.Errorf("%w: parts must contain at least one entry", ErrInvalidInput)
	}
	for i, p := range req.Parts {
		if p.PartNumber < 1 || p.PartNumber > maxPartNumber {
			return nil, fmt.Errorf("%w: parts[%d].part_number must be between 1 and %d", ErrInvalidInput, i, maxPartNumber)
		}
		if strings.TrimSpace(p.ETag) == "" {
			return nil, fmt.Errorf("%w: parts[%d].etag is required", ErrInvalidInput, i)
		}
	}

	providerUploadID, objectKey, err := u.repo.GetActiveSessionByItem(ctx, itemID)
	if err != nil {
		return nil, err
	}

	var batchID uuid.UUID
	if err := u.batchIDForItem(ctx, itemID, &batchID); err != nil {
		return nil, err
	}
	quota, err := u.batchRepo.GetQuotaByOrganization(ctx, orgID)
	if err != nil {
		return nil, err
	}
	batch, err := u.batchRepo.GetBatchByID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch.StorageAccountID != quota.StorageAccountID {
		return nil, ErrForbiddenResource
	}

	if err := u.s3.CompleteMultipartUpload(ctx, objectKey, providerUploadID, req.Parts); err != nil {
		return nil, err
	}

	// The measured object size is authoritative; it reconciles the optimistic expectation.
	actualBytes, err := u.s3.StatObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}

	if err := u.repo.CompleteMultipartSession(ctx, itemID, actualBytes); err != nil {
		return nil, err
	}

	u.recordAudit(ctx, audit.Entry{
		ActorUserID: &actorUserID,
		ActionCode:  "UPLOAD_ITEM_COMPLETED",
		TargetType:  "upload_item",
		TargetID:    &itemID,
		Metadata:    map[string]interface{}{"object_key": objectKey, "actual_bytes": actualBytes, "part_count": len(req.Parts)},
	})

	// The response reports the session outcome named by the API contract; the item row
	// itself carries the storage-level UPLOADED status set by the repository.
	return &CompleteMultipartResult{
		UploadItemID: itemID.String(),
		Status:       "COMPLETED",
		ObjectKey:    objectKey,
		ActualBytes:  actualBytes,
	}, nil
}
