# Galeriku: Phase 2 Task 2.1 Final Acceptance Review
## Critical UUIDv7 Fix, Identity Schema Migration, and Invariant Hardening

- **Review Date:** 15 Oktober 2026
- **Reviewer Role:** Independent Senior Staff Engineer, PostgreSQL Database Reviewer, and Application Security Reviewer
- **Target Deliverable:** Task 2.1 (`000002_identity_and_sessions.up.sql`, `migration_runner.go`, and test suites)
- **Target Environment:** PostgreSQL 16.15 (Debian 16.15-1.pgdg120+1) on host port 5433
- **Final Verdict:** ACCEPT WITH CONDITIONS
- **Execution Scope:** Read-only acceptance review. No source code, migrations, configuration, tests, or Git history were modified during this review.

---

## 1. Executive Summary

This report delivers the independent final acceptance review of Galeriku Phase 2 Task 2.1. The scope of Task 2.1 was strictly limited to repairing the critical PostgreSQL `uuid_generate_v7()` integer overflow defect, establishing the initial identity and session schema (`000002_identity_and_sessions.up.sql`), implementing transactional migration and rollback mechanisms, and verifying referential integrity constraints across Domain 01.

All primary technical claims in [`docs/audits/PHASE_2_TASK_2_1_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_REPORT.md) were independently evaluated and audited against the active codebase and live Docker services.

### Key Acceptance Findings
1. **P0 UUIDv7 Overflow Defect Resolved:** The root cause (`v_time_ms` epoch millisecond cast exceeding PostgreSQL 32-bit signed `INTEGER` range) has been eliminated via bitwise masking `& 255` across all 6 timestamp byte extractions. Mathematical and bit layout conformance to RFC 9562 was directly verified against PostgreSQL 16.
2. **Schema & Referential Hardening Verified:** 4 new tables (`user_credentials`, `auth_refresh_tokens`, `auth_otp_challenges`, `host_bank_accounts`) and 11 Domain 01 foreign keys were verified. The partial unique index `uq_active_host_leader` correctly blocks duplicate active Host Leaders while permitting inactive/historical leaders.
3. **Migration & Rollback Symmetry Verified:** Transactional migration execution and rollback cycles were verified in live tests. The public schema transitions cleanly between 77 base tables and 81 base tables without data leakage.
4. **Conditions for Progression:** No P0 blockers remain. Progression to Task 2.2 is approved under the condition that 2 material items (trailing whitespace in `Makefile:52` failing `git diff --check`, and the non-empty table data preservation caveat) are formally acknowledged.

---

## 2. Repository and Git State

The repository state was audited prior to and during this acceptance review:

- **Repository Root:** `/root/galeriku`
- **Active Branch:** `main`
- **Working Tree State:** All changes remain uncommitted awaiting explicit user authorization. No branch switching, resetting, rebasing, or force-pushing was performed.
- **Critical Files Integrity:**
  - `PRD.md`: Verified completely unmodified (`git diff HEAD -- PRD.md docs/PRD.md` returned empty).
  - `migrations/000001_initial_schema.up.sql`: Verified completely unmodified (`git diff HEAD -- migrations/000001_initial_schema.up.sql` returned empty).
- **Git Diff Inspection:**
  - `git status --short`:
    - Modified tracked files: `Makefile`, `backend/internal/database/migrator.go`, `backend/internal/database/migrator_test.go`, and `graphify-out/` metadata.
    - Untracked Task 2.1 files: `migrations/000002_identity_and_sessions.up.sql`, `migrations/000002_identity_and_sessions.down.sql`, `backend/internal/database/migration_runner.go`, `backend/internal/database/uuid_v7_regression_test.go`, `backend/internal/database/migration_000002_test.go`, `docs/audits/`, and `docs/plans/`.
  - `git diff --check`:
    - Result: Exit code 2.
    - Output: `Makefile:52: new blank line at EOF.` (Logged as finding P1-1; per read-only constraints, not modified during this review).

---

## 3. Files Reviewed

| File Path | Nature of Review | Verification Method |
|---|---|---|
| [`migrations/000002_identity_and_sessions.up.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.up.sql) | DDL Migration | Static analysis, syntax check, live PostgreSQL execution |
| [`migrations/000002_identity_and_sessions.down.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.down.sql) | Rollback DDL | Reverse dependency audit, live rollback execution |
| [`backend/internal/database/migration_runner.go`](file:///root/galeriku/backend/internal/database/migration_runner.go) | Migration Engine | Go code review, transactional boundary check |
| [`backend/internal/database/migrator.go`](file:///root/galeriku/backend/internal/database/migrator.go) | Schema Verifier | Runtime query inspection |
| [`backend/internal/database/migrator_test.go`](file:///root/galeriku/backend/internal/database/migrator_test.go) | Completeness Test | Table count threshold audit |
| [`backend/internal/database/uuid_v7_regression_test.go`](file:///root/galeriku/backend/internal/database/uuid_v7_regression_test.go) | TDD Regression Test | Bit layout regex inspection, live test execution |
| [`backend/internal/database/migration_000002_test.go`](file:///root/galeriku/backend/internal/database/migration_000002_test.go) | Integration Test Suite | Test teardown audit, negative constraint verification |
| [`docs/audits/PHASE_2_TASK_2_1_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_REPORT.md) | Implementation Report | Fact-checking against actual codebase and live database |
| [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) | Phase 2 Master Plan | Compliance verification against Task 2.1 specification |
| [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md) | Preflight Audit | Verification of P0/P1 audit finding resolution |

