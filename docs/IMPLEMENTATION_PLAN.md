# Galeriku — Master Implementation Plan & Development Roadmap
**Sumber Kebutuhan:** PRD.md (Versi 2.2 Enhanced Master Blueprint)
**Dokumen Pendukung:** PRD_REVIEW.md, UI_UX_SPEC.md, DATABASE_SPEC.md, API_SPEC.md, ARCHITECTURE.md
**Target Rilis:** MVP Production-Ready
**Tanggal Dokumen:** 15 Oktober 2026
**Status:** Approved Engineering Master Roadmap — Baseline Tahap 7 Persiapan Development

> [!IMPORTANT]
> **Aturan Eksekusi Engineering Galeriku:**
> 1. **Zero-Trust Backend**: Seluruh validasi otorisasi, hak unduh, kalkulasi harga, potongan komisi, saldo dompet, dan status refund divalidasi dan dihitung di backend Go.
> 2. **Double-Entry Ledger Invariant**: Setiap baris mutasi keuangan wajib memiliki pasangan jurnal yang seimbang (`SUM(debit) == SUM(credit)`).
> 3. **Pemisahan Biometrik (UU PDP No. 27/2022)**: PostgreSQL tidak menyimpan raw face vector atau selfie mentah. Seluruh embedding disimpan eksklusif di Qdrant.
> 4. **Tiga Keputusan Kunci Terkunci**:
>    - **DEC-Q01**: File Non-Hi-Res ber-watermark mikro server-side diberikan seketika pasca-bayar sebelum master Hi-Res tersedia.
>    - **DEC-Q02**: Checkout foto BIB wajib melewati verifikasi kode event atau OTP peserta lomba.
>    - **DEC-Q03**: Upload massal 1-2000 file direct-to-S3 wajib melewati reservasi kuota atomik di backend.

---

## 1. Ikhtisar Tahapan Pengembangan (Phase Overview)

| Fase | Nama Fase | Fokus Utama & Deliverables | Estimasi Sprint | Dependensi |
|:---:|---|---|:---:|:---:|
| **Fase 1** | **Core Foundation & Infrastructure** | Setup Docker Compose (Postgres 16, Qdrant, MinIO, RabbitMQ, Redis), Migrasi 76 Tabel DDL, dan Go Core Skeleton. | Sprint 1 | - |
| **Fase 2** | **Identity, Auth & Host B2B Portal** | Multi-capability RBAC (Buyer, Creator, Host Leader, Host Admin), JWT session, verifikasi OTP, pendaftaran & approval Host. | Sprint 2 | Fase 1 |
| **Fase 3** | **FotoTree & Direct-to-S3 Multipart Upload** | Hirarki FotoTree (Event, Sesi, Lokasi), Reservasi kuota atomik (DEC-Q03), S3 Presigned multipart upload massal 1-2000 file. | Sprint 3 | Fase 2 |
| **Fase 4** | **Python AI Worker Subsystem** | Ekstraksi wajah InsightFace (512-dim), OCR nomor BIB EasyOCR, render watermark mikro (DEC-Q01), dan indexing Qdrant. | Sprint 4 | Fase 3 |
| **Fase 5** | **Discovery & Protected Checkout** | Pencarian wajah selfie, pencarian BIB dengan proteksi OTP (DEC-Q02), keranjang belanja, dan pembuatan pesanan idempoten. | Sprint 5 | Fase 4 |
| **Fase 6** | **Payment, Fulfillment 48h SLA & Downloads** | Integrasi Midtrans/Xendit QRIS/VA, SLA countdown 48 jam, instant Non-Hi-Res download (DEC-Q01), dan pemenuhan master Hi-Res. | Sprint 6 | Fase 5 |
| **Fase 7** | **Double-Entry Ledger, Settlements & Refunds** | Jurnal pembukuan berpasangan, snapshot bagi hasil 3 pihak, masa penahanan settlement (T+7 hari), payout bank, dan auto-refund SLA breach. | Sprint 7 | Fase 6 |
| **Fase 8** | **Frontend Multi-Kanal Delivery** | Integrasi API ke Mobile Flutter, Web Storefront, Creator Studio, Host Portal, dan Admin Dashboard. | Sprint 8-9 | Fase 7 |
| **Fase 9** | **Hardening, Security Audit & Launch** | Uji penetrasi STRIDE, load testing upload 2000 foto bersamaan, DR restore drill, dan deployment produksi. | Sprint 10 | Fase 8 |

