# Galeriku: Phase 2 Implementation Plan (Master Roadmap)
**Dokumen:** `PHASE_2_IMPLEMENTATION_PLAN.md`  
**Domain:** Identity, Authentication & Host B2B Portal (Sprint 2)  
**Tanggal Dokumen:** 15 Oktober 2026  
**Status:** Approved Technical Implementation Plan (Hasil Rekomendasi Audit Pra-Terbang)  
**Referensi:** PRD.md v2.2 Enhanced Master Blueprint, PHASE_2_PREFLIGHT_AUDIT.md, DATABASE_SPEC.md, API_SPEC.md, ARCHITECTURE.md, dan GALERIKU_API_ARCHITECTURE_SPEC.md.

---

## 1. Ikhtisar & Arsitektur Fase 2

Fase 2 membangun sistem identitas berkonkurensi tinggi, autentikasi berbasis kredensial kriptografis dan OTP, otorisasi multi-kemampuan (*multi-capability RBAC*), serta portal B2B untuk organisasi Host (penyelenggara event).

### Prinsip Utama & Aturan Keamanan Mutlak:
1. **Zero-Trust Server-Side Enforcement:** Hak akses, verifikasi peran, dan isolasi tenant ditentukan 100% di backend Go, tidak pernah mempercayai klaim data dari klien tanpa validasi database/token.
2. **Kriptografi Standar Industri:**
   - Password Hashing: **Argon2id** (Memory: 64MB, Iterations: 3, Parallelism: 4).
   - Access Token: Stateless JWT (HS256) dengan masa aktif pendek (15 menit).
   - Refresh Token: Stateful opaque token dengan rotasi otomatis (**Refresh Token Rotation / RTR**) berumur 30 hari, disimpan dalam bentuk hash SHA-256.
   - OTP: 6-digit numerik acak kriptografis (`crypto/rand`) dengan masa aktif 3 menit, disimpan dalam bentuk hash SHA-256 dan dilindungi rate limiting Redis.
3. **Pemisahan Kewenangan Host Leader vs Host Admin:**
   - Host Leader: Otoritas hukum dan finansial tunggal per organisasi (`OPEN-10`). Mengakses dan mengubah rekening bank penerima payout (`host_bank_accounts`), meminta pencairan dana, dan mengelola keanggotaan.
   - Host Admin: Otoritas operasional terdelegasi (manajemen event dan FotoTree). **Dilarang keras** melihat rekening bank, saldo finansial, atau meminta payout.
4. **Isolasi Dokumen Legalitas Privat:**
   - Dokumen KTP, NPWP, NIB diunggah dan disimpan eksklusif pada bucket privat `galeriku-master-vault`. Akses baca hanya melalui Presigned GET URL bertempo singkat (maksimal 5 menit) untuk Host Leader dan Super Admin.

---

## 2. Matriks Dependensi Tugas Fase 2

```
[Phase 2 Dependency Graph]

Task 2.1: Migrasi Skema 000002 & Perbaikan UUIDv7 (P0)
   │
   ├──> Task 2.2: Primitif Kriptografi (Argon2id, JWT, OTP Generator & Redis Limiter)
   │       │
   │       ├──> Task 2.3: Registrasi, Login & Alur Sesi Refresh Token (RTR)
   │       │       │
   │       │       ├──> Task 2.4: Middleware Otorisasi Multi-Capability & Isolasi Tenant
   │       │       │       │
   │       │       │       ├──> Task 2.5: Alur Pengajuan Host B2B & Dokumen Legalitas Privat
   │       │       │       │       │
   │       │       │       │       ├──> Task 2.6: Keanggotaan Host, Batas Admin & Hak Eksklusif Leader
   │       │       │       │       │       │
   │       │       │       │       │       ├──> Task 2.7: Manajemen Rekening Bank Host (Leader Only)
   │       │       │       │       │       │       │
   │       │       │       │       │       │       └──> Task 2.8: Undangan Tim Host & Token Hash Atomik
   │       │       │       │       │       │               │
   │       │       │       │       │       │               └──> Task 2.9: Audit Logging & Security Test Suite
   │       │       │       │       │       │                       │
   │       │       │       │       │       │                       └──> Task 2.10: OpenAPI 3.1 & Web TS Types Hardening
```

---

## 3. Rincian Teknis per Tugas (Task Specifications)

---

### Task 2.1: Perbaikan Bug Kritis `uuid_generate_v7()` & Migrasi Skema Basis Data 000002

