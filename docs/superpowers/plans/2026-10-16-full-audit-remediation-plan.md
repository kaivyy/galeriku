# Full System Audit Remediation & Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate all Critical (P0), High (P1), Medium (P2), and Low (P3) defects identified in [`docs/audits/FULL_SYSTEM_AUDIT_2026_10_16.md`](file:///root/galeriku/docs/audits/FULL_SYSTEM_AUDIT_2026_10_16.md) across the Go Backend, PostgreSQL Migrations, Python AI Worker, Astro Web Storefront, Flutter Mobile App, OpenAPI 3.1 Spec, and Documentation.

**Architecture:** Preserve the Hexagonal Go Backend, Python AI Worker, Astro SSR/Static Hybrid Web Storefront, and Flutter Mobile architecture while restoring strict Zero-Trust tenant isolation, DEC-Q01/Q02/Q03 enforcement, double-entry ledger balance invariants, end-to-end S3/RabbitMQ/Database contracts, WCAG AA contrast, and Antislop Hard Gates.

**Tech Stack:** Go 1.24 (`pgx/v5`, `minio-go/v7`, `amqp091-go`), PostgreSQL 16, Redis 7, RabbitMQ 3.13, Qdrant v1.11, Python 3.13 (`pg8000`, `boto3`, `pika`, `qdrant-client`, `Pillow`), Astro 5 + TypeScript + Vitest, Flutter 3 (Dart, `provider`, `http`).

## Global Constraints

- **Zero-Trust Backend:** All prices, balances, payment statuses, refunds, capabilities, tenant isolation, and download grants are validated and computed server-side in Go.
- **Financial Precision:** All Indonesian Rupiah amounts are stored as exact integer `BIGINT` (`int64`), never floats.
- **Double-Entry Invariant:** Every financial mutation enforces `SUM(debit) == SUM(credit)` and credits/debits the proper normal balance direction.
- **Biometric Compliance (UU PDP No. 27/2022):** PostgreSQL never stores raw face embeddings or raw selfie images; vectors reside exclusively in Qdrant with subject references tracked for right-to-erasure.
- **Antislop Hard Gates:** Zero em dash characters (`U+2014`) in code comments, UI copy, or documentation (preserve `docs/PRD.md` untouched); minimum 44x44px touch targets (`R-03`); WCAG AA 4.5:1 text contrast (`R-25`); zero dead interactive elements (`R-26`); mandatory loading/empty/error states (`R-27`); dials locked at ENERGY 2 / RHYTHM 2 / MOTION 1 (`R-37`).

---

### Task 1: Fix FotoTree Optional Auth/Tenant Resolution & Pre-Mutation Cross-Tenant Ownership Check (P0-01, P0-02, P2-01)

**Files:**
- Modify: `backend/internal/middleware/rbac.go`
- Modify: `backend/internal/handler/router.go:205-218`
- Modify: `backend/internal/handler/event_tree_handler.go:145-260`
- Modify: `backend/internal/repository/event_repository.go:150-165`
- Test: `backend/internal/handler/event_tree_handler_test.go`

**Interfaces:**
- Produces: `middleware.OptionalAuthenticateAndTenant(jwtSecret string, tenantRepo TenantAuthorizer) func(http.Handler) http.Handler`
- Restores: `GET /api/v1/events` and `GET /api/v1/events/{id}` dual behavior (public `PUBLISHED` access for unauthenticated guests; strict tenant isolation + `403 AUTH_FORBIDDEN` on cross-tenant access for authenticated callers) and pre-mutation ownership verification on `PUT /api/v1/events/{id}`.

- [ ] **Step 1: Write the failing test for cross-tenant `PUT /api/v1/events/{id}` non-mutation and `GET /api/v1/events/{id}` 403 enforcement**

