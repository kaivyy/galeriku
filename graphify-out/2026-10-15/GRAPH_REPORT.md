# Graph Report - galeriku  (2026-10-15)

## Corpus Check
- 41 files · ~102,098 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 925 nodes · 926 edges · 73 communities (72 shown, 1 thin omitted)
- Extraction: 98% EXTRACTED · 2% INFERRED · 0% AMBIGUOUS · INFERRED: 15 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `15ebbce4`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Galeriku — Database Specification & Schema Architecture
- PRODUCT REQUIREMENTS DOCUMENT (PRD)
- docs/UI_UX_SPEC.md
- PRODUCT REQUIREMENTS DOCUMENT (PRD)
- Galeriku — Catatan Review PRD dan Keputusan Produk
- Galeriku — Catatan Review PRD dan Keputusan Produk
- 6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)
- 6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)
- 5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10)
- 8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)
- 5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10)
- 8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)
- 2. Sequence Diagram Alur Bisnis Kritis
- 2. Sequence Diagram Alur Bisnis Kritis
- Domain 01: Identity, Auth & Host B2B Portal
- Domain 01: Identity, Auth & Host B2B Portal
- 2. Rincian Teknis per Fase Pengembangan
- 2. Rincian Teknis per Fase Pengembangan
- router.go
- Domain 02 - Events, Creators and Photos
- Domain 02 - Events, Creators and Photos
- Domain 01 - Identity and Host
- Domain 01 - Identity and Host
- UI_UX_SPEC.md
- 8. SPESIFIKASI LAYAR CREATOR WEB STUDIO (FOTOGRAFER)
- Domain 02: Events, FotoTree & Photo Catalog
- Domain 03 - AI and Biometric Privacy
- Domain 06 - Host Fee, Ledger and Settlement
- Domain 02: Events, FotoTree & Photo Catalog
- Domain 03 - AI and Biometric Privacy
- Domain 06 - Host Fee, Ledger and Settlement
- 1. Rincian Bukti Biner Tech Stack FotoYu (v1.11.11)
- 7. SPESIFIKASI LAYAR KANAL WEB STOREFRONT (PUBLIK)
- 1. Rincian Bukti Biner Tech Stack FotoYu (v1.11.11)
- 7. SPESIFIKASI LAYAR KANAL WEB STOREFRONT (PUBLIK)
- Time
- Domain 07 - Upload, Storage and Quota
- Domain 07 - Upload, Storage and Quota
- Global Constraints
- Domain 04: Cart, Orders & Protected Checkout
- EnsureBucketsExist
- Domain 04 - Cart, Orders and Checkout
- Domain 05 - Payments, Refunds, Fulfillment and Downloads
- Domain 08 - Platform Ops, Audit and Reliability
- Domain 04: Cart, Orders & Protected Checkout
- 10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD
- Galeriku — Database Specification & Schema Architecture
- 9. SPESIFIKASI LAYAR KANAL HOST B2B PORTAL
- 1.3 Format Amplop Respons Standar (Response Envelope)
- Domain 05: Payments, Webhooks, Fulfillment & Downloads
- Domain 07: Storage, Quota & S3 Direct Multipart Upload
- 1.3 Format Amplop Respons Standar (Response Envelope)
- Domain 05: Payments, Webhooks, Fulfillment & Downloads
- Domain 07: Storage, Quota & S3 Direct Multipart Upload
- Domain 03: AI Biometrics, Face Search & BIB OCR
- Domain 08: Platform Ops, System Settings & Admin Portal
- 7. Constraint Kritis dan Invariant Bisnis
- Domain 03: AI Biometrics, Face Search & BIB OCR
- Domain 08: Platform Ops, System Settings & Admin Portal
- 2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain
- Domain 06: Finance, Ledger, Settlements & Payouts
- 2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain
- Domain 06: Finance, Ledger, Settlements & Payouts
- 10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery
- 5. MODEL AKUN DAN CAPABILITY UI
- 1. Arsitektur Data, Teknologi Stack, dan Prinsip Desain
- github.com/galeriku/core
- 10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery
- 10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD
- 1. Arsitektur Data, Teknologi Stack, dan Prinsip Desain
- 10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery

## God Nodes (most connected - your core abstractions)
1. `6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)` - 22 edges
2. `6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)` - 22 edges
3. `8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)` - 19 edges
4. `8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)` - 19 edges
5. `Domain 01: Identity, Auth & Host B2B Portal` - 15 edges
6. `Domain 01: Identity, Auth & Host B2B Portal` - 15 edges
7. `PRODUCT REQUIREMENTS DOCUMENT (PRD)` - 14 edges
8. `PRODUCT REQUIREMENTS DOCUMENT (PRD)` - 14 edges
9. `Domain 02 - Events, Creators and Photos` - 13 edges
10. `Domain 02 - Events, Creators and Photos` - 13 edges

## Surprising Connections (you probably didn't know these)
- `TestEnsureBucketsLive()` --calls--> `loadEnv()`  [INFERRED]
  backend/cmd/provisioner/main_test.go → backend/cmd/provisioner/main.go
- `TestEnsureBucketsContract()` --calls--> `EnsureBucketsExist()`  [INFERRED]
  backend/cmd/provisioner/main_test.go → backend/cmd/provisioner/main.go
- `TestEnsureBucketsLive()` --calls--> `EnsureBucketsExist()`  [INFERRED]
  backend/cmd/provisioner/main_test.go → backend/cmd/provisioner/main.go