---

## 4. UUIDv7 Conformance Findings

The SQL implementation of `generate_uuid_v7()` and `uuid_generate_v7()` was reviewed against RFC 9562 (and the UUIDv7 specification standard).

### 4.1 Timestamp Extraction and Overflow Elimination
- In PL/pgSQL, `v_time_ms` is calculated via `FLOOR(EXTRACT(EPOCH FROM CLOCK_TIMESTAMP()) * 1000)::BIGINT`.
- Extracted byte arithmetic:
  - Byte 0: `((v_time_ms >> 40) & 255)::INTEGER` (Bits 47..40)
  - Byte 1: `((v_time_ms >> 32) & 255)::INTEGER` (Bits 39..32)
  - Byte 2: `((v_time_ms >> 24) & 255)::INTEGER` (Bits 31..24)
  - Byte 3: `((v_time_ms >> 16) & 255)::INTEGER` (Bits 23..16)
  - Byte 4: `((v_time_ms >> 8) & 255)::INTEGER` (Bits 15..8)
  - Byte 5: `(v_time_ms & 255)::INTEGER` (Bits 7..0)
- **Verdict:** Because `& 255` restricts the operand to `[0, 255]`, casting to PostgreSQL `INTEGER` cannot exceed the 32-bit signed integer boundary (`2,147,483,647`). Overflow is permanently eliminated.

### 4.2 RFC 9562 Bit Layout Conformance
A live SQL query was executed against the database to decode a newly generated UUIDv7:
```sql
WITH test_uuid AS (SELECT generate_uuid_v7() AS u)
SELECT 
    u::text,
    substring(replace(u::text, '-', '') from 1 for 12) as time_hex,
    ('x' || substring(replace(u::text, '-', '') from 1 for 12))::bit(48)::bigint as time_ms,
    to_timestamp((('x' || substring(replace(u::text, '-', '') from 1 for 12))::bit(48)::bigint) / 1000.0) as timestamp_utc,
    substring(replace(u::text, '-', '') from 13 for 1) as version_digit,
    substring(replace(u::text, '-', '') from 17 for 1) as variant_digit
FROM test_uuid;
```
**Actual Result:**
- UUID text: `01a13f26-aff9-7b07-b5e9-f54c5ac7628d`
- Decoded `time_ms`: `1792060862457`
- Decoded `timestamp_utc`: `2026-10-15 10:41:02.457+00` (matches current wall-clock time)
- Version digit (13th hex char): `7` (binary `0111`, version 7)
- Variant digit (17th hex char): `b` (binary `1011`, variant 1)

