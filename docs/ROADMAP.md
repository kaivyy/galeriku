# Galeriku: Master Development Roadmap & Agent Execution Guide

**Document ID:** `docs/ROADMAP.md`  
**Current Baseline:** Phase 1 Complete (Committed), Phase 2 Tasks 2.1 to 2.4 Complete (Working Tree)  
**Target Completion:** MVP Production-Ready Architecture (Phases 1 through 9)  
**Last Updated:** 2026-10-15  
**Governing Rules:** Antislop Quality Gates (Zero em dash U+2014, WCAG AA, 44px touch targets), Zero-Trust Backend, Financial Precision (BIGINT), Biometric Compliance (UU PDP No. 27/2022).

---

## 1. Executive Summary & Architecture Status

Galeriku is an event photography marketplace platform engineered with zero-trust backend enforcement, direct-to-S3 mass uploads (1 to 2000 photos), asynchronous Python AI workers (InsightFace 512-dim face recognition and EasyOCR BIB recognition), Qdrant vector indexing, double-entry financial ledger accounting, and multi-tenant B2B Host portals.

### Core Architectural Decisions
- **DEC-Q01 (Instant Download):** Server-side micro-watermarked Non-Hi-Res files are delivered immediately upon payment confirmation while waiting for creator Hi-Res master fulfillment.
- **DEC-Q02 (Protected BIB Checkout):** BIB photo checkout requires OTP verification against registered race participant credentials to prevent fraudulent purchases.
- **DEC-Q03 (Atomic Quota Reservation):** Mass direct-to-S3 uploads (1 to 2000 files) require atomic backend quota reservation via `SELECT ... FOR UPDATE` before presigned S3 URLs are issued.
- **Financial Precision:** All Indonesian Rupiah amounts are stored as exact integer `BIGINT` (no floating-point values).
- **Double-Entry Invariant:** Every ledger journal transaction strictly enforces `SUM(debit) == SUM(credit)`.
- **Biometric Compliance:** No raw biometric vectors or face selfies are stored in PostgreSQL; all embeddings are isolated in Qdrant collections under UU PDP No. 27/2022.

---

## 2. Master Phase Checklist (Phases 1 to 9)

| Phase | Phase Name | Status | Primary Focus & Deliverables | Target Sprint | Primary Sources |
|:---:|---|:---:|---|:---:|---|
| **Phase 1** | **Core Foundation & Infrastructure** | [x] **COMPLETED** | Docker infrastructure, MinIO bucket provisioner, 76-table schema verification, Go connection pool, correlation ID, Web TS workspace. | Sprint 1 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L38-L46) |
| **Phase 2** | **Identity, Auth & Host B2B Portal** | [x] **COMPLETED** (10/10 Done) | UUIDv7 fix, Migration 000002, Argon2id, JWT HS256, OTP, RTR sessions, RBAC middleware, Host application, Host Bank accounts, invitations, audit logging, OpenAPI sync. | Sprint 2 | [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) |
| **Phase 3** | **FotoTree & Direct-to-S3 Multipart Upload** | [x] **COMPLETED** | FotoTree hierarchy (Event, Session, Route Location), atomic quota reservation (DEC-Q03), S3 presigned multipart upload 1 to 2000 files. | Sprint 3 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L56-L64) |
| **Phase 4** | **Python AI Worker Subsystem** | [ ] **PENDING** | RabbitMQ AI consumer, InsightFace (ArcFace 512-dim), EasyOCR BIB detection, micro-watermarking (DEC-Q01), Qdrant indexing. | Sprint 4 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L66-L74) |
| **Phase 5** | **Discovery & Protected Checkout** | [ ] **PENDING** | Selfie search (UU PDP), public BIB search, anti-fraud OTP checkout (DEC-Q02), shopping cart, idempotent orders. | Sprint 5 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L76-L83) |
| **Phase 6** | **Payment, Fulfillment 48h SLA & Downloads** | [ ] **PENDING** | Midtrans/Xendit QRIS/VA, instant Non-Hi-Res download (DEC-Q01), 48h SLA countdown, clean Hi-Res master fulfillment. | Sprint 6 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L85-L92) |
| **Phase 7** | **Double-Entry Ledger, Settlements & Refunds** | [ ] **PENDING** | Double-entry ledger (`SUM(debit)==SUM(credit)`), 3-way revenue split, T+7 hold period, SLA breach auto-refunds (REF-01 to REF-10). | Sprint 7 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L94-L104) |
| **Phase 8** | **Frontend Multi-Kanal Delivery** | [ ] **PENDING** | Mobile Flutter app, Web Storefront, Creator Studio, Host B2B Portal, Admin Dashboard. | Sprint 8-9 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L106-L114) |
| **Phase 9** | **Hardening, Security Audit & Launch** | [ ] **PENDING** | 2000-photo mass upload stress test, STRIDE security audit, Disaster Recovery drill (RPO < 5m, RTO < 1h), production launch. | Sprint 10 | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L116-L123) |

