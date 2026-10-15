# Galeriku: Phase 2 Task 2.4 Implementation Plan
## RBAC Authorization Middleware, Tenant Isolation & B2B Organization Context

- **Date:** 15 Oktober 2026
- **Status:** APPROVED FOR IMPLEMENTATION
- **Branch:** `main` (uncommitted per explicit instruction)
- **Scope:** Task 2.4 only (Authentication & RBAC Middleware, Tenant Isolation Middleware, B2B Organization Context Injection, Request-ID sanitization; zero Task 2.5+ endpoints, zero new database migrations)

---

## 1. Authoritative Documents Inspected

The following documents were retrieved, examined, and utilized as the authoritative specification:

| Document Path | Role in Specification Hierarchy | Key Extracted Requirements |
|---|---|---|
| [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) | Technical Implementation Plan | Task 2.4 specifications (lines 187-212): `RequireCapability`, `RequireHostRole`/`RequireHostMembership`, `auth_middleware.go`, `rbac.go`, `request_id.go` regex sanitization (`^[a-zA-Z0-9_-]{1,64}$`), acceptance criteria. |
| [`docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md) | Preceding Acceptance Verdict | Confirms 100% pass on Task 2.3; transfers P2-1 (15-minute stateless JWT capability revocation lag) to Task 2.4; preserves `Makefile:52` whitespace and legacy data caveats. |
| [`docs/audits/PHASE_2_TASK_2_3_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_REPORT.md) | Task 2.3 Evidence | Details authentication handler implementation, JWT structure, password hashing, and repository models. |
| [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md) | Security Baseline Audit | Section 4.4: Host Leader (financial exclusivity, single active leader rule OPEN-10) vs Host Admin (operational delegated permissions); BOLA/IDOR prevention rules. |
| [`docs/API_SPEC.md`](file:///root/galeriku/docs/API_SPEC.md) | API Contract Reference | Section 3 error catalog (lines 1296-1297): `AUTH_UNAUTHORIZED` (401) and `AUTH_FORBIDDEN` (403); standard response envelopes; endpoint contracts. |
| [`docs/DATABASE_SPEC.md`](file:///root/galeriku/docs/DATABASE_SPEC.md) | Database Specification | Schema for Domain 01: `users`, `capabilities`, `user_capabilities`, `host_organizations`, `host_memberships`, `events`. |
| [`PRD.md`](file:///root/galeriku/PRD.md) | Product Authority | User capability taxonomy: `BUYER`, `CREATOR`, `HOST_LEADER`, `SUPER_ADMIN`. |

---

## 2. Requirements Extracted from Official Plan

1. **Authentication Middleware (`Authenticate`):**
   - Intercepts requests, extracts `Authorization: Bearer <token>`.
   - Rejects missing, malformed, expired, or invalid tokens with HTTP 401 `AUTH_UNAUTHORIZED`.
   - Injects strongly typed `AuthIdentity` (`UserID`, `Email`, `Capabilities`, `TokenID`) into request context.
2. **Capability Authorization Middleware (`RequireCapability`):**
   - Enforces that authenticated user possesses the required global capability (e.g. `CREATOR`, `HOST_LEADER`, `SUPER_ADMIN`).
   - Rejects missing identity with HTTP 401 `AUTH_UNAUTHORIZED`.
   - Rejects missing capability with HTTP 403 `AUTH_FORBIDDEN`.
   - Supports live database check for sensitive capabilities to eliminate the 15-minute stateless JWT revocation lag window (Condition F-04).
3. **Tenant Isolation Middleware (`RequireHostMembership`):**
   - Resolves target Host organization from request (URL route path parameter `org_id` / `organization_id`, header `X-Organization-ID`, query string, or server-resolved `event_id`).
   - Authoritatively validates active membership against `host_memberships` in PostgreSQL.
   - Rejects non-members, revoked members, or inactive memberships with HTTP 403 `AUTH_FORBIDDEN`.
   - Strictly enforces role boundaries: distinguishes `LEADER` (exclusive financial authority) from `ADMIN` (operational authority).
   - Validates event-to-organization ownership: resolves event's owning organization from `events` table to prevent cross-tenant event access.
4. **B2B Organization Context Injection:**
   - Injects typed `TenantContext` (`OrganizationID`, `OrganizationName`, `UserID`, `Role`, `IsLeader`, `IsAdmin`) into request context upon successful authorization.
   - Downstream handlers consume `TenantContext` for tenant-scoped database queries, preventing IDOR/BOLA.
5. **Request Correlation ID Hardening:**
   - Sanitizes `X-Request-ID` with regex `^[a-zA-Z0-9_-]{1,64}$`.
   - Replaces invalid, oversized, or header-injection/CRLF strings with auto-generated `req_<uuid_hex>`.

---

## 3. Existing Components to Reuse

- **`security.VerifyAccessToken`**: Reused for RFC 7519 JWT HS256 cryptographic verification and claims extraction.
- **`middleware.SuccessEnvelope` / `middleware.ErrorEnvelope`**: Standard JSON envelope structure with `request_id` and RFC 3339 timestamp.
- **`middleware.GetRequestID`**: Reused for correlation ID propagation.
- **`repository.UserRepository`**: Reused for live user capability verification (`GetUserCapabilities`).
- **PostgreSQL 16 Connection Pool (`*pgxpool.Pool`)**: Reused for direct, atomic, non-destructive queries on `host_memberships`, `host_organizations`, and `events`.

---

## 4. Files Expected to Change / Be Created

| File Path | Action | Description |
|---|---|---|
| [`backend/internal/middleware/request_id.go`](file:///root/galeriku/backend/internal/middleware/request_id.go) | Modify | Add regex validation `^[a-zA-Z0-9_-]{1,64}$` and length guard. |
| [`backend/internal/middleware/request_id_test.go`](file:///root/galeriku/backend/internal/middleware/request_id_test.go) | Modify | Add test cases for header injection, CRLF, and oversized request IDs. |
| [`backend/internal/middleware/context.go`](file:///root/galeriku/backend/internal/middleware/context.go) | Create | Define context keys, `AuthIdentity`, `TenantContext`, and accessor helpers. |
| [`backend/internal/middleware/auth_middleware.go`](file:///root/galeriku/backend/internal/middleware/auth_middleware.go) | Create | Implement `Authenticate` middleware and token validation. |
| [`backend/internal/middleware/auth_middleware_test.go`](file:///root/galeriku/backend/internal/middleware/auth_middleware_test.go) | Create | Unit tests for authentication extraction, expiration, invalid tokens. |
| [`backend/internal/middleware/rbac.go`](file:///root/galeriku/backend/internal/middleware/rbac.go) | Create | Implement `RequireCapability` and `RequireHostMembership` with live database checks. |
| [`backend/internal/middleware/rbac_test.go`](file:///root/galeriku/backend/internal/middleware/rbac_test.go) | Create | Unit and integration tests for RBAC, role hierarchy, cross-tenant isolation. |
| [`backend/internal/domain/tenant.go`](file:///root/galeriku/backend/internal/domain/tenant.go) | Create | Domain models for `HostOrganization` and `HostMembership`. |
| [`backend/internal/repository/tenant_repository.go`](file:///root/galeriku/backend/internal/repository/tenant_repository.go) | Create | Repository interface and pgx implementation for membership & event lookups. |
| [`backend/internal/repository/tenant_repository_test.go`](file:///root/galeriku/backend/internal/repository/tenant_repository_test.go) | Create | Live PostgreSQL integration tests for tenant queries. |
| [`backend/internal/handler/router.go`](file:///root/galeriku/backend/internal/handler/router.go) | Modify | Integrate middlewares into router configuration. |

---

## 5. Test Strategy

1. **Unit Testing:**
   - Mock repositories and token generators to test all permission matrices.
   - Positive and negative cases: unauthenticated caller, missing capability, wrong capability, active member, inactive member, revoked member, wrong tenant, non-existent tenant.
   - Role separation: Host Admin attempting to access Leader-only routes -> 403 Forbidden.
   - Header injection: CRLF, special characters, string > 64 chars in `X-Request-ID`.
2. **Integration Testing:**
   - Live PostgreSQL integration tests inserting organizations, members, and events, verifying that queries use indexed columns and enforce tenant isolation.
   - Event ownership resolution: accessing Organization B's event through Organization A's context is rejected with 403 Forbidden.
3. **Concurrency & Race Testing:**
   - Verify request-scoped context isolation under concurrent requests with different tenant contexts.
   - Confirm zero context leaks between goroutines.

---

## 6. Security Risks and Mitigations

| Threat | Impact | Mitigation in Task 2.4 |
|---|---|---|
| **Stateless Revocation Lag (Condition F-04)** | User retains access after capability revocation in PostgreSQL. | For sensitive actions, `RequireCapability` checks live `user_capabilities` in PostgreSQL. |
| **IDOR / BOLA on Organizations** | Attacker substitutes `org_id` in path/header to access another tenant's data. | `RequireHostMembership` authoritatively checks database membership of authenticated user against target organization. |
| **Cross-Tenant Event Tampering** | Attacker references event belonging to Organization B while member of Organization A. | Middleware resolves event's owning organization from `events` table; mismatch causes immediate 403 Forbidden. |
| **Host Admin Privilege Escalation** | Host Admin accesses bank accounts or requests payouts. | `RequireHostMembership` enforces `requiredRoles = ["LEADER"]` on financial paths; `ADMIN` role is blocked. |
| **Header Injection / Splitting** | Attacker injects CRLF (`\r\n`) in `X-Request-ID`. | Regex whitelist `^[a-zA-Z0-9_-]{1,64}$` rejects CRLF and replaces it with auto-generated ID. |
| **Concurrent Context Bleed** | Concurrent HTTP requests share or overwrite tenant context. | Context values are immutable and request-scoped via `context.WithValue`. |

---

## 7. Out-of-Scope Items

- Host application registration endpoint (`POST /api/v1/hosts/apply`) -> Scheduled for Task 2.5.
- Host document upload and S3 presigned URLs -> Scheduled for Task 2.5.
- Host admin limit enforcement and invitations -> Scheduled for Task 2.6.
- Bank account encryption and payout endpoints -> Scheduled for Task 2.7.
- Changes to pre-existing `Makefile:52` whitespace -> Preserved untouched.

---

## 8. Unresolved Blockers

None. The schema, cryptographic primitives, and authentication infrastructure from Tasks 2.1 to 2.3 provide a complete, verified foundation for Task 2.4.
