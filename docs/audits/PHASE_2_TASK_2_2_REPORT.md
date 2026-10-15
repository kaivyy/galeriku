# Galeriku: Phase 2 Task 2.2 Implementation Report
## Cryptographic Primitives: Argon2id, JWT HS256, OTP Generator & Redis Rate Limiter

- **Date:** 15 Oktober 2026
- **Status:** COMPLETED and VERIFIED
- **Branch:** `main` (uncommitted per explicit instruction)
- **Scope:** Task 2.2 only (Core Cryptographic Primitives & Rate Limiting, zero HTTP handlers, zero new migrations)

---

## 1. Executive Summary

Task 2.2 implements the foundational cryptographic primitives and rate-limiting infrastructure for Galeriku Phase 2 (Identity, Authentication & Host B2B Portal). The implementation strictly adheres to the approved specifications across `PRD.md`, `PHASE_2_IMPLEMENTATION_PLAN.md`, `PHASE_2_PREFLIGHT_AUDIT.md`, and `API_SPEC.md`.

All four candidate components designated for Task 2.2 were designed, implemented, and verified using Test-Driven Development (TDD):
1. **Argon2id Hasher (`backend/internal/security/hasher.go`)**: OWASP-compliant password hashing (`m=64MB, t=3, p=4, salt=16B, key=32B`) with constant-time verification, defensive parameter validation against DoS, and complexity checking.
2. **JWT HS256 Service (`backend/internal/security/jwt.go`)**: Stateless access token minting and verification requiring a minimum 256-bit signing key, strictly enforcing HS256 while proactively blocking algorithm confusion attacks (`none`, `RS256`), with UUIDv7 `jti` tracking and comprehensive claims validation.
3. **OTP Generator (`backend/internal/security/otp.go`)**: Cryptographically secure 6-digit numeric OTP generation (`crypto/rand.Int`), constant-time verification, and deterministic SHA-256 digest computation matching PostgreSQL column `auth_otp_challenges.code_hash VARCHAR(64)`.
4. **Redis Rate Limiter (`backend/internal/security/limiter.go`)**: Atomic multi-window rate limiter executing via a single-roundtrip Redis Lua script enforcing a 1-send-per-60s cooldown limit and a 3-sends-per-10m burst ceiling, with PII-safe key hashing and Redis Cluster hash tagging `{...}`.

All unit and integration tests passed 100%. An independent code review was conducted and its recommendations were addressed.

---

## 2. Documents Actually Inspected

The following repository documents were inspected prior to and during implementation:

| Document Path | Role in Specification Hierarchy | Key Information Extracted |
|---|---|---|
| [`PRD.md`](file:///root/galeriku/PRD.md) | Product Authority | User capability structure, security baseline, UU PDP No. 27/2022 compliance rules. |
| [`docs/IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/IMPLEMENTATION_PLAN.md) | Master Roadmap | Sprint 2 milestone definition, multi-capability RBAC overview, OTP WhatsApp/SMS rate limits. |
| [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) | Technical Implementation Plan | Task 2.2 scope, exact file paths, Argon2id parameters, JWT claims structure, and acceptance criteria (lines 121-146). |
| [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md) | Security Baseline Audit | Section 4.1 (JWT Dual-Token Pattern), Section 4.2 (Argon2id tuning), Section 4.3 (OTP rate limiting rules). |
| [`docs/API_SPEC.md`](file:///root/galeriku/docs/API_SPEC.md) | API Contract Reference | Domain 01 auth endpoints: register, login, refresh, otp/send, otp/verify, and auth/me payloads. |
| [`docs/DATABASE_SPEC.md`](file:///root/galeriku/docs/DATABASE_SPEC.md) | Database Data Dictionary | Schema definitions for Domain 01 (Identity & Host). |
| [`docs/audits/PHASE_2_TASK_2_1_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_REPORT.md) | Task 2.1 Evidence | Migration 000002 details, `user_credentials`, `auth_refresh_tokens`, and `auth_otp_challenges`. |
| [`docs/audits/PHASE_2_TASK_2_1_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_FINAL_ACCEPTANCE.md) | Acceptance Verdict | Conditions of progression and pre-existing findings. |
| [`backend/internal/config/config.go`](file:///root/galeriku/backend/internal/config/config.go) | Configuration Registry | `RedisHost`, `RedisPort`, `JWTSecret` configuration keys. |
| [`.env`](file:///root/galeriku/.env) | Environment Definitions | `REDIS_PORT=6380`, `JWT_SECRET`, and `JWT_ACCESS_TTL_SECONDS`. |

---

## 3. Requirements Traced to Source Files

| Component | Document Source | Section / Line | Documented Requirement | Satisfied In Code |
|---|---|---|---|---|
| **Argon2id** | `PHASE_2_PREFLIGHT_AUDIT.md` | L155-164 | Memory 64MB, Time 3, Parallelism 4, Salt 16B, Key 32B. | [`backend/internal/security/hasher.go`](file:///root/galeriku/backend/internal/security/hasher.go#L29-L35) |
| **Argon2id Format** | `PHASE_2_IMPLEMENTATION_PLAN.md` | L137 | Output format `$argon2id$v=19$m=65536,t=3,p=4$...`. | [`backend/internal/security/hasher.go`](file:///root/galeriku/backend/internal/security/hasher.go#L58-L67) |
| **Password Complexity** | `PHASE_2_PREFLIGHT_AUDIT.md` | L163 | Min 8 chars, 1 uppercase, 1 lowercase, 1 digit, 1 symbol. | [`backend/internal/security/hasher.go`](file:///root/galeriku/backend/internal/security/hasher.go#L143-L177) |
| **JWT Algorithm** | `PHASE_2_PREFLIGHT_AUDIT.md` | L143 | HMAC-SHA256 (`HS256`) with minimum 256-bit key. | [`backend/internal/security/jwt.go`](file:///root/galeriku/backend/internal/security/jwt.go#L36-L38) |
| **JWT Claims** | `PHASE_2_PREFLIGHT_AUDIT.md` | L145 | `sub` (User ID), `email`, `caps` (array), `jti` (UUIDv7). | [`backend/internal/security/jwt.go`](file:///root/galeriku/backend/internal/security/jwt.go#L27-L32) |
| **JWT Lifetime** | `PHASE_2_PREFLIGHT_AUDIT.md` | L144 | 15 minutes default validity. | [`backend/internal/security/jwt.go`](file:///root/galeriku/backend/internal/security/jwt.go#L53) |
| **OTP Format** | `PHASE_2_PREFLIGHT_AUDIT.md` | L166 | 6-digit random numeric generated via `crypto/rand`. | [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go#L26-L38) |
| **OTP Storage** | `PHASE_2_PREFLIGHT_AUDIT.md` | L168 | Database stores only SHA-256 hash, never raw code. | [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go#L40-L44) |
| **OTP Rate Limiting** | `PHASE_2_PREFLIGHT_AUDIT.md` | L170 | Max 1 send / 60s cooldown; max 3 sends / 10m burst window. | [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go#L20-L28) |
| **Rate Limit Atomicity** | `PHASE_2_IMPLEMENTATION_PLAN.md` | L141 | Protected by atomic Redis rate limiter. | [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go#L50-L84) |

---

## 4. Existing Interfaces and Dependencies Reused

- **`golang.org/x/crypto/argon2`**: Pre-existing in `go.mod`, utilized for RFC 9106 Argon2id hashing and key derivation.
- **`crypto/subtle`**: Standard library, utilized for constant-time comparison in password and OTP verification.
- **`crypto/rand`**: Standard library, utilized for CSPRNG salt generation and unbiased OTP generation.
- **`github.com/google/uuid`**: Pre-existing in `go.mod` (v1.6.0), utilized for UUIDv7 `jti` generation via `uuid.NewV7()`.
- **`github.com/golang-jwt/jwt/v5`**: Installed to implement RFC 7519 JSON Web Token signing and claim parsing.
- **`github.com/redis/go-redis/v9`**: Installed to communicate with the Redis 7 Docker container (`galeriku-redis` on port 6380) and execute atomic Lua rate-limiting scripts.

---

## 5. Files Changed and Justification

| File Path | Action | Rationale |
|---|---|---|
| [`backend/go.mod`](file:///root/galeriku/backend/go.mod) | Modified | Added `github.com/golang-jwt/jwt/v5` and `github.com/redis/go-redis/v9`. |
| [`backend/go.sum`](file:///root/galeriku/backend/go.sum) | Modified | Checksums for newly added dependencies. |
| [`backend/internal/security/hasher.go`](file:///root/galeriku/backend/internal/security/hasher.go) | Created | Implements Argon2id password hashing, verification, and complexity checks. |
| [`backend/internal/security/hasher_test.go`](file:///root/galeriku/backend/internal/security/hasher_test.go) | Created | Unit tests for Argon2id hashing, timing attack resistance, and edge cases. |
| [`backend/internal/security/jwt.go`](file:///root/galeriku/backend/internal/security/jwt.go) | Created | Implements stateless JWT HS256 access token minting and verification. |
| [`backend/internal/security/jwt_test.go`](file:///root/galeriku/backend/internal/security/jwt_test.go) | Created | Unit tests for JWT expiration, tampering, algorithm validation, and claims. |
| [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go) | Created | Implements 6-digit CSPRNG OTP generation and SHA-256 constant-time verification. |
| [`backend/internal/security/otp_test.go`](file:///root/galeriku/backend/internal/security/otp_test.go) | Created | Unit tests for OTP length, format, entropy, and hash verification. |
| [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go) | Created | Implements atomic Redis Lua rate limiter for OTP dispatch. |
| [`backend/internal/security/limiter_test.go`](file:///root/galeriku/backend/internal/security/limiter_test.go) | Created | Live Redis integration tests for cooldown (60s), burst (3/10m), and isolation. |

---

## 6. Argon2id Implementation and Parameter Analysis

### 6.1 Cryptographic Parameters
- **Algorithm:** Argon2id (Variant `0x13` / Version 19).
- **Memory Cost (`m`):** `65536` KiB (64 MiB).
- **Time Cost (`t`):** `3` iterations.
- **Parallelism (`p`):** `4` threads.
- **Salt Length:** `16` bytes (128 bits) sampled from `crypto/rand`.
- **Key Length:** `32` bytes (256 bits).
- **Encoded String:** Standard PHC format:
  `$argon2id$v=19$m=65536,t=3,p=4$<salt_base64_raw>$<key_base64_raw>`
  (e.g., length ~97 characters, well within `user_credentials.password_hash VARCHAR(255)`).

### 6.2 Defensive Boundary Hardening
To prevent denial-of-service (DoS) or panics when parsing untrusted or corrupted hash strings, `VerifyPassword` validates:
- `iterations > 0 && iterations <= 10`
- `parallelism > 0`
- `memory >= 8 * parallelism && memory <= 256 * 1024` (max 256 MB)
- `len(salt) >= 8`
- `len(expectedHash) >= 16`
Violations return `ErrInvalidHashFormat` without panicking. Additionally, `ValidatePasswordComplexity` enforces a 1024-byte upper bound on input passwords to prevent algorithmic complexity attacks.

---

## 7. JWT HS256 Implementation and Claims Structure

### 7.1 Security Architecture
- **Algorithm:** HMAC-SHA256 (`HS256`) exclusively.
- **Key Enforcement:** Minimum key length of 32 bytes (256 bits). Shorter keys return `ErrWeakSecret`.
- **Anti-Algorithm-Confusion:**
  1. Inspects unverified header (`parser.ParseUnverified`); if `alg != "HS256"`, returns `ErrInvalidAlgorithm`.
  2. Verifies `token.Method == jwt.SigningMethodHS256` inside `Keyfunc`.
  3. Enforces `jwt.WithValidMethods([]string{"HS256"})`.
  Tokens attempting the `"none"` algorithm attack or asymmetric key confusion are rejected.

### 7.2 Claims Payload (`AccessTokenClaims`)
- Standard Registered Claims:
  - `sub`: User ID string (UUIDv7).
  - `jti`: Unique token identifier generated via `uuid.NewV7()`.
  - `iss`: Constant `"galeriku"`.
  - `aud`: Constant `"galeriku-client"`.
  - `iat`: Epoch seconds at generation time.
  - `exp`: Epoch seconds (`iat + ttl`, default 15 minutes).
- Application Claims:
  - `email`: Normalized email string.
  - `caps`: Array of global capabilities (e.g. `["BUYER"]`, `["CREATOR"]`).

---

## 8. OTP Primitive and Limits

### 8.1 Generation and Representation
- **Code Length:** Exactly 6 numeric digits (`000000` to `999999`).
- **Entropy Source:** `crypto/rand.Int(rand.Reader, big.NewInt(1000000))`, zero-padded via `%06d`.
- **Storage Digest:** Lowercase hexadecimal SHA-256 digest (64 characters).
- **Verification:** Normalized to lowercase and compared using `subtle.ConstantTimeCompare`.
- **Security Invariant:** Raw OTP is never logged or stored. Database holds only `code_hash`.

---

## 9. Redis Rate Limiter and Policy

### 9.1 Policy Rules
1. **Cooldown Limit:** Maximum 1 OTP send per target identifier per 60 seconds.
2. **Burst Limit:** Maximum 3 OTP sends per target identifier per 10 minutes (600 seconds).

### 9.2 Redis Implementation Details
- **Atomic Lua Script:** Evaluates cooldown presence and burst counter in a single atomic transaction.
- **Cluster Compatibility:** Redis keys utilize hash tags `{...}` so that cooldown and burst keys for the same target map to identical hash slots:
  - `ratelimit:otp:{<target_sha256_hash>}:cooldown` (TTL 60s)
  - `ratelimit:otp:{<target_sha256_hash>}:burst` (TTL 600s, max 3)
- **Self-Healing Expiry:** If a burst key lacks a TTL (e.g., after failover), the script restores the 600s expiration.
- **Fail-Closed Stance:** Redis connectivity failures return `Allowed: false` and an error.

---

## 10. Tests Actually Executed and Exit Statuses

All tests were executed against the active repository and live Docker containers (`galeriku-postgres` on port 5433, `galeriku-redis` on port 6380, `galeriku-minio` on port 9010):

| Test Command | Scope | Exit Status | Result Summary |
|---|---|---|---|
| `cd backend && go test -v -count=1 ./internal/security/...` | Security Package | 0 | **21 of 21 tests passed** (Argon2id, JWT HS256, OTP, live Redis limiter). |
| `make test` | Workspace Unit Gates | 0 | Backend short tests PASSED; Web Vitest (4/4) PASSED; `tsc --noEmit` PASSED; `lint-copy` PASSED. |
| `make test-integration` | Live Service Integration | 0 | All live Docker suites PASSED (MinIO provisioner, UUIDv7 regression, 000002 migration, rollback/reapply, 81 tables, and security primitives). |
| `cd backend && go test -v ./...` | Backend Full Suite | 0 | Non-cached live run across all packages PASSED (100%). |

---

## 11. Independent Review Evidence

An independent code review subagent was dispatched to review the Task 2.2 implementation.
- **Reviewer Verdict:** "Ready to proceed: With fixes".
- **Findings Reported:** 0 Critical, 2 Important, 6 Minor.
- **Resolution of Findings:**
  1. *Important 1 (Unhandled Panic in `VerifyPassword`):* Added defensive boundary checks for memory (max 256MB), iterations (1-10), parallelism (>0), salt (min 8B), and hash length (min 16B).
  2. *Important 2 (Claims Error Misclassification):* Added explicit mapping for `jwt.ErrTokenInvalidClaims`, `jwt.ErrTokenInvalidIssuer`, and `jwt.ErrTokenInvalidAudience` to return `ErrInvalidClaims`.
  3. *Minor 1 (Redis Cluster Compatibility):* Added hash tags `{...}` in `cooldownKey` and `burstKey`.
  4. *Minor 2 (Self-Healing TTL):* Updated Lua script to re-apply TTL if `TTL(key) < 0`.
  5. *Minor 3 (UUIDv7 JTI Alignment):* Used `uuid.NewV7()` for access token `jti` claims.
  6. *Minor 4 (Case-Insensitive OTP Hash):* Normalized `expectedHash` to lowercase and trimmed whitespace.
  7. *Minor 5 (Defensive JWT Subject/Email Validation):* Rejected empty `userID` or `email` with `ErrInvalidClaims`.
  8. *Minor 6 (Max Password Length):* Enforced 1024-character upper bound in `ValidatePasswordComplexity`.

---

## 12. Pre-Existing Findings vs Newly Introduced Findings

- **Pre-Existing Finding (Unresolved from Task 2.1):**
  - `Makefile:52: new blank line at EOF.` causes `git diff --check` to exit with code 2. Per Task 2.2 non-negotiable rules, this line was preserved intact and not modified.
- **Pre-Existing Finding (Unverified Data Preservation from Task 2.1):**
  - Legacy data preservation on non-empty tables remains `NOT VERIFIED` (test databases initialized with 0 rows).
- **Newly Introduced Findings in Task 2.2:**
  - Zero new security defects or regressions. All 4 cryptographic components are verified.

---

## 13. Unresolved Requirements and Conflicts

No blocking conflicts were found between `PRD.md`, `API_SPEC.md`, `DATABASE_SPEC.md`, and `PHASE_2_IMPLEMENTATION_PLAN.md` regarding cryptographic primitives.
- **Design Alignment Note:** `API_SPEC.md:133` displays `expires_in: 3600` (1 hour) in sample JSON, whereas `PHASE_2_PREFLIGHT_AUDIT.md:144` and `PHASE_2_IMPLEMENTATION_PLAN.md:139` specify a 15-minute access token lifespan. In accordance with the source hierarchy (security baseline and implementation plan take precedence for session lifespans), `15 * time.Minute` was implemented as the default, while `GenerateAccessToken` accepts a configurable `ttl time.Duration`.

---

## 14. Proposed Changes Deliberately Deferred

The following items belong to subsequent tasks and were deliberately not implemented in Task 2.2:
1. HTTP endpoints (`POST /api/v1/auth/register`, `POST /api/v1/auth/login`) -> Deferred to Task 2.3.
2. Refresh Token Rotation (RTR) database persistence and reuse revocation -> Deferred to Task 2.3.
3. OTP dispatch endpoints and third-party WhatsApp/SMS gateway integration -> Deferred to Task 2.3.
4. Multi-capability RBAC middleware (`RequireCapability`) -> Deferred to Task 2.4.
5. AES-256-GCM / KMS encryption for `host_bank_accounts.account_number_encrypted` -> Deferred to Task 2.7.

---

## 15. Explicit Acceptance Status

Task 2.2 is **COMPLETE and VERIFIED**. All 4 cryptographic components are implemented, tested, and ready for consumption by Task 2.3 authentication handlers.

Progression to Task 2.3 is pending explicit user authorization. Working tree changes remain uncommitted.