- `main()` --calls--> `LoadConfig()`  [INFERRED]
  backend/cmd/server/main.go → backend/internal/config/config.go
- `main()` --calls--> `NewPostgresPool()`  [INFERRED]
  backend/cmd/server/main.go → backend/internal/database/postgres.go

## Import Cycles
- None detected.

## Communities (73 total, 1 thin omitted)

### Community 0 - "Galeriku — Database Specification & Schema Architecture"
Cohesion: 0.18
Nodes (10): 1.1 Stack Teknologi Data, 1.2 Prinsip Integritas dan Standar Representasi, 1. Arsitektur Data, Teknologi Stack, dan Prinsip Desain, 2. Peta 8 Domain dan Ringkasan 76 Tabel, 3. Kamus Data Lengkap (Data Dictionary) 76 Tabel, 4.1 Matriks Kardinalitas Intra-Domain, 4. Kardinalitas dan Relasi Antar Tabel, 6. Relasi Lintas Domain (Cross-Domain Relationships) (+2 more)

### Community 1 - "PRODUCT REQUIREMENTS DOCUMENT (PRD)"
Cohesion: 0.04
Nodes (44): 10.1. Diagram Siklus Hidup Transaksi Pesanan, 10.2. Penjelasan State Transaksi & Pemenuhan Web, 10. SISTEM TRANSAKSI, STATE MACHINE, DAN SLA 48 JAM, 11.1. Penempelan Watermark Server-Side yang Tangguh, 11.2. Proteksi Layar di Aplikasi Mobile, 11. PIPELINE MEDIA, STORAGE, DAN PROTEKSI, 12.1. Rincian Milestone Eksekusi, 12. ROADMAP IMPLEMENTASI (5 FASE MENUJU CLOSED BETA) (+36 more)

### Community 2 - "docs/UI_UX_SPEC.md"
Cohesion: 0.43
Nodes (6): __dirname, __filename, getSourceFiles(), isCarveOut(), lintCopy(), VALID_EXTENSIONS

### Community 3 - "PRODUCT REQUIREMENTS DOCUMENT (PRD)"
Cohesion: 0.04
Nodes (44): 10.1. Diagram Siklus Hidup Transaksi Pesanan, 10.2. Penjelasan State Transaksi & Pemenuhan Web, 10. SISTEM TRANSAKSI, STATE MACHINE, DAN SLA 48 JAM, 11.1. Penempelan Watermark Server-Side yang Tangguh, 11.2. Proteksi Layar di Aplikasi Mobile, 11. PIPELINE MEDIA, STORAGE, DAN PROTEKSI, 12.1. Rincian Milestone Eksekusi, 12. ROADMAP IMPLEMENTASI (5 FASE MENUJU CLOSED BETA) (+36 more)

### Community 4 - "Galeriku — Catatan Review PRD dan Keputusan Produk"
Cohesion: 0.09
Nodes (21): 1. Ringkasan produk, 2. Keputusan resmi yang disetujui, 3. Matriks requirement, decision, assumption, risk, 4.1 Anti-stalker, BIB, dan checkout, 4.2 Non-Hi-Res, SLA, dan refund, 4.3 Kuota dan batch upload, 4.4 Keuangan tiga pihak, 4.5 Privasi dan data anak (+13 more)

### Community 5 - "Galeriku — Catatan Review PRD dan Keputusan Produk"
Cohesion: 0.09
Nodes (21): 1. Ringkasan produk, 2. Keputusan resmi yang disetujui, 3. Matriks requirement, decision, assumption, risk, 4.1 Anti-stalker, BIB, dan checkout, 4.2 Non-Hi-Res, SLA, dan refund, 4.3 Kuota dan batch upload, 4.4 Keuangan tiga pihak, 4.5 Privasi dan data anak (+13 more)

### Community 6 - "6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)"
Cohesion: 0.09
Nodes (22): 6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER), M-01: Splash & Bootstrap (P0), M-02: Guest Onboarding (P0), M-03: Login & Registrasi (P0), M-04: GaleriGram Feed (P0), M-05: Cari Foto AI — Entry Point (P0), M-06: Persetujuan Biometrik (Biometric Consent Gate) (P0), M-07: Kamera Selfie & Panduan Oval (P0) (+14 more)

### Community 7 - "6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)"
Cohesion: 0.09
Nodes (22): 6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER), M-01: Splash & Bootstrap (P0), M-02: Guest Onboarding (P0), M-03: Login & Registrasi (P0), M-04: GaleriGram Feed (P0), M-05: Cari Foto AI — Entry Point (P0), M-06: Persetujuan Biometrik (Biometric Consent Gate) (P0), M-07: Kamera Selfie & Panduan Oval (P0) (+14 more)

### Community 8 - "5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10)"
Cohesion: 0.11
Nodes (19): 5.1 Status Sumber Aturan & Temuan Reverse-Engineering FotoYu, 5.2 Pemicu dan Evaluasi Refund, 5.3 Data Refund, Kolom `refund_items`, dan Presisi Moneter, 5.4 Dompet Internal (Buyer Wallet) dan Arsitektur Double-Entry Ledger, 5.5 Pembalikan Tiga Pihak (Clawback Math) dan Biaya Gateway, 5.6 State Machine Transisi Status Refund, 5.7 Template Notifikasi Transaksional, 5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10) (+11 more)

