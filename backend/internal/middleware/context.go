package middleware

import (
	"context"

	"github.com/google/uuid"
)

type authContextKey string

const (
	// AuthIdentityKey is the context key for the authenticated user identity.
	AuthIdentityKey authContextKey = "galeriku_auth_identity"

	// TenantContextKey is the context key for the verified tenant organization context.
	TenantContextKey authContextKey = "galeriku_tenant_context"
)

// AuthIdentity holds the authenticated caller's identity extracted from a verified JWT.
type AuthIdentity struct {
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	Capabilities []string  `json:"capabilities"`
	TokenID      string    `json:"token_id"`
}

// HasCapability checks whether the authenticated user has a specific global capability in their token.
func (a *AuthIdentity) HasCapability(code string) bool {
	for _, capCode := range a.Capabilities {
		if capCode == code {
			return true
		}
	}
	return false
}

// TenantContext holds the verified B2B Host organization context for downstream handlers and repositories.
type TenantContext struct {
	OrganizationID   uuid.UUID `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	LegalName        string    `json:"legal_name"`
	UserID           uuid.UUID `json:"user_id"`
	Role             string    `json:"role"`
	IsLeader         bool      `json:"is_leader"`
	IsAdmin          bool      `json:"is_admin"`
}

// WithAuthIdentity returns a new context containing the authenticated identity.
func WithAuthIdentity(ctx context.Context, identity *AuthIdentity) context.Context {
	return context.WithValue(ctx, AuthIdentityKey, identity)
}

// GetAuthIdentity retrieves the authenticated identity from the context if present.
func GetAuthIdentity(ctx context.Context) (*AuthIdentity, bool) {
	if identity, ok := ctx.Value(AuthIdentityKey).(*AuthIdentity); ok && identity != nil {
		return identity, true
	}
	return nil, false
}

// WithTenantContext returns a new context containing the verified tenant organization context.
func WithTenantContext(ctx context.Context, tenantCtx *TenantContext) context.Context {
	return context.WithValue(ctx, TenantContextKey, tenantCtx)
}

// GetTenantContext retrieves the tenant organization context from the context if present.
func GetTenantContext(ctx context.Context) (*TenantContext, bool) {
	if tc, ok := ctx.Value(TenantContextKey).(*TenantContext); ok && tc != nil {
		return tc, true
	}
	return nil, false
}
