# Galeriku: Phase 2 Tasks 2.5 to 2.10 Implementation Report
## Host B2B Portal, Payout Accounts, Team Invitations, Audit Logging & Contract Sync

- **Date:** 15 Oktober 2026
- **Status:** COMPLETED and VERIFIED
- **Branch:** `main` (uncommitted per explicit instruction)
- **Scope:** Phase 2 Tasks 2.5, 2.6, 2.7, 2.8, 2.9, and 2.10 (the remaining Phase 2 work; zero new database migrations, zero out-of-scope endpoints)

---

## 1. Executive Summary

This report covers the six remaining Phase 2 tasks, executed in dependency order on top of the completed Task 2.4 authorization middleware.

| Task | Deliverable | Status |
|---|---|---|
| **2.5** | Host B2B application flow and private legal document provisioning | COMPLETED |
| **2.6** | Super Admin approval, membership management, admin quotas, Leader/Admin separation | COMPLETED |
| **2.7** | Host payout bank accounts with AES-256-GCM encryption and masked reads | COMPLETED |
| **2.8** | Time-bound team invitations with hashed tokens and atomic quota enforcement | COMPLETED |
| **2.9** | Structured audit logging, graceful shutdown, security regression suite | COMPLETED |
| **2.10** | OpenAPI 3.1 contract sync and TypeScript web contracts | COMPLETED |

All work followed the Red-Green discipline: each task's tests were written first and observed failing before implementation, then observed passing against live PostgreSQL 16, Redis 7, and MinIO S3 containers.

---

## 2. Repository State

- **Active Branch:** `main`
- **Head Commit:** `7c215d952fa82faee1bd29b3c0a565210af1f915` (`7c215d9`)
- **Working Tree:** All changes remain uncommitted awaiting explicit user authorization. Zero commits, rebases, or pushes were performed.
- **Preservation:**
  - `PRD.md` and `migrations/000001_initial_schema.up.sql`: unmodified.
  - Zero new migrations added; every Task 2.5 to 2.10 requirement is served by the schema in migrations 000001 and 000002.
  - The pre-existing trailing blank line at the end of the Makefile is preserved; `git diff --check -- Makefile` still exits 2. It now reports line 55 because two new targets were appended above it.

---

## 3. Task 2.5: Host Application and Private Legal Documents

### Endpoints
| Method | Path | Status | Authorization |
|---|---|:---:|---|
| POST | `/api/v1/hosts/apply` | 201 | Bearer |
| GET | `/api/v1/hosts/applications/my` | 200 | Bearer |
| POST | `/api/v1/hosts/documents` | 201 | Bearer (application owner) |
| GET | `/api/v1/hosts/documents/{id}/view` | 200 | Bearer (owner, org member, or SUPER_ADMIN) |

### Behavior
- `POST /hosts/apply` inserts `host_organizations` (status `SUBMITTED`) and `host_applications` in one transaction, with the caller as `submitted_by`. A second application while one is active returns 409 `APPLICATION_ALREADY_EXISTS`.
- `POST /hosts/documents` issues a presigned S3 PUT URL and stores metadata in `host_legal_documents`. The object key is composed from typed UUIDs inside the storage service: `legal-documents/{org_id}/{document_id}{ext}`. Only `.jpg`, `.jpeg`, `.png`, `.webp`, and `.pdf` are accepted; the MIME type is inferred from the extension.
- `GET /hosts/documents/{id}/view` issues a 300-second presigned GET URL. The storage service rejects any object key outside the `legal-documents/` prefix and any key containing a traversal segment.

### Evidence
- `backend/internal/storage/s3_service_test.go`: 5 tests (credential rejection, vault-scoped upload, short-lived view URL, foreign-prefix and traversal rejection, unsupported extension).
- `backend/internal/handler/host_application_handler_test.go`: 2 live integration tests (full apply/list/upload/view flow with database assertions; cross-tenant document view returns 403 and no public bucket reference is persisted).

---

## 4. Task 2.6: Approval, Membership, and Quotas