Verify in `backend/internal/handler/event_tree_handler_test.go` (`TestFotoTree_CrossTenantAndOutsiderDenied`) that:
1. Authenticated non-member calling `GET /api/v1/events/{id}` receives `403 AUTH_FORBIDDEN`.
2. Authenticated member of Org B calling `PUT /api/v1/events/{id}` (owned by Org A) with `X-Organization-ID: <OrgB>` receives `403 AUTH_FORBIDDEN` AND the event's `name` and `status` in PostgreSQL remain unchanged.
3. Unauthenticated guest calling `GET /api/v1/events/{id}` receives `404` when `DRAFT` and `200` when `PUBLISHED`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/galeriku/backend && go test -v -run TestFotoTree ./internal/handler/...`
Expected: FAIL (`expected 403 for non-member, got 404`)

- [ ] **Step 3: Implement `OptionalAuthenticateAndTenant` and pre-mutation ownership check**

1. In `backend/internal/middleware/rbac.go`, also check `r.PathValue("id")` when resolving event-scoped routes or add `OptionalAuthenticateAndTenant(jwtSecret string, tenantRepo TenantAuthorizer)`:
```go
// OptionalAuthenticateAndTenant authenticates Bearer tokens when present and resolves
// TenantContext if an organization or event identifier is supplied, while allowing
// unauthenticated guest requests to proceed without AuthIdentity/TenantContext.
func OptionalAuthenticateAndTenant(jwtSecret string, tenantRepo TenantAuthorizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}
			// Delegate to Authenticate when Authorization header is provided
			Authenticate(jwtSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity, ok := GetAuthIdentity(r.Context())
				if !ok || tenantRepo == nil {
					next.ServeHTTP(w, r)
					return
				}
				orgID, statusCode, errCode, errMsg := resolveTargetOrganization(r.Context(), r, tenantRepo)
				if statusCode == http.StatusForbidden {
					writeError(w, r, statusCode, errCode, errMsg)
					return
				}
				if orgID != uuid.Nil {
					membership, err := tenantRepo.GetActiveHostMembership(r.Context(), identity.UserID, orgID)
					if err != nil || membership == nil {
						writeError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "active host organization membership required")
						return
					}
					ctx := WithTenantContext(r.Context(), TenantContext{
						OrganizationID: membership.HostOrganizationID,
						MembershipID:   membership.ID,
						MembershipRole: membership.MembershipRole,
						ActorUserID:    identity.UserID,
					})
					r = r.WithContext(ctx)
				}
				next.ServeHTTP(w, r)
			})).ServeHTTP(w, r)
		})
	}
}
```
2. In `backend/internal/handler/router.go:210-211`, wrap `GET /api/v1/events` and `GET /api/v1/events/{id}` with `middleware.OptionalAuthenticateAndTenant(cfg.JWTSecret, cfg.TenantRepo)`.
3. In `backend/internal/handler/event_tree_handler.go:173-190` (`GetEventTree`), if `_, hasAuth := middleware.GetAuthIdentity(r.Context()); hasAuth` and `!hasTenant`, verify membership via `h.usecase` or return `403 AUTH_FORBIDDEN`.
4. In `backend/internal/handler/event_tree_handler.go:235-256` (`UpdateEvent`), fetch `existing, err := h.usecase.GetEventTree(r.Context(), eventID)` **before** calling `h.usecase.UpdateEvent(...)` and reject with `403 AUTH_FORBIDDEN` if `existing.Event.HostOrganizationID != tc.OrganizationID`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/backend && go test -v -run TestFotoTree ./internal/handler/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware/rbac.go backend/internal/handler/router.go backend/internal/handler/event_tree_handler.go backend/internal/repository/event_repository.go backend/internal/handler/event_tree_handler_test.go
git commit -m "fix(fototree): enforce optional tenant auth on event reads and pre-mutation ownership check"
```

---

### Task 2: Fix Server Wiring (`NoopPublisher`, `PaymentHandler` S3 Nil Panic, Server Timeouts) (P0-05, P0-06, P3-02)

**Files:**
- Modify: `backend/cmd/server/main.go:50-165`
- Modify: `backend/internal/handler/payment_handler.go:167-185`
- Test: `backend/internal/handler/payment_handler_test.go`

**Interfaces:**
- Initializes `rabbitPub` **before** constructing `BatchFinalizeUsecase`.
- Passes `s3svc` and `auditSvc` into `NewPaymentUsecase` / `NewPaymentHandler`.
- Adds defensive nil check on `h.s3` in `PaymentHandler.ServeDownload` and configures HTTP server timeouts.

- [ ] **Step 1: Write test in `backend/internal/handler/payment_handler_test.go` verifying `ServeDownload` handles both `s3 != nil` and `s3 == nil` safely without panicking**

