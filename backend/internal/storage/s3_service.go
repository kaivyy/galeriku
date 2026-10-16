package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	// BucketMasterVault is the private bucket for legal documents and master media.
	BucketMasterVault = "galeriku-master-vault"

	// legalDocumentPrefix scopes every legal document object key.
	legalDocumentPrefix = "legal-documents/"
)

// allowedDocumentMime maps the permitted legal document extensions to their MIME type.
var allowedDocumentMime = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".pdf":  "application/pdf",
}

// S3Service issues presigned URLs scoped to the private master vault bucket.
type S3Service struct {
	client *minio.Client
	core   *minio.Core
}

// NewS3Service builds an S3 client for the given endpoint and static credentials.
func NewS3Service(endpoint, accessKey, secretKey string, useSSL bool) (*S3Service, error) {
	if endpoint == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("endpoint, accessKey, and secretKey must not be empty")
	}
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize minio client: %w", err)
	}
	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize minio core client: %w", err)
	}
	return &S3Service{client: client, core: core}, nil
}

// rawUploadPrefix scopes every photographer upload object key.
const rawUploadPrefix = "raw-uploads/"

// BuildRawUploadKey composes the vault object key for one uploaded photo.
// The key is derived only from typed identifiers and a sanitized extension, so a caller
// can never steer an upload outside the raw-uploads/ prefix.
func BuildRawUploadKey(orgID, batchID, itemID uuid.UUID, originalFilename string) string {
	ext := strings.ToLower(path.Ext(originalFilename))
	if _, ok := allowedDocumentMime[ext]; !ok {
		ext = ".jpg"
	}
	return rawUploadPrefix + orgID.String() + "/" + batchID.String() + "/" + itemID.String() + ext
}

// CreateMultipartUpload opens a multipart session for one object key and returns its upload ID.
func (s *S3Service) CreateMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error) {
	if !strings.HasPrefix(objectKey, rawUploadPrefix) {
		return "", errors.New("object key is outside the raw-uploads prefix")
	}
	uploadID, err := s.core.NewMultipartUpload(ctx, BucketMasterVault, objectKey, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("failed to create multipart upload: %w", err)
	}
	return uploadID, nil
}

// PresignUploadPart returns a presigned PUT URL for one part of an open multipart session.
func (s *S3Service) PresignUploadPart(ctx context.Context, objectKey, uploadID string, partNumber int, ttl time.Duration) (string, error) {
	if !strings.HasPrefix(objectKey, rawUploadPrefix) {
		return "", errors.New("object key is outside the raw-uploads prefix")
	}
	params := url.Values{}
	params.Set("partNumber", strconv.Itoa(partNumber))
	params.Set("uploadId", uploadID)

	signed, err := s.client.Presign(ctx, http.MethodPut, BucketMasterVault, objectKey, ttl, params)
	if err != nil {
		return "", fmt.Errorf("failed to presign upload part: %w", err)
	}
	return signed.String(), nil
}

// CompleteMultipartUpload closes a multipart session with the ETags S3 returned per part.
func (s *S3Service) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, parts []MultipartPart) error {
	if !strings.HasPrefix(objectKey, rawUploadPrefix) {
		return errors.New("object key is outside the raw-uploads prefix")
	}
	completeParts := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		completeParts = append(completeParts, minio.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	if _, err := s.core.CompleteMultipartUpload(ctx, BucketMasterVault, objectKey, uploadID, completeParts, minio.PutObjectOptions{}); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}
	return nil
}

// AbortMultipartUpload discards an open multipart session so no partial object remains.
func (s *S3Service) AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	if err := s.core.AbortMultipartUpload(ctx, BucketMasterVault, objectKey, uploadID); err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}
	return nil
}

// PutPreviewObject stores one preview JPEG in the master vault under previews/.
func (s *S3Service) PutPreviewObject(ctx context.Context, objectKey string, data []byte) error {
	if !strings.HasPrefix(objectKey, "previews/") {
		return errors.New("preview key is outside the previews prefix")
	}
	_, err := s.client.PutObject(ctx, BucketMasterVault, objectKey, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "image/jpeg"})
	if err != nil {
		return fmt.Errorf("failed to put preview object: %w", err)
	}
	return nil
}