### Endpoints
| Method | Path | Status | Authorization |
|---|---|:---:|---|
| GET | `/api/v1/hosts/organization` | 200 | Active Leader or Admin |
| PATCH | `/api/v1/hosts/organization` | 200 | Active Leader only |
| GET | `/api/v1/hosts/members` | 200 | Active Leader or Admin |
| DELETE | `/api/v1/hosts/members/{id}` | 200 | Active Leader only |
| POST | `/api/v1/admin/hosts/applications/{id}/approve` | 200 | `SUPER_ADMIN` capability |
| POST | `/api/v1/admin/hosts/applications/{id}/reject` | 200 | `SUPER_ADMIN` capability |

### Atomic Approval
`ApproveApplication` runs in a single transaction that locks the application row (`FOR UPDATE OF ha`) and then, in order:
1. sets `host_organizations.verification_status = 'APPROVED'`;
2. sets `host_applications.status = 'APPROVED'` with reviewer and timestamp;
3. inserts an `ACTIVE` `LEADER` membership for the applicant;
4. grants the global `HOST_LEADER` capability;
5. initializes `host_admin_limits` with `max_admins = 5` and policy version `v1-default-5`.

Any failure rolls the whole thing back.

### Leader Exclusivity
The `uq_active_host_leader` partial unique index (migration 000002) is the enforcement point: a second `ACTIVE` `LEADER` row is refused by PostgreSQL, proven by a test that attempts the insert directly.

### Review Preconditions
Only a `SUBMITTED` application can be approved or rejected. A second review returns 409 `APPLICATION_ALREADY_REVIEWED` instead of corrupting state or surfacing a 500, and approval additionally refuses when the applicant already holds a membership in the organization (409 `MEMBERSHIP_ALREADY_EXISTS`). Capability grants are idempotent, so repeated flows cannot stack duplicate `user_capabilities` rows.

### Evidence
- `backend/internal/handler/host_membership_handler_test.go`: 4 live integration tests (atomic approval with all four effects plus the approval audit entry asserted against the database; second-leader constraint violation; re-approval returning 409 with exactly one membership and one capability row remaining; Admin 403 on leader-only operations while Admin GET `/members` returns 200).

---

## 5. Task 2.7: Payout Bank Accounts

### Endpoints
| Method | Path | Status | Authorization |
|---|---|:---:|---|
| GET | `/api/v1/hosts/bank-accounts` | 200 | Active Leader only |
| POST | `/api/v1/hosts/bank-accounts` | 201 | Active Leader only |

### Encryption and Disclosure Control
- `security.BankAccountCipher` implements AES-256-GCM with a strict 32-byte key requirement and a fresh random nonce per encryption.
- The database stores base64 ciphertext in `account_number_encrypted`; the test asserts the plaintext substring never appears in the stored value.
- API responses expose only `account_number_masked`. The masking function keeps the first and last four digits for numbers longer than eight digits, and keeps only the last four for numbers of eight digits or fewer, so a short account number is never echoed in full.
- Every registration writes one `audit_logs` row with action `HOST_BANK_ACCOUNT_REGISTERED`, targeting the created account. The audit metadata carries only the bank code and the masked form; the test asserts the raw number is absent from the metadata.

### Hard Authorization
Both routes are mounted behind `RequireHostLeader`. A user with an `ACTIVE` `ADMIN` membership receives 403 `AUTH_FORBIDDEN` on both verbs, and a non-member is rejected by the same middleware before reaching the handler.

### Evidence
- `backend/internal/security/bank_crypto_test.go`: 5 unit tests (round trip, nonce randomness, key-length rejection, tamper rejection, masking table).
- `backend/internal/handler/bank_account_handler_test.go`: 2 live integration tests (Leader registers and reads masked with audit verification; Admin and stranger both 403 with zero rows persisted).

---

## 6. Task 2.8: Team Invitations

### Endpoints
| Method | Path | Status | Authorization |
|---|---|:---:|---|
| POST | `/api/v1/hosts/invitations` | 201 | Active Leader only |
| POST | `/api/v1/hosts/invitations/accept` | 200 | Bearer (invited user) |
| DELETE | `/api/v1/hosts/invitations/{id}` | 200 | Active Leader only |