- [ ] **Step 2: Run test to verify behavior**

Run: `cd /root/galeriku/backend && go test -v -run TestPayment ./internal/handler/...`

- [ ] **Step 3: Fix initialization order in `backend/cmd/server/main.go` and nil guard in `backend/internal/handler/payment_handler.go`**

In `backend/cmd/server/main.go`:
```go
	var publisher messaging.EventPublisher = messaging.NoopPublisher{}
	if cfg.RabbitURL != "" {
		rabbitPub, err := messaging.NewRabbitPublisher(cfg.RabbitURL, cfg.RabbitExchange)
		if err != nil {
			log.Printf("warning: failed to connect to RabbitMQ, falling back to NoopPublisher: %v", err)
		} else {
			publisher = rabbitPub
			defer func() { _ = rabbitPub.Close() }()
		}
	}
```
Place this block **before** `batchFinalizeUC := usecase.NewBatchFinalizeUsecase(uploadRepo, s3svc, publisher, auditSvc)`.
And at `paymentHandler` initialization:
```go
	publicBaseURL := "http://localhost:" + cfg.AppPort
	paymentHandler = handler.NewPaymentHandler(
		usecase.NewPaymentUsecase(paymentRepo, usecase.StubPaymentGateway{}, cfg.OTPSecret, publicBaseURL, auditSvc),
		s3svc,
	)
```
And on `http.Server`:
```go
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
```

- [ ] **Step 4: Run tests and build server binary**

Run: `cd /root/galeriku/backend && go build -o /dev/null ./cmd/server && go test -v ./internal/handler/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/server/main.go backend/internal/handler/payment_handler.go backend/internal/handler/payment_handler_test.go
git commit -m "fix(server): wire RabbitMQ publisher before BatchFinalizeUsecase and pass S3Service to PaymentHandler"
```

---

### Task 3: Fix Order Creation (`SellerUserID`, DEC-Q02 BIB Grant Check) & BIB OTP Verification Security (P0-03, P1-04, P2-02)

**Files:**
- Modify: `backend/internal/repository/discovery_repository.go`
- Modify: `backend/internal/usecase/discovery_usecase.go`
- Modify: `backend/internal/handler/discovery_handler.go`
- Modify: `backend/internal/security/checkout_otp.go`
- Test: `backend/internal/handler/discovery_handler_test.go`

**Interfaces:**
- `DiscoveryUsecase.CreateOrder` queries `photos.uploader_user_id` (setting `SellerUserID` to the real photographer), populates `EventID` and active price, and enforces `ConsumeGrant` when a photo has confirmed BIB tags.
- `DiscoveryUsecase.ConfirmVerification(ctx, callerUserID, verificationID, otpCode)` verifies `verification.UserID == callerUserID` and marks the challenge `VERIFIED` so it cannot be replayed.

- [ ] **Step 1: Write failing tests in `backend/internal/handler/discovery_handler_test.go`**