// StreamObject opens one object for streaming through the response writer.
func (s *S3Service) StreamObject(ctx context.Context, objectKey string) (*minio.Object, error) {
	if !strings.HasPrefix(objectKey, rawUploadPrefix) && !strings.HasPrefix(objectKey, "previews/") && !strings.HasPrefix(objectKey, "hires/") {
		return nil, errors.New("object key is outside the permitted download prefixes")
	}
	return s.client.GetObject(ctx, BucketMasterVault, objectKey, minio.GetObjectOptions{})
}

// PresignPreviewGet issues a short-lived presigned GET for one photo preview.
// Only the previews/ prefix is reachable; TTL is clamped to 60 minutes per PRD.
func (s *S3Service) PresignPreviewGet(ctx context.Context, photoID uuid.UUID, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > 60*time.Minute {
		ttl = 60 * time.Minute
	}
	key := "previews/" + photoID.String() + ".jpg"
	viewURL, err := s.client.PresignedGetObject(ctx, BucketMasterVault, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to presign preview: %w", err)
	}
	return viewURL.String(), nil
}

// StatObject returns the size of one object in the vault, used for quota reconciliation.
func (s *S3Service) StatObject(ctx context.Context, objectKey string) (int64, error) {
	info, err := s.client.StatObject(ctx, BucketMasterVault, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to stat object: %w", err)
	}
	return info.Size, nil
}

// MultipartPart is one uploaded part identified by its number and S3 ETag.
type MultipartPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

// DocumentMimeType returns the MIME type for a supported legal document file name.
func DocumentMimeType(fileName string) (string, error) {
	ext := strings.ToLower(path.Ext(fileName))
	mime, ok := allowedDocumentMime[ext]
	if !ok {
		return "", fmt.Errorf("unsupported legal document extension %q", ext)
	}
	return mime, nil
}

// BuildLegalDocumentKey composes the vault object key for one legal document.
// The key is always derived from typed identifiers, never from caller-supplied paths.
func BuildLegalDocumentKey(orgID, documentID uuid.UUID, fileName string) (string, error) {
	ext := strings.ToLower(path.Ext(fileName))
	if _, ok := allowedDocumentMime[ext]; !ok {
		return "", fmt.Errorf("unsupported legal document extension %q", ext)
	}
	return legalDocumentPrefix + orgID.String() + "/" + documentID.String() + ext, nil
}

// PresignLegalDocumentUpload returns a presigned PUT URL and the object key it targets.
func (s *S3Service) PresignLegalDocumentUpload(ctx context.Context, orgID, documentID uuid.UUID, fileName string, ttl time.Duration) (string, string, error) {
	objectKey, err := BuildLegalDocumentKey(orgID, documentID, fileName)
	if err != nil {
		return "", "", err
	}
	uploadURL, err := s.client.PresignedPutObject(ctx, BucketMasterVault, objectKey, ttl)
	if err != nil {
		return "", "", fmt.Errorf("failed to presign upload: %w", err)
	}
	return uploadURL.String(), objectKey, nil
}

// PresignLegalDocumentView returns a short-lived presigned GET URL for one vault object.
// Only keys inside the legal-documents/ prefix are accepted, and traversal segments are rejected.
func (s *S3Service) PresignLegalDocumentView(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	if !strings.HasPrefix(objectKey, legalDocumentPrefix) {
		return "", errors.New("object key is outside the legal-documents prefix")
	}
	cleaned := path.Clean(objectKey)
	if strings.Contains(objectKey, "..") || !strings.HasPrefix(cleaned, legalDocumentPrefix) {
		return "", errors.New("object key contains an invalid path segment")
	}
	viewURL, err := s.client.PresignedGetObject(ctx, BucketMasterVault, cleaned, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to presign view: %w", err)
	}
	return viewURL.String(), nil
}
