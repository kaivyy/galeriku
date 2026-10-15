# Galeriku — System Architecture & Technical Design Specification
**Sumber Kebutuhan:** PRD.md (Versi 2.2 Enhanced Master Blueprint)
**Dokumen Pendukung:** PRD_REVIEW.md, UI_UX_SPEC.md, DATABASE_SPEC.md, API_SPEC.md
**Pola Arsitektur:** Modular Monolith (Go Backend Core) + Asynchronous AI Worker Services (Python) + Event-Driven Messaging (RabbitMQ)
**Tanggal Dokumen:** 15 Oktober 2026
**Status:** Architectural Master Specification — Baseline Tahap 5 Persiapan Development

> [!IMPORTANT]
> **Prinsip Arsitektur Utama Galeriku:**
> 1. **Modular Monolith Go Core**: Core platform dibangun menggunakan arsitektur modular hexagonal/clean architecture di dalam satu binary Go performa tinggi untuk kemudahan deployment, konsistensi transaksi ACID, dan latensi ultra-rendah (< 50ms p95).
> 2. **Isolasi Beban AI (Python Subsystem)**: Ekstraksi embedding wajah (InsightFace) dan OCR BIB (EasyOCR/PaddleOCR) diisolasi dalam container worker Python terpisah dengan akselerasi GPU/CPU, terhubung secara asinkronus melalui RabbitMQ.
> 3. **Penyimpanan Terpisah Vektor & Relasional**: PostgreSQL 16 menangani seluruh data relasional, transaksi pesanan, kuota, dan double-entry ledger. Qdrant menangani pengindeksan vektor biometrik wajah 512-dimensi secara terisolasi.
> 4. **Direct-to-S3 Upload (Zero Gateway Choke)**: Unggahan foto massal (1 s/d 2000 file) tidak melewati server backend Go, melainkan langsung ke bucket S3 via presigned multipart URL setelah reservasi kuota atomik disetujui (DEC-Q03).
> 5. **Pemisahan Master Hi-Res & Destructive Watermarking**: Master foto resolusi penuh tersimpan di private bucket S3 terenkripsi. Semua pratinjau publik dan file Non-Hi-Res sementara dirasterisasi server-side dengan watermark mikro destruktif (DEC-Q01).

---

## 1. C4 Model Arsitektur Sistem

### 1.1 Level 1: System Context Diagram
Diagram konteks sistem menggambarkan interaksi platform Galeriku dengan seluruh persona pengguna dan layanan eksternal:

```mermaid
flowchart TD
    subgraph Users ["Pengguna Galeriku"]
        buyer["Pembeli / Peserta Lomba"]
        creator["Fotografer Kreator"]
        host["Host Event Organizer / EO"]
        admin["Platform Super Admin"]
    end

    subgraph GalerikuSystem ["Platform Galeriku"]
        mobileApp["Mobile App (Flutter Android/iOS)"]
        webStore["Web Storefront (Next.js)"]
        creatorStudio["Creator Studio Web (Next.js)"]
        hostPortal["Host Management Portal (Next.js)"]
        adminPortal["Super Admin Dashboard (Next.js)"]
        coreBackend["Core Platform Gateway & Backend (Go)"]
        aiWorkers["AI & Computer Vision Cluster (Python)"]
    end

    subgraph ExternalServices ["Layanan Pihak Ketiga (External)"]
        payGw["Payment Gateway (Midtrans / Xendit)"]
        waGw["WhatsApp Gateway (Fonnte / Twilio)"]
        s3Storage["Object Storage (MinIO / AWS S3)"]
        cdn["Global Edge CDN (Cloudflare)"]
    end

    buyer -->|Jelajah, Cari Wajah/BIB, Checkout| mobileApp
    buyer -->|Beli Foto Tanpa Install Aplikasi| webStore
    creator -->|Upload Massal 2000 Foto, Pemenuhan Master| creatorStudio
    host -->|Kelola FotoTree, Tim Admin, Payout| hostPortal
    admin -->|Moderasi, Verifikasi Host, Audit| adminPortal

    mobileApp & webStore & creatorStudio & hostPortal & adminPortal -->|HTTPS / REST API / Idempotency| coreBackend
    coreBackend -->|Enqueue Job via RabbitMQ| aiWorkers
    coreBackend -->|Inisiasi Pembayaran & Webhook| payGw
    coreBackend -->|Kirim OTP / Notifikasi| waGw
    creatorStudio -.->|Direct Multipart Upload| s3Storage
    s3Storage -->|Distribusi Aset Publik & Watermark| cdn
    cdn -->|Cache Gambar Cepat| buyer
```

