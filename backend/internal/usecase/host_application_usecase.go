package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/storage"
	"github.com/google/uuid"
)

const (
	// legalDocumentUploadTTL is how long an upload URL stays valid.
	legalDocumentUploadTTL = 15 * time.Minute

	// legalDocumentViewTTL is the short-lived window for reading one legal document.
	legalDocumentViewTTL = 5 * time.Minute
)

// ErrForbiddenResource is returned when the caller is not entitled to the requested resource.
var ErrForbiddenResource = errors.New("caller is not authorized for this resource")

// HostApplicationUsecase orchestrates Host registration and private document provisioning.
type HostApplicationUsecase interface {
	Apply(ctx context.Context, userID uuid.UUID, req ApplyHostRequest) (*ApplyHostResult, error)
	GetMyApplication(ctx context.Context, userID uuid.UUID) (*MyApplicationResult, error)
	RequestDocumentUpload(ctx context.Context, userID uuid.UUID, req DocumentUploadRequest) (*DocumentUploadResult, error)
	GetDocumentViewURL(ctx context.Context, userID uuid.UUID, capabilities []string, documentID uuid.UUID) (*DocumentViewResult, error)
}

// ApplyHostRequest is the validated input for POST /hosts/apply.
type ApplyHostRequest struct {
	LegalName     string
	DisplayName   string
	BusinessField string
	AddressText   string
}