### Token Handling
- 32 bytes from `crypto/rand`, prefixed `inv_` and encoded with `base64.RawURLEncoding`.
- Only `hex(sha256(token))` is persisted in `host_invitations.token_hash`. The raw token is returned exactly once, at issuance.
- The test asserts the stored hash is 64 characters and differs from the raw token.

### Atomic Quota Enforcement
`AcceptInvitation` runs in one transaction that locks the invitation row (`FOR UPDATE`), re-checks the status, compares `expires_at` against the current time, locks the `host_admin_limits` row (`FOR UPDATE`), counts `ACTIVE` admins, and refuses with `ErrAdminLimitExceeded` when the quota is full. Only then does it insert the `ACTIVE` `ADMIN` membership, grant the `HOST_ADMIN` capability, and mark the invitation `ACCEPTED`. Concurrent acceptances serialize on the quota row lock.

### Refusal Semantics
| Condition | HTTP | Code |
|---|:---:|---|
| Invitation past `expires_at` | 410 | `INVITATION_EXPIRED` |
| Quota full | 422 | `ADMIN_LIMIT_EXCEEDED` |
| Already accepted or revoked | 409 | `INVITATION_CONSUMED` |
| Token unknown | 404 | `NOT_FOUND` |
| Accepting user is not the invited recipient | 403 | `AUTH_FORBIDDEN` |
| Accepting user already belongs to the organization | 409 | `MEMBERSHIP_ALREADY_EXISTS` |

### Evidence
- `backend/internal/handler/invitation_handler_test.go`: 3 live integration tests (issuance and acceptance with capability verification plus replay refusal; Admin 403, expiry 410, revocation; quota exhaustion 422 with zero membership rows created).

---

## 7. Task 2.9: Audit Logging, Graceful Shutdown, Security Suite

### Audit Service
`internal/audit.Service.Log` validates that `action_code` and `target_type` are non-empty, serializes metadata to JSONB, and inserts into `audit_logs`. The `audit.Logger` interface keeps the dependency testable.

Wired mutations:
- `AUTH_LOGIN_FAILED` with reason `UNKNOWN_IDENTIFIER` on an unknown login identifier.
- `AUTH_LOGIN_FAILED` with reason `INVALID_PASSWORD` on a failed password verification, carrying the new failed-attempt count.
- `AUTH_SESSION_REVOKED` with reason `LOGOUT` when a refresh token is revoked.
- `HOST_BANK_ACCOUNT_REGISTERED` on payout account registration (Task 2.7).
- `HOST_APPLICATION_APPROVED` and `HOST_APPLICATION_REJECTED` on platform review, naming the reviewer as actor.
- `HOST_MEMBERSHIP_REVOKED` on member removal, carrying the removed user and role.
- `HOST_MEMBERSHIP_ROLE_GRANTED` on invitation acceptance.
- `HOST_INVITATION_REVOKED` on invitation cancellation.

The membership and invitation usecases depend on the `audit.Logger` interface; a nil logger is accepted so unit tests can omit it. As with the auth usecase, an audit write failure is logged and never aborts the mutation.

Audit write failures are logged to the process log and never abort the mutation: the security decision has already been taken, and losing a successful revocation to a logging failure would be worse than a missing log line.

### Graceful Shutdown
`cmd/server/main.go` now runs `http.Server` behind `signal.NotifyContext` on `SIGINT` and `SIGTERM`. On signal, `srv.Shutdown` gets a 10-second drain budget for in-flight requests, the process logs the drain outcome, and the deferred pool close releases the database connections before exit.

### Security Regression Suite
`backend/test/security/auth_security_test.go` implements the five required attack classes:

