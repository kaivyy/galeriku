# Galeriku: Phase 2 Pre-Flight Audit Report
**Dokumen:** `PHASE_2_PREFLIGHT_AUDIT.md`  
**Tanggal Audit:** 15 Oktober 2026  
**Auditor:** Senior Staff Software Engineer, Application Security Engineer, Database Architect, dan Independent Code Reviewer  
**Status Audit:** COMPLETED (Temuan P0 dan P1 teridentifikasi, Rekomendasi: GO WITH PREREQUISITE FIXES)  
**Referensi:** PRD.md v2.2 Enhanced Master Blueprint, PRD_REVIEW.md, DATABASE_SPEC.md, API_SPEC.md, ARCHITECTURE.md, GALERIKU_API_ARCHITECTURE_SPEC.md, dan repository branch `main`.

---

## 1. Executive Summary

Audit pra-terbang (*pre-flight audit*) ini dilakukan secara menyeluruh terhadap status implementasi Fase 1 (Core Foundation & Infrastructure) sebelum dimulainya pengerjaan Fase 2 (Identity, Authentication & Host B2B Portal).

### Kesimpulan Utama:
1. **Infrastruktur Dasar Berjalan:** Seluruh 5 kontainer dependensi (`postgres`, `redis`, `rabbitmq`, `qdrant`, `minio`) berstatus aktif dan *healthy*. Script pengujian `make test` dan `make test-integration` berjalan dengan status 100% lulus.
2. **Temuan P0 (Blocker Integritas):** Fungsi generator `uuid_generate_v7()` pada PostgreSQL mengalami cacat logika runtime (`ERROR: integer out of range`) saat dieksekusi. Cacat ini tidak terdeteksi oleh pengujian Fase 1 karena pengujian sebelumnya hanya memeriksa keberadaan fungsi di `pg_proc` tanpa pernah memanggil fungsinya atau melakukan `INSERT` data nyata. Karena fungsi ini merupakan `DEFAULT` untuk kolom `id` di hampir seluruh 76 tabel, seluruh operasi `INSERT` yang mengandalkan default UUIDv7 akan gagal total jika tidak diperbaiki.
3. **Temuan P0 (Ketiadaan Skema Autentikasi):** Skema awal 76 tabel tidak memuat kolom hash kata sandi maupun tabel kredensial/identitas autentikasi (`user_credentials`/`auth_identities`) serta tabel manajemen sesi (`sessions`/`refresh_tokens`). Autentikasi pengguna tidak dapat diimplementasikan tanpa penambahan skema tabel ini.
4. **Temuan P1 (Integritas Relasional & Skema):** Seluruh 76 tabel di PostgreSQL saat ini memiliki **0 foreign key constraints** aktif. Tabel `host_bank_accounts` hilang dari DDL awal meskipun kolom `payouts.host_bank_account_id` merujuk kepadanya. Sistem migrasi Go saat ini belum memiliki *migration runner* berpelacak versi (`schema_migrations`).
5. **Rekomendasi Keputusan:** **GO WITH PREREQUISITE FIXES**. Implementasi fitur Fase 2 tidak boleh dimulai sebelum perbaikan fungsi `uuid_generate_v7()` dan migrasi skema pendukung identitas/sesi (`000002_identity_and_sessions.up.sql`) diterapkan.

---

## 2. Status Repositori dan Git

Hasil inspeksi langsung terhadap repositori Git lokal pada tanggal 15 Oktober 2026:

- **Current Working Directory:** `/root/galeriku`
- **Repository Root:** `/root/galeriku`
- **Current Branch:** `main` (Branch default sesuai konvensi GitHub)
- **Git Status (`git status --short`):**
  ```
   M graphify-out/cache/stat-index.json
   M graphify-out/manifest.json
  ```
  *Catatan:* Tidak ada modifikasi pada kode sumber atau konfigurasi produksi; dua file termodifikasi di atas murni merupakan cache AST indeks pengetahuan lokal yang diperbarui oleh perintah `graphify update .`.
- **Riwayat Commit Terakhir (`git log -n 5 --oneline`):**
  ```
  7c215d9 (HEAD -> main) fix(core): resolve final code review findings for phase 1
  d125b18 fix(web): add antislop lint-copy script and update Makefile test targets
  15ebbce feat(web): establish typescript workspace and antislop design system tokens
  3853aaf feat(middleware): implement request correlation ID middleware
  1ec656e fix(health): return 503 degraded on database failure and add short test guard
  ```
