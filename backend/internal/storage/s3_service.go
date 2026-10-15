package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
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
}

// NewS3Service builds an S3 client for the given endpoint and static credentials.
func NewS3Service(endpoint, accessKey, secretKey string, useSSL bool) (*S3Service, error) {
	if endpoint == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("endpoint, accessKey, and secretKey must not be empty")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize minio client: %w", err)
	}
	return &S3Service{client: client}, nil
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
