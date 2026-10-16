# Workspace Cleanup + Preview Signing + Wallet/Stats Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rapikan dokumen root (satu sumber kebenaran di docs/), tambahkan preview image signing (foto asli muncul di UI), endpoint statistik role, dan migrasi web storefront ke Astro + TypeScript.

**Architecture:** Tiga pekerjaan independen: (A) reorganisasi dokumen dengan pintu masuk tunggal README.md; (B) endpoint `GET /photos/{id}/preview-url` yang menerbitkan presigned GET MinIO ber-TTL untuk objek preview; (C) proyek Astro TS di web/ yang menggantikan vanilla SPA, memakai `GalerikuClient` TypeScript yang sudah ada. Rencana mockup tidak dibutuhkan: tokens mengikuti web/src/tokens/design-system.ts yang terkunci.

**Tech Stack:** Go (chi-free stdlib mux), MinIO presigned GET, Astro 5 + TypeScript strict, Vitest.

**Spec:** docs/PRD.md v2.2, docs/UI_UX_SPEC.md, web/src/tokens/design-system.ts (design tokens terkunci).

## Global Constraints

- R-02: 0 em dash di semua file baru (UI copy DAN kode; hanya file deteksi ber módulo yang dikecualikan).
- R-03: tap target minimal 44px (web + mobile).
- Palet terkunci: #0F172A, #4F46E5, #FFFFFF, accent #F59E0B; font Space Grotesk (display) + Inter (body).
- Astro: TypeScript strict (`astro check` 0 error), semua file .ts/.astro, tidak ada .js baru di web Astro.
- Setiap task diakhiri `make test` exit 0 sebelum commit.
- Commit per task, push tunggal di akhir.

## Review Focus

1. Preview URL tidak boleh membocorkan objek non-preview: endpoint harus memaksa prefix `previews/` (bukan raw-uploads/, bukan hires/). Test: minta URL untuk key `raw-uploads/...` harus 400.
2. Presign TTL tidak boleh melebihi 60 menit (kepatuhan PRD): clamp server-side. Test: TTL 9999 menit → URL tetap terbit dengan TTL 60.
3. Tanpa auth di katalog sama seperti sekarang, tapi wallet/stats tetap butuh auth. Test: wallet tanpa token = 401.
4. Astro `astro check` harus benar-benar berjalan (npx astro check), bukan dilewati. Test: exit 0.
5. Em dash scan harus menyertakan file .astro dan .ts baru. Test: `grep -r $'—' web/src web/pages` = 0 baris.

---

### Task 1: Reorganisasi dokumen root

**Files:**
- Move: API_SPEC.md, ARCHITECTURE.md, DATABASE_SPEC.md, IMPLEMENTATION_PLAN.md, PRD.md, PRD_REVIEW.md, ROADMAP.md, UI_UX_SPEC.md, FOTOYU_BENCHMARK_ANALYSIS.md, GALERIKU_API_ARCHITECTURE_SPEC.md → docs/
- Create: README.md (root, pintu masuk dokumen)
- Modify: Makefile (path lint tetap, cek tidak ada path rusak)
- Delete: salinan root setelah verifikasi identik (sudah diverifikasi `cmp -s` IDENTIK untuk 10 file)

**Interfaces:**
- Produces: README.md dengan tabel pintu masuk dokumen; docs/ menjadi satu-satunya lokasi dokumen.

