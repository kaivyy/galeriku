# Galeriku: Phase 2 Task 2.1 Execution Report
## Critical UUIDv7 Fix, Identity and Session Schema Migration, and Invariant Hardening

- **Date:** 15 Oktober 2026
- **Status:** COMPLETED and VERIFIED
- **Environment:** PostgreSQL 16.15 (Debian / Docker container `galeriku-postgres`) on port 5433
- **Branch:** `main` (uncommitted per explicit instruction)
- **Scope:** Task 2.1 only (UUIDv7 bug reproduction and fix, identity/session DDL migration, migration runner, integration tests, invariant hardening)

---

## 1. Executive Summary and Root Cause Analysis

### 1.1 The UUIDv7 Runtime Defect (P0 Blocker)
During the Phase 2 Preflight Audit, a critical runtime defect was discovered in the PostgreSQL `generate_uuid_v7()` function defined in `migrations/000001_initial_schema.up.sql`.

- **Symptom:** Any call to `uuid_generate_v7()` or `generate_uuid_v7()`, as well as any `INSERT` query relying on column defaults such as `users.id`, failed with PostgreSQL runtime error:
  ```
  ERROR: integer out of range (SQLSTATE 22003)
  ```
- **Root Cause:**
  In PL/pgSQL, variable `v_time_ms` stores the epoch timestamp in milliseconds:
  ```sql
  v_time := EXTRACT(EPOCH FROM CLOCK_TIMESTAMP());
  v_time_ms := FLOOR(v_time * 1000)::BIGINT;
  ```
  At current epoch time (October 2026), `v_time_ms` is approximately `1,792,056,000,000` (~1.79T).
  The original implementation extracted bytes by right-shifting and immediately casting to PostgreSQL 32-bit signed `INTEGER`:
  ```sql
  v_bytes := SET_BYTE(v_bytes, 4, ((v_time_ms >> 8))::INTEGER);  -- ~7,000,218,750 > 2,147,483,647!
  v_bytes := SET_BYTE(v_bytes, 5, (v_time_ms)::INTEGER);         -- ~1.79T > 2,147,483,647!
  ```
  Because PostgreSQL signed 32-bit `INTEGER` has a maximum value of `2,147,483,647`, direct casting of values greater than 2.14B triggered an immediate `integer out of range` exception.

### 1.2 The Exact Fix
The defect was resolved by applying a bitwise mask `& 255` across all 6 byte extractions before casting to `INTEGER`:
```sql
v_bytes := SET_BYTE(v_bytes, 0, ((v_time_ms >> 40) & 255)::INTEGER);
v_bytes := SET_BYTE(v_bytes, 1, ((v_time_ms >> 32) & 255)::INTEGER);
v_bytes := SET_BYTE(v_bytes, 2, ((v_time_ms >> 24) & 255)::INTEGER);
v_bytes := SET_BYTE(v_bytes, 3, ((v_time_ms >> 16) & 255)::INTEGER);
v_bytes := SET_BYTE(v_bytes, 4, ((v_time_ms >> 8) & 255)::INTEGER);
v_bytes := SET_BYTE(v_bytes, 5, (v_time_ms & 255)::INTEGER);
```
Masking with `& 255` guarantees that each extracted value is strictly bounded within `[0, 255]`, completely eliminating integer overflow while strictly adhering to RFC 9562:
1. Bytes 0 to 5: 48-bit big-endian millisecond timestamp.
2. Byte 6: Version bits masked to `0111` (UUIDv7).
3. Byte 8: Variant bits masked to `10xx` (RFC 9562 standard variant).
4. Both function aliases (`generate_uuid_v7()` and `uuid_generate_v7()`) are declared `VOLATILE` to ensure fresh timestamp evaluation per row.

---

## 2. Files Created and Modified

