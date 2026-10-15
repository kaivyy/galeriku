package storage_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/galeriku/core/internal/storage"
	"github.com/google/uuid"
)

func liveS3(t *testing.T) *storage.S3Service {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live MinIO test in short mode")
	}
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9010"
	}
	accessKey := os.Getenv("MINIO_ROOT_USER")
	if accessKey == "" {
		accessKey = "galeriku_admin"
	}
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if secretKey == "" {
		secretKey = "galeriku_minio_secret"
	}
	svc, err := storage.NewS3Service(endpoint, accessKey, secretKey, false)
	if err != nil {
		t.Skipf("cannot build S3 service: %v", err)
	}
	return svc
}

func TestS3Service_RejectsEmptyCredentials(t *testing.T) {
	if _, err := storage.NewS3Service("", "", "", false); err == nil {
		t.Fatal("expected error for empty endpoint or credentials, got nil")
	}
}

func TestS3Service_PresignUploadScopedToVaultPrefix(t *testing.T) {
	svc := liveS3(t)
	ctx := context.Background()
	orgID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	uploadURL, objectKey, err := svc.PresignLegalDocumentUpload(ctx, orgID, docID, "nib_oss.pdf", 15*time.Minute)
	if err != nil {
		t.Fatalf("presign upload failed: %v", err)
	}
	if !strings.HasPrefix(objectKey, "legal-documents/"+orgID.String()+"/") {
		t.Fatalf("object key %q is not scoped to legal-documents/{org_id}/", objectKey)
	}
	if !strings.Contains(uploadURL, "galeriku-master-vault") {
		t.Fatalf("presigned upload URL is not bound to the private vault bucket: %s", uploadURL)
	}
	if !strings.Contains(uploadURL, "X-Amz-Signature") {
		t.Fatalf("presigned upload URL is missing a signature: %s", uploadURL)
	}
}

func TestS3Service_PresignViewURLShortLived(t *testing.T) {
	svc := liveS3(t)
	ctx := context.Background()
	orgID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	_, objectKey, err := svc.PresignLegalDocumentUpload(ctx, orgID, docID, "ktp.jpg", 15*time.Minute)
	if err != nil {
		t.Fatalf("presign upload failed: %v", err)
	}

	viewURL, err := svc.PresignLegalDocumentView(ctx, objectKey, 5*time.Minute)
	if err != nil {
		t.Fatalf("presign view failed: %v", err)
	}
	if !strings.Contains(viewURL, "X-Amz-Signature") {
		t.Fatalf("presigned view URL is missing a signature: %s", viewURL)
	}
	if !strings.Contains(viewURL, "X-Amz-Expires=300") {
		t.Fatalf("presigned view URL is not limited to 300 seconds: %s", viewURL)
	}
}

func TestS3Service_ViewRejectsForeignPrefixAndTraversal(t *testing.T) {
	svc := liveS3(t)
	ctx := context.Background()

	foreign := "public-media/01927c8b/photo.jpg"
	if _, err := svc.PresignLegalDocumentView(ctx, foreign, 5*time.Minute); err == nil {
		t.Fatal("expected rejection for object key outside legal-documents/ prefix")
	}

	traversal := "legal-documents/01927c8b/../../public-media/photo.jpg"
	if _, err := svc.PresignLegalDocumentView(ctx, traversal, 5*time.Minute); err == nil {
		t.Fatal("expected rejection for object key containing path traversal")
	}
}

func TestS3Service_RejectsUnsupportedExtension(t *testing.T) {
	svc := liveS3(t)
	ctx := context.Background()
	orgID := uuid.Must(uuid.NewV7())
	docID := uuid.Must(uuid.NewV7())

	if _, _, err := svc.PresignLegalDocumentUpload(ctx, orgID, docID, "payload.exe", 15*time.Minute); err == nil {
		t.Fatal("expected rejection for unsupported document extension")
	}
}