- [ ] **Step 1: Pindahkan 10 file root ke docs/**

```bash
cd /root/galeriku
git mv API_SPEC.md ARCHITECTURE.md DATABASE_SPEC.md IMPLEMENTATION_PLAN.md PRD.md PRD_REVIEW.md ROADMAP.md UI_UX_SPEC.md FOTOYU_BENCHMARK_ANALYSIS.md GALERIKU_API_ARCHITECTURE_SPEC.md docs/
```

- [ ] **Step 2: Cek referensi rusak**

```bash
grep -rn "file:///root/galeriku/PRD.md\|\.\./PRD.md\|(PRD.md)" docs/*.md Makefile CHANGELOG.md | head -20
```
Perbaiki referensi relatif yang rusak (CHANGELOG.md memakai file:///root/galeriku/PRD.md dsb, sudah benar karena menunjuk root yang file-nya kini di docs; update ke docs/).

- [ ] **Step 3: Buat README.md pintu masuk**

Konten: nama proyek, satu paragraf, struktur repo, tabel dokumen (PRD, UI_UX_SPEC, API_SPEC, DATABASE_SPEC, ARCHITECTURE, API_ARCHITECTURE_SPEC, ROADMAP, IMPLEMENTATION_PLAN, FOTOYU_BENCHMARK, PRD_REVIEW, CHANGELOG), cara mulai (docker compose, make test), status fase singkat.

- [ ] **Step 4: make test + commit**

Run: `make test` → exit 0, lalu commit "docs: consolidate documents into docs/ with README entry point".

### Task 2: Preview image signing endpoint

**Files:**
- Modify: `backend/internal/storage/s3_service.go` (tambah PresignPreviewGet)
- Create: `backend/internal/handler/preview_handler.go`
- Modify: `backend/internal/handler/router.go` (registerPreviewRoutes di SetupRoutesWithFinance)
- Modify: `backend/cmd/server/main.go` (wire)
- Test: `backend/internal/handler/preview_handler_test.go`

**Interfaces:**
- Consumes: `storage.S3Service` (ada), katalog publik (Task sebelumnya).
- Produces: `GET /api/v1/photos/{id}/preview-url?ttl_minutes=60` → `{preview_url, expires_at, object_key}`.

- [ ] **Step 1: Test gagal dulu (TDD)**

Test: seed photo READY + objek preview nyata di MinIO `previews/{photo_id}.jpg`, panggil endpoint, assert 200 + `preview_url` berisi `presigned` signature query, assert key `raw-uploads/...` ditolak 400, assert TTL yang diminta 9999 di-clamp ke 60.

- [ ] **Step 2: Implementasi**

```go
// PresignPreviewGet issues a presigned GET for one preview object.
// Only the previews/ prefix is reachable; TTL is clamped to 60 minutes.
func (s *S3Service) PresignPreviewGet(ctx context.Context, photoID uuid.UUID, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > 60*time.Minute {
		ttl = 60 * time.Minute
	}
	key := "previews/" + photoID.String() + ".jpg"
	u, err := s.client.PresignedGetObject(ctx, BucketMasterVault, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to presign preview: %w", err)
	}
	return u.String(), nil
}
```

Handler memvalidasi photo ada di katalog (repo.GetPhotoByID) sebelum menerbitkan URL, sehingga foto yang tidak READY tidak bisa dipresign.

- [ ] **Step 3: make test, commit "feat(core): preview image signing with previews/ prefix and 60m TTL clamp"**

### Task 3: Role stats endpoints (studio/host/admin)

**Files:**
- Create: `backend/internal/handler/stats_handler.go`
- Modify: `backend/internal/handler/router.go`
- Modify: `backend/cmd/server/main.go`
- Test: `backend/internal/handler/stats_handler_test.go`

**Interfaces:**
- Produces: `GET /api/v1/stats/creator` (photo count, sales count, pending SLA count), `GET /api/v1/stats/host` (org count for membership, member count), `GET /api/v1/stats/admin` (host pending count, user count) masing-masing butuh capability yang sesuai via RequireHostMembership / SUPER_ADMIN check.

- [ ] **Step 1: Test** (seed data, panggil dengan JWT sesuai role, assert angka cocok; non-member 403)
- [ ] **Step 2: Implementasi** SQL COUNT sederhana per role, guard capability.
- [ ] **Step 3: test + commit "feat(core): role stats endpoints"**

### Task 4: Astro storefront (TypeScript strict)

**Files:**
- Create: web/astro.config.mjs, web/tsconfig.json, web/package.json update (astro, @astrojs/check, typescript, vitest)
- Create: web/src/pages/index.astro, search.astro, login.astro, cart.astro, collection.astro, studio.astro, host.astro, admin.astro
- Create: web/src/layouts/Base.astro, web/src/components/*.astro (PhotoCard, NavBar), web/src/styles/tokens.css (dari design-system.ts), web/src/lib/api.ts (re-export GalerikuClient)
- Keep: web/src/client.ts, web/src/types/api.ts (dipakai ulang), web/src/tokens/design-system.ts
- Delete: web/index.html, web/app.css, web/app.js (digantikan Astro; memerlukan konfirmasi: ini rencana yang Anda setujui "lakukan semuanya", jadi lanjut)
- Modify: Makefile (openapi-validate path tetap; test menambah astro check)

**Interfaces:**
- Consumes: GalerikuClient (web/src/client.ts), tokens design-system.ts.
- Produces: 8 halaman Astro dengan TS strict, client-side interactivitas via <script> TS yang di-bundle Astro.

- [ ] **Step 1: Scaffold Astro** (`npm create astro@latest` non-interaktif atau manual config), deps + tsconfig strict
- [ ] **Step 2: tokens.css** dari design-system.ts (nilai sama)
- [ ] **Step 3: Base layout + NavBar + PhotoCard components**
- [ ] **Step 4: 8 halaman** (port dari vanilla SPA; auth state via localStorage + script)
- [ ] **Step 5: astro check + vitest kontrak (token test tetap jalan) + hapus SPA lama + commit**

### Task 5: Verifikasi menyeluruh + push

- [ ] `make test` exit 0 (termasuk astro check + vitest)
- [ ] `go test -race -count=1 ./...` semua ok
- [ ] em dash scan web/src, web/pages, mobile, backend baru = 0
- [ ] commit sisa, push tunggal, verifikasi remote == local
