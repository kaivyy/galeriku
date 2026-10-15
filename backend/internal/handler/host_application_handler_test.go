package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// hostTestEnv wires the full Task 2.5 pipeline against live PostgreSQL and MinIO.
type hostTestEnv struct {
	pool    *pgxpool.Pool
	router  http.Handler
	secret  []byte
	userID  uuid.UUID
	token   string
	cleanup func()
}

func newHostTestEnv(t *testing.T) *hostTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live host application integration test in short mode")
	}

	cfg := config.LoadConfig()
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5433"
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, dbHost, dbPort, cfg.DBName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}

	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "localhost:9010"
	}
	accessKey := os.Getenv("MINIO_ROOT_USER")
	if accessKey == "" {
		accessKey = "galeriku_admin"
	}
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if secretKey == "" {
		secretKey = "galeriku_minio_secret"
	}
	s3, err := storage.NewS3Service(s3Endpoint, accessKey, secretKey, false)
	if err != nil {
		pool.Close()
		t.Skipf("cannot build S3 service: %v", err)
	}

	jwtSecret := "test_super_secret_jwt_key_32_bytes_long_12345"

	userRepo := repository.NewPostgresUserRepository(pool)
	hostRepo := repository.NewPostgresHostRepository(pool)
	hostUsecase := usecase.NewHostApplicationUsecase(hostRepo, s3)

	hostHandler := handler.NewHostApplicationHandler(hostUsecase)
	router := handler.SetupRoutesWithHost(pool, nil, hostHandler, []byte(jwtSecret))

	// Seed one registered user with BUYER capability through the real repository path.
	unique := uuid.NewString()[:8]
	email := fmt.Sprintf("host_applicant_%s@galeriku.com", unique)
	user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: "Host Applicant", Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "BUYER"); err != nil {
		pool.Close()
		t.Fatalf("failed to seed applicant user: %v", err)
	}

	token, err := security.GenerateAccessToken([]byte(jwtSecret), user.ID.String(), email, []string{"BUYER"}, 15*time.Minute)
	if err != nil {
		pool.Close()
		t.Fatalf("failed to mint access token: %v", err)
	}

	env := &hostTestEnv{
		pool:   pool,
		router: router,
		secret: []byte(jwtSecret),
		userID: user.ID,
		token:  token,
	}
	env.cleanup = func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM host_legal_documents WHERE host_application_id IN (SELECT id FROM host_applications WHERE submitted_by = $1)", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM host_applications WHERE submitted_by = $1", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM host_organizations WHERE id NOT IN (SELECT host_organization_id FROM host_applications)")
		_, _ = pool.Exec(bg, "DELETE FROM user_capabilities WHERE user_id = $1", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM user_profiles WHERE user_id = $1", user.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
		pool.Close()
	}
	return env
}

