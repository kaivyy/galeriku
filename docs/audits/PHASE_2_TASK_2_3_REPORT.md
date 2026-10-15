# Galeriku: Phase 2 Task 2.3 Implementation Report
## Authentication Endpoints, Repositories, Session Management & Refresh Token Rotation

- **Date:** 15 Oktober 2026
- **Status:** COMPLETED and VERIFIED
- **Branch:** `main` (uncommitted per explicit instruction)
- **Scope:** Task 2.3 only (Authentication endpoints, user & auth repositories, auth usecase, refresh token rotation, keyed OTP challenge workflow, login rate limiting; zero Task 2.4+ features, zero new database migrations)

---

## 1. Executive Summary

Task 2.3 implements the core authentication endpoints, persistent session management, refresh token rotation (RTR), and OTP verification workflows for Galeriku Phase 2 (Identity, Authentication & Host B2B Portal). The implementation builds directly upon the schema defined in Task 2.1 (`migrations/000002_identity_and_sessions.up.sql`) and cryptographic primitives delivered in Task 2.2 (`backend/internal/security/`).

All requirements authorized for Task 2.3 across `PRD.md`, `PHASE_2_IMPLEMENTATION_PLAN.md`, `PHASE_2_PREFLIGHT_AUDIT.md`, `API_SPEC.md`, and `DATABASE_SPEC.md` were implemented using Test-Driven Development (TDD). Furthermore, all mandatory conditions (F-01 through F-04) prescribed in the Task 2.2 Final Acceptance Review ([`docs/audits/PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md)) were rigorously addressed:

1. **Condition F-01 (Keyed OTP Digest):** Stored `code_hash` in `auth_otp_challenges` is computed via `hex(HMAC-SHA256(otp, OTP_SECRET_KEY))`. Server secret enforcement ensures secrets below 16 bytes are rejected. Raw 6-digit OTP codes are never logged, never stored in PostgreSQL, and never returned in API payloads.
2. **Condition F-02 (Login Rate Limiting):** Login endpoint is protected by an atomic Redis Lua rate limiter (`AllowLogin`) enforcing a maximum of 10 failed or total attempts per 60 seconds per target identifier before invoking the CPU-intensive Argon2id verification function.
3. **Condition F-03 (Refresh Token Entropy & Rotation Storage):** Refresh tokens are minted with 32 bytes (256 bits) of CSPRNG entropy from `crypto/rand.Read` and encoded in URL-safe base64. Tokens are stored in PostgreSQL exclusively as a 64-character SHA-256 hex digest (`auth_refresh_tokens.token_hash`). Token rotation uses row-level locking (`SELECT ... FOR UPDATE`); reuse of a previously rotated token immediately revokes all tokens sharing the same `family_id` with reason `REUSE_DETECTED`.
4. **Condition F-04 (Git State Preservation):** All changes reside uncommitted in the working tree. Pre-existing whitespace on `Makefile:52` was preserved unmodified.

All unit and integration tests passed 100%. An independent code review was conducted and all identified findings were remediated.

---

## 2. Documents Actually Inspected

The following repository documents were inspected prior to and during implementation:

| Document Path | Role in Specification Hierarchy | Key Information Extracted |
|---|---|---|
| [`PRD.md`](file:///root/galeriku/PRD.md) | Product Authority | User capability structure (`BUYER`, `CREATOR`, `HOST_MEMBER`, `SUPER_ADMIN`), security baseline, UU PDP No. 27/2022 privacy compliance. |
| [`docs/API_SPEC.md`](file:///root/galeriku/docs/API_SPEC.md) | API Contract Reference | Exact JSON payloads, status codes, and headers for `/api/v1/auth/*` endpoints (lines 90-218, 1309-1318). |
| [`docs/DATABASE_SPEC.md`](file:///root/galeriku/docs/DATABASE_SPEC.md) | Database Specification | Column data types, indices, and foreign keys for `users`, `user_profiles`, `user_credentials`, `auth_refresh_tokens`, and `auth_otp_challenges`. |
| [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) | Technical Implementation Plan | Task 2.3 scope, repository interfaces, usecase methods, handler signatures, and acceptance criteria (lines 149-185). |
| [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md) | Security Baseline Audit | Dual-token auth architecture, Argon2id verification rules, refresh token rotation policies, OTP challenge flows. |
| [`docs/audits/PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md) | Acceptance Verdict & Conditions | Mandatory conditions F-01 (HMAC keyed OTP digest), F-02 (login rate limiter), F-03 (CSPRNG refresh token hash), F-04 (git preservation). |
| [`migrations/000002_identity_and_sessions.up.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.up.sql) | DDL Migration Evidence | Database schema definition for identity tables, partial indices, check constraints, and foreign key cascades. |
| [`backend/internal/config/config.go`](file:///root/galeriku/backend/internal/config/config.go) | Configuration Registry | Environment configuration loader for PostgreSQL, Redis, JWT secrets, and OTP secrets. |

---

## 3. Requirements Traced to Source Files

| Requirement | Document Source | Section / Line | Documented Specification | Satisfied In Code |
|---|---|---|---|---|
| **Register Endpoint** (`POST /api/v1/auth/register`) | `API_SPEC.md` | L90-137 | Accepts email, phone, password, full_name. Returns 201 Created with user summary, tokens, capabilities. | [`backend/internal/handler/auth_handler.go:49-92`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L49-L92) |
| **Registration Transactionality** | `PHASE_2_IMPLEMENTATION_PLAN.md` | L164 | Creates `users`, `user_profiles`, `user_credentials`, assigns initial `BUYER` capability within a single database transaction. | [`backend/internal/repository/user_repository.go:34-80`](file:///root/galeriku/backend/internal/repository/user_repository.go#L34-L80) |
| **Login Endpoint** (`POST /api/v1/auth/login`) | `API_SPEC.md` | L139-170 | Accepts identifier (email or phone) and password. Returns 200 OK with access and refresh tokens. | [`backend/internal/handler/auth_handler.go:94-131`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L94-L131) |
| **Login Rate Limiter (Condition F-02)** | `PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md` | L229-230 | Max 10 attempts per 60s per target identifier evaluated before PostgreSQL query or Argon2id verification. | [`backend/internal/security/limiter.go:121-143`](file:///root/galeriku/backend/internal/security/limiter.go#L121-L143), [`auth_usecase.go:119-124`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go#L119-L124) |
| **Account Enumeration Timing Protection** | Security Standard | OWASP ASVS | Constant-time response behavior when user identifier is not found; executes dummy Argon2id verification. | [`backend/internal/usecase/auth_usecase.go:128-132`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go#L128-L132) |
| **Refresh Endpoint** (`POST /api/v1/auth/refresh`) | `API_SPEC.md` | L172-192 | Accepts refresh_token. Returns 200 OK with new access and rotated refresh token. | [`backend/internal/handler/auth_handler.go:133-157`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L133-L157) |
| **Refresh Token Entropy (Condition F-03)** | `PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md` | L231-232 | Refresh tokens use 32 bytes (256 bits) of CSPRNG entropy from `crypto/rand.Read`, stored as SHA-256 hex digest. | [`backend/internal/usecase/auth_usecase.go:417-430`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go#L417-L430) |
| **Atomic Token Rotation & Reuse Detection** | `PHASE_2_PREFLIGHT_AUDIT.md` | L147-152 | Row-level locking with `FOR UPDATE`. If rotated token is reused, revokes entire family with `REUSE_DETECTED`. | [`backend/internal/repository/auth_repository.go:88-150`](file:///root/galeriku/backend/internal/repository/auth_repository.go#L88-L150), [`auth_usecase.go:204-219`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go#L204-L219) |
| **Logout Endpoint** (`POST /api/v1/auth/logout`) | `API_SPEC.md` | L194-204 | Accepts refresh_token. Revokes token record in database and returns 200 OK. | [`backend/internal/handler/auth_handler.go:159-178`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L159-L178) |
| **OTP Send Endpoint** (`POST /api/v1/auth/otp/send`) | `API_SPEC.md` | L206-218 | Accepts target, channel, purpose. Enforces 60s cooldown and 3/10m burst rate limit. | [`backend/internal/handler/auth_handler.go:180-209`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L180-L209) |
| **Keyed OTP Digest (Condition F-01)** | `PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md` | L227-228 | Computes `code_hash = hex(HMAC-SHA256(otp, secret))`. Rejects secret < 16 bytes. Raw OTP never stored. | [`backend/internal/security/otp.go:58-75`](file:///root/galeriku/backend/internal/security/otp.go#L58-L75), [`auth_usecase.go:297-308`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go#L297-L308) |
| **OTP Verify Endpoint** (`POST /api/v1/auth/otp/verify`) | `API_SPEC.md` | L1309-1318 | Accepts challenge_id, code. Validates status, expiry, attempts. Single-use atomic update. Returns verification token. | [`backend/internal/handler/auth_handler.go:211-240`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L211-L240), [`auth_usecase.go:323-380`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go#L323-L380) |
| **Current User Endpoint** (`GET /api/v1/auth/me`) | `API_SPEC.md` | L104-118 | Protected endpoint. Reads Bearer JWT, verifies claims, loads profile, global capabilities, host memberships. | [`backend/internal/handler/auth_handler.go:242-273`](file:///root/galeriku/backend/internal/handler/auth_handler.go#L242-L273) |
| **Standard JSON Response Envelope** | Architecture Standard | Standard RFC | Enforces `{ success: true/false, data: ..., meta: { request_id, timestamp } }`. | [`backend/internal/handler/router.go:24-42`](file:///root/galeriku/backend/internal/handler/router.go#L24-L42) |

---

## 4. Detailed Component Implementation

### 4.1 Domain Models and Envelopes (`backend/internal/domain/auth.go`)
Defines strongly typed entities matching database tables and API contracts:
- `User`, `UserProfile`, `UserCredential`, `UserCapability`, `HostMembership`.
- `AuthRefreshToken` with fields for `FamilyID`, `TokenHash`, `IsRevoked`, `RevokedReason`, `ExpiresAt`.
- `AuthOTPChallenge` with fields for `ChallengeID`, `Target`, `Channel`, `Purpose`, `CodeHash`, `ExpiresAt`, `VerifiedAt`, `AttemptsCount`, `MaxAttempts`, `Status`.
- Request and response DTOs for registration, login, token refresh, logout, OTP send, OTP verify, and user profile summaries.

### 4.2 Keyed OTP Digest Implementation (Condition F-01)
To eliminate the offline dictionary vulnerability on the 1,000,000 permutation search space of 6-digit numeric OTPs:
- Extended [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go) with `HashKeyedOTP(code, secret)` and `VerifyKeyedOTP(code, secret, expectedHash)`.
- Uses `hmac.New(sha256.New, secret)`. Computes 32-byte HMAC resulting in a 64-character lowercase hex string fitting column `auth_otp_challenges.code_hash VARCHAR(64)` without requiring schema alterations.
- Enforces `len(secret) >= 16`. Shorter keys return `ErrWeakSecret`.
- Comparison uses `subtle.ConstantTimeCompare` with whitespace and case normalization.
- Configuration loaded via `OTP_SECRET_KEY` in `backend/internal/config/config.go` with safe fallback defaults for test and development.

### 4.3 Login Rate Limiting (Condition F-02)
To mitigate CPU exhaustion attacks targeting Argon2id password verification:
- Extended [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go) with `AllowLogin(ctx, identifier)`.
- Uses Redis Lua script `loginRateLimitScript` enforcing a maximum of 10 attempts per 60-second window.
- Key format uses Redis Cluster hash tags `{...}` and SHA-256 identifier truncation for privacy: `ratelimit:login:{<sha256_16B>}:burst`.
- In `auth_usecase.go`, `AllowLogin` is invoked before any database user lookup or Argon2id password derivation. If the limit is exceeded, an error is returned immediately with HTTP 429 Too Many Requests.

### 4.4 Refresh Token Entropy, Storage & Rotation (Condition F-03)
- **Token Generation:** `GenerateCSPRNGRefreshToken()` samples 32 bytes (256 bits) from `crypto/rand.Read` and encodes it in URL-safe base64 (`base64.RawURLEncoding.EncodeToString`). The raw token is 43 characters with 256 bits of entropy.
- **Storage Digest:** In `auth_repository.go`, only the SHA-256 hexadecimal digest (`hex(sha256(rawToken))`) is persisted into `auth_refresh_tokens.token_hash`. The raw token is never written to PostgreSQL.
- **Atomic Rotation under Row Lock:** In `RotateRefreshToken`, the transaction executes:
  ```sql
  SELECT token_id, family_id, user_id, is_revoked, expires_at
  FROM auth_refresh_tokens
  WHERE token_hash = $1
  FOR UPDATE
  ```
- **Reuse Detection & Family Revocation:**
  If the queried token has `is_revoked = true`:
  ```sql
  UPDATE auth_refresh_tokens
  SET is_revoked = true, revoked_at = NOW(), revoked_reason = 'REUSE_DETECTED'
  WHERE family_id = $1 AND is_revoked = false
  ```
  The transaction aborts and returns `ErrTokenReuseDetected`, invalidating all active sessions for that token family.
- If valid, the current token is revoked with `ROTATED`, and the replacement token is inserted with the identical `family_id`.

### 4.5 Account Enumeration & Timing Attack Defenses
To prevent attackers from differentiating registered versus unregistered emails or phone numbers:
- In `auth_usecase.go:Login`, if `repo.GetByIdentifier` returns user not found, the usecase executes a dummy Argon2id verification against a pre-computed valid Argon2id hash with identical parameters (`m=64MB, t=3, p=4`).
- This normalizes response latency to ~300ms regardless of whether the account exists, eliminating observable timing deltas.

### 4.6 Concurrency Control on OTP Verification
- In `auth_repository.go:MarkOTPVerified`, the update query requires:
  ```sql
  UPDATE auth_otp_challenges
  SET status = 'VERIFIED', verified_at = NOW()
  WHERE challenge_id = $1 AND status = 'PENDING'
  ```
- The method checks `RowsAffected() > 0`. If two concurrent requests present the correct OTP simultaneously, only the first request succeeds in transitioning the state. The second request receives zero updated rows and is rejected.

### 4.7 Array Initialization and Null-Safety
- Slices returned from `GetUserCapabilities` and `GetUserHostMemberships` in `backend/internal/repository/user_repository.go` are initialized with `make([]string, 0)` and `make([]domain.HostMembership, 0)`.
- When serialized to JSON, empty collections serialize as `[]` instead of `null`, preventing null-pointer exceptions in frontend and mobile clients.

---

## 5. Files Changed and Justification

| File Path | Action | Rationale |
|---|---|---|
| [`backend/internal/domain/auth.go`](file:///root/galeriku/backend/internal/domain/auth.go) | Created | Domain entities, request/response DTOs, and session summary models. |
| [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go) | Modified | Added `HashKeyedOTP`, `VerifyKeyedOTP`, `GenerateKeyedOTP` with constant-time verification. |
| [`backend/internal/security/otp_test.go`](file:///root/galeriku/backend/internal/security/otp_test.go) | Modified | Unit tests for keyed OTP generation, verification, and secret length checks. |
| [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go) | Modified | Added `AllowLogin` rate limiter with atomic Redis Lua script and PII-safe hashing. |
| [`backend/internal/security/limiter_test.go`](file:///root/galeriku/backend/internal/security/limiter_test.go) | Modified | Live Redis integration tests for login throttling. |
| [`backend/internal/config/config.go`](file:///root/galeriku/backend/internal/config/config.go) | Modified | Added `OTPSecret` configuration key loaded from `OTP_SECRET_KEY`. |
| [`backend/internal/repository/user_repository.go`](file:///root/galeriku/backend/internal/repository/user_repository.go) | Created | Implements transactional user creation, credential queries, capability lookups, failed attempts tracking. |
| [`backend/internal/repository/user_repository_test.go`](file:///root/galeriku/backend/internal/repository/user_repository_test.go) | Created | Integration tests for user repository against live PostgreSQL container. |
| [`backend/internal/repository/auth_repository.go`](file:///root/galeriku/backend/internal/repository/auth_repository.go) | Created | Implements refresh token storage, atomic rotation, family reuse revocation, and OTP challenge lifecycle. |
| [`backend/internal/repository/auth_repository_test.go`](file:///root/galeriku/backend/internal/repository/auth_repository_test.go) | Created | Integration tests for refresh token rotation, reuse detection, and OTP challenges. |
| [`backend/internal/usecase/auth_usecase.go`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go) | Created | Orchestrates registration, login, token rotation, logout, OTP flows, and timing mitigation. |
| [`backend/internal/usecase/auth_usecase_test.go`](file:///root/galeriku/backend/internal/usecase/auth_usecase_test.go) | Created | Unit test suite for usecase workflows with mock repositories and rate limiters. |
| [`backend/internal/handler/auth_handler.go`](file:///root/galeriku/backend/internal/handler/auth_handler.go) | Created | Implements HTTP handlers for all 7 authorized authentication endpoints. |
| [`backend/internal/handler/auth_handler_test.go`](file:///root/galeriku/backend/internal/handler/auth_handler_test.go) | Created | End-to-end HTTP integration tests for all 7 endpoints with JSON validation. |
| [`backend/internal/handler/router.go`](file:///root/galeriku/backend/internal/handler/router.go) | Modified | Added `SetupRoutesWithAuth` to wire authentication endpoints into chi router. |
| [`backend/cmd/server/main.go`](file:///root/galeriku/backend/cmd/server/main.go) | Modified | Wires PostgreSQL, Redis, repositories, usecase, and handler in server entrypoint. |

---

## 6. Verification Tests Actually Executed

All test suites were executed against the active repository and live Docker containers (`galeriku-postgres` on port 5433, `galeriku-redis` on port 6380, `galeriku-minio` on port 9010):

| Test Command | Scope | Exit Code | Result Summary |
|---|---|---|---|
| `cd backend && go test -v -count=1 ./internal/security/...` | Security Primitives | 0 | **24 of 24 tests passed** (Argon2id, JWT HS256, OTP, Keyed OTP, OTP rate limiter, Login rate limiter). |
| `cd backend && go test -v -count=1 ./internal/repository/...` | Database Repositories | 0 | **3 of 3 integration suites passed** against live PostgreSQL 16 container. |
| `cd backend && go test -v -count=1 ./internal/usecase/...` | Business Logic Usecase | 0 | **6 of 6 usecase suites passed** (Registration, weak password rejection, login, rate limiting, RTR reuse detection, OTP). |
| `cd backend && go test -v -count=1 ./internal/handler/...` | HTTP Handlers & Router | 0 | **7 of 7 handler suites passed** (Health check, Registration, Login, Refresh, Reuse detection, OTP flow, Auth Me). |
| `make test` | Workspace Unit Gates | 0 | **100% Passed**. Backend short tests passed; Web Vitest (4/4) passed; `tsc --noEmit` passed; `lint-copy` passed. |
| `make test-integration` | Live Docker Integration | 0 | **100% Passed**. MinIO S3 provisioner, UUIDv7 regression, 000002 migration, rollback/reapply, 81 tables, and all repository integration tests passed. |
| `cd backend && go test -v -count=1 ./...` | Backend Full Suite | 0 | Non-cached run across all packages passed with 0 failures. |

---

## 7. Independent Review Evidence and Resolutions

An independent Senior Application Security Engineer, Database Architect, and Go Code Reviewer subagent was dispatched to audit the implementation.

- **Reviewer Verdict:** "Ready to proceed: With fixes".
- **Findings Reported and Resolved:**
  1. *Finding 1 (Timing Attack Discrepancy on Login):*
     - *Issue:* When user identifier was not found, the code previously computed SHA-256 (~1 microsecond), while real user authentication computed Argon2id (~300 milliseconds).
     - *Resolution:* Replaced SHA-256 with a dummy Argon2id verification (`hasher.VerifyPassword("dummy_pwd", dummyArgon2idHash)`), ensuring uniform latency.
  2. *Finding 2 (OTP Replay Concurrency Gap):*
     - *Issue:* In concurrent requests, two requests with the same valid OTP could both read status `PENDING` before updating to `VERIFIED`.
     - *Resolution:* Added `AND status = 'PENDING'` to the SQL update in `MarkOTPVerified` and checked `RowsAffected() > 0`.
  3. *Finding 3 (Empty Slice JSON Deserialization):*
     - *Issue:* Unallocated slices in `user_repository.go` serialized as `null` in JSON responses.
     - *Resolution:* Initialized empty capability and membership slices with `make(..., 0)` so JSON outputs `[]`.
  4. *Finding 4 (Static Token Digest Collisions in Tests):*
     - *Issue:* Hardcoded token digests in `auth_repository_test.go` caused unique constraint violations (`23505`) on repeated test runs.
     - *Resolution:* Replaced static hashes with dynamic UUIDv7-derived token digests.

All remediations were implemented and re-verified with fresh test runs.

---

## 8. Pre-Existing Findings vs Newly Introduced Findings

- **Pre-Existing Finding (Unresolved from Task 2.1):**
  - `Makefile:52: new blank line at EOF.` triggers exit code 2 on `git diff --check`. Preserved intact per instructions.
- **Pre-Existing Finding (Unverified Data Preservation from Task 2.1):**
  - Table data preservation on non-empty databases remains `NOT VERIFIED` (test databases initialized with 0 rows).
- **Newly Introduced Findings in Task 2.3:**
  - Zero new security defects. All mandatory conditions F-01 through F-04 are verified.

---

## 9. Proposed Changes Deliberately Deferred (Scope Lock)

The following items belong to subsequent tasks and were deliberately not implemented in Task 2.3:
1. Multi-capability RBAC middleware (`RequireCapability("CREATOR")`, `RequireCapability("HOST_MEMBER")`) -> Deferred to Task 2.4.
2. Tenant isolation and Host context middleware (`TenantMiddleware`, `HostOrganization` context injection) -> Deferred to Task 2.4.
3. Host onboarding and B2B portal application submission (`POST /api/v1/hosts/apply`) -> Deferred to Task 2.5.
4. Host team member invitations and management (`POST /api/v1/hosts/invitations`) -> Deferred to Task 2.6.
5. Host bank accounts and AES-256-GCM / KMS encryption -> Deferred to Task 2.7.
6. Third-party WhatsApp/SMS gateway webhook or HTTP client integration -> Deferred to future integration task.

---

## 10. Antislop Craftsmanship Verification

All source files, test suites, documentation, and comments were audited against the Antislop rulebook:
- **Hard Gate R-02 (No Em Dashes):** Verified zero em dash characters (U+2014) in all newly created and modified files via regex scanning.
- **Hard Gate R-03 (Tap Target Metrics):** Web workspace verified to maintain 44px minimum tap targets.
- **Copy Linter:** `npm run lint:copy` executed cleanly with 0 violations.

---

## 11. Explicit Completion Verdict

Task 2.3 is **COMPLETE and VERIFIED**. All 7 authentication endpoints, repositories, session management workflows, refresh token rotation with reuse detection, keyed OTP challenges, and login rate limiting are implemented, verified, and passing 100% of tests.

Progression to Task 2.4 is pending explicit user authorization. All changes reside safely uncommitted in the working tree.
