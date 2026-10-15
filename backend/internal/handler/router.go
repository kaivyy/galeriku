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
