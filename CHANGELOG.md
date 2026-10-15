# Changelog

All notable changes to the Galeriku platform will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Phase 3: FotoTree & Direct-to-S3 Multipart Upload (Completed)

#### Added - Task 3.1: FotoTree Hierarchy Management
- Implemented event, session, and route location CRUD in [`backend/internal/usecase/event_tree_usecase.go`](file:///root/galeriku/backend/internal/usecase/event_tree_usecase.go) with `POST`/`GET /api/v1/events`, `GET`/`PUT /api/v1/events/{id}`, `POST /api/v1/events/{id}/sessions`, and `POST /api/v1/events/{id}/locations`. Every route is mounted behind `Authenticate` plus `RequireHostMembership`, and the organization always comes from the verified TenantContext.

#### Added - Task 3.2: Atomic Quota Reservation (DEC-Q03)
- Implemented `GET /api/v1/storage/quota` and `POST /api/v1/storage/upload-batches/initiate` in [`backend/internal/repository/upload_repository.go`](file:///root/galeriku/backend/internal/repository/upload_repository.go). The reservation locks the quota row with `SELECT ... FOR UPDATE` before checking the balance, so concurrent batches cannot oversubscribe an account; a concurrency test drives ten parallel initiations against a one-gigabyte quota and proves the limit holds.

#### Added - Task 3.3: S3 Multipart Presigned Direct Upload
- Implemented `POST /api/v1/storage/multipart/initiate`, `/sign-part`, and `/complete` in [`backend/internal/usecase/multipart_upload_usecase.go`](file:///root/galeriku/backend/internal/usecase/multipart_upload_usecase.go). Object keys are composed from typed UUIDs under `raw-uploads/{org_id}/{batch_id}/`, and the integration test uploads real parts to MinIO and completes the session with the ETags the provider returned.

#### Fixed - Task 3.3: Multipart Provider Identifier Column Type
- Migration [`000003_multipart_provider_upload_id.up.sql`](file:///root/galeriku/migrations/000003_multipart_provider_upload_id.up.sql) widens `multipart_upload_sessions.provider_upload_id` from `UUID` to `TEXT`. Migration 000001 declared a UUID column, but object storage providers return opaque identifiers (MinIO returns a 124-character base64 value), so no real multipart session could be recorded.

#### Added - Task 3.4: Batch Finalization & RabbitMQ Dispatch
- Implemented `POST /api/v1/storage/upload-batches/{id}/finalize` in [`backend/internal/usecase/batch_finalize_usecase.go`](file:///root/galeriku/backend/internal/usecase/batch_finalize_usecase.go). Finalization releases the reservation and bills the measured object size in one transaction, appends a `storage_usage_entries` record, and queues one pending `ai_jobs` row per uploaded object with an idempotency key.
- Added the RabbitMQ publisher in [`backend/internal/messaging/rabbit_publisher.go`](file:///root/galeriku/backend/internal/messaging/rabbit_publisher.go), publishing persistent `photo.uploaded` events to a durable queue after the reconciliation commits.
- The `migrate-up` and `migrate-down` Makefile targets now apply every migration in the directory in order instead of hardcoding a single version.

### Phase 2: Identity, Authentication & Host B2B Portal (In Progress)

#### Added - Tasks 2.5 to 2.10: Host B2B Portal, Payouts, Invitations, Audit & Contracts
- Implemented the Host application flow in [`backend/internal/usecase/host_application_usecase.go`](file:///root/galeriku/backend/internal/usecase/host_application_usecase.go) with `POST /api/v1/hosts/apply` (201), `GET /api/v1/hosts/applications/my` (200), `POST /api/v1/hosts/documents` (201), and `GET /api/v1/hosts/documents/{id}/view` (200).
- Implemented private legal document provisioning in [`backend/internal/storage/s3_service.go`](file:///root/galeriku/backend/internal/storage/s3_service.go). Object keys are composed only from typed UUIDs and are strictly scoped to `legal-documents/{org_id}/*` in the private bucket `galeriku-master-vault`; view URLs expire after 300 seconds.
- Implemented atomic Host approval in [`backend/internal/repository/host_membership_repository.go`](file:///root/galeriku/backend/internal/repository/host_membership_repository.go): organization and application status, `ACTIVE` LEADER membership, global `HOST_LEADER` capability, and the default admin quota of 5 all commit together. Review now requires a `SUBMITTED` application and refuses duplicates with 409.
- Implemented Host membership endpoints (`GET`/`PATCH /api/v1/hosts/organization`, `GET /api/v1/hosts/members`, `DELETE /api/v1/hosts/members/{id}`) and platform review endpoints (`POST /api/v1/admin/hosts/applications/{id}/approve` and `/reject`) behind `SUPER_ADMIN` with a live database capability check.
- Implemented AES-256-GCM payout account encryption in [`backend/internal/security/bank_crypto.go`](file:///root/galeriku/backend/internal/security/bank_crypto.go) with per-encryption random nonces, masked-only responses, and an audit row per registration. Both bank routes are Leader-only. Production startup rejects the default `BANK_CIPHER_KEY`.
- Implemented time-bound Host invitations in [`backend/internal/repository/invitation_repository.go`](file:///root/galeriku/backend/internal/repository/invitation_repository.go): 32-byte CSPRNG tokens stored only as SHA-256 digests, 72-hour expiry, atomic admin-quota enforcement under row locks, and 410 `INVITATION_EXPIRED` / 422 `ADMIN_LIMIT_EXCEEDED` semantics.
- Implemented structured audit logging in [`backend/internal/audit/audit_service.go`](file:///root/galeriku/backend/internal/audit/audit_service.go) covering failed logins, session revocation, Host approval and rejection, membership revocation, membership role grants, invitation revocation, and bank account registration.
- Implemented graceful shutdown in [`backend/cmd/server/main.go`](file:///root/galeriku/backend/cmd/server/main.go) with `signal.NotifyContext` on SIGINT and SIGTERM and a 10-second drain budget before the database pool closes.
- Added the Phase 2 security regression suite in [`backend/test/security/auth_security_test.go`](file:///root/galeriku/backend/test/security/auth_security_test.go) covering account enumeration, refresh token family reuse, cross-tenant IDOR, Host Admin financial lockout, and `X-Request-ID` header injection.
- Synchronized Domain 01 into [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml) (29 paths, 28 schemas, all references resolving), added [`web/src/types/api.ts`](file:///root/galeriku/web/src/types/api.ts) contract types with drift tests, and added `make openapi-validate` backed by [`backend/scripts/validate_openapi.py`](file:///root/galeriku/backend/scripts/validate_openapi.py).

#### Added - Task 2.4: RBAC Authorization Middleware, Tenant Isolation & B2B Context
- Implemented `Authenticate(jwtSecret)` middleware in [`backend/internal/middleware/auth_middleware.go`](file:///root/galeriku/backend/internal/middleware/auth_middleware.go) to extract Bearer HS256 JWT, validate signature/expiration, and inject `AuthIdentity` into request context. Missing or invalid tokens return HTTP 401 `AUTH_UNAUTHORIZED`.
- Implemented `RequireCapability(code)` middleware in [`backend/internal/middleware/rbac.go`](file:///root/galeriku/backend/internal/middleware/rbac.go) to guard routes with granular capabilities (`BUYER`, `CREATOR`, `HOST_LEADER`, `PLATFORM_ADMIN`). Missing capability returns HTTP 403 `AUTH_FORBIDDEN`.
- Implemented `WithLiveCapabilityCheck` using [`TenantRepository.GetUserActiveCapabilities`](file:///root/galeriku/backend/internal/repository/tenant_repository.go#L83-L103) to eliminate the 15-minute stateless JWT revocation lag window for sensitive operations (Condition F-04).
- Implemented `RequireHostMembership(tenantRepo, requiredRoles...)` and `RequireHostLeader` in [`backend/internal/middleware/rbac.go`](file:///root/galeriku/backend/internal/middleware/rbac.go) to enforce multi-tenant isolation, validate active membership in PostgreSQL `host_memberships`, strictly separate Host Leader (financial exclusivity) from Host Admin (operational authority), and inject immutable `TenantContext`.
- Added multi-source tenant resolution supporting path parameter `org_id`, header `X-Organization-ID`, query parameters, and event ownership resolution via `GetEventHostOrganizationID`.
- Added cross-tenant resource tampering detection that returns HTTP 403 `AUTH_FORBIDDEN` if an organization identifier and event identifier belong to different tenants.
- Hardened `CorrelationIDMiddleware` in [`backend/internal/middleware/request_id.go`](file:///root/galeriku/backend/internal/middleware/request_id.go) with regex validation `^[a-zA-Z0-9_-]{1,64}$`, defending against header injection, CRLF splitting, and buffer attacks.
- Created tenant domain models and role constants in [`backend/internal/domain/tenant.go`](file:///root/galeriku/backend/internal/domain/tenant.go).
- Created PostgreSQL tenant repository in [`backend/internal/repository/tenant_repository.go`](file:///root/galeriku/backend/internal/repository/tenant_repository.go) with integration tests against PostgreSQL 16.
- Added 20-worker concurrent goroutine test in [`backend/internal/middleware/rbac_test.go`](file:///root/galeriku/backend/internal/middleware/rbac_test.go) verifying request context isolation under `-race`.
- Protected `GET /api/v1/auth/me` with `Authenticate` middleware in [`backend/internal/handler/router.go`](file:///root/galeriku/backend/internal/handler/router.go).

#### Added - Task 2.3: Authentication Endpoints, Repositories & Refresh Token Rotation
- Created auth domain models and DTOs in [`backend/internal/domain/auth.go`](file:///root/galeriku/backend/internal/domain/auth.go).
- Implemented PostgreSQL repositories [`backend/internal/repository/user_repository.go`](file:///root/galeriku/backend/internal/repository/user_repository.go) and [`backend/internal/repository/auth_repository.go`](file:///root/galeriku/backend/internal/repository/auth_repository.go) handling transactional user creation with default `BUYER` capability, failed login attempt tracking, refresh token persistence, and OTP challenges.
- Implemented business logic in [`backend/internal/usecase/auth_usecase.go`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go) for register, login, refresh token rotation (RTR), logout, OTP dispatch, OTP verification, and current user profile retrieval.
- Implemented Refresh Token Rotation with 256-bit entropy CSPRNG (`crypto/rand`), storing SHA-256 digests in `auth_refresh_tokens.token_hash`. Raw tokens are never persisted in the database.
- Implemented token reuse detection: if a revoked refresh token is presented, the entire token family (`family_id`) is immediately revoked under database row locks (`SELECT ... FOR UPDATE`).
- Added login rate limiting in [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go) (max 10 attempts per 60 seconds per identifier) evaluated before Argon2id verification to prevent CPU exhaustion attacks.
- Implemented keyed HMAC-SHA-256 OTP hashing in [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go) (`HashKeyedOTP`, `VerifyKeyedOTP`) fitting `auth_otp_challenges.code_hash VARCHAR(64)`.
- Implemented HTTP handlers in [`backend/internal/handler/auth_handler.go`](file:///root/galeriku/backend/internal/handler/auth_handler.go) for endpoints:
  - `POST /api/v1/auth/register` (201 Created)
  - `POST /api/v1/auth/login` (200 OK)
  - `POST /api/v1/auth/refresh` (200 OK)
  - `POST /api/v1/auth/logout` (200 OK)
  - `POST /api/v1/auth/otp/send` (200 OK)
  - `POST /api/v1/auth/otp/verify` (200 OK)
  - `GET /api/v1/auth/me` (200 OK)

#### Added - Task 2.2: Cryptographic Primitives
- Implemented OWASP-compliant Argon2id password hashing and constant-time verification in [`backend/internal/security/hasher.go`](file:///root/galeriku/backend/internal/security/hasher.go) (memory 64MB, iterations 3, parallelism 4, 16-byte salt, 32-byte key) with password complexity validation.
- Implemented stateless JWT HS256 access token service in [`backend/internal/security/jwt.go`](file:///root/galeriku/backend/internal/security/jwt.go) with strict enforcement of 256-bit secret minimum, 15-minute default expiration, UUIDv7 `jti`, and explicit rejection of `none` algorithm.
- Implemented 6-digit numeric OTP generator and constant-time verifier in [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go).
- Implemented atomic Redis-backed rate limiter in [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go) enforcing dual rate limits (1 send per 60s cooldown, max 3 sends per 10m burst window) with SHA-256 identifier hashing to protect PII.

#### Added - Task 2.1: Database Migration 000002 & UUIDv7 Regression Fix
- Fixed integer overflow in PostgreSQL UUIDv7 generator functions `generate_uuid_v7()` and `uuid_generate_v7()` via `& 255` bitwise masking.
- Implemented transactional schema migration in [`migrations/000002_identity_and_sessions.up.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.up.sql):
  - Created table `user_credentials` with Argon2id password hash, lockout tracking, and foreign key cascade to `users`.
  - Created table `auth_refresh_tokens` with token SHA-256 hash, family ID, revocation status, and index coverage.
  - Created table `auth_otp_challenges` with check constraint `chk_otp_target`.
  - Created table `host_bank_accounts` with encrypted account number, display mask, and foreign key to `host_organizations`.
  - Added partial unique index `uq_active_host_leader` on `host_memberships` ensuring strictly one active Leader per host organization.
  - Attached foreign key constraints for Domain 01 (Identity & Host).
- Implemented symmetric rollback in [`migrations/000002_identity_and_sessions.down.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.down.sql).
- Implemented programmatic migration runner in [`backend/internal/database/migration_runner.go`](file:///root/galeriku/backend/internal/database/migration_runner.go) with table `schema_migrations`.
- Added schema completeness verifier in [`backend/internal/database/migrator.go`](file:///root/galeriku/backend/internal/database/migrator.go) asserting 81 base tables.

---

## [0.1.0] - 2026-10-15

### Phase 1: Core Foundation & Infrastructure (Completed)

#### Added
- Docker Compose configuration in [`docker-compose.yml`](file:///root/galeriku/docker-compose.yml) orchestrating PostgreSQL 16, Redis 7, RabbitMQ 3.13, Qdrant v1.11, and MinIO S3 with persistent volumes and healthchecks.
- Automated MinIO S3 bucket provisioner in [`backend/cmd/provisioner/main.go`](file:///root/galeriku/backend/cmd/provisioner/main.go) creating `galeriku-master-vault` (private) and `galeriku-public-media` (public-read) with read policies.
- Initial PostgreSQL DDL migration in [`migrations/000001_initial_schema.up.sql`](file:///root/galeriku/migrations/000001_initial_schema.up.sql) defining 76 initial tables across 8 domains, UUIDv7 generation, and domain indexes.
- Database connection pool service in [`backend/internal/database/postgres.go`](file:///root/galeriku/backend/internal/database/postgres.go) using `pgxpool` with configurable SSL mode and credentials.
- System health check endpoint `GET /api/v1/health` in [`backend/internal/handler/router.go`](file:///root/galeriku/backend/internal/handler/router.go) reporting database connectivity and degraded status (503 Service Unavailable) on connection failure.
- Correlation ID middleware in [`backend/internal/middleware/request_id.go`](file:///root/galeriku/backend/internal/middleware/request_id.go) managing `X-Request-ID` across request contexts and standard HTTP response envelopes.
- Frontend TypeScript workspace under [`web/`](file:///root/galeriku/web) with Vitest configuration and Antislop design system tokens in [`web/src/tokens/design-system.ts`](file:///root/galeriku/web/src/tokens/design-system.ts).
- Antislop copy linter in [`web/scripts/lint-copy.js`](file:///root/galeriku/web/scripts/lint-copy.js) enforcing zero em dash characters (U+2014) per Antislop Hard Gate R-02.
- Unified build and test targets in [`Makefile`](file:///root/galeriku/Makefile) for `test`, `test-backend`, `test-web`, `typecheck`, `lint-copy`, and `test-integration`.