func (e *hostTestEnv) do(t *testing.T, method, path string, body interface{}, withAuth bool) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if withAuth {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)

	var parsed map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestHostApplicationFlow_ApplyListAndUpload(t *testing.T) {
	env := newHostTestEnv(t)
	defer env.cleanup()

	// 1. Unauthenticated apply is rejected with 401 AUTH_UNAUTHORIZED.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/apply", map[string]string{
		"legal_name": "PT Tanpa Token", "display_name": "No Token", "business_field": "Test",
	}, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "AUTH_UNAUTHORIZED" {
		t.Fatalf("expected AUTH_UNAUTHORIZED, got %q", code)
	}

	// 2. Authenticated apply creates the organization and application as SUBMITTED.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/apply", map[string]string{
		"legal_name":    "PT Generasi Olahraga Hebat",
		"display_name":  "Jakarta Marathon Organizer",
		"business_field": "Sport Event Organizer",
		"address_text":  "Jl. Sudirman Kav 20, Jakarta Selatan",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	data := successData(parsed)
	applicationID, _ := data["application_id"].(string)
	if applicationID == "" {
		t.Fatalf("expected application_id in response, got %v", data)
	}
	if status, _ := data["status"].(string); status != "SUBMITTED" {
		t.Fatalf("expected application status SUBMITTED, got %q", status)
	}

	var orgStatus, appStatus string
	var orgID uuid.UUID
	err := env.pool.QueryRow(context.Background(), `
		SELECT ho.verification_status, ha.status, ho.id
		FROM host_applications ha JOIN host_organizations ho ON ho.id = ha.host_organization_id
		WHERE ha.id = $1`, applicationID).Scan(&orgStatus, &appStatus, &orgID)
	if err != nil {
		t.Fatalf("failed to read created application: %v", err)
	}
	if orgStatus != "SUBMITTED" || appStatus != "SUBMITTED" {
		t.Fatalf("expected both statuses SUBMITTED, got org=%q app=%q", orgStatus, appStatus)
	}

	// 3. A second apply while one is active is rejected with 409.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/apply", map[string]string{
		"legal_name": "PT Duplikat", "display_name": "Duplicate", "business_field": "Test",
	}, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate active application, got %d", rec.Code)
	}
	if code := errorCode(parsed); code != "APPLICATION_ALREADY_EXISTS" {
		t.Fatalf("expected APPLICATION_ALREADY_EXISTS, got %q", code)
	}

	// 4. GET /applications/my returns the application with document count.
	rec, parsed = env.do(t, http.MethodGet, "/api/v1/hosts/applications/my", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for my applications, got %d", rec.Code)
	}
	myData := successData(parsed)
	if got, _ := myData["application_id"].(string); got != applicationID {
		t.Fatalf("expected application_id %s, got %q", applicationID, got)
	}

	// 5. POST /documents issues a presigned PUT scoped to legal-documents/{org_id}/.
	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/documents", map[string]string{
		"application_id": applicationID,
		"document_type":  "NIB_OSS",
		"file_name":      "nib_oss_pt_generasi.pdf",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for document presign, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	docData := successData(parsed)
	documentID, _ := docData["document_id"].(string)
	uploadURL, _ := docData["upload_url"].(string)
	if documentID == "" || uploadURL == "" {
		t.Fatalf("expected document_id and upload_url, got %v", docData)
	}
	if !strings.Contains(uploadURL, "galeriku-master-vault") {
		t.Fatalf("upload URL must target the private vault bucket: %s", uploadURL)
	}
	if !strings.Contains(uploadURL, "legal-documents/"+orgID.String()+"/") {
		t.Fatalf("upload URL must be scoped to legal-documents/{org_id}/: %s", uploadURL)
	}

	var objectKey, mimeType string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT object_key, mime_type FROM host_legal_documents WHERE id = $1", documentID,
	).Scan(&objectKey, &mimeType); err != nil {
		t.Fatalf("failed to read document row: %v", err)
	}
	if !strings.HasPrefix(objectKey, "legal-documents/"+orgID.String()+"/") {
		t.Fatalf("stored object_key is not scoped: %q", objectKey)
	}
	if mimeType != "application/pdf" {
		t.Fatalf("expected mime type inferred from extension, got %q", mimeType)
	}

	// 6. View URL is short-lived and signed.
	rec, parsed = env.do(t, http.MethodGet, "/api/v1/hosts/documents/"+documentID+"/view", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for document view, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	viewURL, _ := successData(parsed)["view_url"].(string)
	if !strings.Contains(viewURL, "X-Amz-Expires=300") {
		t.Fatalf("expected 5-minute presigned view URL, got %s", viewURL)
	}
}

func TestHostDocumentView_CrossTenantAccessDenied(t *testing.T) {
	env := newHostTestEnv(t)
	defer env.cleanup()

	// Applicant A creates an application and uploads one document.
	rec, parsed := env.do(t, http.MethodPost, "/api/v1/hosts/apply", map[string]string{
		"legal_name": "PT Organisasi A", "display_name": "Org A", "business_field": "Sports",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	applicationID := successData(parsed)["application_id"].(string)

	rec, parsed = env.do(t, http.MethodPost, "/api/v1/hosts/documents", map[string]string{
		"application_id": applicationID, "document_type": "KTP_DIREKSI", "file_name": "ktp.jpg",
	}, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	documentID := successData(parsed)["document_id"].(string)

	// A different authenticated user (no membership, no ownership) must get 403.
	otherID := uuid.Must(uuid.NewV7())
	otherToken, err := security.GenerateAccessToken(env.secret, otherID.String(), "intruder@galeriku.com", []string{"BUYER"}, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to mint intruder token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/documents/"+documentID+"/view", nil)
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec2 := httptest.NewRecorder()
	env.router.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-tenant document access, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}

	// The document must not be readable through any public bucket path.
	var objectKey string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT object_key FROM host_legal_documents WHERE id = $1", documentID).Scan(&objectKey); err != nil {
		t.Fatalf("failed to read document key: %v", err)
	}
	if strings.Contains(objectKey, "public") {
		t.Fatalf("legal document object key must never reference the public bucket: %q", objectKey)
	}
}

func successData(parsed map[string]interface{}) map[string]interface{} {
	data, _ := parsed["data"].(map[string]interface{})
	return data
}

func errorCode(parsed map[string]interface{}) string {
	errBody, _ := parsed["error"].(map[string]interface{})
	code, _ := errBody["code"].(string)
	return code
}