---

## 3. Phase 2 Detailed Task Breakdown & Execution Status

Reference Plan: [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md)  
Audit Baseline: [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md)

### [x] Task 2.1: Critical UUIDv7 Fix & Identity Schema Migration
- **Status:** COMPLETED
- **Description:** Fixed PostgreSQL integer overflow in `generate_uuid_v7()` via `& 255` masking. Implemented migration `000002_identity_and_sessions.up.sql` (`user_credentials`, `auth_refresh_tokens`, `auth_otp_challenges`, `host_bank_accounts`, `uq_active_host_leader` index, Domain 01 foreign keys) and symmetric rollback `000002_identity_and_sessions.down.sql`.
- **Implementation Report:** [`docs/audits/PHASE_2_TASK_2_1_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_REPORT.md)
- **Acceptance Decision:** [`docs/audits/PHASE_2_TASK_2_1_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_FINAL_ACCEPTANCE.md)
- **Files:** [`migrations/000002_identity_and_sessions.up.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.up.sql), [`backend/internal/database/migration_runner.go`](file:///root/galeriku/backend/internal/database/migration_runner.go), [`backend/internal/database/migrator.go`](file:///root/galeriku/backend/internal/database/migrator.go).
- **Verification Command:** `go test -v ./internal/database/...`

### [x] Task 2.2: Cryptographic Primitives (Argon2id, JWT HS256, OTP Generator, Redis Limiter)
- **Status:** COMPLETED
- **Description:** Implemented OWASP Argon2id password hashing, constant-time verification, stateless JWT HS256 service (min 256-bit secret, 15m expiration, `none` algorithm rejection), 6-digit numeric OTP generator, and Redis atomic Lua rate limiter (1 send / 60s cooldown, max 3 sends / 10m burst).
- **Implementation Report:** [`docs/audits/PHASE_2_TASK_2_2_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_2_REPORT.md)
- **Acceptance Decision:** [`docs/audits/PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_2_FINAL_ACCEPTANCE.md)
- **Files:** [`backend/internal/security/hasher.go`](file:///root/galeriku/backend/internal/security/hasher.go), [`backend/internal/security/jwt.go`](file:///root/galeriku/backend/internal/security/jwt.go), [`backend/internal/security/otp.go`](file:///root/galeriku/backend/internal/security/otp.go), [`backend/internal/security/limiter.go`](file:///root/galeriku/backend/internal/security/limiter.go).
- **Verification Command:** `go test -v ./internal/security/...`

### [x] Task 2.3: Authentication Endpoints, Repositories & Refresh Token Rotation (RTR)
- **Status:** COMPLETED
- **Description:** Implemented auth domain models, PostgreSQL repositories, usecases, and HTTP handlers for Register, Login, Refresh, Logout, Send OTP, Verify OTP, and Current User (`GET /auth/me`). Enforced 256-bit CSPRNG refresh token rotation with SHA-256 digest storage, token reuse detection revoking the entire `family_id`, login rate limiting (max 10 attempts / 60s), and HMAC-SHA-256 keyed OTP digests.
- **Implementation Report:** [`docs/audits/PHASE_2_TASK_2_3_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_REPORT.md)
- **Acceptance Decision:** [`docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_FINAL_ACCEPTANCE.md)
- **Files:** [`backend/internal/domain/auth.go`](file:///root/galeriku/backend/internal/domain/auth.go), [`backend/internal/repository/user_repository.go`](file:///root/galeriku/backend/internal/repository/user_repository.go), [`backend/internal/repository/auth_repository.go`](file:///root/galeriku/backend/internal/repository/auth_repository.go), [`backend/internal/usecase/auth_usecase.go`](file:///root/galeriku/backend/internal/usecase/auth_usecase.go), [`backend/internal/handler/auth_handler.go`](file:///root/galeriku/backend/internal/handler/auth_handler.go).
- **Verification Command:** `go test -v ./internal/handler/... ./internal/repository/... ./internal/usecase/...`

### [x] Task 2.4: RBAC Authorization Middleware, Tenant Isolation & B2B Organization Context
- **Status:** COMPLETED
- **Description:** Implemented `Authenticate` middleware, `RequireCapability` (with live DB check option eliminating the 15-minute token lag window per Condition F-04), `RequireHostMembership` with multi-source tenant resolution (`org_id` path, `X-Organization-ID` header, query, event ID ownership), strict Leader vs Admin separation, cross-tenant resource tampering detection, immutable request context injection, and regex-hardened `X-Request-ID` correlation ID middleware.
- **Implementation Report:** [`docs/audits/PHASE_2_TASK_2_4_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_4_REPORT.md)
- **Pre-Implementation Plan:** [`docs/plans/PHASE_2_TASK_2_4_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_TASK_2_4_PLAN.md)
- **Files:** [`backend/internal/domain/tenant.go`](file:///root/galeriku/backend/internal/domain/tenant.go), [`backend/internal/repository/tenant_repository.go`](file:///root/galeriku/backend/internal/repository/tenant_repository.go), [`backend/internal/middleware/auth_middleware.go`](file:///root/galeriku/backend/internal/middleware/auth_middleware.go), [`backend/internal/middleware/rbac.go`](file:///root/galeriku/backend/internal/middleware/rbac.go), [`backend/internal/middleware/context.go`](file:///root/galeriku/backend/internal/middleware/context.go), [`backend/internal/middleware/request_id.go`](file:///root/galeriku/backend/internal/middleware/request_id.go).
- **Verification Command:** `go test -v -race ./internal/middleware/...`

---

### [x] Task 2.5: Host B2B Application & Private Vault S3 Upload
- **Status:** READY FOR EXECUTION
- **Priority:** P1
- **Source Specification:** [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md#L214-L245), [`API_SPEC.md`](file:///root/galeriku/API_SPEC.md#L310-L380), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L85-L125).
- **Goal:** Allow registered users to apply as a Host Organization (`host_applications`), store organization profile metadata, and generate Presigned S3 PUT/GET URLs for private legal documents (KTP/NPWP/NIB) stored in `galeriku-master-vault`.
- **API Endpoints to Implement:**
  - `POST /api/v1/hosts/apply` (201 Created) - Creates `host_organizations` (status `SUBMITTED`) and `host_applications`.
  - `GET /api/v1/hosts/applications/my` (200 OK) - Lists submitted applications for current user.
  - `POST /api/v1/hosts/documents` (201 Created) - Issues Presigned S3 PUT URL for private legal document upload.
  - `GET /api/v1/hosts/documents/{id}/view` (200 OK) - Issues short-lived Presigned S3 GET URL (max 5 minutes) for Host Leader or Platform Admin.
- **Target Files:**
  - `backend/internal/domain/host.go`
  - `backend/internal/repository/host_repository.go`
  - `backend/internal/usecase/host_application_usecase.go`
  - `backend/internal/handler/host_application_handler.go`
  - `backend/internal/handler/host_application_handler_test.go`
  - `backend/internal/storage/s3_service.go`
  - `backend/internal/handler/router.go`
- **Security & Integrity Constraints:**
  - S3 Presigned PUT URLs must be restricted to path `legal-documents/{org_id}/*` in private bucket `galeriku-master-vault`.
  - Legal documents must never be uploaded or served from `galeriku-public-media`.
  - Access to legal documents requires active Host Leader membership or `PLATFORM_ADMIN` capability (BOLA/IDOR defense).

---

### [x] Task 2.6: Host Membership, Admin Quotas & Host Leader Exclusivity
- **Status:** PENDING
- **Priority:** P1
- **Source Specification:** [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md#L247-L280), [`PRD.md`](file:///root/galeriku/PRD.md#L275-L300), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L110-L140).
- **Goal:** Super Admin review workflow for Host applications, atomic promotion of applicant to `LEADER`, initialization of `host_admin_limits` (default: 5 admins), and enforcement of the single active leader constraint (`uq_active_host_leader`).
- **API Endpoints to Implement:**
  - `GET /api/v1/hosts/organization` (200 OK)
  - `PATCH /api/v1/hosts/organization` (200 OK, Host Leader only)
  - `GET /api/v1/hosts/members` (200 OK, Host Leader & Admin)
  - `DELETE /api/v1/hosts/members/{id}` (200 OK, Host Leader only)
  - `POST /api/v1/admin/hosts/applications/{id}/approve` (200 OK, Platform Admin only)
  - `POST /api/v1/admin/hosts/applications/{id}/reject` (200 OK, Platform Admin only)
- **Target Files:**
  - `backend/internal/usecase/host_membership_usecase.go`
  - `backend/internal/handler/host_membership_handler.go`
  - `backend/internal/handler/host_membership_handler_test.go`
  - `backend/internal/repository/host_repository.go`

---

### [x] Task 2.7: Host Bank Account Management (Host Leader Only)
- **Status:** PENDING
- **Priority:** P1
- **Source Specification:** [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md#L283-L311), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L145-L165).
- **Goal:** Secure registration and retrieval of host bank accounts on table `host_bank_accounts` with AES-256-GCM encryption for stored account numbers and masked display formatting.
- **Security Invariant:** Strictly guarded by `RequireHostLeader`. Host Admins receive HTTP 403 `AUTH_FORBIDDEN`.
- **API Endpoints to Implement:**
  - `GET /api/v1/hosts/bank-accounts` (200 OK, Host Leader only, returns masked account)
  - `POST /api/v1/hosts/bank-accounts` (201 Created, Host Leader only)
- **Target Files:**
  - `backend/internal/domain/bank_account.go`
  - `backend/internal/repository/bank_account_repository.go`
  - `backend/internal/usecase/bank_account_usecase.go`
  - `backend/internal/handler/bank_account_handler.go`
  - `backend/internal/handler/bank_account_handler_test.go`

---

### [x] Task 2.8: Host Team Invitations & Atomic Token Hashing
- **Status:** PENDING
- **Priority:** P1
- **Source Specification:** [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md#L313-L342), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L170-L195).
- **Goal:** Time-bound (72h) team invitation system via cryptographic token hash (SHA-256), enforcing admin quota limits atomically (`current_admins < max_admins`) under row lock.
- **API Endpoints to Implement:**
  - `POST /api/v1/hosts/invitations` (201 Created, Host Leader only)
  - `POST /api/v1/hosts/invitations/accept` (200 OK, Bearer Invited User)
  - `DELETE /api/v1/hosts/invitations/{id}` (200 OK, Host Leader only)
- **Target Files:**
  - `backend/internal/domain/invitation.go`
  - `backend/internal/repository/invitation_repository.go`
  - `backend/internal/usecase/invitation_usecase.go`
  - `backend/internal/handler/invitation_handler.go`
  - `backend/internal/handler/invitation_handler_test.go`

---

### [x] Task 2.9: Structured Audit Logging & Security Regression Test Suite
- **Status:** PENDING
- **Priority:** P1
- **Source Specification:** [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md#L345-L370), [`PRD.md`](file:///root/galeriku/PRD.md#L1300-L1350).
- **Goal:** Structured security and financial mutation recording on table `audit_logs`, Go server graceful shutdown with signal trapping and 10s drain timeout, and comprehensive security penetration tests.
- **Target Files:**
  - `backend/internal/audit/audit_service.go`
  - `backend/cmd/server/main.go`
  - `backend/test/security/auth_security_test.go`

---

### [x] Task 2.10: OpenAPI 3.1 Contract Sync & Web TypeScript Types Hardening
- **Status:** PENDING
- **Priority:** P2
- **Source Specification:** [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md#L372-L392), [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml).
- **Goal:** Synchronize Domain 01 API contracts into OpenAPI 3.1 YAML, generate TypeScript interfaces into `web/src/types/api.ts`, and run contract validation tests without Antislop violations.
- **Target Files:**
  - `GALERIKU_OPENAPI_v1_DRAFT.yaml`
  - `web/src/types/api.ts`
  - `web/test/auth-contracts.test.ts`
  - `Makefile`

---

## 4. Subsequent Phases Technical Roadmap (Phases 3 through 9)

### Phase 3: FotoTree & Direct-to-S3 Multipart Upload Massal (Sprint 3)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L56-L64), [`PRD.md`](file:///root/galeriku/PRD.md#L380-L450), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L200-L280).
- **Key Modules & Tasks:**
  - **Task 3.1: FotoTree Hierarchy Management:** CRUD for Events, Sessions (Start, Track Checkpoints, Finish), and Location checkpoint tags.
  - **Task 3.2: Preflight Atomic Quota Reservation (DEC-Q03):** `POST /storage/upload-batches/initiate` locking user storage quota (`SELECT ... FOR UPDATE`) before upload begins.
  - **Task 3.3: S3 Multipart Presigned Direct Upload:** Generating chunked presigned PUT URLs for direct browser-to-S3 transfer of 1 to 2000 high-resolution RAW/JPEG photos.
  - **Task 3.4: Batch Finalization & RabbitMQ Dispatch:** Verifying S3 ETags, closing multipart sessions, updating consumed quotas, and emitting `photo.uploaded` messages to RabbitMQ.

### Phase 4: Python AI Worker Pipeline - Biometric & OCR (Sprint 4)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L66-L74), [`PRD.md`](file:///root/galeriku/PRD.md#L460-L540), [`ARCHITECTURE.md`](file:///root/galeriku/ARCHITECTURE.md#L110-L160).
- **Key Modules & Tasks:**
  - **Task 4.1: RabbitMQ AI Queue Consumer:** Asynchronous Python worker consuming `photo.uploaded` tasks.
  - **Task 4.2: InsightFace ArcFace Extraction (512-dim):** Multi-face detection on high-resolution images, quality score filtering, and L2-normalized 512-dimensional vector extraction.
  - **Task 4.3: Qdrant Vector Indexing:** Storing embeddings in Qdrant collections with payload `{photo_id, subject_id, bbox}`.
  - **Task 4.4: BIB Number OCR Engine:** EasyOCR model for marathon runner chest number extraction with numeric normalization.
  - **Task 4.5: Server-Side Micro-Watermarking (DEC-Q01):** Applying pixel-level destructive watermarks to generate preview and instant Non-Hi-Res download media.

### Phase 5: Discovery & Protected Checkout (Sprint 5)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L76-L83), [`PRD.md`](file:///root/galeriku/PRD.md#L550-L630), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L300-L360).
- **Key Modules & Tasks:**
  - **Task 5.1: Selfie-Based Face Search (UU PDP):** Ephemeral selfie upload, vector extraction, cosine similarity search in Qdrant, and immediate selfie file purging.
  - **Task 5.2: Public BIB Search Catalog:** Search photos by runner number with mandatory purchase verification flag.
  - **Task 5.3: Anti-Fraud BIB Verification (DEC-Q02):** OTP dispatched to registered event participant contact to grant purchase token.
  - **Task 5.4: Shopping Cart & Idempotent Orders:** Cart management and order creation supporting `Idempotency-Key` headers.

### Phase 6: Payment, Fulfillment 48h SLA & Secure Downloads (Sprint 6)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L85-L92), [`PRD.md`](file:///root/galeriku/PRD.md#L640-L720), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L370-L430).
- **Key Modules & Tasks:**
  - **Task 6.1: Payment Gateway Integration:** Midtrans/Xendit QRIS and Virtual Account generation, signature-verified webhooks, and atomic order status updates.
  - **Task 6.2: Instant Non-Hi-Res Download Delivery (DEC-Q01):** Immediate delivery of micro-watermarked files upon payment confirmation.
  - **Task 6.3: 48-Hour Hi-Res Master SLA Tracker:** Countdown timers (`hires_due_at`) per order item and creator reminder notifications.
  - **Task 6.4: Clean Hi-Res Master Upload:** Photographer upload workflow to fulfill orders with full-resolution master files.

### Phase 7: Double-Entry Ledger, Settlements & Auto-Refunds (Sprint 7)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L94-L104), [`PRD.md`](file:///root/galeriku/PRD.md#L730-L840), [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md#L440-L520).
- **Key Modules & Tasks:**
  - **Task 7.1: Double-Entry Ledger Engine:** Strict `SUM(debit) == SUM(credit)` invariant recorded at transactional stored procedure level.
  - **Task 7.2: Three-Way Revenue Split Snapshot:** Deterministic split locking Platform (15%), Host (15%), and Creator (70%) exactly at payment time.
  - **Task 7.3: T+7 Settlement Hold Period:** Revenue held for 7 calendar days post Hi-Res master verification.
  - **Task 7.4: Automated SLA Breach Refund & Clawback (REF-01 to REF-07):** Cron worker detecting fulfillment delays > 48 hours, creating `refund_items`, and reversing revenue allocations atomically.
  - **Task 7.5: Buyer Internal Wallet & Cash-Out (REF-01 & REF-08):** 100% refund deposited instantly into buyer wallet liability accounts with bank cash-out support.
  - **Task 7.6: Master Lost Report & AI Upscale Defense (REF-10):** Photographer `MASTER_LOST` workflow, plus EXIF and noise analysis rejecting generative AI upscaling.
  - **Task 7.7: External Bank Disbursements:** Automated bank transfer payouts post-hold period.

### Phase 8: Frontend Multi-Kanal Delivery (Sprint 8-9)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L106-L114), [`UI_UX_SPEC.md`](file:///root/galeriku/UI_UX_SPEC.md).
- **Key Deliverables:**
  - **Task 8.1: Mobile Flutter App (iOS & Android):** GaleriGram social feed, AI selfie/BIB search, QRIS checkout, photo library.
  - **Task 8.2: Public Web Storefront:** Unauthenticated photo browsing, FotoTree route viewer, express checkout.
  - **Task 8.3: Creator Studio:** Photographer web console, direct-to-S3 mass upload, Hi-Res fulfillment dashboard.
  - **Task 8.4: Host B2B Portal:** FotoTree route management, photographer assignment, sales analytics, payout requests.
  - **Task 8.5: Super Admin Dashboard:** Host verification review, content moderation, audit logs, financial settlement monitoring.

### Phase 9: Hardening, Security Audit & Launch (Sprint 10)
- **Source Documents:** [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md#L116-L123), [`PRD.md`](file:///root/galeriku/PRD.md#L1200-L1280).
- **Key Activities:**
  - **Task 9.1: Mass Upload Stress Testing:** 50 concurrent photographers uploading 2000 photos each to validate zero gateway choke.
  - **Task 9.2: STRIDE Penetration Testing:** Formal audit of BIB authorization, presigned URL bypass prevention, and biometric data isolation.
  - **Task 9.3: Disaster Recovery Drill:** Database WAL point-in-time recovery test targeting RPO < 5 min, RTO < 1 hour.
  - **Task 9.4: Production Launch:** Cloudflare WAF, TLS certificates, domain cutover, and monitoring deployment.

---

## 5. Mandatory Agent Execution Instructions

Any agent or developer picking up tasks must adhere to these operational protocols:

### A. Environment & Infrastructure Baseline
- **Repository Root:** `/root/galeriku`
- **Active Branch:** `main` (Head commit: `7c215d9`)
- **Docker Core Containers (5 healthy):**
  - PostgreSQL 16: mapped to `localhost:5433` (`galeriku_db`, user: `galeriku`)
  - Redis 7: mapped to `localhost:6380`
  - MinIO S3: API mapped to `localhost:9010`, Console on `9011`
  - RabbitMQ: AMQP on `localhost:5672`, Management on `15672`
  - Qdrant: HTTP on `localhost:6333`

### B. Git Preservation Constraints
1. **Never reset or clean the working tree:** Tasks 2.1 through 2.4 changes exist in the working tree and must be preserved intact.
2. **Never fix `Makefile:52`:** The trailing blank line at `Makefile:52` is an intentional pre-existing test fixture; `git diff --check Makefile` returns exit code 2 and must not be reformatted.
3. **Commit discipline:** Do not commit or amend commits unless explicitly instructed by the user.

### C. Antislop Quality Standards
1. **Hard Gate R-02:** Zero em dash characters (U+2014) anywhere in source code, comments, docstrings, or markdown files. Use hyphens (`-`), colons (`:`), or parentheses.
2. **Hard Gate R-03:** Minimum 44px tap targets on all touch interactive elements.
3. **Hard Gate R-29:** Palette restricted to maximum 3 core colors + 1 accent color.
4. **Hard Gate R-37:** Dials locked to ENERGY 2 / RHYTHM 2 / MOTION 1.
5. **No AI-slop vocabulary:** Avoid buzzwords (*unlock, elevate, empower, seamless, journey, game-changer, revolutionary*). State concrete facts and evidence.

### D. TDD Red-Green-Refactor Protocol
For every task:
1. **Red:** Write failing unit/integration tests covering both success and edge cases (unauthorized, IDOR, expired tokens, duplicate records).
2. **Verify Red:** Run test suite and confirm it fails for expected reasons.
3. **Green:** Write minimal, robust implementation adhering to domain boundaries.
4. **Verify Green:** Run `go test -v ./...` and `make test`. Confirm 100% pass.
5. **Report:** Author pre-implementation plan and post-implementation audit report under `docs/plans/` and `docs/audits/`. Update knowledge graph via `graphify update .`.

---

## 6. Document Cross-Reference Directory

| Topic / Requirement | Canonical Source File | Specific Line Range / Anchor |
|---|---|---|
| Business Requirements & Invariants | [`PRD.md`](file:///root/galeriku/PRD.md) | Full Master Blueprint v2.2 |
| Master 9-Phase Plan | [`IMPLEMENTATION_PLAN.md`](file:///root/galeriku/IMPLEMENTATION_PLAN.md) | Section 1 & 2 (Lines 20-124) |
| Phase 2 Technical Blueprint | [`docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md`](file:///root/galeriku/docs/plans/PHASE_2_IMPLEMENTATION_PLAN.md) | Full document (Lines 1-407) |
| Preflight Database & Security Audit | [`docs/audits/PHASE_2_PREFLIGHT_AUDIT.md`](file:///root/galeriku/docs/audits/PHASE_2_PREFLIGHT_AUDIT.md) | Full audit report |
| Task 2.1 Completion & Review | [`docs/audits/PHASE_2_TASK_2_1_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_1_REPORT.md) | Report & Acceptance |
| Task 2.2 Completion & Review | [`docs/audits/PHASE_2_TASK_2_2_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_2_REPORT.md) | Report & Acceptance |
| Task 2.3 Completion & Review | [`docs/audits/PHASE_2_TASK_2_3_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_3_REPORT.md) | Report & Acceptance |
| Task 2.4 Completion & Review | [`docs/audits/PHASE_2_TASK_2_4_REPORT.md`](file:///root/galeriku/docs/audits/PHASE_2_TASK_2_4_REPORT.md) | Report & Acceptance |
| API Specifications & Error Codes | [`API_SPEC.md`](file:///root/galeriku/API_SPEC.md) | Domain 01 Auth & Host (Lines 90-380) |
| Database 81-Table Schema & DDL | [`DATABASE_SPEC.md`](file:///root/galeriku/DATABASE_SPEC.md) | Identity & Host Domains (Lines 40-200) |
| Architecture & Dataflow | [`ARCHITECTURE.md`](file:///root/galeriku/ARCHITECTURE.md) | Hexagonal Architecture & Message Flow |
| HTTP Envelope & Middleware Spec | [`GALERIKU_API_ARCHITECTURE_SPEC.md`](file:///root/galeriku/GALERIKU_API_ARCHITECTURE_SPEC.md) | Standard Envelopes & Headers |
| OpenAPI 3.1 Draft Specification | [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml) | REST Contracts |
| UI/UX Guidelines & Antislop Dials | [`UI_UX_SPEC.md`](file:///root/galeriku/UI_UX_SPEC.md) | Color Tokens, Layouts, Copy Rules |
