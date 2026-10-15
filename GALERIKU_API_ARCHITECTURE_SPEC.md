Galeriku — API Contract & Architecture Specification
Dokumen: GALERIKU_API_ARCHITECTURE_SPEC.md
Versi: 0.1-draft
Tanggal: 15 Oktober 2026
Status: Draft untuk review arsitektur dan produk, bukan kontrak produksi final
Referensi: PRD.md v2.2 Enhanced Master Blueprint, PRD_REVIEW.md, UI_UX_SPEC.md, dan Galeriku_ERD_Super_Lengkap.md
Dokumen sumber PRD.md dipertahankan apa adanya. Spesifikasi ini merupakan dokumen turunan. Keputusan yang belum disahkan ditandai OPEN dan tidak boleh diam-diam dianggap sebagai aturan final.
1. Tujuan dan ruang lingkup
Dokumen ini menetapkan kontrak awal antara kanal klien Galeriku dan backend, batas tanggung jawab komponen, aturan API lintas domain, alur bisnis utama, keamanan, observabilitas, dan urutan implementasi.
1.1 Kanal produk
Mobile App: Flutter Android/iOS.
Public Web Storefront: Astro untuk landing/katalog publik, dengan komponen interaktif seperlunya.
Creator Web Studio: React + TypeScript, diintegrasikan dengan Astro Islands atau aplikasi web sesuai keputusan deployment.
Host B2B Portal: pengelolaan organisasi, event, kreator, dan operasi event.
Super Admin Dashboard: moderasi, review pengajuan, dukungan, dan rekonsiliasi dengan izin khusus.
Satu akun dapat memiliki beberapa capability. Hak akses harus dievaluasi dari capability global ditambah membership organisasi/event dan scope resource; jangan menganggap satu kolom role global cukup.
1.2 Komponen arsitektur yang dipilih atau direncanakan
Komponen
Teknologi dari PRD
Peran
Mobile
Flutter / Dart
Pengalaman mobile dan lima tab utama
Public Web
Astro
Landing, SEO, katalog, FotoTree publik
Interactive Web
React + TypeScript
Cart, Creator Studio, Host Portal, Admin UI
Core API
Go, modular monolith
Sumber kebenaran untuk identitas, izin, event, order, uang, dan download grants
AI Service
Python FastAPI
OCR BIB, ekstraksi wajah/embedding, pencarian AI sesuai consent
Primary DB
PostgreSQL
Data transaksional dan sumber kebenaran ACID
Vector DB
Qdrant
Penyimpanan/pencarian vektor; bukan sumber kebenaran izin
Object Storage
S3-compatible: MinIO/R2/S3
Master, preview, thumbnail, derivatif dan file legal privat
Queue
RabbitMQ atau Redis Streams
Job asinkron; pilihan final OPEN
Payment
Xendit atau Midtrans
Payment intent, webhook, refund/payout bila didukung; provider final OPEN
Prinsip: mulai dengan Go modular monolith; pisahkan AI ke FastAPI karena kebutuhan runtime/model berbeda. Hindari memecah seluruh domain menjadi microservices sebelum kebutuhan operasional membuktikannya.
2. Prinsip API
Semua endpoint aplikasi berada di prefix /api/v1.
HTTPS wajib di semua lingkungan selain loopback lokal.
JSON menggunakan UTF-8; timestamp ISO 8601 UTC (Z) pada wire format.
Nominal IDR adalah integer rupiah, tidak menggunakan float. Contoh: {"amount_idr": 25000, "currency": "IDR"}.
UUID adalah identifier eksternal yang opaque; klien tidak boleh mengandalkan urutan/isi UUID.
Pagination list menggunakan cursor: ?limit=20&cursor=.... Batas default 20, maksimum awal 100 (PROPOSED, dapat disesuaikan).
Endpoint mutasi finansial, checkout, upload batch, dan job sensitif harus mendukung idempotency.
Respons tidak pernah mengungkap object key privat, secret provider, token OTP, embedding wajah, atau detail internal exception.
Harga, status pembayaran, saldo, refund eligibility, Host Fee, dan hak download dihitung/ditentukan server-side.
Semua transisi state penting dicatat sebagai event/audit; status tidak boleh dimutasi bebas dari klien.
Gunakan request_id/trace_id untuk menghubungkan API, queue job, dan webhook.
API mengembalikan error terstruktur dan tidak memaparkan stack trace.
3. Arsitektur tingkat tinggi
flowchart TB
  subgraph Clients["Kanal Klien"]
    MOB["Flutter Mobile"]
    WEB["Astro Storefront"]
    CST["Creator Studio"]
    HST["Host Portal"]
    ADM["Super Admin"]
  end

  EDGE["HTTPS Edge / WAF / Rate Limit"]
  API["Go Core API - Modular Monolith"]
  DB[("PostgreSQL - source of truth")]
  OBJ[("S3-compatible Object Storage")]
  QDR[("Qdrant Vector DB")]
  AI["Python FastAPI - AI Service"]
  MQ["RabbitMQ / Redis Streams - OPEN"]
  WORK["Async Workers: watermark, media, webhook, SLA, deletion"]
  PAY["Payment Provider: Xendit / Midtrans - OPEN"]

  MOB --> EDGE
  WEB --> EDGE
  CST --> EDGE
  HST --> EDGE
  ADM --> EDGE
  EDGE --> API
  API <--> DB
  API --> OBJ
  API --> AI
  AI <--> QDR
  API --> MQ
  MQ --> WORK
  WORK <--> DB
  WORK <--> OBJ
  WORK --> AI
  API <--> PAY
  PAY -->|signed webhook| EDGE