Add assertions verifying:
1. `CreateOrder` sets `order_items.seller_user_id` to the photo's `uploader_user_id` (not `buyerID`).
2. `ConfirmVerification` by a different user returns `403 AUTH_FORBIDDEN`, and replaying the same OTP code a second time fails.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/galeriku/backend && go test -v -run TestDiscovery ./internal/handler/...`
Expected: FAIL

- [ ] **Step 3: Implement repository and usecase fixes in `discovery_repository.go`, `discovery_usecase.go`, `discovery_handler.go`, and `checkout_otp.go`**

1. In `discovery_repository.go`, fix `GetOrderByID` (`Scan(&order.ID, ..., &order.CreatedAt, &order.UpdatedAt)`), add `GetPhotoCheckoutMeta(ctx, photoID)` returning `(uploaderID uuid.UUID, eventID *uuid.UUID, priceIDR int64, hasBIB bool, err error)`, and update `CreateGrant` to mark `checkout_verifications.status = 'VERIFIED'` and `verification_challenges.status = 'VERIFIED'`.
2. In `discovery_usecase.go:255-275`, populate `SellerUserID: meta.UploaderID`, `EventID: meta.EventID`, `UnitPriceIDR: meta.PriceIDR`, and if `meta.HasBIB` is true, require and consume a valid grant from `req.BIBGrants`.
3. In `discovery_usecase.go:197`, require `verification.UserID == callerUserID` and `verification.Status == "PENDING"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/backend && go test -v -run TestDiscovery ./internal/handler/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/discovery_repository.go backend/internal/usecase/discovery_usecase.go backend/internal/handler/discovery_handler.go backend/internal/security/checkout_otp.go backend/internal/handler/discovery_handler_test.go
git commit -m "fix(discovery): attribute order items to photo uploader and enforce BIB verification ownership"
```

---

### Task 4: Fix Payment Webhook UUID Parsing, DEC-Q01 Hi-Res Fulfillment Gate, Double-Entry Ledger Sign, & Refund Authorization (P0-04, P0-07, P1-01, P1-02, P1-03, P1-05)

**Files:**
- Modify: `backend/internal/repository/ledger_repository.go:173-225`
- Modify: `backend/internal/repository/payment_repository.go:65-225`
- Modify: `backend/internal/usecase/payment_usecase.go:115-270`
- Modify: `backend/internal/usecase/finance_usecase.go:80-235`
- Modify: `backend/internal/handler/stats_handler.go:50-120`
- Modify: `backend/internal/handler/router.go:330-355`
- Test: `backend/internal/usecase/finance_usecase_test.go`
- Test: `backend/internal/handler/payment_handler_test.go`

**Interfaces:**
- `PostgresLedgerRepository.ensureAccountTx` uses `ON CONFLICT DO NOTHING` so concurrent account creation never aborts `tx` with `25P02`.
- `GetWalletBalance` returns positive `CREDIT - DEBIT` balance for `BUYER_WALLET`, `CREATOR_PAYABLE`, `HOST_PAYABLE`, and `PLATFORM_REVENUE`.
- `FinanceUsecase.RequestRefund` verifies `order.BuyerUserID == buyerID`, `order.Status == OrderStatusPaid`, `item.FulfillmentStatus != FULFILLED && != REFUNDED`, uses deterministic `idempKey = "refund:order_item:" + orderItemID.String()`, and marks the item `REFUNDED`.
- `PaymentUsecase.InitiatePayment` uses full `"GLK-" + order.ID.String()` so `HandleWebhook` parses the full UUID cleanly; `GrantDownload` blocks `HIRES_DOWNLOAD` until `item.FulfillmentStatus == FULFILLED`.

- [ ] **Step 1: Write failing tests in `finance_usecase_test.go` and `payment_handler_test.go`**

Add tests asserting:
1. Wallet balance after a `50000` IDR refund credit is `+50000` (not `-50000`).
2. Calling `RequestRefund` on another buyer's `orderItemID` or on an unpaid order fails, and replaying `RequestRefund` with a different `reasonCode` is rejected.
3. Calling `GrantDownload` with `HIRES_DOWNLOAD` before `UploadMaster` fails, and succeeds after `UploadMaster`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /root/galeriku/backend && go test -v ./internal/usecase/... ./internal/handler/...`
Expected: FAIL

- [ ] **Step 3: Implement ledger, refund, payment, and stats authorization fixes**

1. In `backend/internal/repository/ledger_repository.go:194-223`:
   - Add `ON CONFLICT DO NOTHING` to `INSERT INTO ledger_accounts` inside `ensureAccountTx`.
   - In `GetAccountBalance`, compute `SUM(CASE WHEN direction = 'CREDIT' THEN amount_idr ELSE -amount_idr END)` for liability/revenue accounts (`BUYER_WALLET`, `CREATOR_PAYABLE`, `HOST_PAYABLE`, `PLATFORM_REVENUE`, `ESCROW_HOLD`), while keeping `SUM(debit) == SUM(credit)` in `PostTransaction`.
2. In `backend/internal/usecase/finance_usecase.go:157-219` (`RequestRefund`):
   - Load `order, err := u.payRepo.GetOrderByID(ctx, item.OrderID)`, check `order.BuyerUserID == buyerID` and `order.Status == domain.OrderStatusPaid`.
   - Use `idempKey := "refund:order_item:" + orderItemID.String()` and mark `item.FulfillmentStatus = domain.FulfillmentStatusRefunded`.