### Community 9 - "8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)"
Cohesion: 0.11
Nodes (19): 8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18), OPEN-01 — Kebijakan refund ketika file Non-Hi-Res sudah diunduh., OPEN-02 — Daftar kode pengecualian refund dan approver berwenang., OPEN-03 — Durasi penahanan settlement (Hold Period) dan awal penghitungannya., OPEN-04 — Aturan pemenuhan parsial dan refund untuk pesanan multi-item., OPEN-05 — Kategori objek penyimpanan yang masuk kuota yang ditagihkan (Billable)., OPEN-06 — Penghitungan part multipart yang gagal atau file sementara., OPEN-07 — Dukungan relasi satu foto ke beberapa event (Multi-Event Linking). (+11 more)

### Community 10 - "5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10)"
Cohesion: 0.11
Nodes (19): 5.1 Status Sumber Aturan & Temuan Reverse-Engineering FotoYu, 5.2 Pemicu dan Evaluasi Refund, 5.3 Data Refund, Kolom `refund_items`, dan Presisi Moneter, 5.4 Dompet Internal (Buyer Wallet) dan Arsitektur Double-Entry Ledger, 5.5 Pembalikan Tiga Pihak (Clawback Math) dan Biaya Gateway, 5.6 State Machine Transisi Status Refund, 5.7 Template Notifikasi Transaksional, 5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10) (+11 more)

### Community 11 - "8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)"
Cohesion: 0.11
Nodes (19): 8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18), OPEN-01 — Kebijakan refund ketika file Non-Hi-Res sudah diunduh., OPEN-02 — Daftar kode pengecualian refund dan approver berwenang., OPEN-03 — Durasi penahanan settlement (Hold Period) dan awal penghitungannya., OPEN-04 — Aturan pemenuhan parsial dan refund untuk pesanan multi-item., OPEN-05 — Kategori objek penyimpanan yang masuk kuota yang ditagihkan (Billable)., OPEN-06 — Penghitungan part multipart yang gagal atau file sementara., OPEN-07 — Dukungan relasi satu foto ke beberapa event (Multi-Event Linking). (+11 more)

### Community 12 - "2. Sequence Diagram Alur Bisnis Kritis"
Cohesion: 0.12
Nodes (15): 1.1 Level 1: System Context Diagram, 1.2 Level 2: Container Architecture Diagram, 1.3 Level 3: Component Diagram Backend Go Core, 1. C4 Model Arsitektur Sistem, 2.1 Alur Unggahan Massal (1-2000 Foto) Direct-to-S3 & Pemrosesan AI Asinkronus (DEC-Q03), 2.2 Alur Proteksi Pencarian & Checkout Berbasis BIB (DEC-Q02), 2.3 Alur Pembayaran, Unduhan Non-Hi-Res Seketika, dan Pemenuhan SLA 48 Jam (DEC-Q01), 2.4 Alur Klaim Refund Otomatis Jika SLA 48 Jam Terlampaui (OPEN-01 & OPEN-02) (+7 more)

### Community 13 - "2. Sequence Diagram Alur Bisnis Kritis"
Cohesion: 0.12
Nodes (15): 1.1 Level 1: System Context Diagram, 1.2 Level 2: Container Architecture Diagram, 1.3 Level 3: Component Diagram Backend Go Core, 1. C4 Model Arsitektur Sistem, 2.1 Alur Unggahan Massal (1-2000 Foto) Direct-to-S3 & Pemrosesan AI Asinkronus (DEC-Q03), 2.2 Alur Proteksi Pencarian & Checkout Berbasis BIB (DEC-Q02), 2.3 Alur Pembayaran, Unduhan Non-Hi-Res Seketika, dan Pemenuhan SLA 48 Jam (DEC-Q01), 2.4 Alur Klaim Refund Otomatis Jika SLA 48 Jam Terlampaui (OPEN-01 & OPEN-02) (+7 more)

### Community 14 - "Domain 01: Identity, Auth & Host B2B Portal"
Cohesion: 0.13
Nodes (15): Domain 01: Identity, Auth & Host B2B Portal, `GET /api/v1/auth/me`, `GET /api/v1/hosts/applications/my`, `GET /api/v1/hosts/organization`, `GET /api/v1/users/profile`, `POST /api/v1/auth/login`, `POST /api/v1/auth/otp/send`, `POST /api/v1/auth/otp/verify` (+7 more)

### Community 15 - "Domain 01: Identity, Auth & Host B2B Portal"
Cohesion: 0.13
Nodes (15): Domain 01: Identity, Auth & Host B2B Portal, `GET /api/v1/auth/me`, `GET /api/v1/hosts/applications/my`, `GET /api/v1/hosts/organization`, `GET /api/v1/users/profile`, `POST /api/v1/auth/login`, `POST /api/v1/auth/otp/send`, `POST /api/v1/auth/otp/verify` (+7 more)

### Community 16 - "2. Rincian Teknis per Fase Pengembangan"
Cohesion: 0.14
Nodes (13): 1. Ikhtisar Tahapan Pengembangan (Phase Overview), 2. Rincian Teknis per Fase Pengembangan, 3. Strategi Pengujian dan Verifikasi (Testing Matrix), Fase 1: Core Foundation & Infrastructure Setup (Sprint 1), Fase 2: Identity, Auth & Host B2B Portal (Sprint 2), Fase 3: FotoTree & Direct-to-S3 Multipart Upload Massal (DEC-Q03) (Sprint 3), Fase 4: Python AI Worker Pipeline (Biometrik & OCR) (Sprint 4), Fase 5: Penemuan Foto & Proteksi Checkout BIB (DEC-Q02) (Sprint 5) (+5 more)