3.1 Tanggung jawab
Go Core API
Authentication/session validation and authorization.
Users/capabilities, Host organization/membership, events, photos metadata, carts/orders.
Price snapshots, payment initiation, webhook verification coordination.
Refund eligibility orchestration, wallet/ledger, settlement, payout state.
Quota reservations, upload grants, download grants.
Consent status, deletion orchestration, audit and outbox.
API contract and stable business error codes.
AI Service (FastAPI)
OCR BIB, face detection/embedding, similarity search orchestration.
Accept only authorized job requests from Core API/worker identity.
Check consent/deletion status through trusted workflow before processing.
Return references/scores and minimal metadata, not unrestricted database access.
Qdrant access stays server-side.
Workers
Server-side watermark rasterization, thumbnails, asset validation.
Multipart completion reconciliation and physical-object inventory.
AI indexing and deletion jobs.
SLA deadline evaluation, notification delivery, settlement release.
Webhook/outbox retries with idempotency.
Object storage
Private masters, legal documents and sensitive files.
Client uploads only via short-lived scoped presigned URLs after quota reservation.
Never expose permanent public URL to master Hi-Res.
Store object metadata/inventory in PostgreSQL; reconcile against actual storage.
4. API envelope
4.1 Success responses
Resource endpoints return the resource directly or use a consistent envelope. PROPOSED default envelope:
{
  "data": {
    "id": "uuid",
    "status": "example"
  },
  "meta": {
    "request_id": "req_opaque",
    "next_cursor": null
  }
}
For paginated list responses, meta.next_cursor is a string or null.
4.2 Error responses
{
  "error": {
    "code": "CHECKOUT_VERIFICATION_REQUIRED",
    "message": "Verifikasi event diperlukan sebelum checkout.",
    "details": [],
    "request_id": "req_opaque"
  }
}
Do not return internal stack traces or raw provider payloads.
4.3 Common HTTP status
Status
Meaning
200
Request succeeded
201
Resource created
202
Accepted for asynchronous processing
204
Success with no response body
400
Invalid request/schema
401
Missing/invalid authentication
403
Authenticated but not authorized
404
Resource not found or not visible to this actor
409
State conflict, duplicate operation, or idempotency mismatch
413
Payload too large
422
Valid shape but business validation failed
429
Rate limit exceeded
502/503
Upstream unavailable or temporary dependency failure
5. Authentication, capability, and authorization
5.1 Session model
PROPOSED: short-lived access token plus rotating refresh token for mobile; web may use secure, HttpOnly, SameSite cookies where architecture supports it.
Passwordless OTP or other login mechanism is OPEN; checkout OTP is not automatically the same as login OTP.
Tokens must not be stored in browser localStorage if a secure cookie/BFF design is used.
Refresh-token rotation, revocation, device/session listing, account recovery and MFA requirements need a dedicated auth threat review.
5.2 Authorization model
Authorization combines:
global user capability;
organization membership (HOST_LEADER, HOST_ADMIN);
event-specific creator membership (INTERNAL, INDEPENDENT);
resource ownership and state;
explicit finance/admin permission.
Host Admin does not implicitly receive bank account, wallet balance, payout, or financial settings access. Platform Admin permissions are separately granted and audited. UI hiding is not authorization; Core API must enforce all checks.
5.3 Sensitive resource rules
Master Hi-Res and Host legal/bank documents remain private.
Download grants must be bound to user, order item, asset, purpose, and expiry.
Signed URL TTL should be short and configurable; no long-lived URL for private assets.
AI search requires appropriate consent and event/resource authorization.
Revoke biometric consent immediately blocks new processing and queues deletion of subject vectors/derived biometric data. Original photo is not automatically deleted.
Rate limit OTP, BIB search, face search, login, checkout, upload-grant creation, and signed download grants.
Screenshot protection is best-effort platform control, not a guarantee that content cannot be captured.
6. Endpoint catalog
Endpoint names below are a proposed first contract. Exact schemas, auth requirements, and final paths must be reviewed before code generation. Auth means authenticated user; Scoped means additional ownership/membership/resource check.
6.1 Authentication and profile
Method
Path
Access
Purpose
POST
/auth/otp/challenges
Public, rate-limited
Request login challenge if passwordless auth is chosen
POST
/auth/otp/verify
Public, rate-limited
Verify login challenge
POST
/auth/refresh
Session
Rotate refresh credential
POST
/auth/logout
Auth
Revoke current session
GET
/me
Auth
Current user and capability summary
PATCH
/me/profile
Auth
Update allowed profile fields
GET
/me/sessions
Auth
List active sessions
DELETE
/me/sessions/{sessionId}
Auth
Revoke a session
OPEN: final login identity, OTP delivery channel/provider, expiry, retry limit, account recovery, and whether web uses BFF/cookie sessions.
6.2 Public storefront, FotoTree, feed and search
Method
Path
Access
Purpose
GET
/public/events
Public
List published FotoTree/events
GET
/public/events/{eventId}
Public
Published event details
GET
/public/events/{eventId}/sessions
Public
Event sessions
GET
/public/events/{eventId}/photos
Public, rate-limited
Publicly permitted photo previews
GET
/public/galerigram/feed
Public/Auth by policy
Feed page
GET
/public/search/bib
Public, strict rate limit
BIB candidate search; never grants checkout rights
POST
/ai/search-sessions
Auth + required consent/scope
Create AI search session
POST
/ai/search-sessions/{sessionId}/face-query
Auth + consent
Submit permitted face query
GET
/ai/search-sessions/{sessionId}/results
Auth + scope
Fetch result page
DELETE
/ai/search-sessions/{sessionId}
Auth + scope
End/delete search session data where applicable
GET
/me/collections
Auth
List collections
POST
/me/collections
Auth
Create collection
POST
/me/collections/{collectionId}/items
Auth + owner
Add photo
DELETE
/me/collections/{collectionId}/items/{photoId}
Auth + owner
Remove photo
Search responses must expose only assets the caller is allowed to see. BIB and face search results are not proof of identity, registration, consent, or purchase entitlement.
6.3 Checkout verification, cart and orders
Method
Path
Access
Purpose
POST
/checkout-verifications/challenges
Auth or checkout session
Request event/registration verification
POST
/checkout-verifications/verify
Auth or checkout session
Verify OTP/event code
GET
/checkout-verifications/{verificationId}
Owner/session
Get verification status
GET
/cart
Auth/guest cart policy
Read cart
POST
/cart/items
Auth/guest cart policy
Add item
PATCH
/cart/items/{itemId}
Cart owner
Update permitted fields
DELETE
/cart/items/{itemId}
Cart owner
Remove item
POST
/checkout/quote
Auth/checkout session
Recalculate authoritative price and eligibility
POST
/orders
Auth/verified checkout
Create order from validated quote
GET
/orders
Auth
List own orders
GET
/orders/{orderId}
Buyer/authorized support
Order details
GET
/orders/{orderId}/timeline
Buyer/authorized support
Payment and fulfillment timeline
Rules:
The server recalculates price and verifies current product status at checkout.
Quote expiry and price-change behavior are OPEN and must be specified.
Verification grant is scoped to user/session, event, purpose, expiry and optionally order; prevent reuse outside that scope.
Never trust client-submitted total, payment status, seller split, Host Fee or download entitlement.
6.4 Payment, refund, wallet, settlement and payout
Method
Path
Access
Purpose
POST
/orders/{orderId}/payments
Buyer + order owner
Create payment attempt
GET
/orders/{orderId}/payments
Buyer/authorized finance
List payment attempts
POST
/webhooks/payments/{provider}
Provider signature verification
Receive provider webhook
GET
/me/wallet
Auth
Read wallet summary
GET
/me/wallet/transactions
Auth
Read wallet ledger view
POST
/orders/{orderId}/refund-requests
Buyer/authorized system
Request/evaluate refund
GET
/refund-requests/{refundId}
Buyer/authorized finance
Read refund state
GET
/host/settlements
Host Leader/finance permission
Read settlements
GET
/host/payout-accounts
Host Leader + step-up auth if required
Read masked payout accounts
POST
/host/payout-accounts
Host Leader + step-up auth
Add payout destination
POST
/host/payouts
Host Leader + finance permission
Request eligible payout
GET
/host/payouts/{payoutId}
Host Leader + finance permission
Read payout state
GET
/admin/finance/reconciliation
Finance Admin
Reconciliation queue
POST
/admin/refunds/{refundId}/decision
Authorized Admin
Approve/reject with reason and audit
POST
/admin/ledger/adjustments
Narrow finance permission, dual control recommended
Post compensating journal, never edit posted entries
Important OPEN decisions: wallet as refund destination, gateway-fee treatment, refund after Non-Hi-Res download, partial refunds, negative creator/Host balances after payout, withdrawal policy, account closure handling, hold period, and final payment provider. The endpoints above describe possible interfaces, not approval of those policies.
6.5 Creator Web Studio and media pipeline
Method
Path
Access
Purpose
POST
/creator/upload-batches/preflight
Creator + event scope
Validate count/size/quota before reservation
POST
/creator/upload-batches
Creator + event scope
Create batch and reserve quota atomically
GET
/creator/upload-batches/{batchId}
Batch owner/authorized Host
Batch status
POST
/creator/upload-batches/{batchId}/items
Batch owner
Register validated items
POST
/creator/upload-batches/{batchId}/items/{itemId}/multipart
Batch owner
Create multipart session and scoped part grants
POST
/creator/upload-batches/{batchId}/items/{itemId}/complete
Batch owner
Complete upload and trigger server verification
POST
/creator/upload-batches/{batchId}/finalize
Batch owner
Finalize batch after reconciliation
GET
/creator/photos
Creator
List own photos
PATCH
/creator/photos/{photoId}
Creator + owner
Edit allowed metadata
POST
/creator/photos/{photoId}/hires
Creator + order fulfillment scope
Submit master Hi-Res for an eligible item
GET
/creator/fulfillment
Creator
List pending Hi-Res tasks
GET
/creator/fulfillment/{jobId}
Creator + scope
Fulfillment detail
Upload invariant: server preflight is advisory; batch creation must atomically reserve quota before any presigned URL is issued. Validate actual size, content type, checksum as available, object ownership, and upload completion server-side. Reconcile expected vs actual bytes and release unused reservation. Target batch up to 2,000 files is a product requirement to benchmark, not a proven performance guarantee.
6.6 Host B2B Portal
Method
Path
Access
Purpose
POST
/host/applications
Authenticated eligible creator
Submit Host application
GET
/host/applications/{applicationId}
Applicant/authorized reviewer
Application status
POST
/host/applications/{applicationId}/documents
Applicant
Upload private legal document
GET
/host/organizations
Membership
List organizations
GET
/host/organizations/{organizationId}
Membership
Organization details
PATCH
/host/organizations/{organizationId}
Host Leader
Update permitted org fields
GET
/host/organizations/{organizationId}/members
Membership
List members
POST
/host/organizations/{organizationId}/invitations
Host Leader/authorized Host Admin
Invite member within delegated policy
POST
/host/invitations/{invitationId}/accept
Invitee
Accept invitation
DELETE
/host/organizations/{organizationId}/members/{membershipId}
Host Leader/authorized role
Revoke membership
POST
/host/events
Host Leader/authorized Host Admin
Create event
PATCH
/host/events/{eventId}
Event scope
Update event
POST
/host/events/{eventId}/creators
Event scope
Invite/approve creator
PATCH
/host/events/{eventId}/creators/{membershipId}
Event scope
Update event-specific creator status/type
GET
/host/events/{eventId}/moderation-queue
Event scope
Review queue
POST
/host/events/{eventId}/host-fee-rules
Host Leader/finance permission
Create proposed/versioned Host Fee rule
Host Admin maximum count must be a configurable policy and enforced transactionally. The specific cap and whether a user may be Host Admin across multiple organizations/events remain OPEN. Creator type INTERNAL/INDEPENDENT is per event membership, never a global creator field.
6.7 Super Admin and moderation
Method
Path
Access
Purpose
GET
/admin/host-applications
Review permission
List applications
POST
/admin/host-applications/{applicationId}/decision
Review permission
Approve/reject with reason
GET
/admin/events
Admin permission
Search events
POST
/admin/events/{eventId}/suspension
Admin permission + reason
Suspend event
GET
/admin/photos/moderation-queue
Moderation permission
Queue of reported/flagged photos
POST
/admin/photos/{photoId}/moderation-decision
Moderation permission
Record moderation decision
GET
/admin/audit-logs
Audit permission
Search audited actions
GET
/admin/finance/reconciliation
Finance permission
Payment/ledger/settlement reconciliation
Privileged actions require actor, target, reason code, timestamp, request ID and audit event. High-risk finance actions should support separation of duties/dual approval where product operations require it.
6.8 Privacy and biometric consent
Method
Path
Access
Purpose
GET
/me/privacy/consents
Auth
Read consent state
POST
/me/privacy/biometric-consents
Auth
Record explicit consent with policy version
POST
/me/privacy/biometric-deletion-requests
Auth
Withdraw consent and request deletion
GET
/me/privacy/biometric-deletion-requests/{jobId}
Auth
Check deletion progress
GET
/me/privacy/data-export-status
Auth, if export feature is approved
Optional data export status
Deletion workflow must stop new processing immediately, invalidate pending jobs, delete Qdrant vectors and covered derived data, and maintain durable deletion tasks/tombstones to prevent retries or restored backups from re-creating data. The PRD's deletion timing target must be tested against all storage/queue/cache/backup layers before it is promised.
7. Key workflow contracts
7.1 Purchase flow
sequenceDiagram
  participant C as Client
  participant API as Go Core API
  participant DB as PostgreSQL
  participant PG as Payment Provider
  participant W as Worker
  C->>API: Create checkout verification
  API->>DB: Store challenge/status
  C->>API: Verify OTP/event code
  API->>DB: Record scoped verification grant
  C->>API: POST /checkout/quote
  API->>DB: Recalculate current price/eligibility
  C->>API: POST /orders (Idempotency-Key)
  API->>DB: Create order + immutable snapshots
  C->>API: POST /orders/{id}/payments
  API->>PG: Create payment attempt
  PG-->>API: Provider reference / next action
  PG->>API: Signed webhook
  API->>DB: Verify, dedupe, record event, update state
  API->>W: Outbox event
  W->>DB: Trigger post-payment fulfillment workflow
