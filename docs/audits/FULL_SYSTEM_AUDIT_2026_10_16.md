# Galeriku: Full System Audit Report & Comprehensive Remediation Guide

**Audit Date:** 2026-10-16  
**Audited Baseline:** Commit `eb444d0` (`HEAD -> main`) + Uncommitted Working Tree  
**Scope:** End-to-end verification of all documentation (`docs/*`, `CHANGELOG.md`, `README.md`, `GALERIKU_OPENAPI_v1_DRAFT.yaml`, `Makefile`), Database Migrations (`000001` through `000005`), Go Backend (`backend/`), Python AI Worker (`ai-worker/`), Astro Web Storefront (`web/`), and Flutter Mobile App (`mobile/`).  
**Associated Superpowers Execution Plan:** [`docs/superpowers/plans/2026-10-16-full-audit-remediation-plan.md`](file:///root/galeriku/docs/superpowers/plans/2026-10-16-full-audit-remediation-plan.md)

---

## 1. Executive Summary & Live Test Execution Matrix

| Subsystem | Command Executed | Result | Evidence Summary |
|---|---|:---:|---|
| **Backend Short Tests** | `cd backend && go test -v -short ./...` | **PASS** | All unit tests pass in short mode. |
| **Backend Live Integration** | `cd backend && go test -v ./...` | **FAIL** | `TestFotoTree_CrossTenantAndOutsiderDenied` fails at [`event_tree_handler_test.go:273`](file:///root/galeriku/backend/internal/handler/event_tree_handler_test.go#L273) (`expected 403 for non-member, got 404`) due to uncommitted middleware removal in [`router.go:210-211`](file:///root/galeriku/backend/internal/handler/router.go#L210-L211). |
| **Web Unit & Typecheck** | `cd web && npm test && npm run typecheck && npm run check` | **PASS** | 19 Vitest assertions pass; `tsc --noEmit` and `astro check` report 0 errors across 15 files. |
| **Flutter Mobile Tests** | `cd mobile && /opt/flutter/bin/flutter test` | **PASS** | 7 widget & API contract tests pass (`widget_test.dart`, `api_test.dart`). |
| **Antislop Copy Linter** | `cd web && npm run lint:copy` | **PARTIAL** | Passes on 3 `.ts` files, but [`lint-copy.js:14`](file:///root/galeriku/web/scripts/lint-copy.js#L14) omits `.astro` and `.css` extensions (skipping all 9 `.astro` files), and 61 lines in `docs/` + `migrations/` still contain literal em dashes (`U+2014`). |
| **AI Worker Script** | `cd ai-worker && .venv/bin/python test_worker.py` | **PARTIAL** | Top-level script assertions pass against stub data, but [`test_worker.py`](file:///root/galeriku/ai-worker/test_worker.py) has no `unittest.TestCase` class (`python -m unittest` exits code 5: `NO TESTS RAN`) and skips DB paths that crash in production. |
| **OpenAPI Validator** | `make openapi-validate` | **PARTIAL** | YAML syntax and `$ref` resolution pass (35 paths), but 30 of 57 actual routes in [`router.go`](file:///root/galeriku/backend/internal/handler/router.go) are missing from OpenAPI and 10 OpenAPI paths do not match `router.go`. |

### Finding Severity Breakdown
- **CRITICAL (P0):** 16 findings (active integration test failure, nil-pointer download panic, cross-tenant IDOR event mutation, unpaid order refund minting, broken AI worker to backend S3/DB pipeline, WCAG AA contrast failures, unreachable mobile search tab).
- **HIGH (P1):** 16 findings (PostgreSQL aborted transaction `25P02`, inverted double-entry balance sign, Midtrans webhook UUID truncation bug, F-04 live capability check bypass, dead UI buttons, double script execution).
- **MEDIUM (P2):** 12 findings (missing unique slug index on `events`, `placed_at` NULL scan bug, broken `seed_demo_feed.sh`, 48 broken markdown `file://` links, outdated `ROADMAP.md` / `CHANGELOG.md` status headers).
- **LOW (P3):** 4 findings (migration down script index drops, missing HTTP server timeouts / `MaxBytesReader`, emulator localhost default).

---

## 2. Critical Findings (P0) & Complete Remediation Proposals

### P0-01: Uncommitted `router.go` & `event_tree_handler.go` Changes Break Tenant Isolation and Fail `make test-integration`
- **Files:**
  - [`backend/internal/handler/router.go:210-211`](file:///root/galeriku/backend/internal/handler/router.go#L210-L211)
  - [`backend/internal/handler/event_tree_handler.go:147-194`](file:///root/galeriku/backend/internal/handler/event_tree_handler.go#L147-L194)
  - [`backend/internal/handler/event_tree_handler_test.go:273`](file:///root/galeriku/backend/internal/handler/event_tree_handler_test.go#L273)
- **Root Cause:** Uncommitted working-tree edits removed `authMw(anyMember(...))` from `GET /api/v1/events` and `GET /api/v1/events/{id}`. Because no authentication or tenant middleware runs on those routes, `middleware.GetTenantContext(r.Context())` and `middleware.GetAuthIdentity(r.Context())` always return `false`.
- **Impact:**
  1. Authenticated Host members calling `GET /api/v1/events` with `Authorization` + `X-Organization-ID` never receive their own organization's `DRAFT` or `ARCHIVED` events.
  2. Authenticated Host members calling `GET /api/v1/events/{id}` on their own `DRAFT` event receive `404 NOT_FOUND`.
  3. Cross-tenant authenticated users receive `404` instead of `403 AUTH_FORBIDDEN`, failing `TestFotoTree_CrossTenantAndOutsiderDenied`.
- **Saran Perbaikan (Remediation):**
  - Buat middleware `OptionalAuthenticateAndTenant(jwtSecret, tenantRepo)` atau periksa header `Authorization: Bearer ...` dan `X-Organization-ID` secara opsional pada `GET /api/v1/events` dan `GET /api/v1/events/{id}`:
    - Jika header `Authorization` dikirim, validasi token (kembalikan `401` jika token rusak) dan muat keanggotaan dari `tenantRepo.GetActiveHostMembership` berdasarkan `X-Organization-ID` atau `event.HostOrganizationID`.
    - Jika user terautentikasi bukan anggota aktif dari organisasi pemilik event (atau `X-Organization-ID` berbeda dengan pemilik event), kembalikan `403 AUTH_FORBIDDEN`.
    - Jika request dikirim secara anonim (tanpa header `Authorization`), kembalikan hanya event berstatus `PUBLISHED` (`404 NOT_FOUND` untuk event `DRAFT`/`ARCHIVED`).

---

### P0-02: Cross-Tenant Event Mutation (IDOR / BOLA) Executes Database `UPDATE` Before Ownership Check
- **Files:**
  - [`backend/internal/handler/router.go:212`](file:///root/galeriku/backend/internal/handler/router.go#L212)
  - [`backend/internal/middleware/rbac.go:118-126`](file:///root/galeriku/backend/internal/middleware/rbac.go#L118-L126)
  - [`backend/internal/handler/event_tree_handler.go:242-255`](file:///root/galeriku/backend/internal/handler/event_tree_handler.go#L242-L255)
  - [`backend/internal/repository/event_repository.go:152-160`](file:///root/galeriku/backend/internal/repository/event_repository.go#L152-L160)
- **Root Cause:**
  1. `router.go:212` uses path parameter `{id}`, whereas `resolveTargetOrganization` in `rbac.go:118` only checks `r.PathValue("event_id")`.
  2. `EventTreeHandler.UpdateEvent` calls `h.usecase.UpdateEvent(...)` at line 242 **before** checking `if event.HostOrganizationID != tc.OrganizationID` at line 252.
  3. `UpdateEventSchedule` (`event_repository.go:153`) updates `WHERE id = $1` without `AND host_organization_id = $X`.
- **Impact:** Any Host Admin of Organization A can overwrite Organization B's event name, status (`PUBLISHED`/`ARCHIVED`), and timestamps in PostgreSQL before the handler returns `403`.
- **Saran Perbaikan (Remediation):**
  1. Di [`backend/internal/handler/event_tree_handler.go:235`](file:///root/galeriku/backend/internal/handler/event_tree_handler.go#L235), panggil `existing, err := h.usecase.GetEventTree(r.Context(), eventID)` atau teruskan `tc.OrganizationID` ke `h.usecase.UpdateEvent(ctx, tc.OrganizationID, eventID, ...)` **sebelum** mengeksekusi mutasi database.
  2. Di [`backend/internal/repository/event_repository.go:152`](file:///root/galeriku/backend/internal/repository/event_repository.go#L152), tambahkan `AND host_organization_id = $6` pada klausa `WHERE` di `UpdateEventSchedule`.

---

### P0-03: Complete DEC-Q02 BIB Privacy Gate Bypass & Wrong Seller Attribution (`SellerUserID: buyerID`) in `CreateOrder`
- **File:** [`backend/internal/usecase/discovery_usecase.go:229-293`](file:///root/galeriku/backend/internal/usecase/discovery_usecase.go#L229-L293)
- **Root Cause:**
  1. `CreateOrder` accepts `req.BIBGrants` (`discovery_usecase.go:75`), but never checks if requested photos have BIB tags and never calls `u.repo.ConsumeGrant`.
  2. Line 263 hardcodes `SellerUserID: buyerID` instead of querying `photos.uploader_user_id` and leaves `OrderItem.EventID` nil.
- **Impact:**
  1. Buyers can purchase any BIB-tagged marathon photo without OTP verification (violating DEC-Q02).
  2. Because `SellerUserID` equals `buyerID`, the actual photographer is rejected with `403 AUTH_FORBIDDEN` when uploading the Hi-Res master (`PaymentUsecase.UploadMaster` at [`payment_usecase.go:244`](file:///root/galeriku/backend/internal/usecase/payment_usecase.go#L244)), while the buyer can mark their own order item fulfilled and receives creator sales attribution.
- **Saran Perbaikan (Remediation):**
  1. Tambahkan metode repository `GetPhotoForCheckout(ctx, photoID)` yang mengambil `uploader_user_id`, `event_id` (dari `photo_event_links`), harga aktif (dari `photo_prices`), dan apakah foto memiliki tag di `photo_bib_tags`.
  2. Di `CreateOrder`, isi `SellerUserID: photo.UploaderUserID`, `EventID: photo.EventID`, dan `UnitPriceIDR: photo.PriceIDR`.
  3. Jika foto memiliki BIB tag terkonfirmasi, wajibkan `req.BIBGrants` yang valid dan panggil `u.repo.ConsumeGrant(ctx, grantID, buyerID, eventID)`. Jika tidak ada grant valid, tolak dengan error `ErrBIBGrantRequired` (`403 BIB_VERIFICATION_REQUIRED`).

---

### P0-04: Unauthorized Refund Minting (IDOR), Unpaid Order Refund, Missing SLA Check, & Infinite Replay in `RequestRefund`
- **File:** [`backend/internal/usecase/finance_usecase.go:157-219`](file:///root/galeriku/backend/internal/usecase/finance_usecase.go#L157-L219)
- **Root Cause:**
  1. `RequestRefund` loads `item` via `GetOrderItem` without checking `order.BuyerUserID == buyerID`.
  2. Never checks `order.Status == domain.OrderStatusPaid`.
  3. Never checks `fulfillment_jobs.hires_due_at < time.Now()` (unless `reasonCode == "MASTER_LOST"`).
  4. `idempKey` includes user-controlled `reasonCode` (`"refund:" + orderItemID.String() + ":" + reasonCode`) and never updates `order_items.fulfillment_status` to `'REFUNDED'`.
- **Saran Perbaikan (Remediation):**
  1. Muat parent order (`order, err := u.payRepo.GetOrderByID(ctx, item.OrderID)`) dan verifikasi `order.BuyerUserID == buyerID` (jika tidak, kembalikan `403 ErrForbidden`) serta `order.Status == domain.OrderStatusPaid` (jika tidak, kembalikan `422 ErrOrderNotPaid`).
  2. Tolak jika `item.FulfillmentStatus == domain.FulfillmentStatusFulfilled || item.FulfillmentStatus == domain.FulfillmentStatusRefunded`.
  3. Gunakan `idempKey := "refund:order_item:" + orderItemID.String()` (tanpa `reasonCode` agar tidak bisa di-replay dengan mengubah alasan).
  4. Setelah `PostTransaction` berhasil, perbarui `order_items.fulfillment_status = 'REFUNDED'` dan `fulfillment_jobs.status = 'SLA_BREACHED_REFUNDED'`.

---

### P0-05: Nil Pointer Dereference Panic on Download (`GET /api/v1/downloads/file/{token}`) & Miswired `NewPaymentUsecase`
- **Files:**
  - [`backend/cmd/server/main.go:145`](file:///root/galeriku/backend/cmd/server/main.go#L145)
  - [`backend/internal/handler/payment_handler.go:167-180`](file:///root/galeriku/backend/internal/handler/payment_handler.go#L167-L180)
- **Root Cause:** `cmd/server/main.go:145` calls `handler.NewPaymentHandler(..., nil)`, passing `nil` for `*storage.S3Service` even though `s3svc` was initialized at line 42. Calling `GET /api/v1/downloads/file/{token}` invokes `h.s3.StreamObject(...)` at `payment_handler.go:174` and panics. Also, `NewPaymentUsecase` at `main.go:145` passes `cfg.S3Endpoint` as `baseURL` and `nil` for `auditSvc`.
- **Saran Perbaikan (Remediation):**
  1. Di [`backend/cmd/server/main.go:145`](file:///root/galeriku/backend/cmd/server/main.go#L145), ubah menjadi:
     ```go
     publicBaseURL := "http://localhost:" + cfg.AppPort
     paymentHandler = handler.NewPaymentHandler(
         usecase.NewPaymentUsecase(paymentRepo, usecase.StubPaymentGateway{}, cfg.OTPSecret, publicBaseURL, auditSvc),
         s3svc,
     )
     ```
  2. Di [`backend/internal/handler/payment_handler.go:173`](file:///root/galeriku/backend/internal/handler/payment_handler.go#L173), tambahkan guard `if h.s3 == nil` yang mengembalikan `result` JSON jika S3 streamer tidak dikonfigurasi (seperti pada mode unit test tanpa S3).

---

### P0-06: `BatchFinalizeUsecase` Permanently Holds `NoopPublisher` Due to Initialization Order in `cmd/server/main.go`
- **File:** [`backend/cmd/server/main.go:51,119-121,132-139`](file:///root/galeriku/backend/cmd/server/main.go#L119-L139)
- **Root Cause:** `publisher` (`messaging.NoopPublisher{}`) is passed by value to `usecase.NewBatchFinalizeUsecase` at line 120 **before** `rabbitPub` is initialized and assigned to `publisher` at line 138.
- **Saran Perbaikan (Remediation):**
  - Pindahkan blok inisialisasi `messaging.NewRabbitPublisher(cfg.RabbitURL, cfg.RabbitExchange)` ke **sebelum** baris `usecase.NewBatchFinalizeUsecase(uploadRepo, s3svc, publisher, auditSvc)`, dan tambahkan `defer rabbitPub.Close()` saat shutdown.

---

### P0-07: DEC-Q01 Hi-Res Fulfillment Bypass in `GrantDownload` & Unverified S3 Object Key in `UploadMaster`
- **File:** [`backend/internal/usecase/payment_usecase.go:179-272`](file:///root/galeriku/backend/internal/usecase/payment_usecase.go#L179-L272)
- **Root Cause:**
  1. `GrantDownload` does not verify `item.FulfillmentStatus == domain.FulfillmentStatusFulfilled` when `assetType == domain.DownloadGrantTypeHiRes`.
  2. `UploadMaster` ignores `objectKey` and `checksum` and has `_ = errors.New("SLA breached but upload allowed")` at line 259.
- **Saran Perbaikan (Remediation):**
  1. Di `GrantDownload`, jika `assetType == domain.DownloadGrantTypeHiRes` dan `item.FulfillmentStatus != domain.FulfillmentStatusFulfilled`, kembalikan error `ErrHiResNotReady` (`409 CONFLICT` atau `422 UNPROCESSABLE_ENTITY`).
  2. Di `UploadMaster`, tolak jika `now.After(job.HiresDueAt)` dengan `return nil, repository.ErrSLABreached`, dan simpan metadata asset Hi-Res ke `photo_assets`.

---

### P0-08: AI Worker `create_photo_event_link` Fails with PostgreSQL `NOT NULL` Violation
- **Files:** [`ai-worker/db.py:98-108`](file:///root/galeriku/ai-worker/db.py#L98-L108), [`migrations/000001_initial_schema.up.sql:271-278`](file:///root/galeriku/migrations/000001_initial_schema.up.sql#L271-L278)
- **Root Cause:** `photo_event_links` defines `status VARCHAR(64) NOT NULL` and `linked_at TIMESTAMPTZ NOT NULL` with no default values, while `create_photo_event_link` only inserts `(photo_id, event_id, linked_by)`.
- **Saran Perbaikan (Remediation):**
  - Ubah query di [`ai-worker/db.py:99-107`](file:///root/galeriku/ai-worker/db.py#L99-L107) menjadi:
    ```sql
    INSERT INTO photo_event_links (photo_id, event_id, linked_by, status, linked_at)
    VALUES (:photo, :event, :user, 'ACTIVE', CURRENT_TIMESTAMP)
    ON CONFLICT (photo_id, event_id) DO NOTHING
    ```

---

### P0-09: S3 Bucket Split-Brain Between AI Worker (`galeriku-public-media`) and Go Backend (`galeriku-master-vault`)
- **Files:**
  - [`ai-worker/main.py:97-100`](file:///root/galeriku/ai-worker/main.py#L97-L100)
  - [`backend/internal/storage/s3_service.go:22,129-161`](file:///root/galeriku/backend/internal/storage/s3_service.go#L129-L161)
  - [`backend/scripts/seed_demo_feed.sh:54`](file:///root/galeriku/backend/scripts/seed_demo_feed.sh#L54)
- **Root Cause:** `ai-worker/main.py:97-100` uploads watermarked previews (`previews/{photo_id}.jpg`) to `galeriku-public-media`, whereas Go's `S3Service.PresignPreviewGet` and `StreamObject` read from `BucketMasterVault` (`galeriku-master-vault`).
- **Saran Perbaikan (Remediation):**
  - Di [`ai-worker/main.py:97-100`](file:///root/galeriku/ai-worker/main.py#L97-L100), unggah preview ke `cfg.S3_MASTER_BUCKET` (`galeriku-master-vault`) dengan key `previews/{photo_id}.jpg` (dan opsional salin juga ke `galeriku-public-media` jika diperlukan akses publik langsung), sehingga `PresignPreviewGet` dan `ServeDownload` di Go selalu menemukan objek tersebut.
  - Di [`backend/scripts/seed_demo_feed.sh:54`](file:///root/galeriku/backend/scripts/seed_demo_feed.sh#L54), perbaiki bucket tujuan menjadi `local/galeriku-master-vault/previews/${PID}.jpg`.

---

### P0-10: AI Worker Never Populates `photo_bib_tags`, `photo_prices`, or `photo_assets`
- **Files:**
  - [`ai-worker/main.py:128-159`](file:///root/galeriku/ai-worker/main.py#L128-L159)
  - [`ai-worker/db.py:73-128`](file:///root/galeriku/ai-worker/db.py#L73-L128)
  - [`backend/internal/repository/upload_repository.go:235-256`](file:///root/galeriku/backend/internal/repository/upload_repository.go#L235-L256)
- **Root Cause:**
  1. `ai-worker` only writes BIB OCR output to `ai_job_results`, never inserting into `photo_bib_tags` (which `SearchPhotosByBIB` queries).
  2. Neither `upload_repository.go` nor `ai-worker` inserts a default row into `photo_prices`, causing `AddToCart` to fail with `404 ErrPriceNotFound`.
- **Saran Perbaikan (Remediation):**
  1. Tambahkan `JobDB.insert_bib_tag(photo_id, bib_raw, bib_normalized, confidence)` di [`ai-worker/db.py`](file:///root/galeriku/ai-worker/db.py) yang menulis ke tabel `photo_bib_tags` (`status = 'CONFIRMED'`), dan panggil dari `process_one()` di `ai-worker/main.py`.
  2. Tambahkan `JobDB.ensure_default_photo_price(photo_id, price_idr=50000)` di `ai-worker/db.py` (serta saat finalisasi upload di `upload_repository.go`) agar setiap foto yang siap (`READY`) memiliki harga default aktif di `photo_prices`.

---

### P0-11: AI Worker DB Poller Fallback (`poll_pending_jobs`) Synthesizes a Non-Existent S3 Key
- **File:** [`ai-worker/main.py:196-229`](file:///root/galeriku/ai-worker/main.py#L196-L229)
- **Root Cause:** `poll_pending_jobs()` fabricates `object_key = f"raw-uploads/placeholder/{_batch_id}/{_item_id}.jpg"` instead of querying `upload_items.object_key`.
- **Saran Perbaikan (Remediation):**
  - Di [`ai-worker/db.py:52-63`](file:///root/galeriku/ai-worker/db.py#L52-L63), ubah `fetch_pending_jobs` agar melakukan `JOIN upload_items ui ON ui.id = aj.upload_item_id JOIN upload_batches ub ON ub.id = aj.upload_batch_id` dan mengembalikan `ui.object_key`, `ub.owner_user_id`, dan `ub.event_id`.

---

### P0-12: Web WCAG AA Contrast Failures (`R-25` Hard Gate) on `.btn-primary`, `.pill-nav a.active`, `.price-tag`, and OTP Badge
- **Files:**
  - [`web/src/layouts/Base.astro:166,288,332`](file:///root/galeriku/web/src/layouts/Base.astro#L166)
  - [`web/src/pages/search.astro:74`](file:///root/galeriku/web/src/pages/search.astro#L74)
- **Root Cause:**
  - `.btn-primary` (`Base.astro:288`) and `.pill-nav a.active` (`Base.astro:166`) use `color: var(--color-text)` (`#0F172A`) on `background: var(--color-primary)` (`#4F46E5`), giving **2.84:1** contrast (fails 4.5:1 AA).
  - `.price-tag` (`Base.astro:332`) uses `color: var(--color-text)` (`#0F172A`) on `background: rgba(0, 0, 0, 0.7)` (**~1.2:1**).
  - `search.astro:74` uses `color: var(--color-accent)` (`#F59E0B`) on `#F8FAFC` (**2.05:1**).
- **Saran Perbaikan (Remediation):**
  - Ubah `color: var(--color-text)` menjadi `color: #FFFFFF` pada `.pill-nav a.active`, `.btn-primary`, dan `.price-tag` di [`web/src/layouts/Base.astro`](file:///root/galeriku/web/src/layouts/Base.astro) (**6.29:1** dan **>12:1**, lulus WCAG AA).
  - Ubah warna teks badge OTP di [`web/src/pages/search.astro:74`](file:///root/galeriku/web/src/pages/search.astro#L74) menjadi `#B45309` (amber-700, **5.02:1** di atas `#F8FAFC`).

---

### P0-13: `search.astro` Results Hidden on Mobile/Tablet (`< 1024px`) and Dead Elements on Desktop (`R-26`, `R-35`)
- **Files:**
  - [`web/src/layouts/Base.astro:264`](file:///root/galeriku/web/src/layouts/Base.astro#L264)
  - [`web/src/pages/search.astro:67-78`](file:///root/galeriku/web/src/pages/search.astro#L67-L78)
- **Root Cause:** `Base.astro:264` hides `.photo-grid` globally on `< 1024px` (`@media (max-width: 1023px) { .photo-grid { display: none; } }`), which hides `#results-grid.photo-grid` on `/search/`. Additionally, search result cards have no preview image and no click/add-to-cart action.
- **Saran Perbaikan (Remediation):**
  1. Di [`web/src/layouts/Base.astro:264`](file:///root/galeriku/web/src/layouts/Base.astro#L264), batasi aturan responsif hanya untuk `.home-desktop-grid` (bukan semua `.photo-grid`) agar `.photo-grid` pada halaman `/search/` dan `/collection/` tetap tampil responsif di mobile.
  2. Di [`web/src/pages/search.astro:67-78`](file:///root/galeriku/web/src/pages/search.astro#L67-L78), muat gambar preview dari `/api/v1/photos/${p.photo_id}/preview-url` dan tambahkan tombol interaktif `+ Keranjang` pada setiap kartu hasil pencarian.

---

### P0-14: Astro Vite Proxy Port Mismatch (`127.0.0.1:6666` vs `:8080`)
- **File:** [`web/astro.config.mjs:18`](file:///root/galeriku/web/astro.config.mjs#L18)
- **Root Cause:** `astro.config.mjs:18` proxies `/api` to `http://127.0.0.1:6666`, while the Go backend defaults to `:8080` ([`config.go:39`](file:///root/galeriku/backend/internal/config/config.go#L39)).
- **Saran Perbaikan (Remediation):**
  - Gunakan `process.env.PUBLIC_API_ORIGIN || 'http://127.0.0.1:8080'` sebagai target proxy `/api` di [`web/astro.config.mjs:18`](file:///root/galeriku/web/astro.config.mjs#L18), dan selaraskan port dev di `web/package.json:7`.

---

### P0-15: Mobile Bottom Nav "Akun" Tab Redirect Loop Traps Logged-In Mobile Web Users
- **Files:**
  - [`web/src/layouts/Base.astro:84-87,98-122`](file:///root/galeriku/web/src/layouts/Base.astro#L84-L122)
  - [`web/src/pages/login.astro:55-61`](file:///root/galeriku/web/src/pages/login.astro#L55-L61)
- **Root Cause:** On mobile viewports (`< 1024px`), `.top-bar .user-actions` is hidden, while `#bottom-nav-account` points to `/login/`, which immediately bounces logged-in users back to `/`.
- **Saran Perbaikan (Remediation):**
  - Di `updateAuthUI()` pada [`web/src/layouts/Base.astro:98-122`](file:///root/galeriku/web/src/layouts/Base.astro#L98-L122), ketika token ditemukan di `localStorage`, perbarui juga `#bottom-nav-account` agar menampilkan label `Keluar` dan menjalankan fungsi logout saat diketuk di layar mobile.

---

### P0-16: Flutter Mobile App Mounts `ReelsFeedScreen` on Both "Galeri" and "Cari" Tabs (`FeedScreen` Is Unreachable)
- **Files:**
  - [`mobile/lib/main.dart:70-76`](file:///root/galeriku/mobile/lib/main.dart#L70-L76)
  - [`mobile/lib/screens/feed_screen.dart:8-218`](file:///root/galeriku/mobile/lib/screens/feed_screen.dart#L8-L218)
- **Root Cause:** `HomeShell._tabs` mounts `ReelsFeedScreen()` at both index 0 (`Galeri`) and index 1 (`Cari`). `FeedScreen` (containing BIB search) is never mounted anywhere.
- **Saran Perbaikan (Remediation):**
  - Impor `screens/feed_screen.dart` di [`mobile/lib/main.dart`](file:///root/galeriku/mobile/lib/main.dart) dan pasang `FeedScreen()` pada slot ke-2 (`Cari`) di `_tabs`:
    ```dart
    final _tabs = const [
      ReelsFeedScreen(),
      FeedScreen(),
      CartTab(),
      QrisScreen(),
      ProfileScreen(),
    ];
    ```
  - Tambahkan pemanggilan `GET /api/v1/photos/{id}/preview-url` di `mobile/lib/src/api.dart` agar kartu foto merender gambar preview asli dengan `Image.network`.

---

## 3. High Severity Findings (P1) & Remediation Proposals

| ID | Component & Location | Issue Description | Concrete Fix |
|---|---|---|---|
| **P1-01** | [`ledger_repository.go:194-208`](file:///root/galeriku/backend/internal/repository/ledger_repository.go#L194-L208) | `INSERT INTO ledger_accounts` inside `tx` without `ON CONFLICT DO NOTHING` aborts the PostgreSQL transaction (`SQLSTATE 25P02`) on concurrent account creation. | Add `ON CONFLICT DO NOTHING` to `INSERT INTO ledger_accounts` before falling back to `SELECT id FROM ledger_accounts`. |
| **P1-02** | [`ledger_repository.go:217`](file:///root/galeriku/backend/internal/repository/ledger_repository.go#L217), [`finance_usecase.go:108-111`](file:///root/galeriku/backend/internal/usecase/finance_usecase.go#L108-L111), [`payment_usecase.go:156-175`](file:///root/galeriku/backend/internal/usecase/payment_usecase.go#L156-L175) | `GetAccountBalance` computes `DEBIT - CREDIT`, returning negative numbers (`-50000`) for credited wallets/payables; `RecordSale` swallows ledger errors; `HandleWebhook` never triggers `RecordSale`. | Compute `CREDIT - DEBIT` for liability/payable/revenue accounts (or pass account normal balance direction), propagate non-duplicate errors in `RecordSale`, and invoke `RecordSale` upon payment settlement. |
| **P1-03** | [`payment_usecase.go:122,157-158`](file:///root/galeriku/backend/internal/usecase/payment_usecase.go#L122), [`payment_repository.go:148`](file:///root/galeriku/backend/internal/repository/payment_repository.go#L148) | `InitiatePayment` truncates `order.ID` to 12 chars (`"GLK-" + order.ID.String()[:12]`), which fails `uuid.Parse` in `HandleWebhook`. Also `RecordPaymentEvent` lacks `ON CONFLICT (provider_event_id) DO NOTHING`. | Send full `"GLK-" + order.ID.String()` (or look up by `order_number`), use deterministic `uuid.NewSHA1(uuid.NameSpaceOID, []byte(event.TransactionID))` if `TransactionID` is not already a UUID, and add `ON CONFLICT (provider_event_id) DO NOTHING`. |
| **P1-04** | [`discovery_usecase.go:197-227`](file:///root/galeriku/backend/internal/usecase/discovery_usecase.go#L197-L227), [`checkout_otp.go:55`](file:///root/galeriku/backend/internal/security/checkout_otp.go#L55) | `ConfirmVerification` lacks caller `userID` ownership check and never marks challenges `VERIFIED`; `o.sinks` in `checkout_otp.go` leaks plaintext OTPs unboundedly. | Pass authenticated `userID` into `ConfirmVerification`, verify `verification.UserID == userID` and `challenge.Status == "SENT"`, mark `VERIFIED` in `CreateGrant`, and delete consumed sink entries. |
| **P1-05** | [`router.go:336-354`](file:///root/galeriku/backend/internal/handler/router.go#L336-L354), [`stats_handler.go:53-119`](file:///root/galeriku/backend/internal/handler/stats_handler.go#L53-L119) | `HostStats` leaks global pending host application counts to any user; `CreatorStats` lacks `CREATOR` capability check; `AdminStats` and `ReleaseSettlements` omit `WithLiveCapabilityCheck`. | Guard `/api/v1/stats/creator`, `/api/v1/stats/host`, `/api/v1/stats/admin`, and `/api/v1/finance/settlements/release` with `middleware.RequireCapability(..., middleware.WithLiveCapabilityCheck(cfg.TenantRepo))`. Scope `HostStats` strictly to the caller's organizations. |
| **P1-06** | [`rabbit_publisher.go:17-25`](file:///root/galeriku/backend/internal/messaging/rabbit_publisher.go#L17-L25), [`batch_finalize_usecase.go:75-85`](file:///root/galeriku/backend/internal/usecase/batch_finalize_usecase.go#L75-L85), [`ai-worker/main.py:115-128`](file:///root/galeriku/ai-worker/main.py#L115-L128) | `PhotoIngestEvent` omits `UploaderUserID` and passes batch total `ActualBytes` instead of per-item bytes. | Add `UploaderUserID uuid.UUID` (`json:"uploader_user_id"`) to `PhotoIngestEvent` and pass per-photo `ActualBytes`. |
| **P1-07** | [`ai-worker/main.py:138`](file:///root/galeriku/ai-worker/main.py#L138), [`ai-worker/face_index.py:36-54`](file:///root/galeriku/ai-worker/face_index.py#L36-L54) | AI worker sets Qdrant `subject_id` to `uploader` (photographer ID) instead of generating a biometric subject reference. | Generate a distinct biometric subject/vector ref per detected face and record `face_vector_refs` in PostgreSQL. |
| **P1-08** | [`docker-compose.yml:123-143`](file:///root/galeriku/docker-compose.yml#L123-L143), [`ai-worker/main.py:80-84`](file:///root/galeriku/ai-worker/main.py#L80-L84) | `ai-worker` container in `docker-compose.yml` lacks `DB_*` env vars; `main.py` calls `asyncio.run()` repeatedly on a cached `AsyncQdrantClient`. | Add `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` to `ai-worker` in `docker-compose.yml`, and use synchronous `QdrantClient` in `FaceIndex` (or a single persistent event loop). |
| **P1-09** | [`web/src/pages/studio.astro:14`](file:///root/galeriku/web/src/pages/studio.astro#L14), [`web/src/pages/collection.astro:11-23`](file:///root/galeriku/web/src/pages/collection.astro#L11-L23) | `studio.astro` CTA links to `/login/` (redirect loop to `/`); `collection.astro` is a static empty stub (`R-26`). | Replace `/login/` CTA in `studio.astro` with an interactive upload batch initiation form, and connect `collection.astro` to fetch user orders/downloads. |
| **P1-10** | [`web/src/pages/index.astro:245-250`](file:///root/galeriku/web/src/pages/index.astro#L245-L250), [`web/src/layouts/Base.astro:121-122`](file:///root/galeriku/web/src/layouts/Base.astro#L121-L122) | Registering `astro:page-load` AND calling `loadCatalog()` / `updateAuthUI()` at top level executes them twice on initial load and across other pages. | Guard `loadCatalog()` so it only runs when `#reels-feed` or `#desktop-grid` exists in the current DOM, and rely solely on `astro:page-load`. |
| **P1-11** | [`web/src/layouts/Base.astro:242,308`](file:///root/galeriku/web/src/layouts/Base.astro#L242) | `.follow-btn` has `min-height: 32px` (violates `R-03` 44px minimum) and `.form-field input:focus` sets `outline: none` without `:focus-visible` ring (`R-32`). | Set `.follow-btn { min-height: 44px; padding: 8px 16px; }` and add `outline: 2px solid var(--color-primary); outline-offset: 2px;` on `:focus-visible`. |
| **P1-12** | [`mobile/lib/screens/profile_screen.dart:80-100,145-163`](file:///root/galeriku/mobile/lib/screens/profile_screen.dart#L80-L163) | `ProfileScreen` hardcodes `'Akun Galeriku'` / `'Pembeli | Kreator | Host'` (`R-38`) and has 3 dead `onTap: () {}` tiles (`R-26`). | Fetch real profile via `api.me()` in `AppState` and wire the action tiles to informative dialogs/screens. |
| **P1-13** | [`mobile/lib/main.dart:210-219`](file:///root/galeriku/mobile/lib/main.dart#L210-L219), [`mobile/lib/screens/qris_screen.dart:102`](file:///root/galeriku/mobile/lib/screens/qris_screen.dart#L102) | Both `CartTab` (`Pesan`) and `QrisScreen` (`startCheckout`) call `state.createOrder()`, creating duplicate orders for the same cart. | Have `CartTab`'s `Pesan` button create the order (if not already created for current cart) or navigate to the `Bayar` tab (`QrisScreen`), and have `QrisScreen` reuse `state.lastOrder` if already created. |
| **P1-14** | [`mobile/lib/screens/reels_feed_screen.dart:61-75,260-273`](file:///root/galeriku/mobile/lib/screens/reels_feed_screen.dart#L61-L75) | `_loadCatalog` only catches `on ApiError` (leaving infinite spinner on `SocketException` / `ClientException`, violating `R-27`), and `Ikuti` button is ~22dp tall (`R-03`). | Add generic `catch (e)` fallback in `_loadCatalog` (both `reels_feed_screen.dart` and `feed_screen.dart`) and enforce `constraints: const BoxConstraints(minHeight: 44, minWidth: 44)` on `Ikuti`. |
| **P1-15** | [`docs/GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/docs/GALERIKU_OPENAPI_v1_DRAFT.yaml), [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml) | 30 routes in `router.go` are missing from OpenAPI; 10 OpenAPI paths mismatch `router.go`; duplicate YAML exists at root and `docs/`. | Synchronize all 57 `router.go` routes into `docs/GALERIKU_OPENAPI_v1_DRAFT.yaml` and keep root YAML as a symlink or synced copy. |
| **P1-16** | [`web/scripts/lint-copy.js:14`](file:///root/galeriku/web/scripts/lint-copy.js#L14) | `VALID_EXTENSIONS` omits `.astro` and `.css`; 61 lines in `docs/` and `migrations/` contain literal em dashes (`U+2014`). | Add `'.astro'` and `'.css'` to `VALID_EXTENSIONS` in `lint-copy.js`, and replace all literal em dashes in `docs/` (excluding `docs/PRD.md` per non-negotiable preservation rule) and `migrations/`. |

---

## 4. Medium & Low Severity Findings (P2 / P3) & Remediation Proposals

1. **P2-01 (Unique Event Slug Migration):** Create `migrations/000006_events_unique_slug.up.sql` adding `CREATE UNIQUE INDEX IF NOT EXISTS uq_events_org_slug ON events (host_organization_id, slug);` (and corresponding `.down.sql`).
2. **P2-02 (SQL Scan & Order Status Audit Fixes):**
   - In [`discovery_repository.go:407-410`](file:///root/galeriku/backend/internal/repository/discovery_repository.go#L407-L410) and [`payment_repository.go:68-71`](file:///root/galeriku/backend/internal/repository/payment_repository.go#L68-L71), scan `created_at, updated_at` into `&order.CreatedAt, &order.UpdatedAt` instead of scanning nullable `placed_at` into `&order.CreatedAt`.
   - In [`payment_repository.go:214-217`](file:///root/galeriku/backend/internal/repository/payment_repository.go#L214-L217), include `event_id` in `GetOrderItem` `SELECT` and `Scan`.
   - In [`payment_repository.go:127-137`](file:///root/galeriku/backend/internal/repository/payment_repository.go#L127-L137), insert the `order_status_events` row **before** updating `orders.status = 'PAID'` so `from_status` captures `'PENDING_PAYMENT'`.
3. **P2-03 (Capability Re-Grant on Re-Approval/Invitation):** In [`host_membership_repository.go:115-123`](file:///root/galeriku/backend/internal/repository/host_membership_repository.go#L115-L123) and [`invitation_repository.go:201-211`](file:///root/galeriku/backend/internal/repository/invitation_repository.go#L201-L211), update any existing `'REVOKED'` `HOST` capability row to `'ACTIVE'` in addition to `INSERT ... WHERE NOT EXISTS`.
4. **P2-04 (AI Worker DB Engine & Lock Scope):** Cache a single `JobDB` instance in [`ai-worker/main.py`](file:///root/galeriku/ai-worker/main.py) instead of instantiating a new SQLAlchemy engine per message, and atomically mark jobs `'RUNNING'` inside `fetch_pending_jobs()` via `UPDATE ai_jobs SET status = 'RUNNING', started_at = CURRENT_TIMESTAMP WHERE id IN (SELECT id FROM ai_jobs WHERE status = 'PENDING' ORDER BY created_at ASC LIMIT :lim FOR UPDATE SKIP LOCKED) RETURNING ...`.
5. **P2-05 (Self-Contained `seed_demo_feed.sh`):** Fix [`backend/scripts/seed_demo_feed.sh`](file:///root/galeriku/backend/scripts/seed_demo_feed.sh) so it generates valid minimal JPEG bytes inline with Python/Go inside the repo (no `/tmp/seedphotos/gen` dependency), uses the matching file path, and uploads to `local/galeriku-master-vault/previews/${PID}.jpg`.
6. **P2-06 & P2-07 (Documentation Links, `ROADMAP.md`, & `CHANGELOG.md`):**
   - Fix all 48 broken `file:///root/galeriku/<DOC>.md` links in `docs/ROADMAP.md`, `docs/audits/*`, and `docs/plans/*` to point to `file:///root/galeriku/docs/<DOC>.md`.
   - Update [`docs/ROADMAP.md`](file:///root/galeriku/docs/ROADMAP.md) per-task `- **Status:** COMPLETED` fields and baseline metadata.
   - Update [`CHANGELOG.md`](file:///root/galeriku/CHANGELOG.md) to mark Phase 2 `(Completed)` and document commits `11b0f86..eb444d0` plus the public/tenant event tree enhancements.
7. **P2-08 (`Makefile` & Test Targets):** Update [`Makefile`](file:///root/galeriku/Makefile) `.PHONY` and `help` targets, and convert [`ai-worker/test_worker.py`](file:///root/galeriku/ai-worker/test_worker.py) to include a `unittest.TestCase` wrapper so `python -m unittest` exits `0`.
8. **P3-01 & P3-02 (`000002` Down Migration & HTTP Server Hardening):** Remove manual `DELETE FROM schema_migrations` and add missing `DROP INDEX IF EXISTS` in [`migrations/000002_identity_and_sessions.down.sql`](file:///root/galeriku/migrations/000002_identity_and_sessions.down.sql); configure `ReadHeaderTimeout: 10 * time.Second`, `ReadTimeout: 30 * time.Second`, `WriteTimeout: 30 * time.Second`, and `IdleTimeout: 60 * time.Second` on `http.Server` in [`backend/cmd/server/main.go:154-157`](file:///root/galeriku/backend/cmd/server/main.go#L154-L157).