- **Struktur Direktori Repositori:**
  - `ai-worker/`: Worker Python untuk pipeline AI (InsightFace ArcFace, EasyOCR/PaddleOCR).
  - `backend/`: Go 1.22 modular monolith (`cmd/server`, `cmd/provisioner`, `internal/config`, `internal/database`, `internal/domain`, `internal/handler`, `internal/middleware`).
  - `docs/`: Spesifikasi produk, arsitektur, basis data, API, benchmark FotoYu, rencana kerja superpowers, dan direktori audit.
  - `migrations/`: DDL skrip SQL (`000001_initial_schema.up.sql`, `000001_initial_schema.down.sql`).
  - `web/`: Monorepo TypeScript frontend dengan token desain Antislop dan Vitest test runner.
  - `docker-compose.yml`, `Makefile`, `.env`, `.env.example`, `.gitignore`.

---

## 3. Verifikasi Klaim Fase 1

Verifikasi faktual dilakukan dengan mengeksekusi pemeriksaan non-destruktif terhadap seluruh komponen yang dibangun pada Fase 1.

### A. Infrastruktur Docker Compose
- **Hasil Eksekusi `docker compose ps`:**
  | Layanan | Image | Status Kontainer | Port Mapping Host | Keterangan |
  | :--- | :--- | :--- | :--- | :--- |
  | `postgres` | `postgres:16-alpine` | Up (healthy) | `0.0.0.0:5433->5432` | Mapping port host dialihkan ke 5433 untuk isolasi port lokal. |
  | `redis` | `redis:7-alpine` | Up (healthy) | `0.0.0.0:6380->6379` | Mapping port host dialihkan ke 6380. |
  | `rabbitmq` | `rabbitmq:3.13-management-alpine` | Up (healthy) | `5672->5672`, `15672->15672` | Broker AMQP dan Web UI aktif. |
  | `qdrant` | `qdrant/qdrant:v1.11.0` | Up (healthy) | `6333->6333`, `6334->6334` | Healthcheck socket TCP aktif. |
  | `minio` | `alpine/minio:latest-release` | Up (healthy) | `9010->9000`, `9011->9001` | S3 API dan Console aktif. |
- **Isolasi Jaringan:** Seluruh kontainer terhubung pada jaringan default Docker bridge internal. Akses host terikat pada port yang didefinisikan dalam `.env`.
- **Kredensial:** Seluruh kredensial tersimpan dalam `.env` (git-ignored) dan tidak diekspos ke publik.

