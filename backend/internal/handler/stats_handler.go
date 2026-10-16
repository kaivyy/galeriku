package handler

import (
	"encoding/json"
	"net/http"

	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatsHandler serves per-role dashboard counters for the Studio, Host, and
// Admin surfaces. Every endpoint requires authentication and the role's
// capability; counts come from live SQL, never client input.
type StatsHandler struct {
	pool *pgxpool.Pool
}

// NewStatsHandler constructs the stats handler.
func NewStatsHandler(pool *pgxpool.Pool) *StatsHandler {
	return &StatsHandler{pool: pool}
}

func (h *StatsHandler) writeJSON(w http.ResponseWriter, data map[string]int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{
		Success: true,
		Data:    data,
		Meta:    SuccessMeta{Timestamp: nowRFC3339(), RequestID: "req_stats"},
	})
}

func (h *StatsHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Success: false,
		Error:   ErrorBody{Code: code, Message: message},
		Meta:    ErrorMeta{Timestamp: nowRFC3339(), RequestID: "req_stats"},
	})
}

func (h *StatsHandler) count(r *http.Request, query string, args ...interface{}) int64 {
	var n int64
	if err := h.pool.QueryRow(r.Context(), query, args...).Scan(&n); err != nil {
		return 0
	}
	return n
}

// CreatorStats handles GET /api/v1/stats/creator (CREATOR capability).
func (h *StatsHandler) CreatorStats(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	userID := identity.UserID
	photoCount := h.count(r, "SELECT COUNT(*) FROM photos WHERE uploader_user_id = $1", userID)
	salesCount := h.count(r, `
		SELECT COUNT(*) FROM order_items oi
		WHERE oi.seller_user_id = $1`, userID)
	pendingSLA := h.count(r, `
		SELECT COUNT(*) FROM fulfillment_jobs fj
		JOIN order_items oi ON oi.id = fj.order_item_id
		WHERE oi.seller_user_id = $1 AND fj.status = 'WAITING_MASTER'`, userID)
	h.writeJSON(w, map[string]int64{
		"photos":       photoCount,
		"sales":        salesCount,
		"pending_sla":  pendingSLA,
	})
}

// HostStats handles GET /api/v1/stats/host (any Host membership).
func (h *StatsHandler) HostStats(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	userID := identity.UserID
	orgCount := h.count(r, `
		SELECT COUNT(*) FROM host_memberships WHERE user_id = $1 AND status = 'ACTIVE'`, userID)
	memberCount := h.count(r, `
		SELECT COUNT(*) FROM host_memberships m
		WHERE m.host_organization_id IN (
			SELECT host_organization_id FROM host_memberships
			WHERE user_id = $1 AND status = 'ACTIVE'
		) AND m.status = 'ACTIVE'`, userID)
	pendingHosts := h.count(r, `
		SELECT COUNT(*) FROM host_organizations WHERE verification_status = 'SUBMITTED'`)
	h.writeJSON(w, map[string]int64{
		"organizations": orgCount,
		"members":       memberCount,
		"pending_hosts": pendingHosts,
	})
}

// AdminStats handles GET /api/v1/stats/admin (SUPER_ADMIN only).
func (h *StatsHandler) AdminStats(w http.ResponseWriter, r *http.Request) {
	identity, ok := middleware.GetAuthIdentity(r.Context())
	if !ok || identity == nil {
		h.writeError(w, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}
	if !identity.HasCapability("SUPER_ADMIN") {
		h.writeError(w, http.StatusForbidden, "AUTH_FORBIDDEN", "super admin required")
		return
	}
	totalUsers := h.count(r, "SELECT COUNT(*) FROM users")
	pendingHosts := h.count(r, "SELECT COUNT(*) FROM host_organizations WHERE verification_status = 'SUBMITTED'")
	auditCount := h.count(r, "SELECT COUNT(*) FROM audit_logs")
	paidOrders := h.count(r, "SELECT COUNT(*) FROM orders WHERE status = 'PAID'")
	h.writeJSON(w, map[string]int64{
		"users":         totalUsers,
		"pending_hosts": pendingHosts,
		"audit_logs":    auditCount,
		"paid_orders":   paidOrders,
	})
}

// Compile-time reference keeps the repository import meaningful for future
// stats queries that move off raw SQL.
var _ = repository.ErrOrderNotFound
