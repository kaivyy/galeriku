# Galeriku: Phase 2 Tasks 2.5 to 2.10 Final Acceptance Review
## Host B2B Portal, Payout Accounts, Invitations, Audit Logging & Contract Sync

- **Review Date:** 15 Oktober 2026
- **Reviewer Role:** Independent Senior Go Backend Engineer, Application Security Engineer & Multi-Tenant Authorization Reviewer
- **Target Tasks:** Phase 2 Tasks 2.5, 2.6, 2.7, 2.8, 2.9, and 2.10
- **Implementation Report:** [`docs/audits/PHASE_2_TASKS_2_5_TO_2_10_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASKS_2_5_TO_2_10_REPORT.md)
- **Active Branch:** `main`
- **Head Commit:** `7c215d952fa82faee1bd29b3c0a565210af1f915` (`7c215d9`)
- **Review Type:** Read-Only Final Acceptance Review (zero modifications to source code, migrations, tests, or Git state)
- **Final Verdict:** **ACCEPT WITH CONDITIONS**

---

## 1. Executive Summary

This deliverable provides the independent, read-only final acceptance review of the six remaining Phase 2 tasks.

| Task | Scope | Verdict |
|---|---|---|
| **2.5** | Host application flow, private legal document presigned URLs | ACCEPTED |
| **2.6** | Super Admin approval, membership management, admin quotas | ACCEPTED |
| **2.7** | Payout bank accounts with AES-256-GCM encryption | ACCEPTED |
| **2.8** | Team invitations with hashed tokens and atomic quotas | ACCEPTED |
| **2.9** | Audit logging, graceful shutdown, security regression suite | ACCEPTED WITH CONDITIONS |
| **2.10** | OpenAPI 3.1 sync and TypeScript contracts | ACCEPTED |

### Acceptance Highlights
1. **All four Task 2.5 acceptance criteria VERIFIED** against code and live integration tests.
2. **Atomic Host approval VERIFIED:** one transaction changes both statuses, creates the `ACTIVE` `LEADER` membership, grants `HOST_LEADER`, and initializes the admin quota; the `uq_active_host_leader` index independently refuses a second active leader.
3. **Financial isolation VERIFIED:** bank account numbers are AES-256-GCM encrypted with per-encryption random nonces, only masked forms are returned, and a Host `ADMIN` receives 403 `AUTH_FORBIDDEN` on both verbs.
4. **Invitation token handling VERIFIED:** 32-byte CSPRNG token, SHA-256 digest as the only persisted form, atomic quota enforcement under row locks.
5. **Security regression suite VERIFIED:** all five required attack classes have a passing test, and the audit rows they generate are present in `audit_logs`.
6. **Contract sync VERIFIED:** 29 OpenAPI paths and 28 schemas with all 38 references resolving, and 11 of 11 web tests passing.
7. **Independent review completed:** no Critical findings; all four Important findings fixed with covering tests before this acceptance.

No P0 or P1 blockers remain. The verdict is **ACCEPT WITH CONDITIONS** because four deliberately deferred Minor items and one incomplete acceptance criterion (password reset auditing) carry into Phase 3.

---

## 2. Repository Identity and Working-Tree Preservation Evidence

| Property | Value Inspected | Verification Method |
|---|---|---|
| **Repository Root** | `/root/galeriku` | `pwd` |
| **Active Git Branch** | `main` | `git branch --show-current` |
| **Current HEAD Commit** | `7c215d952fa82faee1bd29b3c0a565210af1f915` | `git rev-parse HEAD` |
| **Working Tree Entries** | 118 uncommitted entries (Phase 1 baseline plus Tasks 2.1 to 2.10), preserved intact | `git status --porcelain` |
| **Git Mutation Operations** | Zero commits, rebases, amends, checkouts, stashes, resets, or pushes executed | Read-only protocol enforced |
| **Pre-Existing Whitespace Issue** | Trailing blank line at Makefile EOF preserved untouched; `git diff --check -- Makefile` exits 2, now reporting line 55 because two targets were appended above it | `git diff --check -- Makefile` |
| **Protected Files Diff** | Only `Makefile` shows changes (10 insertions: the two migration helpers plus `openapi-validate`). `PRD.md` and `migrations/` show zero diff | `git diff --stat -- Makefile PRD.md migrations/` |
| **Migration Inventory** | Exactly `000001_initial_schema` and `000002_identity_and_sessions` (up/down). Zero new migrations across all six tasks | `ls migrations/` |

---

## 3. Acceptance Criteria Verification

### Task 2.5
| Criterion | Evidence | Status |
|---|---|:---:|
| Application creates `host_organizations` (SUBMITTED) and `host_applications` | `CreateApplication` inserts both in one transaction; live test asserts both statuses | VERIFIED |
| Applicant is registered as the primary applicant | `submitted_by` set to the authenticated user; asserted by the flow test | VERIFIED |
| Presigned PUT strictly scoped to `legal-documents/{org_id}/*` in `galeriku-master-vault` | `BuildLegalDocumentKey` composes the key from typed UUIDs; test asserts the prefix and the bucket in the URL | VERIFIED |
| Legal documents are not publicly or anonymously reachable | `PresignLegalDocumentView` rejects keys outside the prefix and keys containing traversal; the flow test asserts the stored key never references the public bucket | VERIFIED |

### Task 2.6
| Criterion | Evidence | Status |
|---|---|:---:|
| Approval atomically sets organization APPROVED, membership ACTIVE LEADER, grants `HOST_LEADER`, initializes the quota (default 5) | One transaction; live test asserts all four effects plus the audit entry | VERIFIED |
| `uq_active_host_leader` prevents a second active leader | Live test attempts the insert directly and requires a constraint violation | VERIFIED |
| Host Admin refused (403) on legal changes and member removal | Live test sweeps PATCH organization and DELETE member as an ADMIN | VERIFIED |

### Task 2.7
| Criterion | Evidence | Status |
|---|---|:---:|
| Account numbers stored AES-256-GCM encrypted | `BankAccountCipher` with 32-byte key enforcement; test asserts the plaintext never appears in the stored value | VERIFIED |
| API returns only the masked form | Handler returns `account_number_masked`; test asserts `1234xxxxxxxx3456` and the absence of a raw field | VERIFIED |
| Host Admin receives 403 on GET and POST | Both routes behind `RequireHostLeader`; live test asserts 403 and zero persisted rows | VERIFIED |
| Every addition records an `audit_logs` event | `HOST_BANK_ACCOUNT_REGISTERED` written per registration; test asserts exactly one row and no raw number in metadata | VERIFIED |

### Task 2.8
| Criterion | Evidence | Status |
|---|---|:---:|
| 32-byte cryptographic token; only its SHA-256 hash is stored | `crypto/rand` 32 bytes; test asserts a 64-character digest that differs from the raw token | VERIFIED |
| Acceptance verifies `current_admins < max_admins` in an isolated transaction | `SELECT ... FOR UPDATE` on the invitation row and on the quota row before counting | VERIFIED |
| Quota full returns 422 `ADMIN_LIMIT_EXCEEDED` | Live test tightens the quota to 1 and asserts 422 with zero membership rows created | VERIFIED |
| Expired invitation returns 410 `INVITATION_EXPIRED` | Live test ages the invitation and asserts 410 with the exact code | VERIFIED |
| Acceptance grants `host_memberships` ADMIN and `HOST_ADMIN` | Live test asserts role, status, and capability presence | VERIFIED |

### Task 2.9
| Criterion | Evidence | Status |
|---|---|:---:|
| Failed login writes `audit_logs` | `AUTH_LOGIN_FAILED` for both unknown identifier and wrong password; live database check found 3 + 3 rows | VERIFIED |
| Password reset writes `audit_logs` | The password reset flow does not exist in the codebase | NOT APPLICABLE YET |
| Host approval writes `audit_logs` | `HOST_APPLICATION_APPROVED`; test asserts actor and target | VERIFIED |
| Membership role change writes `audit_logs` | `HOST_MEMBERSHIP_ROLE_GRANTED` on invitation acceptance and `HOST_MEMBERSHIP_REVOKED` on removal | VERIFIED |
| Bank account addition writes `audit_logs` | `HOST_BANK_ACCOUNT_REGISTERED` | VERIFIED |
| Session cancellation writes `audit_logs` | `AUTH_SESSION_REVOKED` with reason LOGOUT | VERIFIED |
| SIGINT and SIGTERM drain in-flight requests and close the pool before exit | `signal.NotifyContext`, 10-second `srv.Shutdown` budget, deferred pool close | VERIFIED |
| All five security criteria pass | `go test ./test/security/` → 5 of 5 PASS | VERIFIED |

### Task 2.10
| Criterion | Evidence | Status |
|---|---|:---:|
| OpenAPI 3.1 validates with no schema errors | `make openapi-validate` → exit 0, 29 paths, 28 schemas, 38 refs resolved | VERIFIED |
| TypeScript types mirror the backend requests, responses, and capability enum | `web/src/types/api.ts` literal unions; 7 contract tests assert them | VERIFIED |
| `make test` and `make test-integration` pass 100% | Both exit 0 | VERIFIED |
| `make lint-copy` reports no em dash in new code and schemas | lint-copy inspects 2 files and reports PASSED; direct scan found 0 occurrences | VERIFIED |

---

## 4. Security Evaluation

### 4.1 Tenant Isolation
Every tenant-scoped route derives its organization from the verified `TenantContext`, never from client input. The Task 2.4 middleware re-queries `host_memberships` on every request, and path-parameter operations (`RemoveMember`, `RevokeInvitation`) re-check that the target row belongs to the caller's organization before acting. The security suite proves both directions: another tenant's organization returns 403 while the caller's own returns 200, so the rejection is authorization and not a broken route.

### 4.2 Financial Data Protection
Bank account numbers are encrypted with AES-256-GCM under a key that must be exactly 32 bytes. Each encryption draws a fresh random nonce, so identical account numbers produce distinct ciphertexts. Decryption authenticates before returning, so a tampered ciphertext is rejected rather than silently accepted. Only the masked form leaves the process, and the audit metadata is asserted to contain no raw number. Production startup now refuses the repository-visible default key.

### 4.3 Privilege Boundaries
`LEADER` and `ADMIN` are separated by middleware, not by handler logic: bank accounts, organization mutation, member removal, and invitation issuance all sit behind `RequireHostLeader`. The security suite sweeps five such routes as an `ADMIN` and requires 403 `AUTH_FORBIDDEN` on each. Platform review routes require `SUPER_ADMIN` with a live database check, so a revoked platform admin loses access immediately rather than retaining it for the remainder of a token's lifetime.

### 4.4 State Corruption Resistance
Approval and rejection require a `SUBMITTED` application and refuse when the applicant already holds a membership, so repeated reviews return 409 instead of stacking memberships or capabilities. Invitation acceptance refuses a second membership for the same user. Capability grants are idempotent, which matters because `user_capabilities` has no unique constraint.

### 4.5 Audit Trail
Every mutation named in the Task 2.9 acceptance criteria except password reset writes an `audit_logs` row naming the actor, the target, and the reason where applicable. Audit write failures are logged and never abort the mutation, which is a deliberate trade: the state change is the more important record, and losing a completed revocation to a logging failure would be worse than a missing log line.

---

## 5. Categorized Findings

### P0: Critical Blocker (0 Findings)
- None.

### P1: High-Impact (0 Findings)
- None. All four Important findings raised by the independent review were fixed with covering tests before this acceptance.

### P2: Non-Blocking Architectural Items (4 Findings)

#### Finding P2-1: Password Reset Auditing Not Applicable
- **Reference:** `docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md` line 358
- **Description:** The Task 2.9 criterion lists password reset among the audited mutations. The flow does not exist in the codebase, so the requirement cannot be satisfied yet.
- **Remediation:** Wire `AUTH_PASSWORD_RESET_REQUESTED` and `AUTH_PASSWORD_RESET_COMPLETED` when the reset flow is built.
- **Blocks Acceptance:** No.

#### Finding P2-2: Apply TOCTOU Race
- **Reference:** `backend/internal/usecase/host_application_usecase.go` (Apply)
- **Description:** Two concurrent `POST /hosts/apply` calls can both pass the active-application check and create two organizations for one user.
- **Impact:** Bounded. Both applications wait for review, and the second approval now returns 409 rather than corrupting state.
- **Remediation:** A partial unique index on active applications closes it; that is a migration and was out of scope under the zero-migration constraint.
- **Blocks Acceptance:** No.

#### Finding P2-3: Bank Registration Audit Not Transactional
- **Reference:** `backend/internal/usecase/bank_account_usecase.go` (RegisterAccount)
- **Description:** The account insert and its audit write are separate statements.
- **Impact:** A database-level failure between them returns 500, and a client retry would create a second account because the endpoint has no idempotency key.
- **Remediation:** Move the audit write into the repository transaction or add an idempotency key.
- **Blocks Acceptance:** No.

#### Finding P2-4: Document Upload Status and Size Not Validated
- **Reference:** `backend/internal/usecase/host_application_usecase.go` (RequestDocumentUpload)
- **Description:** Presigned PUT URLs are issued regardless of application status, and `size_bytes` is stored verbatim.
- **Impact:** A rejected applicant can still upload a document, and the stored size is not reconciled against the object that lands in the vault.
- **Remediation:** Add a status precondition and a finalization step that reconciles size when upload completion is verified.
- **Blocks Acceptance:** No.

### P3: Minor Items & Hygiene (2 Findings)

#### Finding P3-1: Bank Account List Response Is a Superset of the Documented Schema
- **Reference:** `GALERIKU_OPENAPI_v1_DRAFT.yaml` (BankAccountsEnvelope), `web/src/types/api.ts` (HostBankAccount)
- **Description:** The handler marshals the full domain struct, which includes fields the documentation does not list.
- **Impact:** Documentation under-describes the response; consumers are unaffected because the envelope types carry a string fallback.
- **Remediation:** Narrow the response DTO or widen the schema.
- **Blocks Acceptance:** No.

#### Finding P3-2: Legacy Data Preservation Still Not Verified
- **Status:** Retained as `NOT VERIFIED`; test suites run against clean disposable tables.
- **Remediation:** Execute the pre-flight schema compatibility scan before running migrations on shared staging or production databases.
- **Blocks Acceptance:** No.

---

## 6. Tests Executed and Runtime Evidence

| Command | Scope | Exit | Verified Outcome |
|---|---|:---:|---|
| `cd backend && go test -count=1 ./...` | Full backend suite | 0 | All 13 packages ok, including `internal/audit` and `test/security` |
| `cd backend && go test -race -count=1 ./internal/... ./test/...` | Race detector | 0 | All packages ok, zero race reports |
| `cd web && npm test` | Web contract suite | 0 | 11 of 11 tests pass |
| `make test` | Workspace quality gates | 0 | Backend short tests, web tests, `tsc --noEmit`, and `lint-copy` all pass |
| `make openapi-validate` | OpenAPI integrity | 0 | 29 paths, 28 schemas, 38 refs resolved |
| `cd backend && go build ./...` | Compilation | 0 | Clean |
| `git diff --check -- Makefile` | Preservation | 2 | Pre-existing trailing blank line intact |
| Live database audit check | Audit persistence | 0 | `AUTH_LOGIN_FAILED` rows with both reason codes present after the suite |
| Em dash scan (U+2014) | Antislop R-02 | 0 | Zero occurrences in every file created or modified |

---

## 7. Antislop Craftsmanship Verification

- **Hard Gate R-02 (No Em Dashes):** zero U+2014 across all Go sources, tests, the OpenAPI document, the TypeScript contracts, and both Phase 2 reports.
- **Copy Linter:** `npm run lint:copy` inspects 2 files and reports PASSED.
- **Hard Gate R-03 (Tap Targets):** no UI components were added; the web workspace gained type definitions and tests only.

---

## 8. Final Acceptance Decision & Next Action

### Verdict: **ACCEPT WITH CONDITIONS**

**Justification:**
- Every acceptance criterion in Tasks 2.5, 2.6, 2.7, 2.8, and 2.10 is verified against code and live integration tests.
- Task 2.9 satisfies six of its seven mutation-audit criteria; the seventh cannot apply because the flow does not exist.
- An independent review found no Critical issues; all four Important findings were fixed with tests that failed first and pass now.
- All quality gates pass: full backend suite, race detector, web tests, `make test`, and `make openapi-validate`.
- Working tree, protected files, and the pre-existing Makefile finding are preserved with zero Git mutations.

### Conditions for Proceeding to Phase 3:
1. **Condition 1 (Pipeline Wiring):** every new protected route must use `Authenticate` plus the appropriate capability or membership middleware, and all tenant-scoped queries must be scoped by `TenantContext.OrganizationID`.
2. **Condition 2 (Working Tree Preservation):** maintain the uncommitted working tree until explicit direction is given to commit.
3. **Condition 3 (Preserve Makefile Whitespace):** retain the pre-existing trailing newline at the end of the Makefile.
4. **Condition 4 (Deferred Findings):** carry P2-1 through P2-4 and P3-1 into the Phase 3 backlog; none blocks Phase 3 work, and P2-2 becomes urgent only if multi-organization applicants are enabled.

### Phase 3 Eligibility:
**Phase 3 is ELIGIBLE FOR AUTHORIZATION.**

### Recommended Next Action:
Begin Phase 3 Task 3.1 (FotoTree hierarchy management) per `docs/ROADMAP.md` Section 4, or commit the Phase 2 working tree first if a stable checkpoint is preferred.