### 4.3 Function Volatility & Alias Consistency
Both `generate_uuid_v7()` and `uuid_generate_v7()` are declared `VOLATILE`, ensuring that repeated calls within the same query evaluate current timestamps and generate distinct random bytes. Insertion into tables using `DEFAULT generate_uuid_v7()` was tested and succeeded without error.

---

## 5. Schema and Migration Findings

### 5.1 Entity Verification
1. **`user_credentials`:**
   - Stores Argon2id password hash in `password_hash VARCHAR(255)`. Plaintext passwords are not stored.
   - Enforces 1-to-1 relationship with `users` via `user_id UUID NOT NULL UNIQUE`.
   - Redundant duplicate btree index on `user_id` was eliminated per the earlier code review.
   - Cascading deletion: `ON DELETE CASCADE` from `users(id)` is appropriate.
2. **`auth_refresh_tokens`:**
   - Stores only `token_hash VARCHAR(64) UNIQUE` (SHA-256 digest). Raw refresh tokens are never stored.
   - Includes `family_id UUID NOT NULL` to support Refresh Token Rotation (RTR).
   - Indexes: `idx_auth_refresh_tokens_user_id`, `idx_auth_refresh_tokens_family_id`, and `idx_auth_refresh_tokens_expires_at` support efficient session lookup, family invalidation, and background expiration cleanup.
3. **`auth_otp_challenges`:**
   - Stores SHA-256 code digest in `code_hash VARCHAR(64)`. Plaintext OTPs are not stored.
   - Check constraint `chk_otp_target` enforces presence of at least one target (`phone_e164 IS NOT NULL OR email_normalized IS NOT NULL`).
   - Indexes: `idx_auth_otp_challenges_phone_purpose`, `idx_auth_otp_challenges_email_purpose`, and `idx_auth_otp_challenges_expires_at`.
4. **`host_bank_accounts`:**
   - Resolves broken foreign key `fk_payouts_host_bank_account` from `payouts.host_bank_account_id`.
   - Segregates encrypted ciphertext (`account_number_encrypted TEXT`) from masked display (`account_number_masked VARCHAR(32)`).
   - Foreign keys: `ON DELETE RESTRICT` from both `payouts` and `host_organizations`.

### 5.2 Host Leader Invariant (OPEN-10)
- Enforced by partial unique index:
  ```sql
  CREATE UNIQUE INDEX IF NOT EXISTS uq_active_host_leader
  ON host_memberships (host_organization_id)
  WHERE membership_role = 'LEADER' AND status = 'ACTIVE';
  ```
- Verified in `migration_000002_test.go`:
  - Attempting to insert a second `LEADER` with `status = 'ACTIVE'` for the same organization was rejected with `duplicate key value violates unique constraint "uq_active_host_leader" (SQLSTATE 23505)`.
  - Inserting subsequent leaders with `status = 'INACTIVE'` or `status = 'REVOKED'` succeeded without constraint violations.

---

## 6. Migration Runner and Rollback Safety

1. **Transactional Boundaries:**
   - `migration_runner.go` wraps every `.up.sql` and `.down.sql` execution in a dedicated database transaction.
   - In `ApplyMigrationFile`, `INSERT INTO schema_migrations` occurs inside the same transaction as the DDL.
   - In `RollbackMigrationFile`, `DELETE FROM schema_migrations` occurs inside the same transaction as the rollback DDL.
   - In the event of a DDL failure, the transaction aborts and `schema_migrations` remains untouched.
2. **Rollback Symmetry:**
   - `000002_identity_and_sessions.down.sql` drops partial indexes, email OTP indexes, Domain 01 foreign keys, and the 4 newly created tables, and removes the version record from `schema_migrations`.
   - The test `TestMigration000002_Rollback_And_Reapply` executed a live rollback to 77 tables, verified `user_credentials` was dropped, and then re-applied the migration back to 81 verified tables.
3. **Table Count Baseline:**
   - Original Phase 1 tables: 76.
   - After `000002`: 81 base tables (76 initial + `schema_migrations` + `user_credentials` + `auth_refresh_tokens` + `auth_otp_challenges` + `host_bank_accounts`).

