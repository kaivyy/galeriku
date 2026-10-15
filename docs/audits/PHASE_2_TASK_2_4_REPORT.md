# Galeriku: Phase 2 Task 2.4 Implementation Report
## RBAC Authorization Middleware, Tenant Isolation & B2B Organization Context

- **Date:** 15 Oktober 2026
- **Status:** COMPLETED and VERIFIED
- **Branch:** `main` (uncommitted per explicit instruction)
- **Scope:** Task 2.4 only (Authentication & RBAC Middleware, Tenant Isolation Middleware, B2B Organization Context Injection, Request-ID sanitization; zero Task 2.5+ endpoints, zero new database migrations)

---

## 1. Executive Summary

Task 2.4 implements the role-based access control (RBAC) authorization middleware, multi-tenant isolation middleware, and B2B Host organization context injection for the Galeriku platform. The implementation adheres to the specifications in `docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`, `docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`, `docs/API_SPEC.md`, and the architectural conditions established in `docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md`.

All requirements authorized for Task 2.4 were implemented and verified using Test-Driven Development (TDD):
1. **Authentication Middleware (`middleware.Authenticate`):** Intercepts requests, validates Bearer JWT access tokens with HMAC-SHA256, rejects expired or tampered tokens with HTTP 401 `AUTH_UNAUTHORIZED`, and injects typed `AuthIdentity` into request context.
2. **Capability Authorization Middleware (`middleware.RequireCapability`):** Enforces global user capabilities (`BUYER`, `CREATOR`, `HOST_LEADER`, `SUPER_ADMIN`). Provides fast-path token claims evaluation as well as live PostgreSQL verification via `WithLiveCapabilityCheck`, permanently eliminating the 15-minute stateless JWT revocation lag window for sensitive operations (Condition F-04).
3. **Tenant Isolation Middleware (`middleware.RequireHostMembership`):** Authoritatively verifies caller membership against `host_memberships` in PostgreSQL. Enforces role boundaries by distinguishing `LEADER` (exclusive financial authority) from `ADMIN` (operational authority). Resolves event ownership against `events` to block cross-tenant event access.
4. **B2B Organization Context Injection (`middleware.TenantContext`):** Injects verified, request-scoped `TenantContext` (`OrganizationID`, `OrganizationName`, `UserID`, `Role`, `IsLeader`, `IsAdmin`) into request context upon successful authorization, ensuring downstream handlers and repositories can scope queries safely without trusting client-supplied identifiers.
5. **Request Correlation ID Hardening:** Sanitizes `X-Request-ID` against regex `^[a-zA-Z0-9_-]{1,64}$`, defending against HTTP header injection, CRLF splitting, and buffer attacks by substituting invalid or oversized IDs with auto-generated identifiers.

All unit, integration, race-detector, and quality-gate tests passed 100%.

---

## 2. Official Documents Inspected

The following repository documents were inspected prior to and during implementation:

| Document Path | Role in Specification Hierarchy | Key Information Extracted |
|---|---|---|
| [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) | Technical Implementation Plan | Task 2.4 scope (lines 187-212), middleware filenames, error codes `AUTH_UNAUTHORIZED` (401) and `AUTH_FORBIDDEN` (403), regex `^[a-zA-Z0-9_-]{1,64}$`. |
| [`docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md) | Preceding Acceptance Verdict | Acceptance verdict `ACCEPT WITH CONDITIONS`; Condition F-04 transfer (15-minute capability revocation lag); preservation of `Makefile:52` whitespace and legacy data caveats. |
| [`docs/audits/PHASE_2_TASK_2_3_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_REPORT.md) | Task 2.3 Evidence | Details authentication handler implementation, JWT structure, password hashing, and repository models. |
| [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md) | Security Baseline Audit | Section 4.4: Host Leader (financial exclusivity, single active leader rule OPEN-10) vs Host Admin (operational delegated permissions); BOLA/IDOR prevention rules. |
| [`docs/API_SPEC.md`](file:///root/galeriku/docs/API_SPEC.md) | API Contract Reference | Section 3 error catalog (lines 1296-1297): `AUTH_UNAUTHORIZED` (401) and `AUTH_FORBIDDEN` (403); standard response envelopes; endpoint contracts. |
| [`docs/DATABASE_SPEC.md`](file:///root/galeriku/docs/DATABASE_SPEC.md) | Database Specification | Schema for Domain 01: `users`, `capabilities`, `user_capabilities`, `host_organizations`, `host_memberships`, `events`. |
| [`PRD.md`](file:///root/galeriku/PRD.md) | Product Authority | User capability taxonomy: `BUYER`, `CREATOR`, `HOST_LEADER`, `SUPER_ADMIN`. |

---

## 3. Scope Extracted from Implementation Plan

Task 2.4 deliverables:
- Creation of `backend/internal/domain/tenant.go` for Host Organization and Membership domain models.
- Creation of `backend/internal/repository/tenant_repository.go` and its integration tests for authoritative membership and event-owner lookups.
- Creation of `backend/internal/middleware/context.go` for typed context keys, `AuthIdentity`, and `TenantContext`.
- Creation of `backend/internal/middleware/auth_middleware.go` and `auth_middleware_test.go` for JWT token extraction and validation.
- Creation of `backend/internal/middleware/rbac.go` and `rbac_test.go` for capability and tenant membership authorization.
- Modification of `backend/internal/middleware/request_id.go` and `request_id_test.go` for correlation ID regex sanitization.
- Creation of `backend/internal/middleware/tenant_integration_test.go` for live PostgreSQL end-to-end authorization tests.
- Modification of `backend/internal/handler/router.go` to integrate `middleware.Authenticate` on protected routes (`GET /api/v1/auth/me`).

---

## 4. Repository Branch, HEAD, and Working-Tree State

- **Active Branch:** `main`
- **Head Commit:** `7c215d952fa82faee1bd29b3c0a565210af1f915` (`7c215d9`)
- **Working Tree State:** All changes remain uncommitted awaiting explicit user authorization. No commits, rebases, or pushes were performed.
- **Preservation Check:**
  - `Makefile:52`: Trailing newline preserved intact (`git diff --check Makefile` returns exit code 2).
  - `PRD.md`: Completely unmodified.
  - `migrations/000001_initial_schema.up.sql`: Completely unmodified.
  - Zero new migrations added (existing schema in 000001 and 000002 fully supports all Task 2.4 requirements).

---

## 5. Files Changed and Purpose

| File Path | Action | Description |
|---|---|---|
| [`backend/internal/middleware/request_id.go`](file:///root/galeriku/backend/internal/middleware/request_id.go) | Modified | Added regex validation `^[a-zA-Z0-9_-]{1,64}$` and 64-char length guard. |
| [`backend/internal/middleware/request_id_test.go`](file:///root/galeriku/backend/internal/middleware/request_id_test.go) | Modified | Added unit tests for header injection, CRLF splitting, oversized IDs, and valid IDs. |
| [`backend/internal/domain/tenant.go`](file:///root/galeriku/backend/internal/domain/tenant.go) | Created | Domain entities `HostOrganization`, `HostMembership`, `EventSummary`, and role constants. |
| [`backend/internal/repository/tenant_repository.go`](file:///root/galeriku/backend/internal/repository/tenant_repository.go) | Created | Implements `TenantRepository` for membership queries, event ownership, and live capabilities. |
| [`backend/internal/repository/tenant_repository_test.go`](file:///root/galeriku/backend/internal/repository/tenant_repository_test.go) | Created | Integration tests against live PostgreSQL 16 container for tenant queries. |
| [`backend/internal/middleware/context.go`](file:///root/galeriku/backend/internal/middleware/context.go) | Created | Strongly typed context keys, `AuthIdentity`, `TenantContext`, and accessor functions. |
| [`backend/internal/middleware/auth_middleware.go`](file:///root/galeriku/backend/internal/middleware/auth_middleware.go) | Created | Implements `Authenticate` middleware validating Bearer JWTs and error responses. |
| [`backend/internal/middleware/auth_middleware_test.go`](file:///root/galeriku/backend/internal/middleware/auth_middleware_test.go) | Created | Unit tests for authentication extraction, expiration, signature tampering, and claim validation. |
| [`backend/internal/middleware/rbac.go`](file:///root/galeriku/backend/internal/middleware/rbac.go) | Created | Implements `RequireCapability`, `RequireHostMembership`, `RequireHostLeader`, and tenant context injection. |
| [`backend/internal/middleware/rbac_test.go`](file:///root/galeriku/backend/internal/middleware/rbac_test.go) | Created | Deterministic unit tests for permission matrices, role boundaries, and concurrency isolation. |
| [`backend/internal/middleware/tenant_integration_test.go`](file:///root/galeriku/backend/internal/middleware/tenant_integration_test.go) | Created | End-to-end live integration test suite against PostgreSQL 16 container. |
| [`backend/internal/handler/auth_handler.go`](file:///root/galeriku/backend/internal/handler/auth_handler.go) | Modified | Enhanced `Me` handler to consume `AuthIdentity` from request context with fallback support. |
| [`backend/internal/handler/router.go`](file:///root/galeriku/backend/internal/handler/router.go) | Modified | Wrapped `GET /api/v1/auth/me` with `middleware.Authenticate(authHandler.jwtSecret)`. |
| [`docs/plans/PHASE_2_TASK_2_4_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_TASK_2_4_PLAN.md) | Created | Task 2.4 Pre-implementation checkpoint plan. |

---

## 6. Middleware Design and Execution Order

The middleware architecture enforces a strict defense-in-depth pipeline:

```
Incoming Request
       │
       ▼
[1. CorrelationIDMiddleware]  --> Sanitizes X-Request-ID (regex ^[a-zA-Z0-9_-]{1,64}$)
       │
       ▼
[2. Authenticate(jwtSecret)] --> Validates Bearer JWT; injects AuthIdentity into context
       │                          (401 AUTH_UNAUTHORIZED on failure)
       ▼
[3. RequireCapability(code)] --> Validates global capability (optional live database check)
       │                          (403 AUTH_FORBIDDEN on failure)
       ▼
[4. RequireHostMembership]   --> Resolves Org/Event; verifies active role in PostgreSQL;
       │                          injects TenantContext (403 AUTH_FORBIDDEN on failure)
       ▼
[5. Protected HTTP Handler]  --> Consumes TenantContext & AuthIdentity for scoped queries
```

### Response Envelope Standard
Denied requests immediately return standard JSON error envelopes without exposing internal state:
```json
{
  "success": false,
  "error": {
    "code": "AUTH_FORBIDDEN",
    "message": "insufficient host membership role: operation restricted to LEADER",
    "request_id": "req_01927c8b5a127800b611d1029471abcd"
  },
  "meta": {
    "timestamp": "2026-10-15T20:45:00Z",
    "request_id": "req_01927c8b5a127800b611d1029471abcd"
  }
}
```

---

## 7. Capability Authorization Contract

`RequireCapability(requiredCapability string, opts ...CapabilityOption)`:
- Extracts `AuthIdentity` from request context. If missing, returns HTTP 401 `AUTH_UNAUTHORIZED`.
- **Fast-Path Mode:** Evaluates `identity.HasCapability(requiredCapability)` against validated claims in the JWT token.
- **Authoritative Live Mode (`WithLiveCapabilityCheck`):** For sensitive, financial, or administrative operations, queries PostgreSQL table `user_capabilities` directly via `tenantRepo.GetUserActiveCapabilities(ctx, identity.UserID)`. If the capability was revoked in the database after the token was issued, the middleware rejects the request with HTTP 403 `AUTH_FORBIDDEN` ("insufficient permissions: required capability missing or revoked"). This permanently resolves Condition F-04 from the Task 2.2 and 2.3 acceptance reviews.

---

## 8. Host Membership and Tenant Isolation Behavior

`RequireHostMembership(tenantRepo repository.TenantRepository, requiredRoles ...string)`:
1. **Target Organization Resolution:**
   - Evaluates URL path parameters (`r.PathValue("org_id")`, `r.PathValue("organization_id")`).
   - Evaluates header `X-Organization-ID`.
   - Evaluates query parameters (`?organization_id=`, `?org_id=`).
   - Evaluates event-scoped parameters (`r.PathValue("event_id")`, `?event_id=`). Resolves the owning organization from the database via `GetEventHostOrganizationID(ctx, eventID)`.
   - If both an organization ID and an event ID are provided, asserts that the event's owning organization matches the requested organization. If they mismatch, rejects with HTTP 403 `AUTH_FORBIDDEN` ("cross-tenant resource mismatch").
2. **Authoritative Membership Verification:**
   - Queries `host_memberships` in PostgreSQL for `(user_id, host_organization_id)`.
   - If not found: rejects with HTTP 403 `AUTH_FORBIDDEN`.
   - If `status != 'ACTIVE'` (e.g. `REVOKED`, `INACTIVE`): rejects with HTTP 403 `AUTH_FORBIDDEN`.
3. **Role Separation (Host Leader vs Host Admin):**
   - If `requiredRoles` are specified (e.g. `LEADER`), verifies that `hm.MembershipRole` matches one of the allowed roles.
   - Host Admin attempting to access financial or payout routes (which enforce `RequireHostLeader`) is rejected with HTTP 403 `AUTH_FORBIDDEN`.
   - Host Leader is permitted.

---

## 9. Organization Context Contract and Downstream Consumers

Upon successful membership authorization, the middleware constructs an immutable `TenantContext`:
```go
type TenantContext struct {
    OrganizationID   uuid.UUID `json:"organization_id"`
    OrganizationName string    `json:"organization_name"`
    LegalName        string    `json:"legal_name"`
    UserID           uuid.UUID `json:"user_id"`
    Role             string    `json:"role"`
    IsLeader         bool      `json:"is_leader"`
    IsAdmin          bool      `json:"is_admin"`
}
```
Downstream handlers extract this context using `middleware.GetTenantContext(r.Context())`. Downstream repository queries MUST scope operations using `tc.OrganizationID` (`WHERE host_organization_id = $context_org_id`), eliminating Insecure Direct Object References (IDOR/BOLA).

---

## 10. Security Threat Cases and Test Coverage

| Threat Vector | Security Defense in Task 2.4 | Test Coverage Verification |
|---|---|---|
| **Header Injection / CRLF Splitting** | Whitelist regex `^[a-zA-Z0-9_-]{1,64}$` rejects CRLF and replaces ID. | `TestCorrelationIDMiddleware` (Case 3) |
| **Oversized Header DoS** | Length guard rejects IDs > 64 chars and generates fresh ID. | `TestCorrelationIDMiddleware` (Case 4) |
| **Expired Access Token** | `Authenticate` returns HTTP 401 `AUTH_UNAUTHORIZED`. | `TestAuthenticateMiddleware` (Case 3) |
| **Tampered JWT Signature** | `Authenticate` returns HTTP 401 `AUTH_UNAUTHORIZED`. | `TestAuthenticateMiddleware` (Case 4) |
| **Missing Global Capability** | `RequireCapability` returns HTTP 403 `AUTH_FORBIDDEN`. | `TestRequireCapability/DeniedCapability_BuyerAccessingCreator` |
| **Stale JWT Revocation Lag (F-04)** | `WithLiveCapabilityCheck` checks PostgreSQL, blocking revoked users. | `TestRequireCapability/LiveRevocationCheck_ConditionF04` |
| **Non-Member Tenant Access** | `RequireHostMembership` returns HTTP 403 `AUTH_FORBIDDEN`. | `TestRequireHostMembership/NonMember_AccessDenied` |
| **Revoked Member Tenant Access** | `RequireHostMembership` checks `status = 'ACTIVE'`, blocking revoked. | `TestRequireHostMembership/RevokedMember_AccessDenied` |
| **Cross-Tenant IDOR (Org A vs Org B)** | Caller cannot access Org B even if they supply Org B ID in header. | `TestRequireHostMembership/CrossTenantIsolation_MemberOfACannotAccessB` |
| **Admin Privilege Escalation** | Host Admin blocked from `RequireHostLeader` routes. | `TestRequireHostMembership/HostAdmin_ForbiddenOnLeaderExclusiveRoute` |
| **Cross-Tenant Event Tampering** | Middleware resolves event's owner; cross-tenant access blocked. | `TestRequireHostMembership/CrossTenantEventAccess_Blocked` |
| **Cross-Resource Mismatch Attack** | Middleware detects mismatch between header org and event org. | `TestRequireHostMembership/CrossTenantResourceMismatch_TamperingDetected` |
| **Concurrent Context Bleed** | 20 parallel workers with distinct tenants tested under Go race detector. | `TestConcurrentContextIsolation` (-race passed) |

---

## 11. Test Commands and Actual Outcomes

All verification commands were executed live against the repository and active Docker services (`galeriku-postgres` on port 5433, `galeriku-redis` on port 6380, `galeriku-minio` on port 9010):

| Command Line | Purpose / Scope | Exit Code | Verified Outcome |
|---|---|:---:|---|
| `cd backend && go test -v -count=1 ./internal/middleware/...` | Middleware package tests | **0** | **17 of 17 tests passed** in 0.072s (Correlation ID, Auth, RBAC, Live Tenant isolation). |
| `cd backend && go test -race -count=1 ./internal/middleware/...` | Concurrency & race detector | **0** | **Passed** in 1.109s with zero race conditions detected under Go `-race`. |
| `cd backend && go test -v -count=1 ./internal/repository/...` | Database repository tests | **0** | **4 of 4 integration suites passed** in 0.152s (including `TestTenantRepository_HostMembershipAndEvents`). |
| `cd backend && go test -v -count=1 ./internal/handler/...` | HTTP handlers and router | **0** | **7 of 7 handler suites passed** in 1.216s (with `auth/me` protected by `Authenticate`). |
| `make test` | Workspace quality gates | **0** | **100% Passed**. Backend short tests passed; Web Vitest (4/4) passed; `tsc --noEmit` passed; `lint-copy` passed. |
| `make test-integration` | Live Docker service integration | **0** | **100% Passed**. MinIO provisioner, UUIDv7 regression, 000002 migration, rollback & reapply, 81 tables, live repo and tenant tests passed. |
| `cd backend && go test -count=1 ./...` | Backend full non-cached run | **0** | **100% Passed** across all 10 packages with zero failures. |
| `cd backend && go build -o /dev/null ./cmd/server` | Server build compilation | **0** | Clean compilation with zero warnings or errors. |

---

## 12. Known Limitations and Tenant-Isolation Coverage Boundary

1. **Tenant-Isolation Coverage Boundary:**
   The tenant isolation mechanism implemented in Task 2.4 covers:
   - General organization membership authorization (`RequireHostMembership`).
   - Exclusive Host Leader authorization (`RequireHostLeader`).
   - Event-to-organization ownership validation and event-scoped tenant resolution.
   - Request-scoped `TenantContext` injection.
   Full platform-wide isolation will extend to subsequent endpoint implementations in Task 2.5 (`/api/v1/hosts/apply`, `/api/v1/hosts/documents`), Task 2.6 (team management and invitations), Task 2.7 (bank accounts and payouts), and Domain 02 (FotoTree event management).
2. **Pre-Existing Unverified Condition:**
   Table data preservation on non-empty legacy databases remains classified as `NOT VERIFIED` (test suites run against clean disposable tables).

---

## 13. Confirmation of Task 2.3 Conditions and Working Tree Preservation

- **Condition F-01 (Keyed OTP Digest):** Preserved unmodified and passing all tests.
- **Condition F-02 (Login Rate Limiter):** Preserved unmodified and passing all tests.
- **Condition F-03 (Refresh Token Rotation & SHA-256 Storage):** Preserved unmodified and passing all tests.
- **Condition F-04 (Capability Revocation Lag Resolution):** Mitigated via `WithLiveCapabilityCheck` in `RequireCapability`.
- **Pre-Existing Whitespace at `Makefile:52`:** Preserved unmodified (`git diff --check Makefile` returns exit code 2).
- **Working Tree State:** All changes remain uncommitted in the working tree. Zero commits or pushes were made.

---

## 14. Explicit Recommendation for Next Step

Task 2.4 is **COMPLETED and VERIFIED**. All middleware, repositories, domain models, and integration tests have been implemented, tested with race detection, and verified against PostgreSQL 16.

The recommended next step is:
Conduct an independent Final Acceptance Review of Phase 2 Task 2.4 before proceeding to **Task 2.5: Host B2B Onboarding & Private Legal Documents Provisioning**.