3. In `backend/internal/usecase/payment_usecase.go`:
   - Line 122: `orderNumber := "GLK-" + order.ID.String()`.
   - In `GrantDownload`, if `assetType == domain.DownloadGrantTypeHiRes && item.FulfillmentStatus != domain.FulfillmentStatusFulfilled`, return an error.
   - In `UploadMaster`, if `now.After(job.HiresDueAt)`, return `nil, repository.ErrSLABreached`.
4. In `backend/internal/handler/stats_handler.go` & `router.go`:
   - Scope `HostStats` pending applications count to the caller's user/organization and attach `WithLiveCapabilityCheck(cfg.TenantRepo)` to `/api/v1/stats/admin` and `/api/v1/finance/settlements/release`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /root/galeriku/backend && go test -v ./...`
Expected: PASS (100% across all backend packages)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/ledger_repository.go backend/internal/repository/payment_repository.go backend/internal/usecase/payment_usecase.go backend/internal/usecase/finance_usecase.go backend/internal/usecase/finance_usecase_test.go backend/internal/handler/stats_handler.go backend/internal/handler/router.go backend/internal/handler/payment_handler_test.go
git commit -m "fix(finance,payment): enforce refund ownership, positive wallet balance, full webhook order UUID, and Hi-Res gate"
```

---

### Task 5: Fix Python AI Worker Pipeline, S3 Bucket Unification, BIB Tags, Default Photo Price, & Unittest Suite (P0-08, P0-09, P0-10, P0-11, P1-06, P1-07, P1-08, P2-04)

**Files:**
- Modify: `backend/internal/messaging/rabbit_publisher.go`
- Modify: `backend/internal/usecase/batch_finalize_usecase.go`
- Modify: `backend/internal/repository/upload_repository.go`
- Modify: `ai-worker/config.py`
- Modify: `ai-worker/db.py`
- Modify: `ai-worker/face_index.py`
- Modify: `ai-worker/main.py`
- Modify: `ai-worker/test_worker.py`
- Modify: `docker-compose.yml`

**Interfaces:**
- `messaging.PhotoIngestEvent` includes `UploaderUserID uuid.UUID` (`json:"uploader_user_id"`).
- `ai-worker/main.py` writes watermarked previews to `galeriku-master-vault` (`previews/{photo_id}.jpg`) and `galeriku-public-media`, populates `photo_bib_tags` and `photo_prices` (50,000 IDR default), fixes `create_photo_event_link` (`status = 'ACTIVE', linked_at = CURRENT_TIMESTAMP`), and queries real `upload_items.object_key` in `fetch_pending_jobs`.
- `ai-worker/test_worker.py` wraps tests in a `unittest.TestCase` subclass so `python -m unittest test_worker.py` runs and exits `0`.

- [ ] **Step 1: Add `unittest.TestCase` tests in `ai-worker/test_worker.py` for `create_photo_event_link`, `insert_bib_tag`, `ensure_default_photo_price`, and `galeriku-master-vault` preview upload**

- [ ] **Step 2: Run AI worker test to verify it fails**

Run: `cd /root/galeriku/ai-worker && .venv/bin/python -m unittest test_worker.py -v`
Expected: FAIL

- [ ] **Step 3: Implement fixes in `ai-worker/db.py`, `ai-worker/main.py`, `ai-worker/face_index.py`, `docker-compose.yml`, and Go `batch_finalize_usecase.go`**

1. In `ai-worker/db.py`:
   - Fix `create_photo_event_link`:
     ```sql
     INSERT INTO photo_event_links (photo_id, event_id, linked_by, status, linked_at)
     VALUES (:photo, :event, :user, 'ACTIVE', CURRENT_TIMESTAMP)
     ON CONFLICT (photo_id, event_id) DO NOTHING
     ```
   - Add `insert_bib_tag(self, photo_id: str, bib_number: str, confidence: float = 0.95)` inserting into `photo_bib_tags` (`id, photo_id, bib_raw, bib_normalized, confidence, source, status, created_at`).
   - Add `ensure_default_photo_price(self, photo_id: str, price_idr: int = 50000)` inserting into `photo_prices`.
   - Fix `fetch_pending_jobs` to join `upload_items ui ON ui.id = aj.upload_item_id JOIN upload_batches ub ON ub.id = aj.upload_batch_id` so real `object_key`, `owner_user_id`, and `event_id` are returned.
2. In `ai-worker/main.py`:
   - Upload watermarked preview to `cfg.S3_MASTER_BUCKET` (`galeriku-master-vault`) at `previews/{photo_id}.jpg` so Go's `PresignPreviewGet` and `ServeDownload` succeed.
   - Call `db.insert_bib_tag(photo_id, bib)` and `db.ensure_default_photo_price(photo_id, 50000)`.
   - Reuse a single `JobDB` instance instead of creating a new engine per message.
3. In `backend/internal/messaging/rabbit_publisher.go` and `batch_finalize_usecase.go`:
   - Add `UploaderUserID uuid.UUID` (`json:"uploader_user_id"`) to `PhotoIngestEvent` and populate `session.CreatorUserID`.
   - Also insert a default `50000` IDR row into `photo_prices` when creating `photos` rows in `upload_repository.go` (`FinalizeBatch`).

- [ ] **Step 4: Run AI worker and Go backend tests to verify they pass**

Run: `cd /root/galeriku/ai-worker && .venv/bin/python -m unittest test_worker.py -v && cd /root/galeriku/backend && go test -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ai-worker/ backend/internal/messaging/rabbit_publisher.go backend/internal/usecase/batch_finalize_usecase.go backend/internal/repository/upload_repository.go docker-compose.yml
git commit -m "fix(ai-worker): unify preview bucket, populate photo_bib_tags and photo_prices, and fix event link SQL"
```

---

### Task 6: Fix Astro Web Storefront Contrast (R-25), Mobile Search Grid (R-35), Proxy Port, Mobile Logout, & Antislop Linter (P0-12, P0-13, P0-14, P0-15, P1-09, P1-10, P1-11, P1-16)

**Files:**
- Modify: `web/astro.config.mjs`
- Modify: `web/package.json`
- Modify: `web/scripts/lint-copy.js`
- Modify: `web/src/layouts/Base.astro`
- Modify: `web/src/pages/index.astro`
- Modify: `web/src/pages/search.astro`
- Modify: `web/src/pages/studio.astro`
- Modify: `web/src/pages/collection.astro`
- Test: `web/test/design-tokens.test.ts`

**Interfaces:**
- Fixes WCAG AA contrast on `.btn-primary`, `.pill-nav a.active`, `.price-tag` (`color: #FFFFFF`), and OTP badge (`color: #B45309`).
- Restores mobile search visibility by scoping `@media (max-width: 1023px)` hiding only to `#desktop-grid.home-desktop-grid` instead of all `.photo-grid` elements.
- Updates `astro.config.mjs` proxy target to `process.env.PUBLIC_API_ORIGIN || 'http://127.0.0.1:8080'`.
- Wires mobile `#bottom-nav-account` to log out when authenticated, eliminates duplicate `loadCatalog()` execution on `index.astro`, and adds `.astro` and `.css` to `web/scripts/lint-copy.js`.