Rules:
A browser return URL is not proof of payment. Verify signed webhook or query provider using trusted server credentials.
Deduplicate provider event IDs and use idempotency keys for payment attempts.
Keep order state, payment state, fulfillment state, refund state and settlement state distinct.
7.2 Upload and quota
Client sends file count and expected sizes to preflight.
Core API validates event membership, file-count policy, quota and allowed file types.
Batch creation transaction reserves quota atomically.
Only after reservation succeeds does API issue short-lived upload grants.
Client uploads directly to private object storage.
Completion endpoint validates provider state; worker checks actual bytes/checksum/content and creates photo/assets metadata.
Reconcile physical bytes and billable bytes independently; release unused reservation.
Failed/expired batches trigger cleanup and reservation release; cleanup itself must be idempotent.
7.3 Hi-Res fulfillment and SLA
Start timestamp and definition of “payment completed” must be consistent and explicitly selected.
hires_due_at is persisted per order item and never inferred from a client clock.
Creators submit master Hi-Res only against eligible order items.
Server validates that the submitted master is a permitted asset and associates it to the correct order item/photo.
On success, record fulfillment event, notify buyer and allow eligible Hi-Res download grant.
On deadline breach, mark refund candidate/eligible according to approved policy, then run documented exception checks.
A refund-eligible state is not the same as a refund completed state.
7.4 Refund and wallet
Confirmed: SLA breach can qualify for refund subject to documented checks/exceptions.
OPEN: internal wallet vs original payment method, gateway-fee handling, refund after Non-Hi-Res download, partial refund, and clawback after payout.
When policy is approved:
Create refund request and item-level calculation snapshot.
Use integer rupiah and deterministic rounding.
Reverse creator/Host/platform allocations with compensating ledger transactions; never delete posted ledger entries.
If refund destination is internal wallet, post a balanced journal that increases buyer wallet liability/credit and reconciles the original revenue/clearing accounts.
If destination is external payment provider, mark completed only after provider confirms success.
Use idempotency to prevent duplicate credit/refund on worker retries.
Notification delivery failure must not roll back a successful financial transaction.
7.5 Settlement and payout
Paid order alone does not make creator/Host funds available.
Eligibility requires Hi-Res fulfillment and approved hold period, with no blocking refund/dispute status.
Settlement records preserve beneficiary, amount, rule version and source order snapshots.
Payout uses a separate state machine, provider reference and idempotency.
Reconcile provider payout events against ledger and settlement state.
Exact hold duration, payout frequency, minimum payout, fees, negative balance treatment and bank verification remain OPEN.
7.6 Biometric deletion
Authenticate and verify the subject's request.
Record withdrawal and immediately prevent new biometric jobs for the subject.
Mark deletion request and create durable tasks for each relevant system.
Cancel or invalidate in-flight work; workers must recheck current consent before writing vectors.
Delete Qdrant points and derived biometric references covered by policy.
Retry failures, record completion per target, and avoid logging raw biometric data.
Keep minimum audit evidence and deletion tombstones to prevent resurrection from queue retries or backup restore.
Do not automatically delete original photo assets solely because biometric consent was withdrawn.
8. State machine contracts
These are initial proposed states; exact enum values and transition matrix must be reviewed and encoded in tests.
8.1 Order and payment
Order: PENDING_PAYMENT → PAID → fulfillment/complete path; terminal/cancel states depend on policy.
Payment attempt: CREATED → PENDING → SUCCEEDED or FAILED/EXPIRED.
Payment success comes from verified provider state, never client request alone.
Late webhook must not regress a terminal state without explicit reconciliation logic.
8.2 Upload
CREATED → RESERVED → UPLOADING → VERIFYING → COMPLETED, with FAILED/EXPIRED cleanup paths. Only RESERVED/valid batch states may issue upload grants.
8.3 Fulfillment and refund
Fulfillment: PENDING → IN_PROGRESS → HIRES_READY; failure and retries are explicit.
Refund: proposed PENDING_REVIEW → ELIGIBLE/REJECTED → APPROVED → PROCESSING → COMPLETED/FAILED/CANCELLED.
Refund and fulfillment must be race-safe: if Hi-Res becomes ready at the same time SLA job runs, a transaction/locking strategy decides the canonical state.
8.4 Settlement and payout
Settlement: PENDING_FULFILLMENT → HOLD → AVAILABLE → PAID_OUT/BLOCKED.
Payout: REQUESTED → VALIDATING → SUBMITTED → SUCCEEDED/FAILED/CANCELLED.
Posted financial journals are immutable; corrections use compensating entries.
9. Database and transaction boundary rules
PostgreSQL is the source of truth for users, capabilities, events, orders, payment state snapshots, refund state, ledger, settlement, quota reservations, consent and deletion workflow.
Qdrant is a derived search index; it must not grant permissions or become the only record of consent/deletion state.
Object storage holds bytes; PostgreSQL holds object metadata, ownership, lifecycle and inventory.
Use transaction/outbox pattern when a database mutation must trigger a queue event.
Do not attempt a distributed transaction across PostgreSQL, Qdrant, object storage and payment provider. Use durable workflows, retries, idempotency and reconciliation.
Unique constraints should protect idempotency keys, provider event IDs, membership duplicates, order item uniqueness where applicable, and quota reservation scope.
Double-entry journal must balance at posting time; validate through a transaction service/procedure and tests.
Avoid cascade deletion for orders, payment events, ledger, consent audit and other records that must survive for audit/legal reasons.
Store secrets in secret manager/environment injection, not database rows or repository.
Database migrations must be versioned, reviewed and rollback-aware.
10. Reliability, observability and security
10.1 Observability
Structured JSON logs with request_id, trace_id, actor/resource IDs where safe, and stable error code.
Metrics: API latency/error rate, queue age/depth, upload completion/failure, quota reservation leakage, webhook lag, payment reconciliation mismatch, Hi-Res SLA breach, refund duration, ledger imbalance alerts, deletion job age, Qdrant indexing/deletion lag.
Tracing across Core API → outbox/queue → worker → AI/provider.
Alerts for ledger imbalance, duplicate credit prevention, payment/ledger mismatch, expired reservations not released, SLA timer backlog, and biometric deletion tasks stuck.
10.2 Security controls
Validate provider webhook signatures and replay windows.
Least-privilege service credentials; separate runtime identities for API, AI, worker, and admin.
Rate limit and abuse-protect BIB/face search to reduce enumeration/stalking.
Never include private object keys, OTPs, access tokens, face vectors, legal docs or bank details in logs.
Restrict finance actions and require audit reason; consider dual approval for adjustments/payout destination changes.
Validate upload content, actual size, malware/file-type policy as appropriate, and image decoder resource limits.
Use short-lived presigned URLs; keep master bucket private and separate from public preview delivery.
Design privacy and legal obligations with Indonesian counsel; this document is not a legal compliance certification.
11. Open decisions blocking final contract
ID
Decision
Why it blocks implementation
API-OPEN-01
Final auth/login and OTP delivery design
Defines session and challenge endpoints
API-OPEN-02
Final payment provider (Xendit/Midtrans)
Webhook, payment intent, refund and payout contract differ
API-OPEN-03
Refund destination: internal wallet or original method, or both
Ledger and provider state machine
API-OPEN-04
Gateway fee treatment and whether any cost is passed to buyer
Refund amount and accounting
API-OPEN-05
Definition of SLA start time and race condition policy
Fulfillment deadline and refund eligibility
API-OPEN-06
Refund eligibility after Non-Hi-Res download; partial refunds
Order/refund calculation
API-OPEN-07
Settlement hold duration and release criteria
Wallet/available balance semantics
API-OPEN-08
Host Fee calculation base, precedence and rounding
Immutable revenue snapshots
API-OPEN-09
Owner model for storage quota (creator, organization, plan, or account)
Authorization and quota joins
API-OPEN-10
OTP/event code binding and registration-provider integration
Checkout verification
API-OPEN-11
Host Admin cap and cross-organization membership policy
Membership constraints
API-OPEN-12
Final queue (RabbitMQ vs Redis Streams)
Delivery/retry and operational model
API-OPEN-13
AI consent scope, retention and deletion timing target
Privacy and worker guarantees
API-OPEN-14
API deployment topology/BFF for web and CORS/cookie policy
Browser authentication/security
API-OPEN-15
Party/beneficiary normalized model for financial foreign keys
Referential integrity in ledger/settlement
API-OPEN-16
Refund clawback when creator/Host has already been paid
Negative balance/offset/receivable handling
Implementation rule: unresolved items can be implemented behind interfaces and feature flags only if the default behavior is safe and documented. Do not hardcode guessed financial/legal policy.
12. Recommended implementation sequence
Phase A — Contract and foundation
Resolve critical OPEN decisions for auth, payment, refund destination, SLA start, Host Fee formula, and quota owner.
Approve API naming, error envelope, auth scopes, idempotency behavior and OpenAPI schemas.
Review ERD table/constraint changes and produce versioned PostgreSQL migrations.
Create Go modular monolith skeleton, configuration, health/readiness endpoints, migrations, structured logs, tracing and outbox.
Phase B — Identity and event core
Implement auth/session, users/capabilities, organization memberships and authorization tests.
Implement Host application/review, event/Fototree, creator membership, photo metadata and moderation.
Implement private object storage adapter, quota reservations, multipart upload and reconciliation.
Phase C — Media and AI
Implement watermark/thumbnail workers and master/private asset policy.
Implement FastAPI AI adapter, BIB OCR, face search consent checks, Qdrant indexing and deletion jobs.
Benchmark upload volume, AI quality/latency and deletion workflow; treat PRD performance numbers as targets until measured.
Phase D — Commerce and finance
Implement cart, quote, scoped checkout verification, order snapshots and payment adapter.
Implement signed webhooks, reconciliation, item-level fulfillment and SLA timer.
Implement refund workflow and double-entry wallet/ledger only after refund destination and fee policy are approved.
Implement settlement hold, Host Fee snapshot, payout adapter and finance reconciliation.
Phase E — Channels and release
Connect storefront checkout, Creator Studio, Host Portal, Admin Dashboard and Flutter to the same versioned contract.
Add contract tests, integration tests, security tests, concurrency tests and restore drills.
Run closed beta with monitored support/refund process before broad release.
13. Acceptance criteria for API v1
Every endpoint has owner, auth scope, request/response schema, error codes and idempotency behavior where applicable.
No client can set payment success, wallet balance, refund completion, revenue split, quota committed bytes or download entitlement directly.
Duplicate payment webhooks, refund jobs and upload completion requests do not duplicate financial effects or assets.
Upload grants are not issued before atomic quota reservation.
Hi-Res masters are private and download grants are short-lived and order-bound.
BIB search alone cannot authorize checkout.
Biometric withdrawal prevents new jobs and triggers traceable deletion tasks.
Posted ledger transactions are immutable and balanced.
Host Admin cannot access finance settings unless an explicit permission is granted.
Financial/settlement reports can be reconciled to provider events and ledger.
Performance and AI accuracy targets have reproducible benchmarks and are not represented as proven before testing.
14. Companion artifact
GALERIKU_OPENAPI_v1_DRAFT.yaml is a starter OpenAPI contract for the core patterns and selected endpoints. It is intentionally marked draft; it does not yet enumerate every endpoint in this document. The API catalog above is the broader planning scope. Before production, all endpoints should be represented in OpenAPI and covered by schema/contract tests. lanjut ini dulu, pastikan sesuai dari semua docs nya, referensi fotoyu dll

<ADDITIONAL_METADATA>
The current local time is: 2026-10-15T13:56:21+07:00.
</ADDITIONAL_METADATA>