---

## 2. Rincian Teknis per Fase Pengembangan

### Fase 1: Core Foundation & Infrastructure Setup (Sprint 1)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 1.1** | **Orkestrasi Docker Compose** | Mengonfigurasi layanan PostgreSQL 16, Qdrant v1.11, MinIO S3, RabbitMQ 3.13, dan Redis 7 dengan volume terisolasi dan healthcheck otomatis. |
| **Task 1.2** | **Eksekusi Migrasi Basis Data 000001** | Menerapkan skrip DDL `000001_initial_schema.up.sql` yang mencakup fungsi UUIDv7, seluruh 76 tabel di 8 domain, serta indeks performa tinggi. |
| **Task 1.3** | **Go Backend Hexagonal Skeleton** | Membangun struktur kode Go bersih (Domain, UseCase, Repository, Delivery) dengan driver `pgxpool`, middleware CORS, recovery, dan logging terstruktur (slog). |
| **Task 1.4** | **MinIO Bucket Initialization** | Membuat bucket privat `galeriku-master-vault` dan bucket publik `galeriku-public-media` beserta kebijakan akses. |

### Fase 2: Identity, Auth & Host B2B Portal (Sprint 2)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 2.1** | **Autentikasi & Multi-Capability RBAC** | Implementasi register, login, refresh token, dan pengecekan kapabilitas modular (BUYER, CREATOR, HOST_LEADER, PLATFORM_ADMIN). |
| **Task 2.2** | **Integrasi OTP WhatsApp/SMS** | Layanan verifikasi nomor telepon E.164 menggunakan Redis rate limiter (maksimal 3 pengiriman / 10 menit). |
| **Task 2.3** | **Alur Pendaftaran Host B2B** | Formulir pengajuan Host (`host_applications`), unggah dokumen NIB/KTP ke private vault, dan alur peninjauan Super Admin. |
| **Task 2.4** | **Manajemen Tim & Kuota Admin Host** | Undangan anggota tim terikat token hash (`host_invitations`) dan penegakan batas maksimal admin (`max_admin_count`). |

### Fase 3: FotoTree & Direct-to-S3 Multipart Upload Massal (DEC-Q03) (Sprint 3)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 3.1** | **Manajemen Hirarki FotoTree** | CRUD Event, Sesi (Start, Track KM, Finish), dan Lokasi rute checkpoint. |
| **Task 3.2** | **Preflight Reservasi Kuota Atomik (DEC-Q03)** | Endpoint `POST /storage/upload-batches/initiate` yang mengunci baris kuota pengguna (`SELECT FOR UPDATE`) dan membuat reservasi sebelum upload. |
| **Task 3.3** | **S3 Multipart Presigned Direct Upload** | Penerbitan presigned PUT URL per chunk part untuk pengunggahan 1 s/d 2000 file langsung dari browser ke MinIO/S3. |
| **Task 3.4** | **Finalisasi Batch & Rekonsiliasi Kuota** | Verifikasi ETag S3, penutupan sesi multipart, pemicuan event RabbitMQ, dan pembaruan kuota terpakai. |