---

### 1.2 Level 2: Container Architecture Diagram
Diagram kontainer menguraikan komponen perangkat lunak independen yang membentuk ekosistem Galeriku:

```mermaid
flowchart TD
    subgraph FrontendClients ["Frontend Presentation Tier"]
        flutterApp["Flutter App (iOS & Android)"]
        nextApps["Next.js SSR/SPA (Storefront, Studio, Host, Admin)"]
    end

    subgraph EdgeTier ["Edge & Ingress Tier"]
        traefik["Traefik / Nginx API Gateway & Reverse Proxy"]
        cdnEdge["Cloudflare CDN & WAF"]
    end

    subgraph ApplicationTier ["Application & Processing Tier"]
        goCore["Galeriku Go Core Monolith (REST API, Ledger, Quota, Auth)"]
        pyAiWorker["Python AI Worker (InsightFace, EasyOCR, Watermark Engine)"]
    end

    subgraph DataStorageTier ["Data & Persistence Tier"]
        pgDb[("PostgreSQL 16 (Source of Truth, 76 Tables, Ledger)")]
        qdrantDb[("Qdrant Vector DB (512-dim Face Embeddings)")]
        redisCache[("Redis 7 (Distributed Cache, Session, Rate Limiter)")]
        rabbitMQ[("RabbitMQ 3 (AMQP 0-9-1 Message Broker)")]
        minioS3[("MinIO / AWS S3 (Private Master Vault & Public Media)")]
    end

    flutterApp & nextApps -->|HTTPS| cdnEdge
    cdnEdge -->|Route API Traffic| traefik
    traefik -->|Forward HTTP /api/v1| goCore
    goCore -->|Read / Write SQL (ACID)| pgDb
    goCore -->|Session & Rate Limiting| redisCache
    goCore -->|Publish Job (Outbox Pattern)| rabbitMQ
    rabbitMQ -->|Consume AI Tasks| pyAiWorker
    pyAiWorker -->|Index & Search Vectors| qdrantDb
    pyAiWorker -->|Fetch Raw & Write Watermarked Derivative| minioS3
    pyAiWorker -->|Callback / Complete Job Event| rabbitMQ
    rabbitMQ -->|Consume AI Results & Update Status| goCore
```

---

### 1.3 Level 3: Component Diagram Backend Go Core
Backend Go dibangun menggunakan Clean Architecture dengan pembagian layer yang tegas:
- **Domain Entities & Invariants**: Model murni tanpa dependensi eksternal.
- **Use Cases (Application Services)**: Logika bisnis transaksi, kalkulasi revenue split, dan alur pemesanan.
- **Adapters / Repositories**: Implementasi akses database PostgreSQL (pgx/sqlc) dan Redis.
- **Delivery / Handlers**: HTTP Controller RESTful (Gin / Fiber / standard net/http) dan RabbitMQ Consumer.