### Community 17 - "2. Rincian Teknis per Fase Pengembangan"
Cohesion: 0.14
Nodes (13): 1. Ikhtisar Tahapan Pengembangan (Phase Overview), 2. Rincian Teknis per Fase Pengembangan, 3. Strategi Pengujian dan Verifikasi (Testing Matrix), Fase 1: Core Foundation & Infrastructure Setup (Sprint 1), Fase 2: Identity, Auth & Host B2B Portal (Sprint 2), Fase 3: FotoTree & Direct-to-S3 Multipart Upload Massal (DEC-Q03) (Sprint 3), Fase 4: Python AI Worker Pipeline (Biometrik & OCR) (Sprint 4), Fase 5: Penemuan Foto & Proteksi Checkout BIB (DEC-Q02) (Sprint 5) (+5 more)

### Community 18 - "router.go"
Cohesion: 0.09
Nodes (25): main(), getEnv(), LoadConfig(), Context, Pool, NewPostgresPool(), Handler, Pool (+17 more)

### Community 19 - "Domain 02 - Events, Creators and Photos"
Cohesion: 0.15
Nodes (13): Domain 02 - Events, Creators and Photos, Tabel: `event_access_codes`, Tabel: `event_creator_memberships`, Tabel: `event_locations`, Tabel: `event_sessions`, Tabel: `events`, Tabel: `photo_assets`, Tabel: `photo_bib_tags` (+5 more)

### Community 20 - "Domain 02 - Events, Creators and Photos"
Cohesion: 0.15
Nodes (13): Domain 02 - Events, Creators and Photos, Tabel: `event_access_codes`, Tabel: `event_creator_memberships`, Tabel: `event_locations`, Tabel: `event_sessions`, Tabel: `events`, Tabel: `photo_assets`, Tabel: `photo_bib_tags` (+5 more)

### Community 21 - "Domain 01 - Identity and Host"
Cohesion: 0.17
Nodes (12): Domain 01 - Identity and Host, Tabel: `capabilities`, Tabel: `host_admin_limits`, Tabel: `host_applications`, Tabel: `host_invitations`, Tabel: `host_legal_documents`, Tabel: `host_locations`, Tabel: `host_memberships` (+4 more)

### Community 22 - "Domain 01 - Identity and Host"
Cohesion: 0.17
Nodes (12): Domain 01 - Identity and Host, Tabel: `capabilities`, Tabel: `host_admin_limits`, Tabel: `host_applications`, Tabel: `host_invitations`, Tabel: `host_legal_documents`, Tabel: `host_locations`, Tabel: `host_memberships` (+4 more)

### Community 23 - "UI_UX_SPEC.md"
Cohesion: 0.05
Nodes (42): 10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD, 11.1. Palet Warna Fungsional (*Functional Colors*), 11.2. Aksesibilitas (WCAG 2.1 AA Standard), 11. SISTEM DESAIN DAN TOKEN UI, 12. STANDAR STATE DAN PENANGANAN ERROR GLOBAL, 13. REKAPITULASI ACCEPTANCE CRITERIA LINTAS KANAL, 1. TUJUAN DAN CAKUPAN, 2. PRINSIP FUNDAMENTAL DESAIN UI/UX (+34 more)

### Community 24 - "8. SPESIFIKASI LAYAR CREATOR WEB STUDIO (FOTOGRAFER)"
Cohesion: 0.17
Nodes (12): 8. SPESIFIKASI LAYAR CREATOR WEB STUDIO (FOTOGRAFER), CS-01: Login & Capability Gate (P0), CS-02: Dashboard Ringkasan Performa (P0), CS-03: Pemilih FotoTree Target (P0), CS-04: Dropzone Unggah Massal (P0), CS-05: Preflight Check & Alokasi Kuota (P0; Keputusan `DEC-Q03`), CS-06: Monitoring Progres Upload Massal (P0), CS-07: Antrean Pemrosesan Server (*Processing Queue*) (P0) (+4 more)

### Community 25 - "Domain 02: Events, FotoTree & Photo Catalog"
Cohesion: 0.18
Nodes (11): Domain 02: Events, FotoTree & Photo Catalog, `GET /api/v1/events`, `GET /api/v1/events/{id}/photos`, `GET /api/v1/events/{slug}`, `GET /api/v1/photos/feed`, `GET /api/v1/photos/{id}`, `POST /api/v1/events`, `POST /api/v1/events/{id}/creators/apply` (+3 more)

### Community 26 - "Domain 03 - AI and Biometric Privacy"
Cohesion: 0.18
Nodes (11): Domain 03 - AI and Biometric Privacy, Tabel: `ai_job_results`, Tabel: `ai_jobs`, Tabel: `ai_search_results`, Tabel: `ai_search_sessions`, Tabel: `biometric_consents`, Tabel: `biometric_deletion_jobs`, Tabel: `biometric_deletion_tasks` (+3 more)

### Community 27 - "Domain 06 - Host Fee, Ledger and Settlement"
Cohesion: 0.18
Nodes (11): Domain 06 - Host Fee, Ledger and Settlement, Tabel: `host_fee_rule_versions`, Tabel: `host_fee_rules`, Tabel: `ledger_accounts`, Tabel: `ledger_entries`, Tabel: `ledger_transactions`, Tabel: `order_revenue_splits`, Tabel: `payout_events` (+3 more)