- [ ] **Step 1: Add test assertions in `web/test/design-tokens.test.ts` and update `web/scripts/lint-copy.js` to scan `.astro` and `.css`**

In `web/scripts/lint-copy.js:14`:
```js
const VALID_EXTENSIONS = new Set(['.ts', '.tsx', '.js', '.jsx', '.json', '.mjs', '.cjs', '.astro', '.css']);
```

- [ ] **Step 2: Run web tests and linter**

Run: `cd /root/galeriku/web && npm test && npm run lint:copy`

- [ ] **Step 3: Fix `Base.astro`, `astro.config.mjs`, `index.astro`, `search.astro`, `studio.astro`, and `collection.astro`**

1. In `web/astro.config.mjs:18`:
   ```js
   '/api': {
     target: process.env.PUBLIC_API_ORIGIN || 'http://127.0.0.1:8080',
     changeOrigin: true,
   },
   ```
2. In `web/src/layouts/Base.astro`:
   - Set `color: #FFFFFF;` on `.pill-nav a.active` (`L166`), `.btn-primary` (`L288`), and `.price-tag` (`L332`).
   - Change `.feed-card .overlay .follow-btn` (`L242`) to `min-height: 44px; padding: 8px 16px;`.
   - Change `@media (max-width: 1023px) { .photo-grid { display: none; } }` (`L264`) to `@media (max-width: 1023px) { .home-desktop-grid { display: none; } }` and add class `home-desktop-grid` to `#desktop-grid` in `web/src/pages/index.astro:14`.
   - Add `:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 2px; }` on inputs (`L308`).
   - In `updateAuthUI()` (`L98-122`), also update `#bottom-nav-account` when logged in so mobile users can log out cleanly, and remove the duplicate top-level `updateAuthUI()` call if `astro:page-load` is registered.