```mermaid
flowchart LR
    subgraph DeliveryLayer ["Delivery Layer (HTTP / AMQP)"]
        authHandler["Auth Handler"]
        eventHandler["Event & FotoTree Handler"]
        orderHandler["Order & Checkout Handler"]
        storageHandler["Storage & Multipart Handler"]
        amqpConsumer["Outbox & AI Event Consumer"]
    end

    subgraph UseCaseLayer ["Application Use Cases"]
        authUC["Auth & RBAC Service"]
        orderUC["Order Processing & BIB Verify Service"]
        ledgerUC["Double-Entry Ledger Posting Service"]
        quotaUC["Atomic Quota Reservation Service"]
        downloadUC["Secure Download Grant Service"]
    end

    subgraph RepositoryLayer ["Infrastructure & Persistence"]
        pgRepo["PostgreSQL Repositories (sqlc / pgxpool)"]
        redisRepo["Redis Cache & Lock Adapter"]
        s3Adapter["AWS S3 / MinIO SDK Adapter"]
        rabbitProducer["RabbitMQ AMQP Publisher"]
    end

    authHandler --> authUC
    orderHandler --> orderUC
    orderUC --> ledgerUC
    storageHandler --> quotaUC
    amqpConsumer --> downloadUC

    authUC & orderUC & ledgerUC & quotaUC --> pgRepo
    orderUC & quotaUC --> redisRepo
    quotaUC --> s3Adapter
    orderUC & quotaUC --> rabbitProducer
```

---

## 2. Sequence Diagram Alur Bisnis Kritis

### 2.1 Alur Unggahan Massal (1-2000 Foto) Direct-to-S3 & Pemrosesan AI Asinkronus (DEC-Q03)
Alur ini mendemonstrasikan bagaimana fotografer mengunggah hingga 2000 foto secara paralel tanpa membebani memori backend Go:

```mermaid
sequenceDiagram
    autonumber
    actor Creator as Fotografer (Web Studio)
    participant GoAPI as Backend Go Core
    participant DB as PostgreSQL 16
    participant S3 as MinIO / AWS S3
    participant RMQ as RabbitMQ
    participant AI as Python AI Worker
    participant Qdrant as Qdrant Vector DB

    Creator->>GoAPI: POST /storage/upload-batches/initiate (Total 500 files, 7.5 GB)
    GoAPI->>DB: SELECT quota FOR UPDATE (Cek kapasitas & kunci baris atomik)
    DB-->>GoAPI: Quota cukup (Sisa: 45 GB)
    GoAPI->>DB: INSERT INTO quota_reservations (Status: HELD, 7.5 GB)
    GoAPI-->>Creator: 201 Created (upload_batch_id, reservation valid 24h)

    loop Setiap Berkas (Paralel di Peramban)
        Creator->>GoAPI: POST /storage/multipart/initiate & sign-part
        GoAPI-->>Creator: Presigned PUT URL S3
        Creator->>S3: PUT Chunk Direct-to-S3 (Bypass Go Gateway)
        Creator->>GoAPI: POST /storage/multipart/complete (ETag, Checksum)
        GoAPI->>DB: INSERT INTO photos & photo_assets (Status: PROCESSING)
        GoAPI->>RMQ: Publish Job {photo_id, object_key, event_id}
    end

    Creator->>GoAPI: POST /storage/upload-batches/{id}/finalize
    GoAPI->>DB: Rekonsiliasi kuota: Kurangi reserved_bytes, Tambah billable_bytes
    GoAPI-->>Creator: 200 OK (Batch Complete)

    RMQ->>AI: Consume Job (photo_id)
    AI->>S3: Download Raw Master
    AI->>AI: InsightFace: Deteksi Wajah -> Ekstrak 512-dim Embedding
    AI->>AI: EasyOCR: Deteksi Angka BIB Dada Pelari
    AI->>AI: Render Server-Side Watermark Mikro (Destruktif)
    AI->>S3: Upload watermarked_preview & thumbnail
    AI->>Qdrant: Upsert Points {point_id, vector: 512f, payload: {photo_id}}
    AI->>RMQ: Publish JobCompleted {photo_id, bibs: ['10452'], faces: 2}
    RMQ->>GoAPI: Consume JobCompleted
    GoAPI->>DB: UPDATE photos (status: PUBLISHED), INSERT bib_tags & face_refs
```

---