### Community 28 - "Domain 02: Events, FotoTree & Photo Catalog"
Cohesion: 0.18
Nodes (11): Domain 02: Events, FotoTree & Photo Catalog, `GET /api/v1/events`, `GET /api/v1/events/{id}/photos`, `GET /api/v1/events/{slug}`, `GET /api/v1/photos/feed`, `GET /api/v1/photos/{id}`, `POST /api/v1/events`, `POST /api/v1/events/{id}/creators/apply` (+3 more)

### Community 29 - "Domain 03 - AI and Biometric Privacy"
Cohesion: 0.18
Nodes (11): Domain 03 - AI and Biometric Privacy, Tabel: `ai_job_results`, Tabel: `ai_jobs`, Tabel: `ai_search_results`, Tabel: `ai_search_sessions`, Tabel: `biometric_consents`, Tabel: `biometric_deletion_jobs`, Tabel: `biometric_deletion_tasks` (+3 more)

### Community 30 - "Domain 06 - Host Fee, Ledger and Settlement"
Cohesion: 0.18
Nodes (11): Domain 06 - Host Fee, Ledger and Settlement, Tabel: `host_fee_rule_versions`, Tabel: `host_fee_rules`, Tabel: `ledger_accounts`, Tabel: `ledger_entries`, Tabel: `ledger_transactions`, Tabel: `order_revenue_splits`, Tabel: `payout_events` (+3 more)

### Community 31 - "1. Rincian Bukti Biner Tech Stack FotoYu (v1.11.11)"
Cohesion: 0.18
Nodes (10): 1.1 Mobile Client Runtime (Android / iOS), 1.2 Media, Image Processing & Kamera, 1.3 Pemetaan & Geolokasi (Spatial Pipeline), 1.4 Layanan Cloud, Autentikasi & Database, 1.5 Finansial, Payment Gateway & Payouts, 1.6 Machine Learning & Pemrosesan AI, 1. Rincian Bukti Biner Tech Stack FotoYu (v1.11.11), 2. Matriks Komparasi Mendalam: FotoYu vs Galeriku (+2 more)

### Community 32 - "7. SPESIFIKASI LAYAR KANAL WEB STOREFRONT (PUBLIK)"
Cohesion: 0.05
Nodes (42): 10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD, 11.1. Palet Warna Fungsional (*Functional Colors*), 11.2. Aksesibilitas (WCAG 2.1 AA Standard), 11. SISTEM DESAIN DAN TOKEN UI, 12. STANDAR STATE DAN PENANGANAN ERROR GLOBAL, 13. REKAPITULASI ACCEPTANCE CRITERIA LINTAS KANAL, 1. TUJUAN DAN CAKUPAN, 2. PRINSIP FUNDAMENTAL DESAIN UI/UX (+34 more)

### Community 33 - "1. Rincian Bukti Biner Tech Stack FotoYu (v1.11.11)"
Cohesion: 0.18
Nodes (10): 1.1 Mobile Client Runtime (Android / iOS), 1.2 Media, Image Processing & Kamera, 1.3 Pemetaan & Geolokasi (Spatial Pipeline), 1.4 Layanan Cloud, Autentikasi & Database, 1.5 Finansial, Payment Gateway & Payouts, 1.6 Machine Learning & Pemrosesan AI, 1. Rincian Bukti Biner Tech Stack FotoYu (v1.11.11), 2. Matriks Komparasi Mendalam: FotoYu vs Galeriku (+2 more)

### Community 34 - "7. SPESIFIKASI LAYAR KANAL WEB STOREFRONT (PUBLIK)"
Cohesion: 0.22
Nodes (9): Domain 04 - Cart, Orders and Checkout, Tabel: `cart_items`, Tabel: `carts`, Tabel: `checkout_verification_grants`, Tabel: `checkout_verifications`, Tabel: `order_items`, Tabel: `order_status_events`, Tabel: `orders` (+1 more)

### Community 35 - "Time"
Cohesion: 0.29
Nodes (6): WorkerConfig, Order, Photo, User, Time, UUID

### Community 36 - "Domain 07 - Upload, Storage and Quota"
Cohesion: 0.20
Nodes (10): Domain 07 - Upload, Storage and Quota, Tabel: `multipart_upload_sessions`, Tabel: `quota_policy_versions`, Tabel: `quota_reservations`, Tabel: `storage_accounts`, Tabel: `storage_objects`, Tabel: `storage_quotas`, Tabel: `storage_usage_entries` (+2 more)

### Community 37 - "Domain 07 - Upload, Storage and Quota"
Cohesion: 0.20
Nodes (10): Domain 07 - Upload, Storage and Quota, Tabel: `multipart_upload_sessions`, Tabel: `quota_policy_versions`, Tabel: `quota_reservations`, Tabel: `storage_accounts`, Tabel: `storage_objects`, Tabel: `storage_quotas`, Tabel: `storage_usage_entries` (+2 more)

### Community 38 - "Global Constraints"
Cohesion: 0.20
Nodes (9): 4. Self-Review Checklist, 5. Execution Handoff, Global Constraints, Phase 1: Core Foundation & Infrastructure Implementation Plan, Task 1: Docker Infrastructure Orchestration & MinIO S3 Provisioner, Task 2: Database Migration Runner & 76-Table Verification Test, Task 3: Go Database Connection Pool (`pgxpool`) & Health Check Service, Task 4: Standard Envelope & Request Correlation ID Middleware (+1 more)