**Deskripsi:**
Memperbaiki cacat overflow integer pada fungsi `generate_uuid_v7()` PostgreSQL yang ditemukan pada audit (P0-1) dan menerapkan migrasi DDL `000002_identity_and_sessions.up.sql` untuk menyediakan tabel kredensial, sesi token, tantangan OTP umum, tabel rekening bank Host yang hilang (P1-2), serta partial unique index untuk single Host Leader (P1-3).

**Dependensi:** Tidak ada.

**File yang Dikerjakan:**
- Create: `migrations/000002_identity_and_sessions.up.sql`
- Create: `migrations/000002_identity_and_sessions.down.sql`
- Create: `backend/internal/database/migration_runner.go`
- Create: `backend/internal/database/migration_runner_test.go`
- Modify: `backend/internal/database/migrator.go`
- Modify: `backend/internal/database/migrator_test.go`

**Perubahan Skema Basis Data:**
1. **Perbaikan `generate_uuid_v7()`:**
   ```sql
   CREATE OR REPLACE FUNCTION generate_uuid_v7() RETURNS UUID AS $$
   DECLARE
       v_time DOUBLE PRECISION;
       v_time_ms BIGINT;
       v_bytes BYTEA;
   BEGIN
       v_time := EXTRACT(EPOCH FROM CLOCK_TIMESTAMP());
       v_time_ms := FLOOR(v_time * 1000)::BIGINT;
       v_bytes := gen_random_bytes(16);
       v_bytes := SET_BYTE(v_bytes, 0, ((v_time_ms >> 40) & 255)::INTEGER);
       v_bytes := SET_BYTE(v_bytes, 1, ((v_time_ms >> 32) & 255)::INTEGER);
       v_bytes := SET_BYTE(v_bytes, 2, ((v_time_ms >> 24) & 255)::INTEGER);
       v_bytes := SET_BYTE(v_bytes, 3, ((v_time_ms >> 16) & 255)::INTEGER);
       v_bytes := SET_BYTE(v_bytes, 4, ((v_time_ms >> 8) & 255)::INTEGER);
       v_bytes := SET_BYTE(v_bytes, 5, (v_time_ms & 255)::INTEGER);
       v_bytes := SET_BYTE(v_bytes, 6, (GET_BYTE(v_bytes, 6) & 15) | 112);
       v_bytes := SET_BYTE(v_bytes, 8, (GET_BYTE(v_bytes, 8) & 63) | 128);
       RETURN ENCODE(v_bytes, 'hex')::UUID;
   END;
   $$ LANGUAGE plpgsql VOLATILE;
   ```
2. **Tabel Baru:**
   - `user_credentials`: Menyimpan `user_id`, `password_hash`, `password_algo`, `failed_attempts`, `locked_until`, `created_at`, `updated_at`.
   - `auth_refresh_tokens`: Menyimpan `id`, `user_id`, `token_hash`, `family_id`, `is_revoked`, `expires_at`, `user_agent`, `ip_address`, `created_at`.
   - `auth_otp_challenges`: Menyimpan `id`, `phone_e164`, `purpose`, `code_hash`, `attempt_count`, `max_attempts`, `expires_at`, `status`, `created_at`.
   - `host_bank_accounts`: Menyimpan `id`, `host_organization_id`, `bank_code`, `account_number_encrypted`, `account_holder_name`, `verification_status`, `verified_at`, `created_at`.
3. **Indeks & Foreign Keys:**
   - `CREATE UNIQUE INDEX uq_active_host_leader ON host_memberships (host_organization_id) WHERE membership_role = 'LEADER' AND status = 'ACTIVE';`
   - Foreign key constraints untuk domain Identity dan Host.

**Kriteria Penerimaan (Acceptance Criteria):**
- Kueri `SELECT uuid_generate_v7();` berhasil menghasilkan string UUIDv7 valid tanpa error runtime.
- Kueri `INSERT INTO users (email_normalized, account_status) VALUES ('test@galeriku.com', 'ACTIVE')` berhasil mengeksekusi dengan default ID.
- Skrip migrasi terpasang dan dapat di-rollback secara simetris tanpa sisa objek.

**Pengujian:**
- Unit Test: Validasi eksekusi migrator runner.
- Integration Test: Eksekusi DDL migrasi pada PostgreSQL dan pengujian integritas tabel baru.
- Security Test: Memastikan tabel kredensial dan rekening bank menolak constraint null.

---

### Task 2.2: Primitif Kriptografi (Argon2id, JWT Service, & OTP Manager)