### 2.2 Alur Proteksi Pencarian & Checkout Berbasis BIB (DEC-Q02)
Menjamin nomor BIB tidak disalahgunakan untuk membeli foto milik orang lain tanpa izin:

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Pelari / Pembeli
    participant GoAPI as Backend Go Core
    participant DB as PostgreSQL 16
    participant WA as WhatsApp Gateway

    Buyer->>GoAPI: POST /search/bib {bib_number: '10452', event_id: '...'}
    GoAPI->>DB: SELECT photos WHERE bib = '10452'
    DB-->>GoAPI: Ditemukan 8 foto (Watermarked preview)
    GoAPI-->>Buyer: 200 OK (8 foto, bib_checkout_requires_verification: true)

    Buyer->>GoAPI: POST /cart/items (Pilih 2 foto BIB 10452)
    GoAPI-->>Buyer: 201 Created

    Buyer->>GoAPI: POST /orders (Mencoba checkout)
    GoAPI->>GoAPI: Evaluasi item: Terdeteksi foto terikat BIB tanpa grant!
    GoAPI-->>Buyer: 403 Forbidden (BIB_VERIFICATION_REQUIRED)

    Buyer->>GoAPI: POST /checkout/verify-bib/initiate {bib_number: '10452'}
    GoAPI->>DB: Cek registrasi peserta lomba untuk BIB 10452
    DB-->>GoAPI: Ditemukan nomor HP: +62812****7890
    GoAPI->>WA: Kirim kode OTP 6-digit ke HP peserta
    GoAPI-->>Buyer: 200 OK (OTP Dikirim, challenge_id)

    Buyer->>GoAPI: POST /checkout/verify-bib/confirm {challenge_id, code: '849201'}
    GoAPI->>DB: Validasi hash OTP & batas percobaan (< 3x)
    GoAPI->>DB: INSERT INTO checkout_verification_grants (valid 30 min)
    GoAPI-->>Buyer: 200 OK (grant_token: 'chk_grant_7812bc...')

    Buyer->>GoAPI: POST /orders {photo_ids, bib_grants: ['chk_grant_7812bc...']}
    GoAPI->>DB: Validasi grant_token aktif & cocok
    GoAPI->>DB: INSERT INTO orders (Status: PENDING_PAYMENT)
    GoAPI-->>Buyer: 201 Created (order_number: 'GLK-20261015-0012')
```

---

### 2.3 Alur Pembayaran, Unduhan Non-Hi-Res Seketika, dan Pemenuhan SLA 48 Jam (DEC-Q01)

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Pembeli
    actor Creator as Fotografer
    participant GoAPI as Backend Go Core
    participant PayGW as Payment Gateway (Midtrans)
    participant DB as PostgreSQL 16
    participant S3 as MinIO / S3 Private Vault

    Buyer->>GoAPI: POST /payments/initiate {order_id: 'ord01', method: 'QRIS'}
    GoAPI->>PayGW: Create Transaction (Rp 90.000)
    PayGW-->>GoAPI: QRIS String & Transaction ID
    GoAPI-->>Buyer: 201 Created (Tampilkan QRIS)

    Buyer->>PayGW: Bayar melalui Mobile Banking BCA
    PayGW->>GoAPI: Webhook POST /webhooks/payments/midtrans (settlement)
    GoAPI->>DB: Validasi signature webhook & INSERT webhook_inbox
    GoAPI->>DB: UPDATE orders SET status = 'PAID'
    GoAPI->>DB: INSERT INTO fulfillment_jobs (hires_due_at: NOW() + 48 Hours)
    GoAPI->>DB: Snapshot 3-Way Split (order_revenue_splits)
    GoAPI->>DB: Catat Buku Besar: DEBIT Escrow Gateway, CREDIT Creator & Host Payable
    GoAPI-->>PayGW: 200 OK

    Buyer->>GoAPI: POST /downloads/grant/oi01 {requested_asset: 'NON_HIRES_DOWNLOAD'}
    Note over Buyer,GoAPI: Master Hi-Res belum siap, pembeli dapat Non-Hi-Res ber-watermark mikro (DEC-Q01)
    GoAPI->>DB: Cek order status = PAID -> Terbitkan download grant Non-Hi-Res
    GoAPI-->>Buyer: 200 OK (Signed Download Token)
    Buyer->>GoAPI: GET /downloads/file/{token}
    GoAPI->>S3: Stream rasterized micro-watermarked file
    GoAPI-->>Buyer: File Biner Gambar Diterima Pembeli

    Note over Creator,GoAPI: Dalam batas waktu < 48 jam fotografer mengunggah master Hi-Res
    Creator->>GoAPI: POST /fulfillment/upload-master/oi01 {storage_key, checksum}
    GoAPI->>DB: UPDATE fulfillment_jobs SET status = 'FULFILLED'
    GoAPI->>DB: Notifikasi pembeli: 'Master Hi-Res Sudah Tersedia!'
    Buyer->>GoAPI: POST /downloads/grant/oi01 {requested_asset: 'MASTER_HIRES'}
    GoAPI-->>Buyer: 200 OK (Signed Download Token Master Hi-Res Bersih)
```