### B. Object Storage (MinIO S3)
- **Bucket Provisioner:** Dikelola oleh [`backend/cmd/provisioner/main.go`](file:///root/galeriku/backend/cmd/provisioner/main.go).
- **Verifikasi Kebijakan Bucket:**
  1. `galeriku-master-vault`: Tidak memiliki policy publik. Seluruh akses anonim ditolak secara default oleh MinIO (private storage).
  2. `galeriku-public-media`: Diterapkan policy publik `s3:GetObject` untuk principal `*` pada resource `arn:aws:s3:::galeriku-public-media/*`.
- **Analisis Risiko:** Dokumen legalitas Host (KTP, NPWP, NIB) dan master foto resolusi tinggi **wajib** diarahkan ke `galeriku-master-vault`. Backend harus mencegah penerbitan presigned URL upload untuk aset privat ke bucket `galeriku-public-media`.

### C. Database & Migrations
- **Verifikasi Jumlah Tabel:** Kueri `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'` menghasilkan tepat **76 tabel**.
- **Presisi Finansial:**
  - 17 kolom nominal IDR (misal: `ledger_entries.amount_idr`, `orders.total_idr`, `refund_items.eligible_amount_idr`) 100% menggunakan tipe data `bigint`. Tidak ditemukan tipe data `float` atau `double precision` untuk mata uang.
  - Kolom bagi hasil porsi komisi (`host_fee_rule_versions.fee_rate_bps`, `order_revenue_splits.rate_bps`) menggunakan tipe data `integer` (basis points / bps).
- **Pengujian Eksekusi UUIDv7 (TEMUAN KRITIS P0):**
  - Perintah eksekusi:
    ```sql
    SELECT uuid_generate_v7();
    ```
  - Hasil aktual:
    ```
    ERROR:  integer out of range
    CONTEXT:  PL/pgSQL function generate_uuid_v7() line 14 at assignment
    PL/pgSQL function uuid_generate_v7() line 3 at RETURN
    ```
  - **Akar Masalah:** Pada fungsi `generate_uuid_v7()` di baris 27-28 [`migrations/000001_initial_schema.up.sql`](file:///root/galeriku/migrations/000001_initial_schema.up.sql#L27-L28):
    ```sql
    v_bytes := SET_BYTE(v_bytes, 4, (v_time_ms >> 8)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 5, (v_time_ms)::INTEGER);
    ```
    Variabel `v_time_ms` bertipe `BIGINT` bernilai milidetik epoch (~1,792,000,000,000 pada tahun 2026). Nilai ini jauh melampaui batas maksimum integer 32-bit bertanda di PostgreSQL (`2,147,483,647`). Melakukan *type cast* langsung `(v_time_ms)::INTEGER` dan `(v_time_ms >> 8)::INTEGER` tanpa operasi masking bitwise `& 255` memicu eksepsi `integer out of range`.
  - **Dampak:** Karena `generate_uuid_v7()` adalah nilai `DEFAULT` pada kolom `PRIMARY KEY` di hampir seluruh tabel, kueri `INSERT` tanpa menyuplai ID secara manual akan selalu gagal dengan runtime error.

- **Verifikasi Foreign Key:**
  - Kueri:
    ```sql
    SELECT count(*) FROM information_schema.table_constraints WHERE constraint_type = 'FOREIGN KEY';
    ```
  - Hasil aktual: **0 baris**.
  - **Temuan:** DDL awal tidak mendefinisikan klausa `REFERENCES` antar tabel. Relasi integritas referensial saat ini belum ditegakkan pada tingkat mesin basis data.

- **Verifikasi Pelacakan Migrasi:**
  - Tidak ditemukan tabel pelacak versi migrasi (seperti `schema_migrations`). Skema 000001 diaplikasikan melalui init-script Docker (`/docker-entrypoint-initdb.d/01-init.sql`).

### D. Go Core Backend
- **Connection Pool:** Dikonfigurasi menggunakan `pgxpool` di [`backend/internal/database/postgres.go`](file:///root/galeriku/backend/internal/database/postgres.go) dengan pool batas min 5, max 25, dan timeout ping startup 3 detik. Kredensial di-escape menggunakan `url.QueryEscape` dan mendukung konfigurasi dinamis `DB_SSLMODE`.
- **Health Check Endpoint:** Endpoint `GET /api/v1/health` di [`backend/internal/handler/router.go`](file:///root/galeriku/backend/internal/handler/router.go) mengembalikan:
  - HTTP `200 OK` dengan `"status": "UP"` jika database aktif.
  - HTTP `503 Service Unavailable` dengan `"status": "DEGRADED"` jika database gagal merespons ping dalam 1 detik.
  - HTTP `503 Service Unavailable` dengan `"status": "DOWN"` jika connection pool bernilai `nil`.
- **Correlation ID Middleware:** Middleware [`backend/internal/middleware/request_id.go`](file:///root/galeriku/backend/internal/middleware/request_id.go) membungkus seluruh rute. ID otomatis di-generate dengan format `req_<uuid32>` (panjang tepat 36 karakter dengan entropi penuh 128-bit).
  - *Temuan Keamanan:* Header `X-Request-ID` dari klien belum divalidasi panjang atau karakternya sebelum di-echo kembali ke respons.
- **Graceful Shutdown:** `backend/cmd/server/main.go` menjalankan `http.ListenAndServe` secara sinkron tanpa menangani sinyal sistem operasi (`SIGINT`/`SIGTERM`) via `signal.NotifyContext`.

### E. Web Workspace & Antislop Quality Gates
- **TypeScript & Vitest:** Proyek di `web/` menggunakan TypeScript 5.4, konfigurasi `NodeNext`, dan runner Vitest.
- **Antislop Compliance:**
  - Token visual dideklarasikan pada [`web/src/tokens/design-system.ts`](file:///root/galeriku/web/src/tokens/design-system.ts): `ENERGY: 2`, `RHYTHM: 2`, `MOTION: 1`.
  - Metrik tap target minimum ditetapkan 44px (`minTapTargetPx: 44`) sesuai aturan R-03.
  - Palet warna dibatasi pada 3 warna inti + 1 warna aksen sesuai aturan R-29.
  - Skrip audit copy [`web/scripts/lint-copy.js`](file:///root/galeriku/web/scripts/lint-copy.js) aktif memindai dan menggagalkan build jika terdapat karakter em dash (`—`, `\u2014`, `&mdash;`) sesuai Antislop Hard Gate R-02.
- **Hasil Uji Makefile:**
  - `make test`: Exit code 0 (Backend unit tests, Web Vitest 4/4 pass, `tsc --noEmit` 0 error, `lint-copy` pass).
  - `make test-integration`: Exit code 0 (MinIO live bucket verification, 76-table count verification, Health endpoint pool verification).

---

## 4. Security Review Sebelum Autentikasi (Phase 2 Baseline)

Sebelum membangun identitas pengguna dan portal Host B2B, berikut adalah evaluasi arsitektur keamanan yang harus dipatuhi:

### 4.1 Pemilihan Mekanisme Autentikasi: JWT vs Session
- **Karakteristik Klien Galeriku:** Melayani kanal Web (Astro Storefront, React Creator Studio, Host Portal, Admin Dashboard) dan Mobile App (Flutter Android & iOS).
- **Rekomendasi Arsitektur:** **Dual-Token Pattern (Short-Lived Access JWT + Rotated Persistent Refresh Token)**:
  1. **Access Token (JWT Stateless):**
     - Algoritma: HMAC-SHA256 (`HS256`) dengan kunci minimum 256-bit, atau asymmetric `RS256`/`EdDSA` jika verifikasi terdistribusi ke FastAPI AI worker.
     - Masa berlaku pendek: 15 menit.
     - Payload memuat: `sub` (User ID UUIDv7), `email`, `caps` (array kapabilitas global), `jti` (UUIDv7 identifier).
  2. **Refresh Token (Stateful & Stored):**
     - Opaque cryptographically secure random token (32 bytes / 256-bit hex/base64).
     - Database menyimpan hash token (SHA-256), bukan plaintext.
     - Masa berlaku: 30 hari kalender.
     - Wajib menerapkan **Refresh Token Rotation (RTR)**: Setiap kali refresh token digunakan, token lama dicabut (*revoked*) dan token baru diterbitkan dalam *family ID* yang sama. Jika token yang sudah dicabut digunakan kembali, seluruh *family token* langsung dibatalkan (deteksi pencurian token).
  3. **Penyimpanan di Sisi Klien:**
     - Web Frontends: Disimpan dalam cookie `HttpOnly`, `Secure`, `SameSite=Lax`.
     - Mobile Flutter: Disimpan dalam secure storage native (Android Keystore / EncryptedSharedPreferences, iOS Keychain Services).

### 4.2 Algoritma Hashing Kata Sandi
- **Rekomendasi:** **Argon2id** (`golang.org/x/crypto/argon2`) sesuai standar OWASP Password Storage Cheat Sheet.
- **Parameter Minimum:**
  - Memory: 64 MB (`64 * 1024` KiB)
  - Iterasi / Waktu: 3
  - Paralelisme: 4 threads
  - Salt: 16 bytes kriptografis acak
  - Key length: 32 bytes
- **Aturan Kompleksitas:** Minimum 8 karakter, mengandung setidaknya 1 huruf besar, 1 huruf kecil, 1 angka, dan 1 simbol.

### 4.3 Alur OTP (WhatsApp / SMS) & Pencegahan Brute Force
- Kode OTP 6 digit acak numerik via `crypto/rand`.
- Masa berlaku: 3 menit (180 detik).
- Database/Redis hanya menyimpan hash SHA-256 dari kode OTP, tidak pernah menyimpan kode mentah.
- **Rate Limiting:**
  - Pengiriman OTP: Maksimal 1 pengiriman per nomor per 60 detik; maksimal 3 pengiriman per nomor per 10 menit.
  - Verifikasi OTP: Maksimal 3 kali percobaan gagal per tantangan (*challenge*). Percobaan ke-4 langsung membatalkan tantangan tersebut.

### 4.4 Isolasi Tenant & Pemisahan Hak Akses Host Leader vs Host Admin
- **Host Leader:**
  - Memiliki otoritas hukum penuh atas organisasi Host.
  - Memiliki akses eksklusif ke informasi rekening bank penerima payout (`host_bank_accounts`), pengajuan pencairan saldo (*payout request*), pengaturan aturan bagi hasil (*host fee rules*), dan pengalihan kepemimpinan (*ownership transfer*).
  - Setiap organisasi Host **hanya boleh memiliki tepat 1 Host Leader aktif** (`OPEN-10`).
- **Host Admin:**
  - Hak operasional terdelegasi: membuat event, mengatur sesi FotoTree, mengundang/menyetujui kreator fotografer terakreditasi, dan melihat analitik foto.
  - **Larangan Mutlak:** Host Admin dilarang keras melihat rekening bank organisasi, melihat saldo kas bersih, atau meminta pencairan dana payout.
- **Pencegahan IDOR (Insecure Direct Object Reference / BOLA):**
  - Seluruh query operasional Host pada Core API **wajib** menyertakan filter organisasi dari token sesi pengguna (`WHERE host_organization_id = $context_org_id`), bukan mengandalkan parameter URL yang dikirim oleh klien.

### 4.5 Perlindungan Dokumen Legal Privat
- Dokumen legalitas organisasi (KTP, NPWP, NIB) diunggah langsung ke bucket privat `galeriku-master-vault` dengan path prefix terisolasi: `legal-documents/{host_org_id}/{document_id}.pdf`.
- URL unduh diterbitkan menggunakan Presigned GET URL berbatas waktu singkat (maksimal 5 menit) dan hanya diizinkan untuk Host Leader dari organisasi bersangkutan serta Platform Admin / Verifikator.

---

## 5. Phase 2 Domain Requirements & Mapping Aturan Bisnis

Mengacu pada PRD v2.2, DATABASE_SPEC.md, dan API_SPEC.md:

| Kebutuhan Bisnis | Status Keputusan | Dasar Spesifikasi | Implementasi Database / API yang Diperlukan |
| :--- | :---: | :--- | :--- |
| **Multi-Capability Account** | `DECIDED` | PRD v2.2 Bagian 2.1 | Tabel `users`, `capabilities`, `user_capabilities`. Satu pengguna dapat memiliki capability BUYER, CREATOR, HOST_LEADER, dan PLATFORM_ADMIN sekaligus. |
| **Single Active Host Leader** | `DECIDED` | `DATABASE_SPEC.md` OPEN-10 | Organisasi Host hanya memiliki tepat 1 Host Leader aktif. Perlu partial unique index pada `host_memberships`. |
| **Batas Kuota Host Admin** | `DECIDED` | `DATABASE_SPEC.md` OPEN-11 | Jumlah maksimal admin Host per organisasi dibatasi secara terkonfigurasi pada tabel `host_admin_limits`. |
| **Creator Event Membership** | `DECIDED` | `PRD.md` & `DATABASE_SPEC.md` | Tipe kreator `INTERNAL` vs `INDEPENDENT` melekat pada event via `event_creator_memberships`, bukan properti global user. |
| **Siklus Pengajuan Host B2B** | `DECIDED` | PRD v2.2 Bagian 3.2 | Form `host_applications`, unggah berkas legalitas, peninjauan Super Admin dengan alasan tertulis jika ditolak. |
| **Undangan Anggota Host Terenkripsi** | `DECIDED` | PRD v2.2 & `DATABASE_SPEC.md` | Undangan dikirim via email/tautan dengan token hash berbatas waktu (72 jam) pada `host_invitations`. Penerimaan undangan wajib mengecek kuota admin. |
| **Audit Jejak Keamanan** | `DECIDED` | PRD v2.2 & STRIDE Model | Setiap login gagal, perubahan role, pengubahan rekening bank, dan approval Host wajib dicatat dalam `audit_logs`. |

---

## 6. Gap Analysis Basis Data dan Kontrak API

Berikut adalah perbandingan antara spesifikasi kebutuhan Fase 2 dengan kondisi repositori dan skema saat ini:

```
[Kondisi Saat Ini vs Kebutuhan Fase 2]
1. users table                ──> Belum ada password_hash / user_credentials table
2. sessions / refresh_tokens  ──> Belum ada tabel penyimpanan sesi / refresh token
3. host_bank_accounts         ──> Hilang dari DDL awal (disebut di payouts tapi tabel belum ada)
4. uuid_generate_v7()         ──> Error integer out of range pada pemanggilan fungsi
5. foreign keys               ──> 0 foreign keys pada seluruh 76 tabel di PostgreSQL
6. API Contract               ──> GALERIKU_OPENAPI_v1_DRAFT.yaml belum memuat rute /auth/*
```

### Tabel Rincian Gap:

| No | Komponen / Entitas | Status Saat Ini di Repositori | Kebutuhan Fase 2 | Dampak Risiko | Rekomendasi Solusi Teknis | Butuh Migrasi DB? |
| :---: | :--- | :--- | :--- | :--- | :--- | :---: |
| **G-01** | **Fungsi `uuid_generate_v7()`** | Terpasang di DDL 000001 namun memicu runtime error `integer out of range` pada `v_time_ms >> 8` dan `v_time_ms`. | Generator UUIDv7 harus menghasilkan string UUIDv7 valid tanpa error overflow. | **P0 (Kritis)**: Gagal membuat baris baru di semua tabel yang menggunakan default UUIDv7. | Perbaiki fungsi PL/pgSQL dengan menambahkan masking bitwise `& 255` pada setiap byte. | **Ya** (Migrasi DDL perbaikan fungsi) |
| **G-02** | **Kredensial Kata Sandi Pengguna** | Tabel `users` hanya memiliki kolom identitas (`email_normalized`, `phone_e164`, `account_status`). | Penyimpanan kredensial hash kata sandi yang aman. | **P0 (Kritis)**: Pengguna tidak dapat mendaftar dengan kata sandi atau melakukan verifikasi login. | Buat tabel `user_credentials` terpisah (`user_id`, `password_hash`, `password_algo`, `failed_login_attempts`, `locked_until`). | **Ya** (Tabel baru) |
| **G-03** | **Sesi dan Refresh Token** | Tidak ada tabel sesi atau refresh token di skema 000001. | Mekanisme pencabutan sesi, refresh token rotation (RTR), dan multi-device management. | **P0 (Kritis)**: Refresh token stateless rentan terhadap pencurian token tanpa kemampuan pembatalan (*revocation*). | Buat tabel `auth_refresh_tokens` (`id`, `user_id`, `token_hash`, `family_id`, `is_revoked`, `expires_at`, `created_at`, `user_agent`, `ip_address`). | **Ya** (Tabel baru) |
| **G-04** | **Tabel `host_bank_accounts`** | Kolom `payouts.host_bank_account_id` ada, tetapi tabel `host_bank_accounts` tidak didefinisikan dalam DDL 000001. | Host Leader perlu mendaftarkan rekening bank penerima payout terverifikasi. | **P1 (Tinggi)**: Rekening bank Host tidak dapat disimpan secara terstruktur dan terisolasi dari Host Admin. | Buat tabel `host_bank_accounts` (`id`, `host_organization_id`, `bank_code`, `account_number_encrypted`, `account_holder_name`, `verification_status`, `verified_at`). | **Ya** (Tabel baru) |
| **G-05** | **Tantangan OTP Verifikasi Nomor** | Tabel `verification_challenges` saat ini terikat kolom `checkout_verification_id NOT NULL` (khusus BIB checkout). | Verifikasi nomor ponsel / registrasi akun baru membutuhkan tantangan OTP umum. | **P1 (Tinggi)**: Endpoint `POST /auth/otp/send` tidak dapat menggunakan tabel verifikasi yang ada. | Tambahkan tabel `auth_otp_challenges` (`id`, `phone_e164`, `purpose`, `code_hash`, `attempt_count`, `max_attempts`, `expires_at`, `status`). | **Ya** (Tabel baru) |
| **G-06** | **Integritas Referensial (Foreign Keys)** | 0 foreign keys pada 76 tabel di PostgreSQL. | Konsistensi relasi antar entitas (misal: `user_capabilities.user_id` ke `users.id`). | **P1 (Tinggi)**: Risiko timbulnya *orphan data* jika referensi dihapus atau tidak valid. | Tambahkan foreign key constraints secara bertahap pada domain Identity dan Host. | **Ya** (Constraint DDL) |
| **G-07** | **Integritas Partial Index Host Leader** | Belum ada partial unique index untuk menegakkan aturan single active Host Leader per org. | Memastikan hanya ada 1 Leader aktif per organisasi (`OPEN-10`). | **P1 (Tinggi)**: Risiko inkonsistensi kewenangan jika terdapat lebih dari satu Host Leader aktif. | Tambahkan indeks: `CREATE UNIQUE INDEX uq_active_host_leader ON host_memberships (host_organization_id) WHERE membership_role = 'LEADER' AND status = 'ACTIVE';`. | **Ya** (Index DDL) |
| **G-08** | **OpenAPI Draft Endpoint Auth** | [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml) saat ini hanya mendefinisikan rute checkout, bib search, upload, dan order. | Rute Domain 01 (`/auth/*`, `/hosts/*`) harus sinkron dalam skema OpenAPI 3.1. | **P2 (Sedang)**: Frontend TypeScript tidak dapat men-generate tipe data Auth secara otomatis dari OpenAPI. | Sinkronkan endpoint Domain 01 dari `API_SPEC.md` ke file OpenAPI YAML. | Tidak (Update YAML) |
| **G-09** | **Validasi Header `X-Request-ID`** | Middleware mencerminkan nilai header apa pun tanpa validasi format atau pemotongan panjang. | Header harus divalidasi (panjang maksimal 64 karakter, karakter alfanumerik dan tanda hubung saja). | **P1 (Tinggi)**: Risiko HTTP response splitting dan log injection (CWE-113). | Tambahkan validasi dan pembersihan regex pada `CorrelationIDMiddleware`. | Tidak (Kode Go) |
| **G-10** | **Graceful Shutdown Server** | Server berhenti seketika saat menerima SIGINT/SIGTERM. | Koneksi in-flight dan connection pool harus di-drain secara rapi. | **P1 (Tinggi)**: Pemutusan mendadak transaksi yang sedang berjalan saat restart deployment. | Tambahkan `signal.NotifyContext` dan `server.Shutdown` dengan timeout 10 detik. | Tidak (Kode Go) |

---

## 7. Klasifikasi Temuan Audit (Findings Breakdown)

### P0 — Blocking Security or Integrity Issue (Wajib Diselesaikan Sebelum Kode Fitur Ditulis)
1. **P0-1: Runtime Error Fungsi `uuid_generate_v7()`**
   - *Lokasi:* [`migrations/000001_initial_schema.up.sql:27-28`](file:///root/galeriku/migrations/000001_initial_schema.up.sql#L27-L28)
   - *Bukti Uji:* `SELECT uuid_generate_v7();` menghasilkan `ERROR: integer out of range`.
   - *Dampak:* Seluruh penambahan data baru menggunakan UUID default gagal total.
2. **P0-2: Ketiadaan Tabel Penyimpanan Kredensial & Sesi Autentikasi**
   - *Lokasi:* [`migrations/000001_initial_schema.up.sql:47-54`](file:///root/galeriku/migrations/000001_initial_schema.up.sql#L47-L54)
   - *Bukti:* Tabel `users` tidak memiliki kolom password, dan tidak ada tabel `user_credentials` maupun `auth_refresh_tokens`.
   - *Dampak:* Fitur register dan login tidak memiliki tempat penyimpanan yang aman.

### P1 — Must Resolve Before or During Early Phase 2
1. **P1-1: Ketiadaan Foreign Keys pada Domain Identity & Host**
   - *Bukti:* 0 constraint `FOREIGN KEY` di PostgreSQL `information_schema.table_constraints`.
   - *Dampak:* Risiko *data corruption* dan *orphan record* pada tabel keanggotaan Host dan kapabilitas.
2. **P1-2: Hilangnya Tabel `host_bank_accounts` dari Skema**
   - *Bukti:* DDL 000001 tidak memiliki `CREATE TABLE IF NOT EXISTS host_bank_accounts`, padahal kolom `payouts.host_bank_account_id` merujuk kepadanya.
   - *Dampak:* Pengelolaan rekening bank Host tidak dapat diimplementasikan.
3. **P1-3: Penegakan Aturan Single Active Host Leader via Partial Index**
   - *Bukti:* Indeks `uq_active_host_leader` belum terpasang pada `host_memberships`.
   - *Dampak:* Pelanggaran aturan bisnis `OPEN-10` jika ada 2 user berstatus Leader aktif pada satu organisasi.
4. **P1-4: Skema Tabel OTP Khusus Autentikasi (`auth_otp_challenges`)**
   - *Bukti:* Tabel `verification_challenges` saat ini terikat erat pada kolom `checkout_verification_id`.
   - *Dampak:* Alur OTP login/registrasi via WhatsApp tidak dapat menggunakan tabel verifikasi checkout.
5. **P1-5: Validasi Header `X-Request-ID` dari Klien**
   - *Lokasi:* [`backend/internal/middleware/request_id.go:18-25`](file:///root/galeriku/backend/internal/middleware/request_id.go#L18-L25)
   - *Dampak:* Potensi log injection / response header splitting jika klien mengirim karakter berbahaya.
6. **P1-6: Implementasi Graceful Shutdown pada Go Server**
   - *Lokasi:* [`backend/cmd/server/main.go:34-36`](file:///root/galeriku/backend/cmd/server/main.go#L34-L36)
   - *Dampak:* Pemutusan koneksi HTTP dan PostgreSQL connection pool secara paksa saat deployment.

### P2 — Improvement That Can Be Scheduled Later
1. **P2-1: Konfigurasi Dinamis Nama Bucket S3 Provisioner**
   - Nama bucket saat ini berupa konstanta di provisioner daripada membaca dari `.env`.
2. **P2-2: Sinkronisasi OpenAPI 3.1 untuk Endpoint Auth & Host B2B**
   - [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml) perlu dimutakhirkan dengan skema Domain 01.

---

## 8. Open Product Decisions (Keputusan yang Perlu Dikonfirmasi)

Sebelum implementasi teknis dimulai, keputusan berikut harus dicatat secara transparan:
1. **OP-01: Durasi Kedaluwarsa Sesi Refresh Token Mobile vs Web:**
   - *Usulan:* Web browser: 7 hari kalender; Aplikasi mobile Flutter: 30 hari kalender dengan sliding expiry saat aktif digunakan.
2. **OP-02: Metode Verifikasi Dokumen Legalitas Host:**
   - *Usulan:* Dokumen NIB/KTP di-review manual oleh Super Admin melalui portal admin. Pengajuan ditandai `UNDER_REVIEW`, dan Host Admin belum dapat mempublikasikan event komersial sebelum disetujui (`APPROVED`).
3. **OP-03: Mekanisme Verifikasi Kepemilikan Rekening Bank Host:**
   - *Usulan:* Fase 2 awal mengandalkan pencocokan nama pemilik rekening via Name Inquiries API (Xendit/Midtrans Iris) terhadap nama entitas legal Host, atau validasi dokumen buku tabungan/rekening koran manual.

---

## 9. Rekomendasi Go / No-Go

> [!IMPORTANT]
> **REKOMENDASI: GO WITH PREREQUISITE FIXES**
>
> Pengembangan Fase 2 **DISETUJUI UNTUK DILANJUTKAN** dengan ketentuan bahwa **Task 1 dan Task 2 pada rencana kerja Fase 2 difokuskan untuk memperbaiki bug kritis `uuid_generate_v7()` dan menambahkan migrasi basis data `000002_identity_and_sessions.up.sql`** sebelum kode handler bisnis ditulis.

---

## 10. Keterbatasan Audit & Batasan Verifikasi
- Audit ini bersifat **read-only** pada basis data dan repositori kode sumber sesuai instruksi (tidak memodifikasi schema atau kode selama sesi audit berlangsung).
- Pengujian penetrasi keamanan dinamis (DAST) terhadap alur login belum dapat dijalankan karena endpoint autentikasi belum diimplementasikan.
- Layanan pengiriman OTP WhatsApp pihak ketiga (Fonnte/Waba) belum diintegrasikan dengan API key produksi sehingga pengujian OTP saat ini diverifikasi pada level unit test dan mock interface.