**Deskripsi:**
Membangun modul inti layanan keamanan kredensial: hashing kata sandi Argon2id, penerbitan dan validasi token JWT (access token), serta pembangkitan kode OTP terenkripsi dengan proteksi rate limit Redis.

**Dependensi:** Task 2.1.

**File yang Dikerjakan:**
- Create: `backend/internal/security/hasher.go`
- Create: `backend/internal/security/hasher_test.go`
- Create: `backend/internal/security/jwt.go`
- Create: `backend/internal/security/jwt_test.go`
- Create: `backend/internal/security/otp.go`
- Create: `backend/internal/security/otp_test.go`

**Kriteria Penerimaan (Acceptance Criteria):**
- `HashPassword(password)` menghasilkan hash Argon2id berformat `$argon2id$v=19$m=65536,t=3,p=4$...`.
- `VerifyPassword(password, hash)` mencocokkan kata sandi secara konstan (*timing attack resistant*).
- `GenerateAccessToken(user, capabilities, ttl)` menghasilkan token JWT valid dengan header, claims `sub`, `caps`, `jti`, dan kedaluwarsa 15 menit.
- `VerifyAccessToken(tokenStr)` mengembalikan `Claims` jika valid, dan error terstruktur jika kedaluwarsa atau signature salah.
- `GenerateOTP(phone, purpose)` menghasilkan 6 digit numerik, menyimpan hash SHA-256 ke basis data, dan menghormati rate limiter Redis (maks 1 kirim / 60s, maks 3 kirim / 10m).

**Pengujian:**
- Unit Test: Pengujian edge cases password pendek/panjang, token kedaluwarsa, tampering signature token, dan invalid OTP.
- Security Test: Pengujian resistance brute-force OTP (percobaan ke-4 otomatis membatalkan challenge).

---

### Task 2.3: Registrasi, Login, dan Alur Sesi Refresh Token (RTR)

**Deskripsi:**
Mengimplementasikan use case dan handler API untuk pendaftaran akun baru, autentikasi login (email/ponsel + kata sandi), pembaruan access token via refresh token rotation, serta logout.

**Dependensi:** Task 2.2.

**File yang Dikerjakan:**
- Create: `backend/internal/domain/auth.go`
- Create: `backend/internal/repository/user_repository.go`
- Create: `backend/internal/repository/auth_repository.go`
- Create: `backend/internal/usecase/auth_usecase.go`
- Create: `backend/internal/handler/auth_handler.go`
- Create: `backend/internal/handler/auth_handler_test.go`
- Modify: `backend/internal/handler/router.go`

**Perubahan API:**
- `POST /api/v1/auth/register` (201 Created)
- `POST /api/v1/auth/login` (200 OK)
- `POST /api/v1/auth/refresh` (200 OK)
- `POST /api/v1/auth/logout` (200 OK)
- `POST /api/v1/auth/otp/send` (200 OK)
- `POST /api/v1/auth/otp/verify` (200 OK)
- `GET /api/v1/auth/me` (200 OK)

**Kriteria Penerimaan (Acceptance Criteria):**
- Registrasi sukses otomatis memberikan kapabilitas dasar `BUYER` pada `user_capabilities`.
- Login mengembalikan `access_token` (15m), `refresh_token` (30d), dan data user.
- Penggunaan refresh token lama yang sudah di-rotasi memicu deteksi pencurian token dan membatalkan seluruh *family session* pengguna terkait.
- Header `X-Request-ID` dan format amplop respons konsisten dengan spesifikasi API.

**Pengujian:**
- Unit Test: Mocking repository untuk usecase login, register, dan token rotation.
- Integration Test: Endpoint test memverifikasi alur penuh registrasi, login, refresh, dan me.
- Security Test: Pengujian token reuse attack dan timing-attack account enumeration.

---

### Task 2.4: Middleware Otorisasi Multi-Capability & Isolasi Tenant

**Deskripsi:**
Membangun middleware otorisasi yang mengekstrak identitas dari Bearer JWT, memvalidasi kapabilitas global pengguna, memuat keanggotaan organisasi Host aktif, dan menginjeksi konteks keamanan ke request handler.

**Dependensi:** Task 2.3.

**File yang Dikerjakan:**
- Create: `backend/internal/middleware/auth_middleware.go`
- Create: `backend/internal/middleware/auth_middleware_test.go`
- Create: `backend/internal/middleware/rbac.go`
- Create: `backend/internal/middleware/rbac_test.go`
- Modify: `backend/internal/middleware/request_id.go` (menambahkan sanitasi regex dan batas panjang 64 karakter)