---

### 2.4 Alur Klaim Refund Otomatis Jika SLA 48 Jam Terlampaui (OPEN-01 & OPEN-02)

```mermaid
sequenceDiagram
    autonumber
    actor Buyer as Pembeli
    participant Worker as SLA Monitoring Cron Worker
    participant GoAPI as Backend Go Core
    participant DB as PostgreSQL 16
    participant PayGW as Payment Gateway Refund API

    Note over Worker: Cron mengevaluasi: NOW() > hires_due_at AND status = 'WAITING_MASTER'
    Worker->>DB: Tandai fulfillment_job sebagai 'SLA_BREACHED'
    Worker->>DB: Kirim notifikasi pembeli: 'Pesanan berhak klaim refund 100%'

    Buyer->>GoAPI: POST /refunds/request {order_item_id: 'oi01', reason: 'SLA_BREACH_HIRES'}
    GoAPI->>DB: Cek status SLA_BREACHED -> Persetujuan Otomatis (OPEN-02)
    GoAPI->>DB: INSERT INTO refund_requests & refund_items (100% nominal)
    GoAPI->>PayGW: POST /refund (Midtrans / Xendit API)
    PayGW-->>GoAPI: Refund Disetujui Gateway
    GoAPI->>DB: Batalkan download grant Hi-Res & expired-kan Non-Hi-Res
    GoAPI->>DB: Jurnal Balik Pembukuan: DEBIT Creator & Host Payable, CREDIT Escrow
    GoAPI-->>Buyer: 201 Created (Dana dikembalikan ke rekening/e-wallet asal)
```

---

### 2.5 Alur Penarikan Persetujuan Biometrik (UU PDP Right to be Forgotten)

```mermaid
sequenceDiagram
    autonumber
    actor User as Pengguna / Pemilik Wajah
    participant GoAPI as Backend Go Core
    participant DB as PostgreSQL 16
    participant RMQ as RabbitMQ
    participant AI as Biometric Purge Worker
    participant Qdrant as Qdrant Vector DB

    User->>GoAPI: DELETE /biometrics/consent {reason: 'Tarik izin wajah'}
    GoAPI->>DB: INSERT INTO consent_audit_logs (event_type: 'CONSENT_REVOKED')
    GoAPI->>DB: UPDATE biometric_subjects SET processing_status = 'PURGED'
    GoAPI->>DB: INSERT INTO biometric_deletion_jobs (status: 'PROCESSING')
    GoAPI->>RMQ: Publish BiometricPurgeCommand {subject_id, user_id}
    GoAPI-->>User: 202 Accepted (Pembersihan data sedang berjalan)

    RMQ->>AI: Consume BiometricPurgeCommand
    AI->>DB: Ambil seluruh point_id di tabel face_vector_refs untuk subject_id ini
    AI->>Qdrant: DELETE /collections/galeriku_faces_v1/points (Hapus seluruh vektor)
    Qdrant-->>AI: Vektor berhasil dihapus permanen
    AI->>DB: UPDATE face_vector_refs SET status = 'PURGED'
    AI->>DB: UPDATE biometric_deletion_jobs SET status = 'COMPLETED'
    Note over DB: Foto asli di photos TIDAK dihapus; hanya referensi biometrik yang dihapus
```