### Fase 4: Python AI Worker Pipeline (Biometrik & OCR) (Sprint 4)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 4.1** | **Consumer Antrean RabbitMQ AI** | Worker Python yang mendengarkan event foto baru dan memproses berkas secara asinkronus. |
| **Task 4.2** | **Ekstraksi Wajah InsightFace (ArcFace 512-dim)** | Deteksi multi-wajah pada foto beresolusi tinggi, penapisan kualitas visual, dan ekstraksi vektor normalisasi L2. |
| **Task 4.3** | **Integrasi Pengindeksan Vektor Qdrant** | Menyimpan titik vektor wajah ke koleksi Qdrant dengan payload metadata `{photo_id, subject_id, bbox}`. |
| **Task 4.4** | **Deteksi Nomor BIB (OCR Engine)** | Ekstraksi nomor dada pelari menggunakan model OCR berakurasi tinggi dengan normalisasi karakter numerik. |
| **Task 4.5** | **Watermarking Mikro Server-Side (DEC-Q01)** | Penerapan watermark mikro destruktif pada tingkat piksel untuk file preview dan Non-Hi-Res download. |

### Fase 5: Penemuan Foto & Proteksi Checkout BIB (DEC-Q02) (Sprint 5)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 5.1** | **Pencarian Wajah Berbasis Selfie (UU PDP)** | Upload selfie sementara, ekstraksi fitur wajah, pencarian kemiripan kosinus di Qdrant, dan pembersihan instan file selfie. |
| **Task 5.2** | **Pencarian Publik Nomor BIB** | Katalog pencarian foto berdasarkan nomor dada pelari dengan penanda wajib verifikasi checkout. |
| **Task 5.3** | **Verifikasi Anti-Fraud Checkout BIB (DEC-Q02)** | Pengiriman OTP ke nomor kontak terdaftar peserta lomba untuk menerbitkan grant token sebelum pembelian diizinkan. |
| **Task 5.4** | **Keranjang Belanja & Validasi Pesanan** | Pengelolaan cart dan pembuatan pesanan dengan header `Idempotency-Key`. |

### Fase 6: Pembayaran, Pemenuhan 48h SLA & Secure Downloads (DEC-Q01) (Sprint 6)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 6.1** | **Integrasi Payment Gateway Midtrans/Xendit** | Penerbitan QRIS dan Virtual Account, penanganan webhook terverifikasi signature, dan pembaruan status pesanan. |
| **Task 6.2** | **Penerbitan Unduhan Non-Hi-Res Seketika (DEC-Q01)** | Penyediaan akses unduhan file ber-watermark mikro pasca-pembayaran berhasil. |
| **Task 6.3** | **Pelacakan SLA Pemenuhan Master Hi-Res (48 Jam)** | Timer countdown 48 jam per item pesanan dan notifikasi pengingat ke fotografer. |
| **Task 6.4** | **Unggah Master Hi-Res Bersih** | Fotografer mengunggah file master resolusi penuh untuk menyelesaikan pesanan pembeli. |

### Fase 7: Double-Entry Ledger, Settlement, Refund & Revenue Clawback (REF-01 s/d REF-10) (Sprint 7)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 7.1** | **Mesin Pembukuan Berpasangan (Double-Entry Engine)** | Pencatatan seluruh mutasi transaksi keuangan dengan invariant mutlak `SUM(debit) == SUM(credit)` di level stored procedure transaksional. |
| **Task 7.2** | **Snapshot Pembagian Hasil 3 Pihak (Revenue Split)** | Kalkulasi deterministik pembagian porsi Platform (15%), Host (15%), dan Kreator (70%) tanpa deviasi sen, dikunci tepat saat pembayaran lunas. |
| **Task 7.3** | **Masa Penahanan Settlement (Hold Period T+7 Hari)** | Penguncian saldo hasil penjualan selama 7 hari kalender pasca master Hi-Res terverifikasi untuk melindungi platform dari risiko wanprestasi/sengketa. |
| **Task 7.4** | **Otomatisasi Refund SLA Breach & Clawback 3 Pihak (REF-01 s/d REF-07)** | Cron worker mendeteksi keterlambatan pemenuhan > 48 jam (`hires_due_at`), membuat record `refund_items`, dan membalikkan alokasi pendapatan kreator, Host, dan platform secara atomik. |
| **Task 7.5** | **Dompet Internal Pembeli & Pencairan Tunai Bank (REF-01 & REF-08)** | Dana refund 100% masuk seketika ke akun kewajiban dompet pembeli (`ledger_accounts`), dengan fasilitas penarikan tunai (*cash-out*) ke rekening bank terdaftar (minimal Rp 50.000, 2FA verified). |
| **Task 7.6** | **Alur Laporan Master Hilang & Deteksi Generative Upscale (REF-10)** | Fotografer dapat melaporkan `MASTER_LOST` untuk membuka refund instan bagi pembeli. Pengecekan otomatis metadata EXIF & noise sensor untuk menolak master hasil generative upscaling AI. |
| **Task 7.7** | **Pencairan Dana Eksternal (Disbursement Payouts)** | Integrasi API transfer bank (Iris / Xendit Disbursements) untuk pencairan saldo tersedia fotografer dan Host setelah masa hold berakhir. |