### Community 39 - "Domain 04: Cart, Orders & Protected Checkout"
Cohesion: 0.22
Nodes (9): `DELETE /api/v1/cart/items/{id}`, Domain 04: Cart, Orders & Protected Checkout, `GET /api/v1/cart`, `GET /api/v1/orders`, `GET /api/v1/orders/{id}`, `POST /api/v1/cart/items`, `POST /api/v1/checkout/verify-bib/confirm`, `POST /api/v1/checkout/verify-bib/initiate` (+1 more)

### Community 40 - "EnsureBucketsExist"
Cohesion: 0.36
Nodes (7): EnsureBucketsExist(), Context, loadEnv(), main(), T, TestEnsureBucketsContract(), TestEnsureBucketsLive()

### Community 41 - "Domain 04 - Cart, Orders and Checkout"
Cohesion: 0.22
Nodes (9): Domain 04 - Cart, Orders and Checkout, Tabel: `cart_items`, Tabel: `carts`, Tabel: `checkout_verification_grants`, Tabel: `checkout_verifications`, Tabel: `order_items`, Tabel: `order_status_events`, Tabel: `orders` (+1 more)

### Community 42 - "Domain 05 - Payments, Refunds, Fulfillment and Downloads"
Cohesion: 0.22
Nodes (9): Domain 05 - Payments, Refunds, Fulfillment and Downloads, Tabel: `download_events`, Tabel: `download_grants`, Tabel: `fulfillment_events`, Tabel: `fulfillment_jobs`, Tabel: `payment_events`, Tabel: `payments`, Tabel: `refund_items` (+1 more)

### Community 43 - "Domain 08 - Platform Ops, Audit and Reliability"
Cohesion: 0.22
Nodes (9): Domain 08 - Platform Ops, Audit and Reliability, Tabel: `admin_actions`, Tabel: `audit_logs`, Tabel: `idempotency_keys`, Tabel: `notification_deliveries`, Tabel: `notifications`, Tabel: `outbox_events`, Tabel: `system_settings` (+1 more)

### Community 44 - "Domain 04: Cart, Orders & Protected Checkout"
Cohesion: 0.22
Nodes (9): `DELETE /api/v1/cart/items/{id}`, Domain 04: Cart, Orders & Protected Checkout, `GET /api/v1/cart`, `GET /api/v1/orders`, `GET /api/v1/orders/{id}`, `POST /api/v1/cart/items`, `POST /api/v1/checkout/verify-bib/confirm`, `POST /api/v1/checkout/verify-bib/initiate` (+1 more)

### Community 45 - "10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD"
Cohesion: 0.22
Nodes (9): Domain 05 - Payments, Refunds, Fulfillment and Downloads, Tabel: `download_events`, Tabel: `download_grants`, Tabel: `fulfillment_events`, Tabel: `fulfillment_jobs`, Tabel: `payment_events`, Tabel: `payments`, Tabel: `refund_items` (+1 more)

### Community 46 - "Galeriku — Database Specification & Schema Architecture"
Cohesion: 0.18
Nodes (10): 1.1 Stack Teknologi Data, 1.2 Prinsip Integritas dan Standar Representasi, 1. Arsitektur Data, Teknologi Stack, dan Prinsip Desain, 2. Peta 8 Domain dan Ringkasan 76 Tabel, 3. Kamus Data Lengkap (Data Dictionary) 76 Tabel, 4.1 Matriks Kardinalitas Intra-Domain, 4. Kardinalitas dan Relasi Antar Tabel, 6. Relasi Lintas Domain (Cross-Domain Relationships) (+2 more)

### Community 47 - "9. SPESIFIKASI LAYAR KANAL HOST B2B PORTAL"
Cohesion: 0.22
Nodes (9): Domain 08 - Platform Ops, Audit and Reliability, Tabel: `admin_actions`, Tabel: `audit_logs`, Tabel: `idempotency_keys`, Tabel: `notification_deliveries`, Tabel: `notifications`, Tabel: `outbox_events`, Tabel: `system_settings` (+1 more)

### Community 48 - "1.3 Format Amplop Respons Standar (Response Envelope)"
Cohesion: 0.29
Nodes (7): 1.1 Base URL dan Gateway, 1.2 Header Standar Permintaan (Request Headers), 1.3 Format Amplop Respons Standar (Response Envelope), 1. Konvensi Global dan Desain Protokol, Format Berhasil dengan Paginasi (Paginated List):, Format Berhasil (Single Object):, Format Galat Terstruktur (Error Envelope):

### Community 49 - "Domain 05: Payments, Webhooks, Fulfillment & Downloads"
Cohesion: 0.29
Nodes (7): Domain 05: Payments, Webhooks, Fulfillment & Downloads, `GET /api/v1/downloads/file/{token}`, `POST /api/v1/downloads/grant/{order_item_id}`, `POST /api/v1/fulfillment/upload-master/{order_item_id}`, `POST /api/v1/payments/initiate`, `POST /api/v1/refunds/request`, `POST /api/v1/webhooks/payments/midtrans`

### Community 50 - "Domain 07: Storage, Quota & S3 Direct Multipart Upload"
Cohesion: 0.29
Nodes (7): Domain 07: Storage, Quota & S3 Direct Multipart Upload, `GET /api/v1/storage/quota`, `POST /api/v1/storage/multipart/complete`, `POST /api/v1/storage/multipart/initiate`, `POST /api/v1/storage/multipart/sign-part`, `POST /api/v1/storage/upload-batches/{id}/finalize`, `POST /api/v1/storage/upload-batches/initiate`