---

## 3. Matriks Keamanan dan Model Ancaman (STRIDE Threat Model)

| Kategori Ancaman | Deskripsi Potensi Serangan di Galeriku | Mitigasi Arsitektur & Aturan Keamanan |
|---|---|---|
| **Spoofing (Pemalsuan Identitas)** | Pelaku mengaku sebagai peserta lomba untuk mengunduh foto orang lain via pencarian BIB. | **DEC-Q02**: Otorisasi checkout BIB wajib memasukkan kode OTP nomor HP terdaftar peserta atau kode event unik. |
| **Tampering (Manipulasi Data)** | Fotografer atau pembeli memanipulasi payload HTTP untuk mengubah harga foto atau persentase komisi bagi hasil. | **Zero-Trust Backend**: Harga diambil murni dari database master. Snapshot pembagian komisi 3 pihak dihitung di backend. |
| **Repudiation (Penyangkalan Tindakan)** | Host Admin menyangkal telah mencairkan dana payout atau fotografer menyangkal telah mengunggah master rusak. | **Immutable Audit Trail**: Semua tindakan administratif dicatat di `admin_actions`. Mutasi saldo dicatat di `ledger_entries`. |
| **Information Disclosure (Kebocoran Data)** | Scraping URL master foto resolusi penuh tanpa membayar atau pencurian data wajah biometrik peserta. | **S3 Private Vault**: Master Hi-Res disimpan di private bucket; download hanya via short-lived signed URL (15 menit). Raw selfie dihapus dari memori. |
| **Denial of Service (DoS / DDoS)** | Flooding upload ribuan berkas palsu untuk menghabiskan kapasitas penyimpanan server atau kehabisan memori Go. | **Direct-to-S3 Multipart + Atomic Quota Hold (DEC-Q03)**: Sesi upload divalidasi dan dikunci kapasitasnya sebelum presigned URL diterbitkan. |
| **Elevation of Privilege (Eskalasi Hak Akses)** | Anggota tim Host Admin mencoba mengakses rekening bank organisasi atau mengubah kepemimpinan Host Leader. | **Role-Based Capability Bounds**: Host Admin dibatasi kuotanya (`max_admin_count`) dan dilarang mengubah data payout atau rekening bank. |

---

## 4. Keandalan, Skalabilitas, dan Disaster Recovery

### 4.1 Skalabilitas Horizontal Komponen
- **Go Core API Nodes**: Bersifat sepenuhnya *stateless*. Dapat diskalakan horizontal secara otomatis (Kubernetes HPA) berdasarkan utilisasi CPU (> 70%) atau koneksi aktif HTTP.
- **Python AI Workers**: Bekerja berdasarkan *competing consumer pattern* pada RabbitMQ. Dapat diskalakan dari 2 worker hingga puluhan worker sesuai antrean unggahan foto setelah event besar selesai.
- **Database Read Replicas**: Beban kueri penjelajahan katalog foto dan pencarian BIB publik dialihkan ke PostgreSQL Read Replica melalui PgBouncer connection pooler.

### 4.2 Disaster Recovery & Target SLA
- **RPO (Recovery Point Objective)**: < 5 menit melalui PostgreSQL continuous WAL archiving ke S3 bucket lintas zona (*cross-region replication*).
- **RTO (Recovery Time Objective)**: < 1 jam melalui infrastruktur *Infrastructure-as-Code* (Terraform / Docker Compose) terautomasi.
- **Zero Data Loss on Crash**: Pola *Transactional Outbox* menjamin seluruh pesan ke RabbitMQ disimpan dalam transaksi PostgreSQL yang sama sebelum dipublikasikan.

---
*Dokumen ini disetujui sebagai cetak biru arsitektur teknis resmi pengembangan platform Galeriku.*