| File Path | Status | Purpose |
|---|---|---|
| [`migrations/000002_identity_and_sessions.up.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.up.sql) | Created | DDL migration correcting UUIDv7, creating 4 new identity/session/banking tables, adding 11 Domain 01 foreign keys, and enforcing `uq_active_host_leader` index. |
| [`migrations/000002_identity_and_sessions.down.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.down.sql) | Created | Symmetric rollback script dropping constraints, indexes, tables, and deregistering migration version from `schema_migrations`. |
| [`backend/internal/database/migration_runner.go`](file:///root/galeriku/backend/internal/database/migration_runner.go) | Created | Transactional migration runner with `schema_migrations` tracking, up migration execution, and programmatic rollback functions. |
| [`backend/internal/database/migrator.go`](file:///root/galeriku/backend/internal/database/migrator.go) | Modified | Updated schema completeness checker to assert 81 base tables and verify live `uuid_generate_v7()` runtime execution. |
| [`backend/internal/database/migrator_test.go`](file:///root/galeriku/backend/internal/database/migrator_test.go) | Modified | Updated baseline expected table count assertion from 76 to 81 tables. |
| [`backend/internal/database/uuid_v7_regression_test.go`](file:///root/galeriku/backend/internal/database/uuid_v7_regression_test.go) | Created | TDD regression test suite reproducing the overflow defect, verifying version/variant bits, and testing uniqueness. |
| [`backend/internal/database/migration_000002_test.go`](file:///root/galeriku/backend/internal/database/migration_000002_test.go) | Created | Integration tests verifying schema completeness, table counts, foreign keys, cascades, check constraints, Host Leader invariants, and rollback cycles. |
| [`Makefile`](file:///root/galeriku/Makefile) | Modified | Added recipes for `migrate-up` and `migrate-down` targets. |

---

## 3. Database Schema Changes, Constraints, and Indexes

Migration `000002_identity_and_sessions.up.sql` extends the database from 76 to 81 base tables (including `schema_migrations`).

### 3.1 New Tables

#### A. `user_credentials`
- **Purpose:** Secure credential storage for email/password authentication (Task 2.2 preparation).
- **Columns:**
  - `id UUID PRIMARY KEY DEFAULT generate_uuid_v7()`
  - `user_id UUID NOT NULL UNIQUE` (1-to-1 relationship with `users`)
  - `password_hash VARCHAR(255) NOT NULL` (Argon2id encoded hash, never plaintext)
  - `password_algo VARCHAR(32) NOT NULL DEFAULT 'argon2id'`
  - `failed_attempts INTEGER NOT NULL DEFAULT 0`
  - `locked_until TIMESTAMPTZ NULL`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
- **Constraints & Indexes:**
  - `fk_user_credentials_user`: Foreign key to `users(id) ON DELETE CASCADE`.
  - Automatic unique index on `user_id` (redundant duplicate index eliminated per code review).

#### B. `auth_refresh_tokens`
- **Purpose:** Secure session persistence and Refresh Token Rotation (RTR) tracking.
- **Columns:**
  - `id UUID PRIMARY KEY DEFAULT generate_uuid_v7()`
  - `user_id UUID NOT NULL`
  - `token_hash VARCHAR(64) NOT NULL UNIQUE` (SHA-256 digest, raw tokens never persisted)
  - `family_id UUID NOT NULL` (tracks token ancestry for reuse detection)
  - `is_revoked BOOLEAN NOT NULL DEFAULT FALSE`
  - `revoked_reason VARCHAR(64) NULL`
  - `expires_at TIMESTAMPTZ NOT NULL`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
  - `user_agent TEXT NULL`
  - `ip_address VARCHAR(45) NULL`
- **Constraints & Indexes:**
  - `fk_auth_refresh_tokens_user`: Foreign key to `users(id) ON DELETE CASCADE`.
  - `idx_auth_refresh_tokens_user_id`: Index on `(user_id)`.
  - `idx_auth_refresh_tokens_family_id`: Index on `(family_id)`.
  - `idx_auth_refresh_tokens_expires_at`: Index on `(expires_at)`.

#### C. `auth_otp_challenges`
- **Purpose:** Short-lived one-time code challenges for registration, login, and BIB verification.
- **Columns:**
  - `id UUID PRIMARY KEY DEFAULT generate_uuid_v7()`
  - `phone_e164 VARCHAR(32) NULL`
  - `email_normalized VARCHAR(128) NULL`
  - `purpose VARCHAR(64) NOT NULL` (e.g., `REGISTRATION`, `PASSWORD_RESET`, `BIB_SEARCH`)
  - `code_hash VARCHAR(64) NOT NULL` (SHA-256 digest, raw OTP never persisted)
  - `attempt_count INTEGER NOT NULL DEFAULT 0`
  - `max_attempts INTEGER NOT NULL DEFAULT 3`
  - `status VARCHAR(32) NOT NULL DEFAULT 'PENDING'`
  - `expires_at TIMESTAMPTZ NOT NULL`
  - `verified_at TIMESTAMPTZ NULL`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
- **Constraints & Indexes:**
  - `chk_otp_target`: Check constraint ensuring at least one recipient identifier is present (`CHECK (phone_e164 IS NOT NULL OR email_normalized IS NOT NULL)`).
  - `idx_auth_otp_challenges_phone_purpose`: Index on `(phone_e164, purpose, status)`.
  - `idx_auth_otp_challenges_email_purpose`: Index on `(email_normalized, purpose, status)`.
  - `idx_auth_otp_challenges_expires_at`: Index on `(expires_at)`.

#### D. `host_bank_accounts`
- **Purpose:** Resolves broken foreign key reference in `payouts.host_bank_account_id` (G-04 / Finding P1-2).
- **Columns:**
  - `id UUID PRIMARY KEY DEFAULT generate_uuid_v7()`
  - `host_organization_id UUID NOT NULL`
  - `bank_code VARCHAR(32) NOT NULL` (e.g., `BCA`, `MANDIRI`, `BRI`)
  - `account_number_encrypted TEXT NOT NULL` (ciphertext for secure payouts)
  - `account_number_masked VARCHAR(32) NOT NULL` (safe display for client interfaces)
  - `account_holder_name VARCHAR(128) NOT NULL`
  - `verification_status VARCHAR(32) NOT NULL DEFAULT 'PENDING'`
  - `verified_at TIMESTAMPTZ NULL`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
- **Constraints & Indexes:**
  - `fk_host_bank_accounts_org`: Foreign key to `host_organizations(id) ON DELETE RESTRICT`.
  - `fk_payouts_host_bank_account`: Foreign key attached to `payouts(host_bank_account_id) ON DELETE RESTRICT`.
  - `idx_host_bank_accounts_org`: Index on `(host_organization_id)`.

### 3.2 Domain 01 Foreign Keys Added
The following 11 foreign keys were added with explicit existence guards:
1. `user_profiles(user_id) -> users(id) ON DELETE CASCADE`
2. `user_capabilities(user_id) -> users(id) ON DELETE CASCADE`
3. `user_capabilities(capability_id) -> capabilities(id) ON DELETE CASCADE`
4. `host_applications(host_organization_id) -> host_organizations(id) ON DELETE RESTRICT`
5. `host_applications(submitted_by) -> users(id) ON DELETE RESTRICT`
6. `host_legal_documents(host_application_id) -> host_applications(id) ON DELETE CASCADE`
7. `host_memberships(host_organization_id) -> host_organizations(id) ON DELETE RESTRICT`
8. `host_memberships(user_id) -> users(id) ON DELETE RESTRICT`
9. `host_invitations(host_organization_id) -> host_organizations(id) ON DELETE CASCADE`
10. `host_invitations(invited_by) -> users(id) ON DELETE RESTRICT`
11. `host_admin_limits(host_organization_id) -> host_organizations(id) ON DELETE CASCADE`

### 3.3 Host Leader Invariant (OPEN-10 Hardening)
- **Constraint:** Partial unique index `uq_active_host_leader`:
  ```sql
  CREATE UNIQUE INDEX IF NOT EXISTS uq_active_host_leader
  ON host_memberships (host_organization_id)
  WHERE membership_role = 'LEADER' AND status = 'ACTIVE';
  ```
- **Behavior:**
  - Allows exactly one user to have `membership_role = 'LEADER'` and `status = 'ACTIVE'` per organization.
  - Permits multiple historical or inactive leaders (`status = 'INACTIVE'`, `'REVOKED'`, `'SUSPENDED'`) for the same organization.
  - Prevents race conditions during leader appointments at the database level.

---

## 4. Migration Runner and Symmetric Rollback Mechanism

### 4.1 Transactional Migration Runner
File [`backend/internal/database/migration_runner.go`](file:///root/galeriku/backend/internal/database/migration_runner.go) provides transactional schema version management:
- `EnsureMigrationTable(ctx, pool)`: Creates `schema_migrations (version VARCHAR(64) PRIMARY KEY, applied_at TIMESTAMPTZ)`.
- `GetAppliedMigrations(ctx, pool)`: Retrieves all registered migration versions.
- `ApplyMigrationFile(ctx, pool, filePath)`: Executes migration DDL inside a database transaction (`BEGIN ... COMMIT`). If an error occurs, the transaction rolls back cleanly.
- `RunPendingMigrations(ctx, pool, dir)`: Scans all `.up.sql` files, sorts lexicographically, and applies unapplied migrations in order.
- `RollbackMigrationFile(ctx, pool, filePath)`: Executes a `.down.sql` file within a transaction and removes its version from `schema_migrations`.
- `RollbackLastMigration(ctx, pool, dir)`: Finds the highest applied migration version and executes its corresponding `.down.sql` rollback script.

### 4.2 Rollback Symmetry
File [`migrations/000002_identity_and_sessions.down.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.down.sql) safely reverses all Task 2.1 modifications:
1. Drops index `uq_active_host_leader` and `idx_auth_otp_challenges_email_purpose`.
2. Drops foreign key constraints in reverse dependency order.
3. Drops tables `host_bank_accounts`, `auth_otp_challenges`, `auth_refresh_tokens`, and `user_credentials`.
4. Deletes `'000002'` from `schema_migrations`.

A full rollback-and-reapply cycle was verified in automated testing via `TestMigration000002_Rollback_And_Reapply`, confirming that the schema can be rolled back to 77 tables and re-applied to 81 tables without errors or orphaned state.

---

## 5. Commands Executed and Actual Exit Statuses

| # | Command | Working Directory | Exit Status | Result Summary |
|---|---|---|---|---|
| 1 | `git status --short` | `/root/galeriku` | 0 | Verified git status and tracked file changes |
| 2 | `docker compose exec postgres psql ...` | `/root/galeriku` | 0 | Verified table counts and schema state |
| 3 | `make test` | `/root/galeriku` | 0 | Ran Go unit tests (short), Vitest (4/4 passed), `tsc --noEmit` (0 errors), `lint-copy` (clean) |
| 4 | `make test-integration` | `/root/galeriku` | 0 | Executed full live tests against PostgreSQL and MinIO Docker services |
| 5 | `cd backend && go test -v -count=1 ./...` | `/root/galeriku` | 0 | Live integration tests without cache: all 9 suites passed 100% |
| 6 | `graphify update .` | `/root/galeriku` | 0 | Rebuilt knowledge graph: 995 nodes, 1018 edges, 85 communities |

---

## 6. Test Suite and Before/After Results

### 6.1 TDD Defect Reproduction (Red Phase)
- **Test:** [`TestUUIDv7_Regression`](file:///root/galeriku/backend/internal/database/uuid_v7_regression_test.go)
- **Before Fix:**
  ```
  === RUN   TestUUIDv7_Regression
      uuid_v7_regression_test.go:34: uuid_generate_v7() failed: ERROR: integer out of range (SQLSTATE 22003)
  --- FAIL: TestUUIDv7_Regression (0.01s)
  ```
  The defect was successfully reproduced and proven to be caused by 32-bit signed integer overflow.

### 6.2 Post-Fix Verification (Green Phase)
- **After Fix:**
  ```
  === RUN   TestUUIDv7_Regression
  --- PASS: TestUUIDv7_Regression (0.03s)
  ```
  - Direct execution of `uuid_generate_v7()`: PASS.
  - Direct execution of `generate_uuid_v7()`: PASS.
  - Regex validation of RFC 9562 bit layout (`xxxxxxxx-xxxx-7xxx-[89ab]xxx-xxxxxxxxxxxx`): PASS.
  - Sample uniqueness test (100 sequential UUIDs without collisions): PASS.

### 6.3 Schema Migration and Negative Integration Tests
- **Test:** [`TestMigration000002_Integration`](file:///root/galeriku/backend/internal/database/migration_000002_test.go)
  - `RunPendingMigrations`: Applied migration `000002` cleanly.
  - Schema completeness: Verified 81 base tables in PostgreSQL.
  - `INSERT INTO users`: Succeeded using column default `generate_uuid_v7()`.
  - `user_credentials`: Tested foreign key violation with invalid user ID (rejected as expected).
  - `user_credentials`: Verified cascade deletion when parent user is deleted.
  - `auth_refresh_tokens`: Tested token hash storage and family ID rotation.
  - `auth_otp_challenges`: Tested check constraint `chk_otp_target` rejecting rows with null phone AND null email.
  - `host_bank_accounts`: Linked to `host_organizations` and `payouts`.
  - Host Leader invariant:
    - Positive test: INACTIVE leader coexists with ACTIVE leader (PASS).
    - Positive test: REVOKED leader coexists with ACTIVE leader (PASS).
    - Negative test: Second ACTIVE leader rejected with `duplicate key value violates unique constraint "uq_active_host_leader" (SQLSTATE 23505)` (PASS).

### 6.4 Rollback and Reapply Test
- **Test:** [`TestMigration000002_Rollback_And_Reapply`](file:///root/galeriku/backend/internal/database/migration_000002_test.go)
  - `RollbackLastMigration`: Rolled back version `000002`.
  - Verification: Confirmed `user_credentials` table was dropped and table count decreased.
  - `RunPendingMigrations`: Re-applied migration `000002` (1 applied).
  - Verification: Table count restored to 81, and schema completeness verified.

### 6.5 Quality Gate Suite
- **`make test`:**
  - Backend short tests: PASS (live tests cleanly skipped).
  - Web Vitest tokens test: 4 passed.
  - TypeScript typecheck: 0 errors.
  - Antislop copywriting linter: PASSED (zero em dashes in inspected source files).

---

## 7. Independent Code Review Findings and Resolution

An independent code review subagent was dispatched to review the implementation. The reviewer assessed the task as **"Ready to proceed: With fixes"**, reporting 2 Critical issues, 3 Important issues, and 4 Minor issues. All actionable findings were addressed and resolved.

### 7.1 Critical Findings Resolved
1. **Antislop Hard Gate R-02 Violation (Em Dash in Migration Files):**
   - *Finding:* Lines 2 of `000002_identity_and_sessions.up.sql` and `000002_identity_and_sessions.down.sql` contained em dashes (`\u2014`).
   - *Resolution:* Replaced em dashes with colons (`:`). Repository scan confirmed zero em dashes in all newly created files.
2. **Rollback Asymmetry and Migration State Desynchronization:**
   - *Finding:* `000002_identity_and_sessions.down.sql` did not delete `'000002'` from `schema_migrations`, causing future `RunPendingMigrations` to skip re-applying.
   - *Resolution:* Added `DELETE FROM schema_migrations WHERE version = '000002';` to down migration, and implemented `RollbackLastMigration` in `migration_runner.go`.

### 7.2 Important Findings Resolved
1. **Test Teardown Foreign Key Violation and Data Leak:**
   - *Finding:* `testLeaderOrg` in `migration_000002_test.go` attempted to delete `host_organizations` without deleting referencing `host_bank_accounts` and `host_memberships` first.
   - *Resolution:* Updated `defer` block to delete `host_memberships` and `host_bank_accounts` prior to deleting `host_organizations`. Executed database query confirming 0 leaked rows in all host tables.
2. **Missing Index for Email OTP Challenges:**
   - *Finding:* `auth_otp_challenges` indexed phone lookup but lacked an index for `(email_normalized, purpose, status)`.
   - *Resolution:* Added composite index `idx_auth_otp_challenges_email_purpose` to `up.sql` and dropped it in `down.sql`.
3. **Redundant Duplicate Index on `user_credentials(user_id)`:**
   - *Finding:* Column `user_id UUID NOT NULL UNIQUE` automatically creates a unique btree index in PostgreSQL; explicit index `idx_user_credentials_user_id` was redundant.
   - *Resolution:* Removed redundant index from `up.sql`.

### 7.3 Minor Findings and Decisions
1. **`migrator_test.go` Baseline Table Count:**
   - *Resolution:* Updated assertion from `expectedTables = 76` to `expectedTables = 81`.
2. **Missing Invariant Test for Inactive Leaders:**
   - *Resolution:* Added positive assertions verifying that INACTIVE and REVOKED leaders can coexist with an ACTIVE leader.
3. **Makefile Migration Recipes:**
   - *Resolution:* Added `migrate-up` and `migrate-down` targets to `Makefile`.
4. **Preservation of `000001_initial_schema.up.sql`:**
   - *Decision:* Kept `000001_initial_schema.up.sql` strictly unmodified in accordance with non-negotiable instruction: *"Do not modify the already-merged '000001' migration to conceal the defect. Use '000002' to correct the function and extend the schema."*

---

## 8. Remaining Architectural Risks and Scope Boundaries

1. **Foreign Key Coverage in Remaining Domains:**
   - Task 2.1 intentionally scoped relational integrity to Domain 01 (Identity and Host) and the broken `payouts.host_bank_account_id` reference.
   - Domains 02 through 08 (Photo, AI, Privacy, Commerce, Storage, etc.) currently lack foreign keys. These remain scheduled for their respective domain implementation tasks in later phases.
2. **Password Hashing & Authentication Endpoints:**
   - Table `user_credentials` is ready, but no HTTP authentication endpoints, Argon2id hashing services, or JWT/session issuance handlers were implemented in Task 2.1, per strict task boundaries.
   - These are scheduled for Task 2.2 (`POST /api/v1/auth/register`, `POST /api/v1/auth/login`).
3. **Bank Account Encryption Key Management:**
   - Column `host_bank_accounts.account_number_encrypted` currently stores ciphertext. Integration with AES-GCM-256 / KMS envelope encryption is scheduled for Task 2.7 (Host Bank Account Management).

---

## 9. Explicit Completion Checklist

- [x] The original UUIDv7 failure is reproduced and covered by a regression test (`uuid_v7_regression_test.go`).
- [x] Both UUID function entry points (`generate_uuid_v7()` and `uuid_generate_v7()`) work as intended.
- [x] UUID version (`0111`) and variant (`10xx`) bits are correct and verified by regex.
- [x] Generated UUIDs are valid and do not collide in tested sample.
- [x] `INSERT` operations using UUID defaults succeed on `users` and child tables.
- [x] Migration `000002_identity_and_sessions.up.sql` runs successfully on the database.
- [x] New identity/session tables (`user_credentials`, `auth_refresh_tokens`, `auth_otp_challenges`, `host_bank_accounts`) have appropriate constraints and indexes.
- [x] The broken host bank account reference (`payouts.host_bank_account_id`) is resolved with a valid foreign key.
- [x] The active Host Leader uniqueness rule (`uq_active_host_leader`) is enforced and verified with both positive and negative tests.
- [x] Existing data remains intact (0 rows corrupted, clean teardowns).
- [x] Negative integrity tests pass (foreign key violations, duplicate active leaders, null OTP targets).
- [x] `make test` passes (100% unit tests, Vitest, typecheck, lint-copy).
- [x] `make test-integration` passes against live Docker PostgreSQL and MinIO services.
- [x] Independent review findings are resolved and documented.
- [x] No unrelated source files or original `PRD.md` content were changed.
- [x] Working tree changes remain uncommitted awaiting user authorization.
- [x] Subsequent Phase 2 tasks (Task 2.2+) have NOT been started.
