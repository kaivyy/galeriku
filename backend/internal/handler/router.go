package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SuccessEnvelope format standar respons sukses sesuai GALERIKU_API_ARCHITECTURE_SPEC.md Bab 4.1
type SuccessEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Meta    SuccessMeta `json:"meta"`
}

type SuccessMeta struct {
	Timestamp  string  `json:"timestamp,omitempty"`
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor,omitempty"`
}

// ErrorEnvelope format standar respons galat sesuai GALERIKU_API_ARCHITECTURE_SPEC.md Bab 4.2
type ErrorEnvelope struct {
	Success bool      `json:"success"`
	Error   ErrorBody `json:"error"`
	Meta    ErrorMeta `json:"meta"`
}

type ErrorBody struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
}

type ErrorMeta struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"request_id"`
}

func SetupRoutes(pool *pgxpool.Pool) http.Handler {
	return SetupRoutesWithAuth(pool, nil)
}

// registerCoreRoutes mounts the health probe and the Task 2.3 authentication endpoints.
func registerCoreRoutes(mux *http.ServeMux, pool *pgxpool.Pool, authHandler *AuthHandler) {
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		reqID := middleware.GetRequestID(r.Context())

		overallStatus := "UP"
		httpCode := http.StatusOK
		dbStatus := "UNKNOWN"

		if pool == nil {
			overallStatus = "DOWN"
			dbStatus = "DISCONNECTED"
			httpCode = http.StatusServiceUnavailable
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()
			if err := pool.Ping(ctx); err == nil {
				dbStatus = "UP"
			} else {
				dbStatus = "DOWN"
				overallStatus = "DEGRADED"
				httpCode = http.StatusServiceUnavailable
			}
		}

		resp := SuccessEnvelope{
			Success: true,
			Data: map[string]interface{}{
				"status":    overallStatus,
				"service":   "galeriku-core-api",
				"version":   "0.1.0-draft",
				"database":  dbStatus,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
			Meta: SuccessMeta{
				RequestID:  reqID,
				NextCursor: nil,
			},
		}

		w.WriteHeader(httpCode)
		json.NewEncoder(w).Encode(resp)
	})

	if authHandler != nil {
		mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
		mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
		mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
		mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)
		mux.HandleFunc("POST /api/v1/auth/otp/send", authHandler.SendOTP)
		mux.HandleFunc("POST /api/v1/auth/otp/verify", authHandler.VerifyOTP)
		mux.Handle("GET /api/v1/auth/me", middleware.Authenticate(authHandler.jwtSecret)(http.HandlerFunc(authHandler.Me)))
	}
}

func SetupRoutesWithAuth(pool *pgxpool.Pool, authHandler *AuthHandler) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	return middleware.CorrelationIDMiddleware(mux)
}

// SetupRoutesWithHost wires the Task 2.5 Host application endpoints behind authentication.
func SetupRoutesWithHost(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)

	if hostHandler != nil {
		authMw := middleware.Authenticate(jwtSecret)
		mux.Handle("POST /api/v1/hosts/apply", authMw(http.HandlerFunc(hostHandler.Apply)))
		mux.Handle("GET /api/v1/hosts/applications/my", authMw(http.HandlerFunc(hostHandler.MyApplications)))
		mux.Handle("POST /api/v1/hosts/documents", authMw(http.HandlerFunc(hostHandler.RequestDocumentUpload)))
		mux.Handle("GET /api/v1/hosts/documents/{id}/view", authMw(http.HandlerFunc(hostHandler.ViewDocument)))
	}

	return middleware.CorrelationIDMiddleware(mux)
}

// registerHostRoutes mounts every authenticated /hosts and /admin/hosts route.
// Handlers that are nil are simply not mounted, so each task's setup wrapper can pass what it has.
// tenantRepo doubles as the live capability checker for platform-admin routes, closing the
// 15-minute JWT revocation lag on the most sensitive operations (Condition F-04).
func registerHostRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, tenantRepo repository.TenantRepository) {
	if hostHandler != nil {
		mux.Handle("POST /api/v1/hosts/apply", authMw(http.HandlerFunc(hostHandler.Apply)))
		mux.Handle("GET /api/v1/hosts/applications/my", authMw(http.HandlerFunc(hostHandler.MyApplications)))
		mux.Handle("POST /api/v1/hosts/documents", authMw(http.HandlerFunc(hostHandler.RequestDocumentUpload)))
		mux.Handle("GET /api/v1/hosts/documents/{id}/view", authMw(http.HandlerFunc(hostHandler.ViewDocument)))
	}

	// Acceptance is reachable by any authenticated invitee: the invitation itself
	// carries the authorization, so no tenant context exists yet.
	if invitationHandler != nil {
		mux.Handle("POST /api/v1/hosts/invitations/accept", authMw(http.HandlerFunc(invitationHandler.AcceptInvitation)))
	}

	if tenantRepo == nil {
		return
	}
	anyMember := middleware.RequireHostMembership(tenantRepo)
	leaderOnly := middleware.RequireHostLeader(tenantRepo)

	if membershipHandler != nil {
		platformAdmin := middleware.RequireCapability("SUPER_ADMIN", middleware.WithLiveCapabilityCheck(tenantRepo))
		mux.Handle("GET /api/v1/hosts/organization", authMw(anyMember(http.HandlerFunc(membershipHandler.GetOrganization))))
		mux.Handle("PATCH /api/v1/hosts/organization", authMw(leaderOnly(http.HandlerFunc(membershipHandler.UpdateOrganization))))
		mux.Handle("GET /api/v1/hosts/members", authMw(anyMember(http.HandlerFunc(membershipHandler.ListMembers))))
		mux.Handle("DELETE /api/v1/hosts/members/{id}", authMw(leaderOnly(http.HandlerFunc(membershipHandler.RemoveMember))))
		mux.Handle("POST /api/v1/admin/hosts/applications/{id}/approve", authMw(platformAdmin(http.HandlerFunc(membershipHandler.ApproveApplication))))
		mux.Handle("POST /api/v1/admin/hosts/applications/{id}/reject", authMw(platformAdmin(http.HandlerFunc(membershipHandler.RejectApplication))))
	}

	if bankHandler != nil {
		mux.Handle("GET /api/v1/hosts/bank-accounts", authMw(leaderOnly(http.HandlerFunc(bankHandler.ListAccounts))))
		mux.Handle("POST /api/v1/hosts/bank-accounts", authMw(leaderOnly(http.HandlerFunc(bankHandler.RegisterAccount))))
	}

	if invitationHandler != nil {
		mux.Handle("POST /api/v1/hosts/invitations", authMw(leaderOnly(http.HandlerFunc(invitationHandler.CreateInvitation))))
		mux.Handle("DELETE /api/v1/hosts/invitations/{id}", authMw(leaderOnly(http.HandlerFunc(invitationHandler.RevokeInvitation))))
	}
}

// SetupRoutesWithMembership wires the Task 2.6 organization, membership, and review endpoints.
func SetupRoutesWithMembership(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, nil, nil, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// SetupRoutesWithBankAccounts wires the Task 2.7 payout account endpoints (Leader only).
func SetupRoutesWithBankAccounts(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, nil, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// SetupRoutesWithInvitations wires the Task 2.8 team invitation endpoints.
func SetupRoutesWithInvitations(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// registerFotoTreeRoutes mounts the Phase 3 event, session, and route location endpoints.
// Every route requires an authenticated caller who is an ACTIVE member of the target Host
// organization, resolved from the verified TenantContext.
func registerFotoTreeRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, eventTreeHandler *EventTreeHandler, tenantRepo repository.TenantRepository) {
	if eventTreeHandler == nil || tenantRepo == nil {
		return
	}
	anyMember := middleware.RequireHostMembership(tenantRepo)

	mux.Handle("POST /api/v1/events", authMw(anyMember(http.HandlerFunc(eventTreeHandler.CreateEvent))))
	mux.Handle("GET /api/v1/events", authMw(anyMember(http.HandlerFunc(eventTreeHandler.ListEvents))))
	mux.Handle("GET /api/v1/events/{id}", authMw(anyMember(http.HandlerFunc(eventTreeHandler.GetEventTree))))
	mux.Handle("PUT /api/v1/events/{id}", authMw(anyMember(http.HandlerFunc(eventTreeHandler.UpdateEvent))))
	mux.Handle("POST /api/v1/events/{id}/sessions", authMw(anyMember(http.HandlerFunc(eventTreeHandler.CreateSession))))
	mux.Handle("POST /api/v1/events/{id}/locations", authMw(anyMember(http.HandlerFunc(eventTreeHandler.CreateLocation))))
}

// SetupRoutesWithFotoTree wires the Phase 3 FotoTree hierarchy endpoints.
func SetupRoutesWithFotoTree(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// registerStorageRoutes mounts the Phase 3 quota and mass upload reservation endpoints.
func registerStorageRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, uploadHandler *UploadBatchHandler, tenantRepo repository.TenantRepository) {
	if uploadHandler == nil || tenantRepo == nil {
		return
	}
	anyMember := middleware.RequireHostMembership(tenantRepo)
	mux.Handle("GET /api/v1/storage/quota", authMw(anyMember(http.HandlerFunc(uploadHandler.GetQuota))))
	mux.Handle("POST /api/v1/storage/upload-batches/initiate", authMw(anyMember(http.HandlerFunc(uploadHandler.InitiateBatch))))
}

// SetupRoutesWithUploads wires the Phase 3 storage quota and upload batch reservation endpoints.
func SetupRoutesWithUploads(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, uploadHandler *UploadBatchHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	registerStorageRoutes(mux, middleware.Authenticate(jwtSecret), uploadHandler, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// registerMultipartRoutes mounts the Phase 3 direct-to-S3 multipart upload endpoints.
func registerMultipartRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, multipartHandler *MultipartUploadHandler, tenantRepo repository.TenantRepository) {
	if multipartHandler == nil || tenantRepo == nil {
		return
	}
	anyMember := middleware.RequireHostMembership(tenantRepo)
	mux.Handle("POST /api/v1/storage/multipart/initiate", authMw(anyMember(http.HandlerFunc(multipartHandler.Initiate))))
	mux.Handle("POST /api/v1/storage/multipart/sign-part", authMw(anyMember(http.HandlerFunc(multipartHandler.SignPart))))
	mux.Handle("POST /api/v1/storage/multipart/complete", authMw(anyMember(http.HandlerFunc(multipartHandler.Complete))))
}

// registerFinalizeRoutes mounts the Phase 3 batch finalization endpoint.
func registerFinalizeRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, finalizeHandler *BatchFinalizeHandler, tenantRepo repository.TenantRepository) {
	if finalizeHandler == nil || tenantRepo == nil {
		return
	}
	anyMember := middleware.RequireHostMembership(tenantRepo)
	mux.Handle("POST /api/v1/storage/upload-batches/{id}/finalize", authMw(anyMember(http.HandlerFunc(finalizeHandler.Finalize))))
}

// registerDiscoveryRoutes mounts the Phase 5 discovery, cart, checkout, and order endpoints.
func registerDiscoveryRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, discoveryHandler *DiscoveryHandler) {
	if discoveryHandler == nil {
		return
	}
	// BIB search is public.
	mux.Handle("POST /api/v1/search/bib", http.HandlerFunc(discoveryHandler.SearchBIB))

	buyerAuth := authMw
	mux.Handle("GET /api/v1/cart", buyerAuth(http.HandlerFunc(discoveryHandler.GetCart)))
	mux.Handle("POST /api/v1/cart/items", buyerAuth(http.HandlerFunc(discoveryHandler.AddCartItem)))
	mux.Handle("DELETE /api/v1/cart/items/{id}", buyerAuth(http.HandlerFunc(discoveryHandler.RemoveCartItem)))
	mux.Handle("POST /api/v1/checkout/verify-bib/initiate", buyerAuth(http.HandlerFunc(discoveryHandler.InitiateVerification)))
	mux.Handle("POST /api/v1/checkout/verify-bib/confirm", buyerAuth(http.HandlerFunc(discoveryHandler.ConfirmVerification)))
	mux.Handle("POST /api/v1/orders", buyerAuth(http.HandlerFunc(discoveryHandler.CreateOrder)))
	mux.Handle("GET /api/v1/orders/{id}", buyerAuth(http.HandlerFunc(discoveryHandler.GetOrder)))
}

// SetupRoutesWithMultipart wires the Phase 3 multipart upload endpoints on top of the quota surface.
func SetupRoutesWithMultipart(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, uploadHandler *UploadBatchHandler, multipartHandler *MultipartUploadHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	registerStorageRoutes(mux, middleware.Authenticate(jwtSecret), uploadHandler, tenantRepo)
	registerMultipartRoutes(mux, middleware.Authenticate(jwtSecret), multipartHandler, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// SetupRoutesWithFinalize wires the complete Phase 3 surface.
func SetupRoutesWithFinalize(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, uploadHandler *UploadBatchHandler, multipartHandler *MultipartUploadHandler, finalizeHandler *BatchFinalizeHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	registerStorageRoutes(mux, middleware.Authenticate(jwtSecret), uploadHandler, tenantRepo)
	registerMultipartRoutes(mux, middleware.Authenticate(jwtSecret), multipartHandler, tenantRepo)
	registerFinalizeRoutes(mux, middleware.Authenticate(jwtSecret), finalizeHandler, tenantRepo)
	return middleware.CorrelationIDMiddleware(mux)
}

// registerPaymentRoutes mounts the Phase 6 payment, download, and fulfillment endpoints.
func registerPaymentRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, paymentHandler *PaymentHandler) {
	if paymentHandler == nil {
		return
	}
	mux.Handle("POST /api/v1/payments/initiate", authMw(http.HandlerFunc(paymentHandler.InitiatePayment)))
	mux.Handle("POST /api/v1/webhooks/payments/midtrans", http.HandlerFunc(paymentHandler.Webhook))
	mux.Handle("POST /api/v1/downloads/grant/{id}", authMw(http.HandlerFunc(paymentHandler.GrantDownload)))
	mux.Handle("GET /api/v1/downloads/file/{token}", http.HandlerFunc(paymentHandler.ServeDownload))
	mux.Handle("POST /api/v1/fulfillment/upload-master/{id}", authMw(http.HandlerFunc(paymentHandler.UploadMaster)))
}

// registerCatalogRoutes mounts the public photo catalog endpoints (no auth).
func registerCatalogRoutes(mux *http.ServeMux, catalogHandler *CatalogHandler) {
	if catalogHandler == nil {
		return
	}
	mux.Handle("GET /api/v1/photos", http.HandlerFunc(catalogHandler.ListPhotos))
	mux.Handle("GET /api/v1/photos/{id}", http.HandlerFunc(catalogHandler.GetPhoto))
}

// registerFinanceRoutes mounts the Phase 7 wallet, refund, and settlement endpoints.
func registerFinanceRoutes(mux *http.ServeMux, authMw func(http.Handler) http.Handler, financeHandler *FinanceHandler) {
	if financeHandler == nil {
		return
	}
	mux.Handle("GET /api/v1/finance/wallet", authMw(http.HandlerFunc(financeHandler.GetWallet)))
	mux.Handle("POST /api/v1/refunds/request", authMw(http.HandlerFunc(financeHandler.RequestRefund)))
	mux.Handle("POST /api/v1/finance/settlements/release", authMw(http.HandlerFunc(financeHandler.ReleaseSettlements)))
}

// SetupRoutesWithFinance wires the full Phase 7 surface on top of Phase 6.
func SetupRoutesWithFinance(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, uploadHandler *UploadBatchHandler, multipartHandler *MultipartUploadHandler, finalizeHandler *BatchFinalizeHandler, discoveryHandler *DiscoveryHandler, paymentHandler *PaymentHandler, catalogHandler *CatalogHandler, financeHandler *FinanceHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerCatalogRoutes(mux, catalogHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	registerStorageRoutes(mux, middleware.Authenticate(jwtSecret), uploadHandler, tenantRepo)
	registerMultipartRoutes(mux, middleware.Authenticate(jwtSecret), multipartHandler, tenantRepo)
	registerFinalizeRoutes(mux, middleware.Authenticate(jwtSecret), finalizeHandler, tenantRepo)
	registerDiscoveryRoutes(mux, middleware.Authenticate(jwtSecret), discoveryHandler)
	registerPaymentRoutes(mux, middleware.Authenticate(jwtSecret), paymentHandler)
	registerFinanceRoutes(mux, middleware.Authenticate(jwtSecret), financeHandler)
	return middleware.CorrelationIDMiddleware(mux)
}

// SetupRoutesWithPayment wires the full Phase 6 surface on top of Phase 5.
func SetupRoutesWithPayment(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, uploadHandler *UploadBatchHandler, multipartHandler *MultipartUploadHandler, finalizeHandler *BatchFinalizeHandler, discoveryHandler *DiscoveryHandler, paymentHandler *PaymentHandler, catalogHandler *CatalogHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerCatalogRoutes(mux, catalogHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	registerStorageRoutes(mux, middleware.Authenticate(jwtSecret), uploadHandler, tenantRepo)
	registerMultipartRoutes(mux, middleware.Authenticate(jwtSecret), multipartHandler, tenantRepo)
	registerFinalizeRoutes(mux, middleware.Authenticate(jwtSecret), finalizeHandler, tenantRepo)
	registerDiscoveryRoutes(mux, middleware.Authenticate(jwtSecret), discoveryHandler)
	registerPaymentRoutes(mux, middleware.Authenticate(jwtSecret), paymentHandler)
	return middleware.CorrelationIDMiddleware(mux)
}

// SetupRoutesWithDiscovery wires the complete Phase 5 surface on top of Phase 3.
func SetupRoutesWithDiscovery(pool *pgxpool.Pool, authHandler *AuthHandler, hostHandler *HostApplicationHandler, membershipHandler *HostMembershipHandler, tenantRepo repository.TenantRepository, bankHandler *BankAccountHandler, invitationHandler *InvitationHandler, eventTreeHandler *EventTreeHandler, uploadHandler *UploadBatchHandler, multipartHandler *MultipartUploadHandler, finalizeHandler *BatchFinalizeHandler, discoveryHandler *DiscoveryHandler, jwtSecret []byte) http.Handler {
	mux := http.NewServeMux()
	registerCoreRoutes(mux, pool, authHandler)
	registerHostRoutes(mux, middleware.Authenticate(jwtSecret), hostHandler, membershipHandler, bankHandler, invitationHandler, tenantRepo)
	registerFotoTreeRoutes(mux, middleware.Authenticate(jwtSecret), eventTreeHandler, tenantRepo)
	registerStorageRoutes(mux, middleware.Authenticate(jwtSecret), uploadHandler, tenantRepo)
	registerMultipartRoutes(mux, middleware.Authenticate(jwtSecret), multipartHandler, tenantRepo)
	registerFinalizeRoutes(mux, middleware.Authenticate(jwtSecret), finalizeHandler, tenantRepo)
	registerDiscoveryRoutes(mux, middleware.Authenticate(jwtSecret), discoveryHandler)
	return middleware.CorrelationIDMiddleware(mux)
}
