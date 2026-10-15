package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	// UploadBatchStatusReserved marks a batch whose quota is held but nothing is uploaded yet.
	UploadBatchStatusReserved = "RESERVED"

	// UploadBatchStatusCompleted marks a fully finalized batch.
	UploadBatchStatusCompleted = "COMPLETED"

	// UploadBatchStatusExpired marks a batch whose reservation lapsed.
	UploadBatchStatusExpired = "EXPIRED"

	// UploadItemStatusPending marks an item whose object has not landed yet.
	UploadItemStatusPending = "PENDING"

	// UploadItemStatusUploaded marks an item whose object was confirmed.
	UploadItemStatusUploaded = "UPLOADED"

	// ReservationStatusActive marks a reservation holding quota.
	ReservationStatusActive = "ACTIVE"

	// ReservationStatusCommitted marks a reservation converted into billable usage.
	ReservationStatusCommitted = "COMMITTED"

	// ReservationStatusReleased marks a reservation returned to the available balance.
	ReservationStatusReleased = "RELEASED"

	// StorageOwnerHostOrganization is the storage_accounts owner_type used for Host organizations.
	StorageOwnerHostOrganization = "HOST_ORGANIZATION"

	// MaxFilesPerBatch is the documented ceiling for one mass upload batch.
	MaxFilesPerBatch = 2000

	// ReservationTTL is how long a batch reservation holds quota before it lapses.
	ReservationTTL = 24 * time.Hour
)

// StorageQuota is the quota summary for one storage account.
type StorageQuota struct {
	ID               uuid.UUID `json:"id"`
	StorageAccountID uuid.UUID `json:"storage_account_id"`
	QuotaLimitBytes  int64     `json:"quota_limit_bytes"`
	BillableBytes    int64     `json:"billable_bytes"`
	ReservedBytes    int64     `json:"reserved_bytes"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RemainingBytes is the quota still available for new reservations.
func (q StorageQuota) RemainingBytes() int64 {
	remaining := q.QuotaLimitBytes - q.BillableBytes - q.ReservedBytes
	if remaining < 0 {
		return 0
	}
	return remaining
}

// UploadBatch is one mass upload session covering between 1 and MaxFilesPerBatch files.
type UploadBatch struct {
	ID                uuid.UUID  `json:"id"`
	StorageAccountID  uuid.UUID  `json:"storage_account_id"`
	OwnerUserID       uuid.UUID  `json:"owner_user_id"`
	Status            string     `json:"status"`
	ExpectedFileCount int        `json:"expected_file_count"`
	ExpectedBytes     int64      `json:"expected_bytes"`
	ActualBytes       int64      `json:"actual_bytes"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// UploadItem is one file slot inside a batch, addressed by the client's own token.
type UploadItem struct {
	ID              uuid.UUID `json:"id"`
	UploadBatchID   uuid.UUID `json:"upload_batch_id"`
	ClientFileToken string    `json:"client_file_token"`
	ExpectedBytes   int64     `json:"expected_bytes"`
	ObjectKey       *string   `json:"object_key,omitempty"`
	Status          string    `json:"status"`
}

// UploadBatchRequest is one file slot supplied when initiating a batch.
type UploadBatchRequest struct {
	ClientFileToken  string
	OriginalFilename string
	ExpectedBytes    int64
}

// BatchFinalization is the outcome of reconciling one finalized batch against its reservation.
// EventPublished reports whether every ingest event reached the message broker.
type BatchFinalization struct {
	BatchID            uuid.UUID `json:"upload_batch_id"`
	Status             string    `json:"status"`
	TotalFilesUploaded int       `json:"total_files_uploaded"`
	ActualBytes        int64     `json:"actual_bytes"`
	QuotaReconciled    bool      `json:"quota_reconciled"`
	EventPublished     bool      `json:"event_published"`
}