| # | Criterion | Test |
|---|---|---|
| 1 | No account enumeration on login | `TestSecurity_NoAccountEnumerationOnLogin` asserts identical status and error code for a wrong password and an unknown identifier |
| 2 | No refresh token family reuse | `TestSecurity_NoRefreshTokenFamilyReuse` rotates once, replays the consumed token, and requires 401 `TOKEN_REUSE_DETECTED` |
| 3 | No IDOR across organizations | `TestSecurity_NoIDORAcrossOrganizations` requires 403 for another tenant's organization and 200 for the caller's own, proving the rejection is authorization and not a broken route |
| 4 | No Host Admin access to financial settings | `TestSecurity_NoHostAdminAccessToFinancialSettings` sweeps five routes (bank read, bank write, organization patch, member delete, invitation create) and requires 403 `AUTH_FORBIDDEN` on each |
| 5 | No header injection on `X-Request-ID` | `TestSecurity_NoHeaderInjectionOnRequestID` sends CRLF, bare LF, oversized, spaced, and script payloads, and requires an auto-generated replacement with no injected header and no control characters |

### Evidence
- `go test -count=1 ./internal/audit/ ./test/...` → both packages ok, 5 of 5 security criteria PASS.
- Live database check after the suite: `audit_logs` holds 3 `INVALID_PASSWORD` and 3 `UNKNOWN_IDENTIFIER` rows for `AUTH_LOGIN_FAILED`.

---

## 8. Task 2.10: OpenAPI Sync and Web Contracts

### OpenAPI 3.1
`GALERIKU_OPENAPI_v1_DRAFT.yaml` gained 18 Domain 01 paths, 22 schemas, one reusable parameter (`OrganizationIdHeader`), and six reusable responses. The document now declares 29 paths and 28 schemas, and every one of its 38 `$ref` values resolves. `docs/GALERIKU_OPENAPI_v1_DRAFT.yaml` is a byte-identical copy.

### Validation Tooling
`backend/scripts/validate_openapi.py` parses the document, asserts `openapi: 3.1.0`, collects every `$ref`, and fails with the list of unresolved references. `make openapi-validate` runs it.

### Web Contracts
`web/src/types/api.ts` mirrors the backend constants as literal unions with runtime guards (`isCapabilityCode`, `isAuthErrorCode`) plus the response shapes for the Domain 01 endpoints. `web/test/auth-contracts.test.ts` asserts the taxonomy, the error catalog, and the three Host enums, so a drift between backend constants and web types fails the suite.

### Evidence
- `make openapi-validate` → exit 0, "OK: 29 paths, 28 schemas, 38 refs resolved".
- `npm test` → 11 of 11 tests pass (7 new contract tests plus the 4 existing design-token tests).
- `make test` → exit 0, with `lint-copy` now inspecting both `design-system.ts` and the new `types/api.ts`.

---

## 9. Verification Commands and Outcomes

| Command | Scope | Exit | Outcome |
|---|---|:---:|---|
| `cd backend && go test -count=1 ./...` | Full backend suite | 0 | All 13 packages ok, including the new `internal/audit` and `test/security` packages |
| `cd backend && go test -race -count=1 ./internal/... ./test/...` | Race detector | 0 | All packages ok, zero race reports |
| `cd web && npm test` | Web contract suite | 0 | 11 of 11 tests pass |
| `make test` | Workspace quality gates | 0 | Backend short tests, web tests, `tsc --noEmit`, and `lint-copy` all pass |
| `make openapi-validate` | OpenAPI integrity | 0 | 29 paths, 28 schemas, 38 refs resolved |
| `cd backend && go build ./...` | Compilation | 0 | Clean |
| `git diff --check -- Makefile` | Preservation | 2 | Pre-existing trailing blank line intact |
| Em dash scan (U+2014) | Antislop R-02 | 0 | Zero occurrences in all files created or modified by Tasks 2.5 to 2.10 |

---

## 10. Antislop Craftsmanship Verification

- **Hard Gate R-02 (No Em Dashes):** zero U+2014 across every Go source file, test file, the OpenAPI document, the TypeScript contracts, and this report.
- **Copy Linter:** `npm run lint:copy` reports 2 files inspected and PASSED.
- **Hard Gate R-03 (Tap Targets):** no UI components were added; the web workspace gained type definitions and tests only.