### Community 51 - "1.3 Format Amplop Respons Standar (Response Envelope)"
Cohesion: 0.29
Nodes (7): 1.1 Base URL dan Gateway, 1.2 Header Standar Permintaan (Request Headers), 1.3 Format Amplop Respons Standar (Response Envelope), 1. Konvensi Global dan Desain Protokol, Format Berhasil dengan Paginasi (Paginated List):, Format Berhasil (Single Object):, Format Galat Terstruktur (Error Envelope):

### Community 52 - "Domain 05: Payments, Webhooks, Fulfillment & Downloads"
Cohesion: 0.29
Nodes (7): Domain 05: Payments, Webhooks, Fulfillment & Downloads, `GET /api/v1/downloads/file/{token}`, `POST /api/v1/downloads/grant/{order_item_id}`, `POST /api/v1/fulfillment/upload-master/{order_item_id}`, `POST /api/v1/payments/initiate`, `POST /api/v1/refunds/request`, `POST /api/v1/webhooks/payments/midtrans`

### Community 53 - "Domain 07: Storage, Quota & S3 Direct Multipart Upload"
Cohesion: 0.29
Nodes (7): Domain 07: Storage, Quota & S3 Direct Multipart Upload, `GET /api/v1/storage/quota`, `POST /api/v1/storage/multipart/complete`, `POST /api/v1/storage/multipart/initiate`, `POST /api/v1/storage/multipart/sign-part`, `POST /api/v1/storage/upload-batches/{id}/finalize`, `POST /api/v1/storage/upload-batches/initiate`

### Community 54 - "Domain 03: AI Biometrics, Face Search & BIB OCR"
Cohesion: 0.33
Nodes (6): `DELETE /api/v1/biometrics/consent`, Domain 03: AI Biometrics, Face Search & BIB OCR, `POST /api/v1/biometrics/consent`, `POST /api/v1/photos/{id}/bib-tags`, `POST /api/v1/search/bib`, `POST /api/v1/search/face`

### Community 55 - "Domain 08: Platform Ops, System Settings & Admin Portal"
Cohesion: 0.33
Nodes (6): Domain 08: Platform Ops, System Settings & Admin Portal, `GET /api/v1/admin/audit-logs`, `GET /api/v1/admin/settings`, `POST /api/v1/admin/hosts/{id}/approve`, `POST /api/v1/admin/hosts/{id}/reject`, `PUT /api/v1/admin/settings`

### Community 56 - "7. Constraint Kritis dan Invariant Bisnis"
Cohesion: 0.33
Nodes (6): 7.1 Invariant Double-Entry Ledger (Penyeimbangan Buku Besar), 7.2 Integritas Nilai Moneter Rupiah Bulat, 7.3 Atomisitas Reservasi Kuota Unggahan (DEC-Q03), 7.4 Batas Maksimal Pengembalian Dana (Refund Boundary), 7.5 Hak Akses Host Admin Terbatas, 7. Constraint Kritis dan Invariant Bisnis

### Community 57 - "Domain 03: AI Biometrics, Face Search & BIB OCR"
Cohesion: 0.33
Nodes (6): `DELETE /api/v1/biometrics/consent`, Domain 03: AI Biometrics, Face Search & BIB OCR, `POST /api/v1/biometrics/consent`, `POST /api/v1/photos/{id}/bib-tags`, `POST /api/v1/search/bib`, `POST /api/v1/search/face`

### Community 58 - "Domain 08: Platform Ops, System Settings & Admin Portal"
Cohesion: 0.33
Nodes (6): Domain 08: Platform Ops, System Settings & Admin Portal, `GET /api/v1/admin/audit-logs`, `GET /api/v1/admin/settings`, `POST /api/v1/admin/hosts/{id}/approve`, `POST /api/v1/admin/hosts/{id}/reject`, `PUT /api/v1/admin/settings`

### Community 59 - "2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain"
Cohesion: 0.40
Nodes (4): 2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain, 3. Katalog Kode Galat Resmi (System Error Catalog), 4. Kebijakan Rate Limiting & Keamanan API, Galeriku — API Specification & Contract Architecture

### Community 60 - "Domain 06: Finance, Ledger, Settlements & Payouts"
Cohesion: 0.40
Nodes (5): Domain 06: Finance, Ledger, Settlements & Payouts, `GET /api/v1/finance/ledger-entries`, `GET /api/v1/finance/settlements`, `GET /api/v1/finance/wallet`, `POST /api/v1/finance/payouts/request`

### Community 61 - "2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain"
Cohesion: 0.20
Nodes (9): 2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain, 3. Katalog Kode Galat Resmi (System Error Catalog), 4. Kebijakan Rate Limiting & Keamanan API, Domain 06: Finance, Ledger, Settlements & Payouts, Galeriku — API Specification & Contract Architecture, `GET /api/v1/finance/ledger-entries`, `GET /api/v1/finance/settlements`, `GET /api/v1/finance/wallet` (+1 more)

### Community 62 - "Domain 06: Finance, Ledger, Settlements & Payouts"
Cohesion: 0.15
Nodes (12): typescript, vitest, devDependencies, typescript, vitest, name, private, scripts (+4 more)