**Kriteria Penerimaan (Acceptance Criteria):**
- Request tanpa header `Authorization: Bearer <token>` pada endpoint terproteksi ditolak dengan HTTP 401 `AUTH_UNAUTHORIZED`.
- Request dengan token tidak valid atau kedaluwarsa ditolak dengan HTTP 401.
- Middleware `RequireCapability(code)` menolak pengguna yang tidak memiliki kapabilitas yang diminta dengan HTTP 403 `AUTH_FORBIDDEN`.
- Middleware `RequireHostRole(role)` memverifikasi bahwa pengguna memiliki keanggotaan aktif pada organisasi terkait di tabel `host_memberships`.
- Header `X-Request-ID` divalidasi dengan regex `^[a-zA-Z0-9_-]{1,64}$`; nilai invalid otomatis digantikan oleh auto-generated ID.

**Pengujian:**
- Unit Test: Pengujian matriks otorisasi (Buyer mencoba akses Host, Host Admin mencoba akses rute Leader).
- Security Test: Verifikasi header injection dan request splitting resistance.

---

### Task 2.5: Alur Pengajuan Host B2B & Dokumen Legalitas Privat

**Deskripsi:**
Mengembangkan alur pengajuan pendaftaran organisasi Host (`host_applications`), penyimpanan metadata profil organisasi, dan penerbitan Presigned S3 URL untuk pengunggahan dokumen legalitas privat (KTP/NPWP/NIB) ke `galeriku-master-vault`.

**Dependensi:** Task 2.4.

**File yang Dikerjakan:**
- Create: `backend/internal/domain/host.go`
- Create: `backend/internal/repository/host_repository.go`
- Create: `backend/internal/usecase/host_application_usecase.go`
- Create: `backend/internal/handler/host_application_handler.go`
- Create: `backend/internal/handler/host_application_handler_test.go`
- Create: `backend/internal/storage/s3_service.go`

**Perubahan API:**
- `POST /api/v1/hosts/apply` (201 Created)
- `GET /api/v1/hosts/applications/my` (200 OK)
- `POST /api/v1/hosts/documents` (201 Created, mengembalikan Presigned S3 PUT URL)
- `GET /api/v1/hosts/documents/{id}/view` (200 OK, mengembalikan Presigned S3 GET URL bertempo 5 menit)

**Kriteria Penerimaan (Acceptance Criteria):**
- Pengajuan pendaftaran membuat record `host_organizations` (status `SUBMITTED`) dan `host_applications`.
- Pengguna yang mengajukan otomatis terdaftar sebagai pemohon utama.
- Presigned S3 URL upload dokumen legalitas secara ketat dibatasi pada path `legal-documents/{org_id}/*` di bucket privat `galeriku-master-vault`.
- File dokumen legalitas tidak dapat diakses secara publik atau anonim.

**Pengujian:**
- Integration Test: Alur pengajuan organisasi dan penerbitan presigned URL.
- Security Test: Percobaan mengunduh dokumen legalitas organisasi lain ditolak dengan HTTP 403 (IDOR/BOLA prevention).

---

### Task 2.6: Manajemen Keanggotaan Host, Batas Kuota Admin & Hak Eksklusif Leader

**Deskripsi:**
Mengimplementasikan verifikasi approval Host oleh Super Admin, pemberian kapabilitas `HOST_LEADER`, penegakan batas maksimal kuota Host Admin via `host_admin_limits`, serta pemisahan hak akses finansial antara Leader dan Admin.

**Dependensi:** Task 2.5.

**File yang Dikerjakan:**
- Create: `backend/internal/usecase/host_membership_usecase.go`
- Create: `backend/internal/handler/host_membership_handler.go`
- Create: `backend/internal/handler/host_membership_handler_test.go`
- Modify: `backend/internal/repository/host_repository.go`

**Perubahan API:**
- `GET /api/v1/hosts/organization` (200 OK)
- `PATCH /api/v1/hosts/organization` (200 OK, Host Leader only)
- `GET /api/v1/hosts/members` (200 OK, Host Leader & Admin)
- `DELETE /api/v1/hosts/members/{id}` (200 OK, Host Leader only)
- `POST /api/v1/admin/hosts/applications/{id}/approve` (200 OK, Platform Admin only)
- `POST /api/v1/admin/hosts/applications/{id}/reject` (200 OK, Platform Admin only)