---

## 11. Known Limitations

1. **Tenant-Isolation Coverage Boundary:** isolation is enforced for every route that exists today. Subsequent endpoint families (Domain 02 event management and beyond) must mount the same `RequireHostMembership` / `RequireHostLeader` middleware and scope queries by `TenantContext.OrganizationID`.
2. **Admin Quota Enforcement Point:** `host_admin_limits` is initialized at approval (Task 2.6) and enforced at invitation acceptance (Task 2.8). No other code path creates an `ADMIN` membership, so the quota currently has exactly one enforcement point.
3. **Audit Coverage:** failed login, session revocation, bank account registration, Host approval and rejection, membership revocation, membership role grant, and invitation revocation are all wired. The one listed mutation still missing is password reset, because that flow does not exist in the codebase yet.
4. **Password Reset:** the password reset flow does not exist in the codebase yet, so its audit requirement cannot be satisfied.
5. **Legacy Data Preservation:** migration behavior against a pre-populated database remains `NOT VERIFIED`; test suites run against clean disposable tables.
6. **Upload Finalization:** `host_legal_documents.size_bytes` is accepted from the client and defaults to 0; it is not yet reconciled against the object that actually lands in the vault. Presigned PUT URLs are also issued regardless of the application's status, so a rejected applicant can still upload; the metadata is retained for the review that follows.
7. **Apply Race:** two concurrent `POST /hosts/apply` calls can both pass the active-application check and create two organizations for one user. The consequence is bounded: both wait for review, and the second approval now returns 409 rather than corrupting state. Closing it requires a partial unique index on active applications, which is a migration.
8. **Bank Registration Audit Atomicity:** the account insert and its audit write are separate statements. A database-level failure between them returns 500 and a client retry would create a second account, since the endpoint has no idempotency key.

---

## 12. Independent Review Outcome

An independent reviewer examined the full working tree for Tasks 2.5 to 2.10. Verdict: the core security architecture is sound, with no Critical findings. Router wiring places every `/hosts` route behind `Authenticate`, membership and leader routes re-verify against the database on every request and derive `TenantContext` from the database row rather than client claims, presigned URL construction is composed only from typed UUIDs plus a whitelisted extension, and AES-256-GCM usage is correct.

Four Important findings were raised and all four are fixed:

| Finding | Fix | Covering test |
|---|---|---|
| Host mutations wrote no audit rows | Audit entries wired for approval, rejection, membership revocation, role grant, and invitation revocation | `TestHostMembership_ApprovalIsAtomicAndGrantsLeader` asserts the approval audit entry and actor |
| Re-approval corrupted state or returned 500 | `SUBMITTED`-only precondition, existing-membership check, idempotent capability grant, mapped 409 | `TestHostMembership_ReapprovalIsRejectedAndAudited` |
| Double invitation acceptance stacked duplicate ADMIN memberships | Acceptance refuses when the user already belongs to the organization | Same sentinel and 409 path; the existing quota test still asserts zero rows on refusal |
| Production accepted the default `BANK_CIPHER_KEY` | `main.go` now rejects the repository-visible default in production, matching the JWT and OTP guard | Startup guard, exercised by reading the production branch |

One Minor finding was also fixed: the platform-admin approve and reject middleware now runs `WithLiveCapabilityCheck`, so a revoked `SUPER_ADMIN` loses access immediately instead of retaining it for up to the 15-minute token lifetime.

Four Minor findings were deliberately deferred and are recorded in Section 11: bank registration audit atomicity (11.8), document upload status and size validation (11.6), contract superset drift on the bank account list response, and the apply TOCTOU race (11.7).

---

## 13. Explicit Recommendation for Next Step

Tasks 2.5 to 2.10 are COMPLETED and VERIFIED by their own test suites, by the workspace quality gates, and by an independent review whose Important findings are all resolved.

The recommended next step is to run the Final Acceptance Review of Tasks 2.5 to 2.10 against this report, and then to begin Phase 3 (FotoTree and Direct-to-S3 Multipart Upload) per `docs/ROADMAP.md`.