---

## 7. Security and Integrity Assessment

| Security Dimension | Current Implementation Status | Assessment & Future Mandate |
|---|---|---|
| **Password Storage** | Hash column `password_hash VARCHAR(255)`, `password_algo = 'argon2id'` | **SOUND**. No plaintext passwords stored. Task 2.2 must implement standard Argon2id hashing parameters (`m=64MB, t=3, p=4`). |
| **Refresh Token Security** | Stores SHA-256 hash `token_hash VARCHAR(64) UNIQUE`, `family_id UUID` | **SOUND**. Raw tokens never persisted. **MANDATE FOR TASK 2.3:** Rotation replay detection (revoking entire family upon reuse) must be enforced in service layer. |
| **OTP Brute-Force & Replay** | SHA-256 hash `code_hash VARCHAR(64)`, `attempt_count`, `max_attempts` (3) | **SOUND**. Plaintext OTPs never stored. **MANDATE FOR TASK 2.2:** Redis rate limiting must protect OTP dispatch before endpoints go live. |
| **Bank Account Data Exposure** | `account_number_encrypted TEXT` and `account_number_masked VARCHAR(32)` | **SOUND**. Plaintext account numbers not stored. **MANDATE FOR TASK 2.7:** AES-256-GCM / KMS encryption key management must be wired before storing real payouts. |
| **Cascade Behavior** | CASCADE on owned profiles/tokens; RESTRICT on legal, banking, and payouts | **SOUND**. Prevents accidental cascade deletion of financial records. |
| **Test Data Leaks** | Teardown deletes `host_memberships`, `host_bank_accounts`, then `host_organizations` | **SOUND**. Verified 0 residual rows across all tables via direct psql query. |

---

## 8. Test Commands, Exit Statuses, and Environments

All verification commands were executed directly against the local repository and live Docker containers (`galeriku-postgres` on port 5433, `galeriku-minio` on port 9010):

| Command Line | Working Directory | Exit Status | Environment | Result Summary |
|---|---|---|---|---|
| `git diff --check` | `/root/galeriku` | 2 | Local Git | Failed due to trailing newline in `Makefile:52` (P1-1). |
| `make test` | `/root/galeriku` | 0 | Local Host | Backend short unit tests PASSED; Web Vitest (4/4) PASSED; `tsc --noEmit` (0 errors) PASSED; `lint-copy` PASSED. |
| `make test-integration` | `/root/galeriku` | 0 | Live Docker | Live MinIO provisioner test PASSED; UUIDv7 regression PASSED; Migration 000002 integration PASSED; Rollback & Reapply PASSED; Schema completeness (81 tables) PASSED. |
| `cd backend && go test -v -count=1 ./...` | `/root/galeriku` | 0 | Live Docker | Non-cached run of all 9 test suites PASSED (100%). |
| `docker compose exec postgres psql ...` | `/root/galeriku` | 0 | Live PostgreSQL | Verified 0 residual rows in `host_organizations`, `host_bank_accounts`, `host_memberships`, and `users`. |

---

## 9. Categorized Findings

### P0: Critical Blocker (0 Findings)
- None. The previous P0 integer overflow defect in `uuid_generate_v7()` is verified fixed.

### P1: Material Issues (2 Findings)
1. **Finding P1-1: Trailing Newline in `Makefile:52` Causes `git diff --check` Non-Zero Exit**
   - *Evidence:* `git diff --check` returned exit code 2: `Makefile:52: new blank line at EOF.`.
   - *Impact:* CI/CD pre-commit hooks enforcing clean git diffs will fail.
   - *Remediation:* Remove the trailing blank line in `Makefile` when preparing the working tree for commit.