**Kriteria Penerimaan (Acceptance Criteria):**
- Approval aplikasi oleh Super Admin secara atomik:
  1. Mengubah status organisasi menjadi `APPROVED`.
  2. Memberikan status `ACTIVE` pada membership pemohon sebagai `LEADER`.
  3. Memberikan kapabilitas global `HOST_LEADER` pada `user_capabilities`.
  4. Menginisialisasi default kuota admin pada `host_admin_limits` (default: 5 admin).
- Partial unique index `uq_active_host_leader` mencegah penambahan Leader aktif kedua.
- Host Admin yang mencoba mengubah detail legal organisasi atau menghapus anggota lain ditolak dengan HTTP 403.

**Pengujian:**
- Integration Test: Persetujuan pengajuan Host dan validasi perubahan status.
- Security Test: Pengujian konkurensi penambahan Leader kedua (wajib ditolak oleh database constraint).

---

### Task 2.7: Manajemen Rekening Bank Host Penerima Payout (Host Leader Only)

**Deskripsi:**
Mengimplementasikan pendaftaran dan pengelolaan rekening bank Host untuk pencairan dana penjualan foto pada tabel `host_bank_accounts`. Informasi rekening bank wajib diisolasi secara ketat dan hanya dapat diakses oleh Host Leader.

**Dependensi:** Task 2.6.

**File yang Dikerjakan:**
- Create: `backend/internal/domain/bank_account.go`
- Create: `backend/internal/repository/bank_account_repository.go`
- Create: `backend/internal/usecase/bank_account_usecase.go`
- Create: `backend/internal/handler/bank_account_handler.go`
- Create: `backend/internal/handler/bank_account_handler_test.go`

**Perubahan API:**
- `GET /api/v1/hosts/bank-accounts` (200 OK, Host Leader only, mengembalikan nomor rekening tersensor/masked)
- `POST /api/v1/hosts/bank-accounts` (201 Created, Host Leader only, mendaftarkan rekening)

**Kriteria Penerimaan (Acceptance Criteria):**
- Nomor rekening bank disimpan dalam bentuk terenkripsi (AES-256-GCM) di basis data.
- Respons API hanya menampilkan nomor rekening yang telah di-masking (contoh: `1234xxxx5678`).
- **Pemeriksaan Otorisasi Keras:** Jika pengguna dengan role `HOST_ADMIN` mencoba memanggil `GET` atau `POST /hosts/bank-accounts`, server mengembalikan HTTP 403 `AUTH_FORBIDDEN` seketika.
- Setiap penambahan atau pengubahan rekening bank mencatat event pada `audit_logs`.

**Pengujian:**
- Unit Test: Enkripsi dan dekripsi nomor rekening bank.
- Security Test: Percobaan akses oleh Host Admin dan non-anggota organisasi menghasilkan 403 Forbidden.

---

### Task 2.8: Undangan Tim Host & Token Hash Atomik

**Deskripsi:**
Mengembangkan sistem undangan anggota tim Host (`host_invitations`) berbatas waktu (72 jam) yang dikirim via email/tautan, disimpan dalam bentuk token hash SHA-256, dan diverifikasi terhadap kuota maksimal admin organisasi.

**Dependensi:** Task 2.6.

**File yang Dikerjakan:**
- Create: `backend/internal/domain/invitation.go`
- Create: `backend/internal/repository/invitation_repository.go`
- Create: `backend/internal/usecase/invitation_usecase.go`
- Create: `backend/internal/handler/invitation_handler.go`
- Create: `backend/internal/handler/invitation_handler_test.go`

**Perubahan API:**
- `POST /api/v1/hosts/invitations` (201 Created, Host Leader only)
- `POST /api/v1/hosts/invitations/accept` (200 OK, Bearer Invited User)
- `DELETE /api/v1/hosts/invitations/{id}` (200 OK, Host Leader only, pembatalan undangan)

**Kriteria Penerimaan (Acceptance Criteria):**
- Penerbitan undangan membuat token acak kriptografis 32-byte; database hanya menyimpan hash SHA-256 dari token tersebut.
- Penerimaan undangan memverifikasi batas `current_admins < max_admins` dari `host_admin_limits` dalam transaksi terisolasi (`SELECT FOR UPDATE`).
- Jika kuota admin telah penuh, penerimaan undangan ditolak dengan HTTP 422 `ADMIN_LIMIT_EXCEEDED`.
- Undangan kedaluwarsa (> 72 jam) otomatis ditolak dengan HTTP 410 `INVITATION_EXPIRED`.
- Pengguna yang menerima undangan otomatis mendapatkan `host_memberships` (role `ADMIN`) dan kapabilitas `HOST_ADMIN`.