### Fase 8: Integrasi Frontend 5 Kanal (Sprint 8-9)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 8.1** | **Mobile Flutter App (Android & iOS)** | Implementasi GaleriGram feed, Cari Foto AI (Selfie/BIB), keranjang, pembayaran QRIS, dan koleksi. |
| **Task 8.2** | **Web Storefront Publik** | Pencarian foto tanpa login, event FotoTree viewer, dan checkout cepat. |
| **Task 8.3** | **Creator Web Studio** | Dasbor fotografer, upload massal 2000 file direct-to-S3, dan pemenuhan master Hi-Res. |
| **Task 8.4** | **Host B2B Management Portal** | Pengelolaan FotoTree, pembagian rute, monitoring penjualan, dan pencairan dana. |
| **Task 8.5** | **Super Admin Dashboard** | Verifikasi Host, moderasi konten, audit log, dan pengaturan platform. |

### Fase 9: Pengujian, Keamanan & Kesiapan Produksi (Sprint 10)

| ID Tugas | Nama Modul | Rincian Pekerjaan & Kriteria Keberhasilan (Acceptance Criteria) |
|:---:|---|---|
| **Task 9.1** | **Load Testing & Stres Upload 2000 Foto** | Simulasi 50 fotografer mengunggah 2000 berkas secara serentak untuk memvalidasi zero gateway choke. |
| **Task 9.2** | **Uji Penetrasi Keamanan STRIDE** | Audit celah eksploitasi otorisasi BIB, bypass presigned URL, dan keamanan data biometrik. |
| **Task 9.3** | **Disaster Recovery Drill** | Simulasi pemulihan database dari WAL continuous archive dengan target RPO < 5 menit dan RTO < 1 jam. |
| **Task 9.4** | **Go-Live Production Deployment** | Konfigurasi domain, sertifikat TLS, Cloudflare WAF, dan peluncuran resmi. |

---

## 3. Strategi Pengujian dan Verifikasi (Testing Matrix)

| Kategori Pengujian | Lingkup Komponen | Target Metrik / Standar Kelulusan |
|---|---|---|
| **Unit Testing (Go)** | Domain Invariants, Ledger Balancer, Quota Math | Coverage > 85%, Zero data race (`go test -race`) |
| **Integration Testing** | REST API Endpoints, PostgreSQL Queries, Qdrant Vector Search | 100% endpoint lulus pengujian kontrak |
| **Double-Entry Ledger Audit** | Stored Procedure Balancing Verification | `SUM(debit) == SUM(credit)` pada setiap transaksi jurnal |
| **Load & Stress Testing** | Upload Massal 2000 File Direct-to-S3 | P95 latency API < 200ms, Server memory stabil < 512MB |
| **Biometric Compliance Audit** | UU PDP Deletion Workflow (Right to be Forgotten) | Vektor terhapus di Qdrant dalam < 5 detik pasca penarikan consent |

---
*Dokumen ini disetujui sebagai rencana kerja implementasi resmi pengembangan platform Galeriku.*