2. **Finding P1-2: Data Preservation on Non-Empty Tables Unverified**
   - *Evidence:* The test database had 0 initial user and organization rows prior to running migration 000002.
   - *Impact:* While DDL statements (`CREATE TABLE`, `ALTER TABLE ADD CONSTRAINT`) are non-destructive, the migration has not been tested against pre-existing legacy data containing invalid foreign key references or duplicate active leaders.
   - *Remediation:* Classify pre-existing data preservation as `NOT VERIFIED`. If applying migration 000002 to any shared or legacy staging database, run a pre-migration integrity scan on existing records.

### P2: Non-Blocking Improvements (2 Findings)
1. **Finding P2-1: Lack of Distributed Advisory Lock in Migration Runner**
   - *Evidence:* `migration_runner.go` relies on transaction boundaries and table locks.
   - *Impact:* Simultaneous startup of multiple backend pods in Kubernetes could contend for `schema_migrations`.
   - *Remediation:* Add `pg_advisory_lock` in future infrastructure hardening (Phase 9).
2. **Finding P2-2: Container Init Entrypoint Still Points to Merged 000001 Migration**
   - *Evidence:* `docker-compose.yml` mounts `000001_initial_schema.up.sql` as `/docker-entrypoint-initdb.d/01-init.sql`.
   - *Impact:* Fresh containers spin up with the defective 000001 function until `RunPendingMigrations` runs migration 000002.
   - *Remediation:* Acceptable per current rules (000001 must not be modified retroactively). During Phase 9 release preparation, consolidate migrations into a single baseline init script.

### INFO (Verified Facts)
- Antislop Hard Gate R-02 is fully satisfied across all newly authored migration files, Go code, and reports (zero em dashes `\u2014` present).
- `PRD.md` and `migrations/000001_initial_schema.up.sql` remain completely untouched.

---

## 10. Claims That Remain Unverified

In accordance with strict audit standards, the following claims are classified as **NOT VERIFIED**:
1. **Preservation of Pre-Existing Records Across Migration:** Unverified because the initial database tables contained 0 rows before migration 000002 was executed.
2. **Replay Protection Security Guarantee of `family_id`:** Unverified at this stage because token rotation logic and replay revocation are service-layer concerns scheduled for Task 2.3, not schema DDL.
3. **Global Referential Integrity of 76 Base Tables:** Unverified beyond Domain 01. Domains 02 through 08 remain without foreign keys, as explicitly planned for future phases.

---

## 11. Final Acceptance Decision

### Recommendation: **ACCEPT WITH CONDITIONS**

**Justification:**
Task 2.1 has successfully achieved its authorized scope:
- The critical UUIDv7 runtime defect is permanently resolved and independently verified.
- The identity, session, and banking schema DDL is clean, idempotent, and backed by a transactional runner.
- Invariants (`uq_active_host_leader`, check constraints, cascades) function correctly under positive and negative test cases.
- All quality gates (`make test`, `make test-integration`) pass 100%.

The conditions for proceeding to Task 2.2 are:
1. When authorizing git commits, remove the trailing blank line in `Makefile:52` to satisfy `git diff --check`.
2. Acknowledge that data preservation was verified on empty initial tables, and that relational constraints for Domains 02-08 remain scheduled for subsequent phases.

---

## 12. Prerequisites for Task 2.2

Before beginning Task 2.2 (*Cryptographic Primitives: Argon2id, JWT HS256, OTP Generator & Redis Rate Limiter*), the following boundaries must be observed:
1. Explicit user authorization is required before beginning Task 2.2 implementation.
2. The working tree remains uncommitted until the user directs commit or branch creation.
3. Task 2.2 must focus strictly on the cryptographic packages (`internal/auth/hasher`, `internal/auth/jwt`, `internal/auth/otp`, `internal/auth/limiter`) and their unit tests, without implementing HTTP routing or handler endpoints (which belong to Task 2.3).

---

## 13. Review Confirmation

It is hereby confirmed that during the conduct of this final acceptance review:
- No source code files were edited or overwritten.
- No migration files were altered.
- No database tables were reset or dropped outside the automated test harness.
- No Git branches, commits, stashes, or configuration were modified.
- Only this acceptance report [`docs/audits/PHASE_2_TASK_2_1_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_FINAL_ACCEPTANCE.md) was created.