**Pengujian:**
- Integration Test: Alur kirim undangan, penerimaan undangan oleh user terdaftar, dan pengecekan kuota.
- Security Test: Pengujian race condition penerimaan undangan melebihi batas kuota (concurrency lock).

---

### Task 2.9: Audit Logging Terstruktur & Security Test Suite

**Deskripsi:**
Mengintegrasikan pencatatan audit log otomatis untuk seluruh mutasi keamanan dan finansial pada tabel `audit_logs`, menambahkan penanganan graceful shutdown pada server Go, serta membangun rangkaian pengujian penetrasi keamanan (Security Regression Suite).

**Dependensi:** Task 2.3 sampai Task 2.8.

**File yang Dikerjakan:**
- Create: `backend/internal/audit/audit_service.go`
- Modify: `backend/cmd/server/main.go` (implementasi `signal.NotifyContext` dan drain timeout 10 detik)
- Create: `backend/test/security/auth_security_test.go`

**Kriteria Penerimaan (Acceptance Criteria):**
- Setiap mutasi: Login gagal, reset password, persetujuan Host, perubahan role keanggotaan, penambahan rekening bank, dan pembatalan sesi wajib menulis ke `audit_logs`.
- Server Go menangani `SIGINT` dan `SIGTERM` secara anggun, menunggu request in-flight selesai, dan menutup pool database sebelum exit dengan kode 0.
- Seluruh rangkaian uji keamanan lulus:
  1. No Account Enumeration on Registration / Password Reset.
  2. No Token Family Reuse on Revoked Refresh Tokens.
  3. No IDOR on Organization Resources across tenants.
  4. No Host Admin access to bank accounts or financial settings.
  5. No Header Injection on `X-Request-ID`.

**Pengujian:**
- Security Test: Menjalankan suite pengujian penetrasi terisolasi.

---

### Task 2.10: Sinkronisasi OpenAPI 3.1 & Penguatan TypeScript Web Types

**Deskripsi:**
Menyinkronkan seluruh kontrak API Domain 01 (Identity, Auth & Host B2B) ke dalam [`GALERIKU_OPENAPI_v1_DRAFT.yaml`](file:///root/galeriku/GALERIKU_OPENAPI_v1_DRAFT.yaml), men-generate tipe data TypeScript untuk web, serta menguji kontrak frontend tanpa melanggar aturan Antislop.

**Dependensi:** Task 2.9.

**File yang Dikerjakan:**
- Modify: `GALERIKU_OPENAPI_v1_DRAFT.yaml`
- Modify: `docs/GALERIKU_OPENAPI_v1_DRAFT.yaml`
- Create: `web/src/types/api.ts` (hasil export/generate OpenAPI)
- Create: `web/test/auth-contracts.test.ts`
- Modify: `Makefile`

**Kriteria Penerimaan (Acceptance Criteria):**
- OpenAPI 3.1 tervalidasi tanpa error skema menggunakan validator OpenAPI.
- Tipe data TypeScript mencerminkan secara presisi seluruh request, response, dan enum kapabilitas backend.
- `make test` dan `make test-integration` berjalan 100% lulus.
- Skrip `make lint-copy` memastikan tidak ada karakter em dash (`—`) pada seluruh kode dan skema baru.

---

## 4. Strategi Pengujian TDD & Standar Review

Setiap tugas wajib mengikuti disiplin TDD 5 langkah:
1. **Red:** Tulis tes yang gagal untuk kasus positif dan kasus negatif (unauthorized/invalid).
2. **Verify Red:** Jalankan `go test -v ./...` dan pastikan tes gagal dengan alasan yang tepat.
3. **Green:** Tulis implementasi minimal untuk membuat tes berhasil.
4. **Verify Green:** Jalankan tes dan pastikan seluruh pengujian lulus 100%.
5. **Refactor & Commit:** Rapikan kode, lakukan self-review, dan buat commit Git berpesan semantik.

### Standar Reviewer Gate (Subagent Review):
- Setiap tugas wajib di-review oleh task reviewer independen sebelum di-merge ke branch kerja.
- Dua vonis wajib: **Spec Compliance** (✅ Spec compliant) dan **Code Quality** (Task quality: Approved).
- Temuan berstatus *Critical* atau *Important* wajib diselesaikan dengan *fix subagent* sebelum melanjutkan ke tugas berikutnya.