// ApplyHostResult is the response payload for a successful application.
type ApplyHostResult struct {
	ApplicationID string     `json:"application_id"`
	Status        string     `json:"status"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
}

// MyApplicationResult is the response payload for GET /hosts/applications/my.
type MyApplicationResult struct {
	ApplicationID    string `json:"application_id"`
	Status           string `json:"status"`
	LegalName        string `json:"legal_name"`
	DisplayName      string `json:"display_name"`
	DocumentsUploaded int   `json:"documents_uploaded"`
}

// DocumentUploadRequest is the validated input for POST /hosts/documents.
type DocumentUploadRequest struct {
	ApplicationID string
	DocumentType  string
	FileName      string
	SizeBytes     int64
}

// DocumentUploadResult carries the presigned PUT URL and stored metadata.
type DocumentUploadResult struct {
	DocumentID string `json:"document_id"`
	ObjectKey  string `json:"object_key"`
	UploadURL  string `json:"upload_url"`
	MimeType   string `json:"mime_type"`
	ExpiresIn  int    `json:"expires_in_seconds"`
}

// DocumentViewResult carries the short-lived presigned GET URL.
type DocumentViewResult struct {
	DocumentID string `json:"document_id"`
	ViewURL    string `json:"view_url"`
	ExpiresIn  int    `json:"expires_in_seconds"`
}

// hostApplicationUsecase implements HostApplicationUsecase.
type hostApplicationUsecase struct {
	repo repository.HostRepository
	s3   *storage.S3Service
}

// NewHostApplicationUsecase constructs the Host application usecase.
func NewHostApplicationUsecase(repo repository.HostRepository, s3 *storage.S3Service) HostApplicationUsecase {
	return &hostApplicationUsecase{repo: repo, s3: s3}
}

var allowedDocumentTypes = map[string]bool{
	domain.HostDocumentTypeKTP:   true,
	domain.HostDocumentTypeNPWP:  true,
	domain.HostDocumentTypeNIB:   true,
	domain.HostDocumentTypeAKTA:  true,
}

func (u *hostApplicationUsecase) Apply(ctx context.Context, userID uuid.UUID, req ApplyHostRequest) (*ApplyHostResult, error) {
	req.LegalName = strings.TrimSpace(req.LegalName)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.BusinessField = strings.TrimSpace(req.BusinessField)

	if req.LegalName == "" || req.DisplayName == "" || req.BusinessField == "" {
		return nil, fmt.Errorf("%w: legal_name, display_name, and business_field are required", ErrInvalidInput)
	}
	if len(req.LegalName) > 255 || len(req.DisplayName) > 255 {
		return nil, fmt.Errorf("%w: legal_name and display_name must be at most 255 characters", ErrInvalidInput)
	}

	if _, _, err := u.repo.GetActiveApplicationByUser(ctx, userID); err == nil {
		return nil, repository.ErrActiveApplicationExists
	} else if !errors.Is(err, repository.ErrApplicationNotFound) {
		return nil, err
	}

	org := &domain.HostOrganization{
		LegalName:          req.LegalName,
		DisplayName:        req.DisplayName,
		BusinessField:      req.BusinessField,
		VerificationStatus: domain.HostApplicationStatusSubmitted,
	}
	app := &domain.HostApplication{
		SubmittedBy: userID,
		Status:      domain.HostApplicationStatusSubmitted,
	}
	if err := u.repo.CreateApplication(ctx, org, app); err != nil {
		return nil, err
	}

	return &ApplyHostResult{
		ApplicationID: app.ID.String(),
		Status:        app.Status,
		SubmittedAt:   app.SubmittedAt,
	}, nil
}

func (u *hostApplicationUsecase) GetMyApplication(ctx context.Context, userID uuid.UUID) (*MyApplicationResult, error) {
	app, org, err := u.repo.GetActiveApplicationByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	count, err := u.repo.CountDocumentsByApplication(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	return &MyApplicationResult{
		ApplicationID:     app.ID.String(),
		Status:            app.Status,
		LegalName:         org.LegalName,
		DisplayName:       org.DisplayName,
		DocumentsUploaded: count,
	}, nil
}

func (u *hostApplicationUsecase) RequestDocumentUpload(ctx context.Context, userID uuid.UUID, req DocumentUploadRequest) (*DocumentUploadResult, error) {
	applicationID, err := uuid.Parse(strings.TrimSpace(req.ApplicationID))
	if err != nil {
		return nil, fmt.Errorf("%w: application_id must be a valid UUID", ErrInvalidInput)
	}
	if !allowedDocumentTypes[req.DocumentType] {
		return nil, fmt.Errorf("%w: unsupported document_type", ErrInvalidInput)
	}
	mimeType, err := storage.DocumentMimeType(req.FileName)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	app, org, err := u.repo.GetApplicationByID(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if app.SubmittedBy != userID {
		return nil, ErrForbiddenResource
	}

	documentID := uuid.Must(uuid.NewV7())
	uploadURL, objectKey, err := u.s3.PresignLegalDocumentUpload(ctx, org.ID, documentID, req.FileName, legalDocumentUploadTTL)
	if err != nil {
		return nil, err
	}

	doc := &domain.HostLegalDocument{
		HostApplicationID: app.ID,
		ObjectKey:         objectKey,
		DocumentType:      req.DocumentType,
		MimeType:          mimeType,
		SizeBytes:         req.SizeBytes,
		ReviewStatus:      domain.HostDocumentReviewStatusPending,
	}
	if err := u.repo.CreateLegalDocument(ctx, doc); err != nil {
		return nil, err
	}

	return &DocumentUploadResult{
		DocumentID: doc.ID.String(),
		ObjectKey:  objectKey,
		UploadURL:  uploadURL,
		MimeType:   mimeType,
		ExpiresIn:  int(legalDocumentUploadTTL.Seconds()),
	}, nil
}

func (u *hostApplicationUsecase) GetDocumentViewURL(ctx context.Context, userID uuid.UUID, capabilities []string, documentID uuid.UUID) (*DocumentViewResult, error) {
	doc, app, err := u.repo.GetLegalDocumentByID(ctx, documentID)
	if err != nil {
		return nil, err
	}

	authorized := app.SubmittedBy == userID
	if !authorized {
		for _, c := range capabilities {
			if c == "SUPER_ADMIN" {
				authorized = true
				break
			}
		}
	}
	if !authorized {
		return nil, ErrForbiddenResource
	}

	viewURL, err := u.s3.PresignLegalDocumentView(ctx, doc.ObjectKey, legalDocumentViewTTL)
	if err != nil {
		return nil, err
	}
	return &DocumentViewResult{
		DocumentID: doc.ID.String(),
		ViewURL:    viewURL,
		ExpiresIn:  int(legalDocumentViewTTL.Seconds()),
	}, nil
}
