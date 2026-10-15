package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

// LiveCapabilityChecker defines an interface for live verification of user capabilities against PostgreSQL.
type LiveCapabilityChecker interface {
	GetUserActiveCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// CapabilityOption configures behavior for RequireCapability middleware.
type CapabilityOption func(*capabilityConfig)

type capabilityConfig struct {
	liveChecker LiveCapabilityChecker
}

// WithLiveCapabilityCheck configures RequireCapability to perform an authoritative database lookup.
// This permanently mitigates the 15-minute stateless JWT revocation lag window (Condition F-04).
func WithLiveCapabilityCheck(checker LiveCapabilityChecker) CapabilityOption {
	return func(cfg *capabilityConfig) {
		cfg.liveChecker = checker
	}
}

// RequireCapability returns a middleware enforcing that the authenticated user possesses the specified global capability.
// If the user lacks the capability, it returns HTTP 403 AUTH_FORBIDDEN.
func RequireCapability(requiredCapability string, opts ...CapabilityOption) func(http.Handler) http.Handler {
	cfg := &capabilityConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := GetAuthIdentity(r.Context())
			if !ok || identity == nil {
				writeMiddlewareError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
				return
			}

			// If live checker is configured, verify authoritatively from PostgreSQL to eliminate revocation lag
			if cfg.liveChecker != nil {
				activeCaps, err := cfg.liveChecker.GetUserActiveCapabilities(r.Context(), identity.UserID)
				if err != nil {
					writeMiddlewareError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to verify user capabilities")
					return
				}
				hasCap := false
				for _, c := range activeCaps {
					if c == requiredCapability {
						hasCap = true
						break
					}
				}
				if !hasCap {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "insufficient permissions: required capability missing or revoked")
					return
				}
			} else {
				// Fast-path: check claims in validated JWT token
				if !identity.HasCapability(requiredCapability) {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "insufficient permissions: required capability missing")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// resolveTargetOrganization extracts and validates the target host organization ID from path parameters,
// headers, query strings, or by resolving ownership of a tenant-scoped event.
func resolveTargetOrganization(r *http.Request, tenantRepo repository.TenantRepository) (uuid.UUID, error) {
	// 1. Try Go 1.22+ PathValue for organization
	var orgIDStr string
	if val := r.PathValue("org_id"); val != "" {
		orgIDStr = val
	} else if val := r.PathValue("organization_id"); val != "" {
		orgIDStr = val
	}

	// 2. Try Header
	if orgIDStr == "" {
		orgIDStr = r.Header.Get("X-Organization-ID")
	}

	// 3. Try Query parameter for organization
	if orgIDStr == "" {
		if val := r.URL.Query().Get("organization_id"); val != "" {
			orgIDStr = val
		} else if val := r.URL.Query().Get("org_id"); val != "" {
			orgIDStr = val
		}
	}

	var parsedOrgID uuid.UUID
	var hasOrgID bool
	if strings.TrimSpace(orgIDStr) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(orgIDStr))
		if err == nil {
			parsedOrgID = parsed
			hasOrgID = true
		}
	}

	// 4. Try resolving through event ID (Path or Query)
	var eventIDStr string
	if val := r.PathValue("event_id"); val != "" {
		eventIDStr = val
	} else if val := r.URL.Query().Get("event_id"); val != "" {
		eventIDStr = val
	}

	if strings.TrimSpace(eventIDStr) != "" {
		eventID, err := uuid.Parse(strings.TrimSpace(eventIDStr))
		if err == nil {
			eventOrgID, err := tenantRepo.GetEventHostOrganizationID(r.Context(), eventID)
			if err != nil {
				if errors.Is(err, repository.ErrEventNotFound) {
					return uuid.Nil, repository.ErrEventNotFound
				}
				return uuid.Nil, err
			}

			// Defense-in-depth: Cross-resource mismatch detection
			if hasOrgID && parsedOrgID != eventOrgID {
				return uuid.Nil, errors.New("cross-tenant resource mismatch: event does not belong to specified organization")
			}

			return eventOrgID, nil
		}
	}

	if hasOrgID {
		return parsedOrgID, nil
	}

	return uuid.Nil, errors.New("missing or malformed host organization identifier")
}

// RequireHostMembership returns a middleware that authoritatively verifies the caller's active membership
// in the requested Host Organization, enforces role constraints (e.g. LEADER vs ADMIN), and injects TenantContext.
func RequireHostMembership(tenantRepo repository.TenantRepository, requiredRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := GetAuthIdentity(r.Context())
			if !ok || identity == nil {
				writeMiddlewareError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
				return
			}

			targetOrgID, err := resolveTargetOrganization(r, tenantRepo)
			if err != nil {
				if errors.Is(err, repository.ErrEventNotFound) {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "event not found or inaccessible")
					return
				}
				if strings.Contains(err.Error(), "cross-tenant") {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", err.Error())
					return
				}
				writeMiddlewareError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error())
				return
			}

			// Authoritatively verify membership in PostgreSQL
			hm, ho, err := tenantRepo.GetActiveHostMembership(r.Context(), identity.UserID, targetOrgID)
			if err != nil {
				if errors.Is(err, repository.ErrMembershipNotFound) {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: caller is not an active member of this host organization")
					return
				}
				if errors.Is(err, repository.ErrMembershipInactive) {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "access denied: host membership is suspended or revoked")
					return
				}
				writeMiddlewareError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to verify host membership")
				return
			}

			// Verify role constraints if specified (e.g. LEADER exclusive financial authority)
			if len(requiredRoles) > 0 {
				roleAllowed := false
				for _, reqRole := range requiredRoles {
					if hm.MembershipRole == reqRole {
						roleAllowed = true
						break
					}
				}
				if !roleAllowed {
					writeMiddlewareError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "insufficient host membership role: operation restricted to "+strings.Join(requiredRoles, ", "))
					return
				}
			}

			// Inject verified tenant context
			tenantCtx := &TenantContext{
				OrganizationID:   targetOrgID,
				OrganizationName: ho.DisplayName,
				LegalName:        ho.LegalName,
				UserID:           identity.UserID,
				Role:             hm.MembershipRole,
				IsLeader:         hm.MembershipRole == domain.HostRoleLeader,
				IsAdmin:          hm.MembershipRole == domain.HostRoleAdmin,
			}

			ctx := WithTenantContext(r.Context(), tenantCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireHostLeader is a convenience middleware requiring active LEADER role in the target host organization.
func RequireHostLeader(tenantRepo repository.TenantRepository) func(http.Handler) http.Handler {
	return RequireHostMembership(tenantRepo, domain.HostRoleLeader)
}