### Community 63 - "10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery"
Cohesion: 0.17
Nodes (11): src/**/*, test/**/*, compilerOptions, esModuleInterop, forceConsistentCasingInFileNames, module, moduleResolution, skipLibCheck (+3 more)

### Community 64 - "5. MODEL AKUN DAN CAPABILITY UI"
Cohesion: 0.29
Nodes (5): Context, Pool, T, TestVerifySchemaCompleteness(), VerifySchemaCompleteness()

### Community 65 - "1. Arsitektur Data, Teknologi Stack, dan Prinsip Desain"
Cohesion: 0.33
Nodes (6): 7.1 Invariant Double-Entry Ledger (Penyeimbangan Buku Besar), 7.2 Integritas Nilai Moneter Rupiah Bulat, 7.3 Atomisitas Reservasi Kuota Unggahan (DEC-Q03), 7.4 Batas Maksimal Pengembalian Dana (Refund Boundary), 7.5 Hak Akses Host Admin Terbatas, 7. Constraint Kritis dan Invariant Bisnis

### Community 69 - "10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery"
Cohesion: 0.50
Nodes (4): 10.1 Strategi Partisi Rentang Waktu (Declarative Range Partitioning), 10.2 Prosedur Migrasi Skema Zero-Downtime (Expand-and-Contract), 10.3 Strategi Backup dan Pemulihan Bencana (Disaster Recovery), 10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery

### Community 70 - "10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD"
Cohesion: 0.60
Nodes (4): AntislopDials, BrandPalette, LayoutMetrics, validateAntislopCopy()

### Community 73 - "1. Arsitektur Data, Teknologi Stack, dan Prinsip Desain"
Cohesion: 0.17
Nodes (12): 8. SPESIFIKASI LAYAR CREATOR WEB STUDIO (FOTOGRAFER), CS-01: Login & Capability Gate (P0), CS-02: Dashboard Ringkasan Performa (P0), CS-03: Pemilih FotoTree Target (P0), CS-04: Dropzone Unggah Massal (P0), CS-05: Preflight Check & Alokasi Kuota (P0; Keputusan `DEC-Q03`), CS-06: Monitoring Progres Upload Massal (P0), CS-07: Antrean Pemrosesan Server (*Processing Queue*) (P0) (+4 more)

### Community 76 - "10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery"
Cohesion: 0.50
Nodes (4): 10.1 Strategi Partisi Rentang Waktu (Declarative Range Partitioning), 10.2 Prosedur Migrasi Skema Zero-Downtime (Expand-and-Contract), 10.3 Strategi Backup dan Pemulihan Bencana (Disaster Recovery), 10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery

## Knowledge Gaps
- **702 isolated node(s):** `github.com/galeriku/core`, `contextKey`, `name`, `version`, `private` (+697 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `3. Kamus Data Lengkap (Data Dictionary) 76 Tabel` connect `Galeriku — Database Specification & Schema Architecture` to `Domain 07 - Upload, Storage and Quota`, `Domain 04 - Cart, Orders and Checkout`, `Domain 05 - Payments, Refunds, Fulfillment and Downloads`, `Domain 08 - Platform Ops, Audit and Reliability`, `Domain 02 - Events, Creators and Photos`, `Domain 01 - Identity and Host`, `Domain 03 - AI and Biometric Privacy`, `Domain 06 - Host Fee, Ledger and Settlement`?**
  _High betweenness centrality (0.019) - this node is a cross-community bridge._
- **Why does `3. Kamus Data Lengkap (Data Dictionary) 76 Tabel` connect `Galeriku — Database Specification & Schema Architecture` to `7. SPESIFIKASI LAYAR KANAL WEB STOREFRONT (PUBLIK)`, `Domain 07 - Upload, Storage and Quota`, `10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD`, `9. SPESIFIKASI LAYAR KANAL HOST B2B PORTAL`, `Domain 02 - Events, Creators and Photos`, `Domain 01 - Identity and Host`, `Domain 03 - AI and Biometric Privacy`, `Domain 06 - Host Fee, Ledger and Settlement`?**
  _High betweenness centrality (0.019) - this node is a cross-community bridge._
- **Why does `Galeriku — Database Specification & Schema Architecture` connect `Galeriku — Database Specification & Schema Architecture` to `5.8 Resolusi 10 Keputusan Terbuka Khusus Refund (REF-01 s/d REF-10)`, `8. Analisis dan Resolusi 18 Keputusan Terbuka (OPEN-01 s/d OPEN-18)`, `7. Constraint Kritis dan Invariant Bisnis`, `10. Strategi Partisi Tabel, Migrasi, Backup, dan Disaster Recovery`?**
  _High betweenness centrality (0.014) - this node is a cross-community bridge._
- **What connects `github.com/galeriku/core`, `contextKey`, `name` to the rest of the system?**
  _702 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `PRODUCT REQUIREMENTS DOCUMENT (PRD)` be split into smaller, more focused modules?**
  _Cohesion score 0.044444444444444446 - nodes in this community are weakly interconnected._
- **Should `PRODUCT REQUIREMENTS DOCUMENT (PRD)` be split into smaller, more focused modules?**
  _Cohesion score 0.044444444444444446 - nodes in this community are weakly interconnected._
- **Should `Galeriku — Catatan Review PRD dan Keputusan Produk` be split into smaller, more focused modules?**
  _Cohesion score 0.08695652173913043 - nodes in this community are weakly interconnected._