3. In `web/src/pages/index.astro:245-250`, remove the duplicate top-level `void loadCatalog();` call (keep `document.addEventListener('astro:page-load', ...)` guarded by `if (!document.getElementById('reels-feed')) return;`).
4. In `web/src/pages/search.astro:65-80`, render real preview images via `/api/v1/photos/${p.photo_id}/preview-url`, use accessible badge contrast (`#B45309`), and add an interactive `+ Keranjang` button per result card.
5. In `web/src/pages/studio.astro:14` and `web/src/pages/collection.astro`, replace the `/login/` redirect link and static stub with live API-backed state.

- [ ] **Step 4: Run web tests, typecheck, Astro check, and copy linter**

Run: `cd /root/galeriku/web && npm test && npm run typecheck && npm run check && npm run lint:copy`
Expected: PASS (0 errors, 0 warnings, 12+ files inspected by `lint:copy`)

- [ ] **Step 5: Commit**

```bash
git add web/
git commit -m "fix(web): resolve WCAG AA contrast, mobile search visibility, proxy port, and antislop gates"
```

---

### Task 7: Fix Flutter Mobile App Navigation (`FeedScreen` on Tab 1), Preview Rendering, Profile Identity, & Duplicate Order Bug (P0-16, P1-12, P1-13, P1-14, P2-09)

**Files:**
- Modify: `mobile/lib/main.dart`
- Modify: `mobile/lib/src/api.dart`
- Modify: `mobile/lib/src/models.dart`
- Modify: `mobile/lib/src/state.dart`
- Modify: `mobile/lib/screens/reels_feed_screen.dart`
- Modify: `mobile/lib/screens/feed_screen.dart`
- Modify: `mobile/lib/screens/profile_screen.dart`
- Modify: `mobile/lib/screens/qris_screen.dart`
- Test: `mobile/test/widget_test.dart`
- Test: `mobile/test/api_test.dart`

**Interfaces:**
- Mounts `FeedScreen()` at tab index 1 (`Cari`) in `HomeShell._tabs`.
- Adds `Future<String?> getPreviewUrl(String photoId)` to `GalerikuApi`.
- Loads authenticated user profile via `api.me()` in `AppState` and displays real email/capabilities in `ProfileScreen`.
- Catches all network exceptions in `_loadCatalog` (preventing infinite spinners), enforces `minHeight: 44` on `Ikuti`, and reuses `state.lastOrder` in `QrisScreen` instead of creating duplicate orders.

- [ ] **Step 1: Add test in `mobile/test/widget_test.dart` verifying tapping Tab 1 ("Cari") renders BIB search UI (`FeedScreen`)**

- [ ] **Step 2: Run Flutter tests to verify it fails**

Run: `cd /root/galeriku/mobile && /opt/flutter/bin/flutter test`
Expected: FAIL

- [ ] **Step 3: Implement fixes in `mobile/lib/`**

1. In `mobile/lib/main.dart:1-76`, import `screens/feed_screen.dart` and change `_tabs` to mount `FeedScreen()` at index 1.
2. In `mobile/lib/src/api.dart`, add `getPreviewUrl(String photoId)` calling `GET /api/v1/photos/$photoId/preview-url`.
3. In `mobile/lib/src/state.dart`, call `refreshCart()` and `me()` after `login()` / `register()`, and avoid recreating an order in `QrisScreen` if `lastOrder` already matches the current cart.
4. In `mobile/lib/screens/reels_feed_screen.dart` and `feed_screen.dart`, add generic `catch (_) { ... }` fallback in `_loadCatalog()` so network failures render the error state with `Coba Lagi`, and set `constraints: const BoxConstraints(minHeight: 44, minWidth: 44)` on the `Ikuti` button.
5. In `mobile/lib/screens/profile_screen.dart`, display the user's real email and capabilities from `AppState` and wire the profile tiles to show feedback sheets/dialogs.

- [ ] **Step 4: Run Flutter tests to verify they pass**

Run: `cd /root/galeriku/mobile && /opt/flutter/bin/flutter test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add mobile/
git commit -m "fix(mobile): mount FeedScreen on search tab, handle network errors, and wire profile state"
```

---

### Task 8: Synchronize OpenAPI 3.1 Spec, Fix Broken Docs Links, Update `ROADMAP.md` & `CHANGELOG.md`, Fix `seed_demo_feed.sh`, & Purge Em Dashes (P1-15, P1-16, P2-05, P2-06, P2-07, P2-10)

**Files:**
- Modify: `docs/GALERIKU_OPENAPI_v1_DRAFT.yaml`
- Modify: `GALERIKU_OPENAPI_v1_DRAFT.yaml`
- Modify: `docs/ROADMAP.md`
- Modify: `CHANGELOG.md`
- Modify: `Makefile`
- Modify: `backend/scripts/seed_demo_feed.sh`
- Modify: `migrations/000001_initial_schema.up.sql:2`
- Modify: `migrations/000002_identity_and_sessions.down.sql`
- Modify: `docs/*.md` (excluding `docs/PRD.md`, which must never be modified)

**Interfaces:**
- Synchronizes `docs/GALERIKU_OPENAPI_v1_DRAFT.yaml` and `GALERIKU_OPENAPI_v1_DRAFT.yaml` with `backend/internal/handler/router.go`.
- Fixes all 48 broken `file:///root/galeriku/<DOC>.md` links in `docs/` to `file:///root/galeriku/docs/<DOC>.md`.
- Updates `docs/ROADMAP.md` and `CHANGELOG.md` to reflect all completed phases and tasks accurately.
- Fixes `backend/scripts/seed_demo_feed.sh` to be self-contained and upload to `local/galeriku-master-vault/previews/${PID}.jpg`.
- Removes all literal em dashes (`U+2014`) from `migrations/` and `docs/` (preserving `docs/PRD.md` untouched).

- [ ] **Step 1: Verify current broken links and em-dash count**

Run: `grep -rnP "\xe2\x80\x94" docs/ migrations/ --exclude="PRD.md" | wc -l`

- [ ] **Step 2: Apply fixes across `docs/`, `CHANGELOG.md`, `Makefile`, `backend/scripts/seed_demo_feed.sh`, and `migrations/`**

1. Replace all `file:///root/galeriku/(IMPLEMENTATION_PLAN|PRD|DATABASE_SPEC|API_SPEC|ARCHITECTURE|UI_UX_SPEC|GALERIKU_API_ARCHITECTURE_SPEC)\.md` references in `docs/` with `file:///root/galeriku/docs/$1.md`.
2. Replace all literal em dashes (`U+2014`) in `docs/` (excluding `docs/PRD.md`) and `migrations/000001_initial_schema.up.sql` with `-` or `:`.
3. Update `docs/ROADMAP.md` and `CHANGELOG.md` so Tasks 2.5-2.10 and Phases 1-9 reflect their actual implementation status and post-Phase-9 commits.
4. Make `backend/scripts/seed_demo_feed.sh` generate JPEG previews via inline Python (`Pillow`) and copy them to `local/galeriku-master-vault/previews/${PID}.jpg`.
5. Update `Makefile` `.PHONY`, `help`, and `test` targets.

- [ ] **Step 3: Run full verification across the entire repository**

Run:
```bash
make test && make test-integration && make openapi-validate && (cd ai-worker && .venv/bin/python -m unittest test_worker.py -v) && (cd mobile && /opt/flutter/bin/flutter test) && (! grep -rnP "\xe2\x80\x94" backend/ web/src/ ai-worker/ mobile/lib/ migrations/ docs/ CHANGELOG.md README.md --exclude="PRD.md" --exclude="design-system.ts")
```
Expected: 100% PASS, exit code 0.

- [ ] **Step 4: Update Knowledge Graph & Commit**

```bash
graphify update .
git add .
git commit -m "docs(audit): sync roadmap, changelog, links, seed script, and antislop compliance"
```
