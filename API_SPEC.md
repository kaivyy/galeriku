# Galeriku — API Specification & Contract Architecture
**Sumber Kebutuhan:** PRD.md (Versi 2.2 Enhanced Master Blueprint)
**Dokumen Pendukung:** PRD_REVIEW.md, UI_UX_SPEC.md, DATABASE_SPEC.md
**Standar Protokol:** RESTful API over HTTPS (TLS 1.3), JSON Payload, OpenAPI 3.1 Architecture
**Tanggal Dokumen:** 15 Oktober 2026
**Status:** Architectural Master Specification — Baseline Tahap 4 Persiapan Development

> [!IMPORTANT]
> **Aturan Mutlak Kontrak API Backend Galeriku:**
> 1. **Zero-Trust Backend**: Seluruh harga final, pembagian komisi 3 pihak (Platform, Host, Creator), validasi kuota penyimpanan, status pembayaran, dan penerbitan izin unduh (*download grants*) dihitung dan divalidasi otoritatif di backend Go. Klien frontend/mobile dilarang keras mengirimkan nominal harga atau status transaksi.
> 2. **Idempotency Key**: Semua operasi mutasi yang sensitif (pembuatan pesanan, inisiasi pembayaran, eksekusi refund, inisiasi upload batch massal) **WAJIB** menyertakan header `Idempotency-Key: <UUIDv7>` untuk mencegah eksekusi ganda akibat retry jaringan.
> 3. **Pemisahan Biometrik (UU PDP No. 27/2022)**: Data wajah selfie untuk pencarian visual tidak disimpan permanen di database relasional. Endpoint selfie search menerima file sementara, melakukan inferensi ekstraksi embedding, membandingkan dengan indeks Qdrant, dan segera menghapus berkas sementara dari memori/disk.
> 4. **Nilai Moneter Integer Rupiah (BIGINT)**: Seluruh atribut nilai moneter dikirimkan dalam integer Rupiah bulat (misal: Rp 45.000 dikirimkan sebagai `45000`). Dilarang keras menggunakan float/double.
> 5. **Proteksi Checkout BIB (DEC-Q02)**: Pencarian publik nomor BIB hanya untuk discovery/preview. Eksekusi checkout wajib menyertakan token otorisasi dari verifikasi kode event atau OTP peserta.
> 6. **Watermark Mikro Server-Side (DEC-Q01)**: File Non-Hi-Res yang diunduh pasca-bayar sebelum master Hi-Res tersedia dirasterisasi server-side dengan watermark mikro.
> 7. **Reservasi Kuota Atomik (DEC-Q03)**: Sesi unggahan massal wajib melalui preflight reservasi kuota backend sebelum presigned URL S3 diterbitkan.

---

## 1. Konvensi Global dan Desain Protokol

### 1.1 Base URL dan Gateway
- **Production Gateway**: `https://api.galeriku.com/api/v1`
- **Staging Gateway**: `https://staging-api.galeriku.com/api/v1`
- **Local Development**: `http://localhost:8080/api/v1`

### 1.2 Header Standar Permintaan (Request Headers)
| Header | Wajib / Opsional | Tipe Data | Deskripsi |
|---|:---:|---|---|
| `Content-Type` | Wajib | String | `application/json` (kecuali multipart/form-data pada selfie upload) |
| `Accept` | Wajib | String | `application/json` |
| `Authorization` | Kondisional | String | `Bearer <JWT_TOKEN>` untuk seluruh rute terproteksi |
| `Idempotency-Key` | Kondisional | UUIDv7 | Wajib untuk request POST/PUT yang berstatus state-mutating |
| `X-Client-Platform` | Wajib | String | `flutter-android`, `flutter-ios`, `web-storefront`, `creator-studio`, `host-portal`, `admin-portal` |
| `X-Client-Version` | Wajib | String | Versi semantik aplikasi klien (e.g. `1.0.0+12`) |
| `X-Request-ID` | Opsional | UUID | ID pelacakan kueri terdistribusi (OpenTelemetry trace context) |

### 1.3 Format Amplop Respons Standar (Response Envelope)
#### Format Berhasil (Single Object):
```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "timestamp": "2026-10-15T14:30:00Z",
    "request_id": "01927c8b-5a12-7800-b611-e2345678abcd"
  }
}
```

#### Format Berhasil dengan Paginasi (Paginated List):
```json
{
  "success": true,
  "data": [ ... ],
  "meta": {
    "page": 1,
    "limit": 20,
    "total_records": 1420,
    "total_pages": 71,
    "timestamp": "2026-10-15T14:30:00Z",
    "request_id": "01927c8b-5a12-7800-b611-e2345678abcd"
  }
}
```

#### Format Galat Terstruktur (Error Envelope):
```json
{
  "success": false,
  "error": {
    "code": "BIB_VERIFICATION_REQUIRED",
    "message": "Pencarian BIB publik tidak mengizinkan checkout langsung. Diperlukan verifikasi OTP kepemilikan nomor peserta.",
    "details": {
      "bib_number": "10452",
      "event_id": "01927c8b-5a12-7800-b611-d1029471abcd"
    }
  },
  "meta": {
    "timestamp": "2026-10-15T14:30:00Z",
    "request_id": "01927c8b-5a12-7800-b611-e2345678abcd"
  }
}
```

---

## 2. Rincian 78 Endpoint RESTful Berdasarkan 8 Domain

### Domain 01: Identity, Auth & Host B2B Portal
Domain ini menangani manajemen sesi, verifikasi multi-faktor, pendaftaran organisasi Host, dan manajemen tim admin.

#### `POST /api/v1/auth/register`
**Deskripsi:** Pendaftaran akun baru (Pembeli / Fotografer / Calon Host)  
**Otorisasi:** `None (Public)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "email": "user@example.com",
  "phone_e164": "+6281234567890",
  "password": "StrongPassword123!",
  "display_name": "Budi Santoso"
}
```
**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "email_normalized": "user@example.com",
  "display_name": "Budi Santoso",
  "capabilities": ["BUYER"]
}
```

#### `POST /api/v1/auth/login`
**Deskripsi:** Autentikasi kredensial pengguna dan penerbitan token JWT  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "identifier": "user@example.com",
  "password": "StrongPassword123!"
}
```
**Contoh Response Body:**
```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "eyJhbGciOi...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "user": {
    "id": "01927c8b-5a12-7800-b611-d1029471abcd",
    "display_name": "Budi Santoso",
    "capabilities": ["BUYER", "CREATOR"]
  }
}
```

#### `POST /api/v1/auth/refresh`
**Deskripsi:** Pembaruan access token menggunakan refresh token  
**Otorisasi:** `None (Refresh Bearer)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "refresh_token": "eyJhbGciOi..."
}
```
**Contoh Response Body:**
```json
{
  "access_token": "eyJhbGciOi...",
  "expires_in": 3600
}
```

#### `POST /api/v1/auth/otp/send`
**Deskripsi:** Pengiriman kode OTP via WhatsApp/SMS untuk verifikasi nomor  
**Otorisasi:** `Bearer (User) / Public` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "phone_e164": "+6281234567890",
  "purpose": "PHONE_VERIFICATION"
}
```
**Contoh Response Body:**
```json
{
  "challenge_id": "01927c8b-5a12-7800-b611-f1122334abcd",
  "expires_in_seconds": 180,
  "delivery_channel": "WHATSAPP"
}
```

#### `POST /api/v1/auth/otp/verify`
**Deskripsi:** Validasi kode OTP yang dimasukkan pengguna  
**Otorisasi:** `Bearer (User) / Public` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "challenge_id": "01927c8b-5a12-7800-b611-f1122334abcd",
  "otp_code": "482910"
}
```
**Contoh Response Body:**
```json
{
  "verified": true,
  "verification_token": "vtok_98ab712c4180..."
}
```

#### `GET /api/v1/auth/me`
**Deskripsi:** Mendapatkan data sesi aktif pengguna beserta daftar kapabilitas dan keanggotaan  
**Otorisasi:** `Bearer (Any User)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "display_name": "Budi Santoso",
  "email_normalized": "user@example.com",
  "capabilities": ["BUYER", "CREATOR", "HOST_LEADER"],
  "host_memberships": [
    {
      "organization_id": "01927c8b-5a12-7800-b611-aaaa11112222",
      "organization_name": "Jakarta Marathon EO",
      "role": "LEADER"
    }
  ]
}
```

#### `GET /api/v1/users/profile`
**Deskripsi:** Mendapatkan profil publik pengguna  
**Otorisasi:** `Bearer (Any User)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "display_name": "Budi Santoso",
  "avatar_url": "https://cdn.galeriku.com/avatars/user123.jpg",
  "locale": "id-ID"
}
```

#### `PUT /api/v1/users/profile`
**Deskripsi:** Memperbarui profil publik pengguna  
**Otorisasi:** `Bearer (Any User)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "display_name": "Budi Santoso Pro",
  "locale": "id-ID"
}
```
**Contoh Response Body:**
```json
{
  "display_name": "Budi Santoso Pro",
  "locale": "id-ID",
  "updated_at": "2026-10-15T14:35:00Z"
}
```

#### `POST /api/v1/hosts/apply`
**Deskripsi:** Pengajuan permohonan pendaftaran organisasi Host B2B  
**Otorisasi:** `Bearer (User)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "legal_name": "PT Generasi Olahraga Hebat",
  "display_name": "Jakarta Marathon Organizer",
  "business_field": "Sport Event Organizer",
  "address_text": "Jl. Sudirman Kav 20, Jakarta Selatan"
}
```
**Contoh Response Body:**
```json
{
  "application_id": "01927c8b-5a12-7800-b611-b11122223333",
  "status": "SUBMITTED",
  "submitted_at": "2026-10-15T14:36:00Z"
}
```

#### `GET /api/v1/hosts/applications/my`
**Deskripsi:** Memeriksa status pengajuan Host  
**Otorisasi:** `Bearer (User)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "application_id": "01927c8b-5a12-7800-b611-b11122223333",
  "status": "SUBMITTED",
  "legal_name": "PT Generasi Olahraga Hebat",
  "documents_uploaded": 2
}
```

#### `POST /api/v1/hosts/documents`
**Deskripsi:** Mengunggah dokumen legalitas Host (KTP/NPWP/NIB) ke private S3  
**Otorisasi:** `Bearer (User)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "application_id": "01927c8b-5a12-7800-b611-b11122223333",
  "document_type": "NIB_OSS",
  "file_name": "nib_oss_pt_generasi.pdf"
}
```
**Contoh Response Body:**
```json
{
  "document_id": "01927c8b-5a12-7800-b611-c11122223333",
  "upload_url": "https://vault.galeriku.com/documents/0192.../nib.pdf?X-Amz-Signature=..."
}
```

#### `GET /api/v1/hosts/organization`
**Deskripsi:** Mendapatkan detail organisasi Host yang dikelola  
**Otorisasi:** `Bearer (Host Leader/Admin)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-aaaa11112222",
  "legal_name": "PT Generasi Olahraga Hebat",
  "display_name": "Jakarta Marathon EO",
  "verification_status": "APPROVED",
  "max_admin_count": 5,
  "current_admin_count": 2
}
```

#### `POST /api/v1/hosts/invitations`
**Deskripsi:** Mengundang anggota baru ke tim Host (Leader only)  
**Otorisasi:** `Bearer (Host Leader)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "invited_email": "admin2@sporteo.com",
  "membership_role": "ADMIN"
}
```
**Contoh Response Body:**
```json
{
  "invitation_id": "01927c8b-5a12-7800-b611-e11122223333",
  "status": "PENDING",
  "expires_at": "2026-10-18T14:36:00Z"
}
```

#### `POST /api/v1/hosts/invitations/accept`
**Deskripsi:** Menerima undangan keanggotaan tim Host  
**Otorisasi:** `Bearer (Invited User)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "invitation_token": "inv_sec_892173abcdef..."
}
```
**Contoh Response Body:**
```json
{
  "membership_id": "01927c8b-5a12-7800-b611-f11122223333",
  "organization_name": "Jakarta Marathon EO",
  "role": "ADMIN",
  "status": "ACTIVE"
}
```

---

### Domain 02: Events, FotoTree & Photo Catalog
Domain ini menangani hirarki FotoTree, pencarian event, penjelajahan katalog foto, dan moderasi konten.

#### `GET /api/v1/events`
**Deskripsi:** Daftar event publik dengan filter kota, tanggal, dan status  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "id": "01927c8b-5a12-7800-b611-d1029471abcd",
    "slug": "jakarta-marathon-2026",
    "name": "Jakarta Marathon 2026",
    "event_type": "RUNNING_MARATHON",
    "starts_at": "2026-10-25T05:00:00Z",
    "photo_count": 18450
  }
]
```

#### `GET /api/v1/events/{slug}`
**Deskripsi:** Detail lengkap gelaran event beserta struktur sesi FotoTree  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "slug": "jakarta-marathon-2026",
  "name": "Jakarta Marathon 2026",
  "host_name": "Jakarta Marathon EO",
  "starts_at": "2026-10-25T05:00:00Z",
  "sessions": [
    {
      "id": "01927c8b-5a12-7800-b611-s01",
      "name": "Start Line 42K",
      "session_type": "START_LINE"
    },
    {
      "id": "01927c8b-5a12-7800-b611-s02",
      "name": "Finish Gate Monas",
      "session_type": "FINISH_LINE"
    }
  ]
}
```

#### `POST /api/v1/events`
**Deskripsi:** Membuat event FotoTree baru (Host Leader/Admin)  
**Otorisasi:** `Bearer (Host Leader/Admin)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "name": "Jakarta Marathon 2026",
  "slug": "jakarta-marathon-2026",
  "event_type": "RUNNING_MARATHON",
  "starts_at": "2026-10-25T05:00:00Z",
  "ends_at": "2026-10-25T13:00:00Z"
}
```
**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "slug": "jakarta-marathon-2026",
  "status": "DRAFT",
  "created_at": "2026-10-15T14:40:00Z"
}
```

#### `PUT /api/v1/events/{id}`
**Deskripsi:** Memperbarui data dan jadwal event  
**Otorisasi:** `Bearer (Host Leader/Admin)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "status": "PUBLISHED"
}
```
**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "status": "PUBLISHED",
  "updated_at": "2026-10-15T14:41:00Z"
}
```

#### `POST /api/v1/events/{id}/sessions`
**Deskripsi:** Menambahkan sesi / sub-cabang FotoTree pada event  
**Otorisasi:** `Bearer (Host Leader/Admin)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "name": "KM 21 Halfway Point",
  "session_type": "TRACK_KM21",
  "starts_at": "2026-10-25T06:00:00Z",
  "ends_at": "2026-10-25T10:00:00Z"
}
```
**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-s03",
  "event_id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "name": "KM 21 Halfway Point"
}
```

#### `POST /api/v1/events/{id}/creators/apply`
**Deskripsi:** Fotografer mendaftar meliput event (INTERNAL/INDEPENDENT)  
**Otorisasi:** `Bearer (Creator)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "membership_type": "INDEPENDENT",
  "notes": "Fotografer spesialis area finish gate"
}
```
**Contoh Response Body:**
```json
{
  "membership_id": "01927c8b-5a12-7800-b611-cm01",
  "status": "ACTIVE",
  "approved_at": "2026-10-15T14:42:00Z"
}
```

#### `GET /api/v1/events/{id}/photos`
**Deskripsi:** Katalog foto publik dalam event dengan filter sesi, fotografer, dan urutan waktu  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "id": "01927c8b-5a12-7800-b611-p01",
    "preview_watermark_url": "https://cdn.galeriku.com/previews/wm_p01.jpg",
    "thumbnail_url": "https://cdn.galeriku.com/thumbs/th_p01.jpg",
    "photographer_name": "Ahmad Dani",
    "price_idr": 45000,
    "bib_tags": ["10452"],
    "taken_at": "2026-10-25T06:45:12Z"
  }
]
```

#### `GET /api/v1/photos/{id}`
**Deskripsi:** Detail foto publik (menampilkan resolusi preview ber-watermark mikro)  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "id": "01927c8b-5a12-7800-b611-p01",
  "event_name": "Jakarta Marathon 2026",
  "preview_watermark_url": "https://cdn.galeriku.com/previews/wm_p01.jpg",
  "price_idr": 45000,
  "photographer": {
    "id": "01927c8b-5a12-7800-b611-u02",
    "display_name": "Ahmad Dani"
  },
  "bib_tags": ["10452"],
  "has_faces_detected": true,
  "is_master_available": true
}
```

#### `GET /api/v1/photos/feed`
**Deskripsi:** Feed eksplorasi publik GaleriGram  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "id": "01927c8b-5a12-7800-b611-p01",
    "event_slug": "jakarta-marathon-2026",
    "thumbnail_url": "https://cdn.galeriku.com/thumbs/th_p01.jpg",
    "price_idr": 45000
  }
]
```

#### `POST /api/v1/photos/{id}/report`
**Deskripsi:** Melaporkan konten foto melanggar moderasi atau privasi  
**Otorisasi:** `Bearer (User) / Public` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "reason_code": "PRIVACY_CONCERN",
  "notes": "Foto ini menampilkan wajah saya tanpa izin dalam konteks tidak pantas"
}
```
**Contoh Response Body:**
```json
{
  "report_id": "01927c8b-5a12-7800-b611-rep01",
  "status": "UNDER_REVIEW"
}
```

---

### Domain 03: AI Biometrics, Face Search & BIB OCR
Domain ini menangani persetujuan biometrik UU PDP, pencarian visual wajah via Qdrant, pencarian nomor BIB, dan penarikan persetujuan (*Right to be Forgotten*).

#### `POST /api/v1/biometrics/consent`
**Deskripsi:** Pencatatan persetujuan biometrik eksplisit pengguna (UU PDP)  
**Otorisasi:** `Bearer (User)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "consent_purpose": "FACE_SEARCH_DISCOVERY",
  "consent_version": "v1.0-202610"
}
```
**Contoh Response Body:**
```json
{
  "consent_id": "01927c8b-5a12-7800-b611-con01",
  "is_granted": true,
  "timestamp": "2026-10-15T14:45:00Z"
}
```

#### `DELETE /api/v1/biometrics/consent`
**Deskripsi:** Penarikan persetujuan biometrik & pemicuan penghapusan data wajah  
**Otorisasi:** `Bearer (User)` | **Status Kode:** `202 Accepted`

**Contoh Request Body:**
```json
{
  "reason": "Pengguna memilih menarik data biometrik dari pencarian publik"
}
```
**Contoh Response Body:**
```json
{
  "deletion_job_id": "01927c8b-5a12-7800-b611-del01",
  "status": "PROCESSING",
  "message": "Pembersihan vektor di Qdrant dan referensi database sedang dijalankan secara asinkronus."
}
```

#### `POST /api/v1/search/face`
**Deskripsi:** Pencarian foto berbasis unggahan selfie sementara (AI Cosine Match Qdrant)  
**Otorisasi:** `Bearer / Public Consent` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
multipart/form-data: [field: selfie_file (image/jpeg), field: event_id (optional)]
```
**Contoh Response Body:**
```json
{
  "search_session_id": "01927c8b-5a12-7800-b611-ss01",
  "faces_detected_in_selfie": 1,
  "candidates_count": 14,
  "photos": [
    {
      "photo_id": "01927c8b-5a12-7800-b611-p01",
      "similarity_score": 0.8842,
      "preview_watermark_url": "https://cdn.galeriku.com/previews/wm_p01.jpg",
      "price_idr": 45000
    }
  ]
}
```

#### `POST /api/v1/search/bib`
**Deskripsi:** Pencarian foto berdasarkan nomor dada pelari (BIB)  
**Otorisasi:** `None (Public)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "bib_number": "10452",
  "event_id": "01927c8b-5a12-7800-b611-d1029471abcd"
}
```
**Contoh Response Body:**
```json
{
  "bib_normalized": "10452",
  "total_photos": 8,
  "photos": [
    {
      "photo_id": "01927c8b-5a12-7800-b611-p01",
      "preview_watermark_url": "https://cdn.galeriku.com/previews/wm_p01.jpg",
      "price_idr": 45000,
      "bib_checkout_requires_verification": true
    }
  ]
}
```

#### `POST /api/v1/photos/{id}/bib-tags`
**Deskripsi:** Fotografer atau Admin mengoreksi nomor BIB foto  
**Otorisasi:** `Bearer (Creator/Admin)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "bib_normalized": "10452",
  "review_status": "CONFIRMED"
}
```
**Contoh Response Body:**
```json
{
  "tag_id": "01927c8b-5a12-7800-b611-bt01",
  "photo_id": "01927c8b-5a12-7800-b611-p01",
  "bib_normalized": "10452",
  "status": "CONFIRMED"
}
```

---

### Domain 04: Cart, Orders & Protected Checkout
Domain ini menangani keranjang belanja, tantangan verifikasi anti-fraud checkout BIB (DEC-Q02), dan pembuatan pesanan.

#### `GET /api/v1/cart`
**Deskripsi:** Mendapatkan isi keranjang belanja pembeli aktif  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "cart_id": "01927c8b-5a12-7800-b611-cart01",
  "total_items": 2,
  "total_amount_idr": 90000,
  "items": [
    {
      "cart_item_id": "01927c8b-5a12-7800-b611-ci01",
      "photo_id": "01927c8b-5a12-7800-b611-p01",
      "thumbnail_url": "https://cdn.galeriku.com/thumbs/th_p01.jpg",
      "price_idr": 45000
    }
  ]
}
```

#### `POST /api/v1/cart/items`
**Deskripsi:** Menambahkan foto ke dalam keranjang  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "photo_id": "01927c8b-5a12-7800-b611-p01"
}
```
**Contoh Response Body:**
```json
{
  "cart_item_id": "01927c8b-5a12-7800-b611-ci01",
  "photo_id": "01927c8b-5a12-7800-b611-p01",
  "price_idr": 45000
}
```

#### `DELETE /api/v1/cart/items/{id}`
**Deskripsi:** Menghapus item dari keranjang  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "deleted": true,
  "remaining_items": 1
}
```

#### `POST /api/v1/checkout/verify-bib/initiate`
**Deskripsi:** Memicu tantangan verifikasi OTP/kode event untuk checkout foto berbasis BIB (DEC-Q02)  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "bib_number": "10452",
  "event_id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "verification_method": "EVENT_REGISTRATION_OTP"
}
```
**Contoh Response Body:**
```json
{
  "verification_id": "01927c8b-5a12-7800-b611-ver01",
  "target_bib": "10452",
  "destination_hint": "+62 812-****-7890",
  "expires_in_seconds": 1800
}
```

#### `POST /api/v1/checkout/verify-bib/confirm`
**Deskripsi:** Validasi kode OTP peserta lomba dan penerbitan checkout verification grant  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "verification_id": "01927c8b-5a12-7800-b611-ver01",
  "code": "849201"
}
```
**Contoh Response Body:**
```json
{
  "grant_token": "chk_grant_7812bc81...",
  "bib_number": "10452",
  "expires_at": "2026-10-15T15:15:00Z"
}
```

#### `POST /api/v1/orders`
**Deskripsi:** Membuat pesanan pembelian foto (Wajib menyertakan Idempotency-Key dan grant jika foto terikat BIB)  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "photo_ids": [
    "01927c8b-5a12-7800-b611-p01",
    "01927c8b-5a12-7800-b611-p02"
  ],
  "bib_grants": [
    "chk_grant_7812bc81..."
  ]
}
```
**Contoh Response Body:**
```json
{
  "order_id": "01927c8b-5a12-7800-b611-ord01",
  "order_number": "GLK-20261015-0012",
  "total_amount_idr": 90000,
  "currency": "IDR",
  "status": "PENDING_PAYMENT",
  "created_at": "2026-10-15T14:48:00Z"
}
```

#### `GET /api/v1/orders/{id}`
**Deskripsi:** Detail pesanan, rincian foto, dan status ketersediaan master Hi-Res  
**Otorisasi:** `Bearer (Buyer/Creator)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "order_id": "01927c8b-5a12-7800-b611-ord01",
  "order_number": "GLK-20261015-0012",
  "status": "PAID",
  "total_amount_idr": 90000,
  "items": [
    {
      "order_item_id": "01927c8b-5a12-7800-b611-oi01",
      "photo_id": "01927c8b-5a12-7800-b611-p01",
      "unit_price_idr": 45000,
      "fulfillment_status": "WAITING_MASTER",
      "hires_due_at": "2026-10-17T14:48:00Z",
      "can_download_non_hires": true
    }
  ]
}
```

#### `GET /api/v1/orders`
**Deskripsi:** Histori seluruh pesanan pembelian akun pembeli  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "order_id": "01927c8b-5a12-7800-b611-ord01",
    "order_number": "GLK-20261015-0012",
    "total_amount_idr": 90000,
    "status": "PAID",
    "created_at": "2026-10-15T14:48:00Z"
  }
]
```

---

### Domain 05: Payments, Webhooks, Fulfillment & Downloads
Domain ini menangani integrasi payment gateway, webhook pemrosesan pembayaran, pemenuhan master Hi-Res (SLA 48 jam), izin unduh aman (DEC-Q01), dan klaim refund.

#### `POST /api/v1/payments/initiate`
**Deskripsi:** Inisiasi transaksi pembayaran melalui payment gateway (QRIS / VA / E-Wallet)  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "order_id": "01927c8b-5a12-7800-b611-ord01",
  "payment_method": "QRIS"
}
```
**Contoh Response Body:**
```json
{
  "payment_id": "01927c8b-5a12-7800-b611-pay01",
  "provider": "MIDTRANS",
  "qris_string": "00020101021226...",
  "qris_image_url": "https://api.midtrans.com/qris/...",
  "amount_idr": 90000,
  "expires_at": "2026-10-15T15:05:00Z"
}
```

#### `POST /api/v1/webhooks/payments/midtrans`
**Deskripsi:** Penerima webhook resmi dari Midtrans (Divalidasi tanda tangan SHA512)  
**Otorisasi:** `Public Webhook (Signature)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "order_id": "GLK-20261015-0012",
  "transaction_status": "settlement",
  "gross_amount": "90000.00",
  "signature_key": "9812abc...",
  "transaction_id": "mid-tx-99120"
}
```
**Contoh Response Body:**
```json
{
  "status": "RECEIVED",
  "processed": true
}
```

#### `POST /api/v1/downloads/grant/{order_item_id}`
**Deskripsi:** Permintaan tiket unduh aman untuk file Non-Hi-Res ber-watermark mikro atau Master Hi-Res (DEC-Q01)  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "requested_asset": "NON_HIRES_DOWNLOAD"
}
```
**Contoh Response Body:**
```json
{
  "download_token": "dl_tok_8912bc780...",
  "asset_type": "NON_HIRES_DOWNLOAD",
  "watermark_applied": "MICRO_WATERMARK_SERVER_RASTERIZED",
  "download_url": "https://api.galeriku.com/api/v1/downloads/file/dl_tok_8912bc780...",
  "expires_at": "2026-10-15T15:20:00Z"
}
```

#### `GET /api/v1/downloads/file/{token}`
**Deskripsi:** Pengunduhan langsung file biner foto dengan pencatatan IP, user agent, dan byte count  
**Otorisasi:** `None (Token in Path)` | **Status Kode:** `200 OK (Binary stream)`

**Contoh Response Body:**
```json
Binary Image Stream (image/jpeg)
```

#### `POST /api/v1/fulfillment/upload-master/{order_item_id}`
**Deskripsi:** Fotografer mengunggah file master resolusi penuh (*Hi-Res*) untuk menyelesaikan kewajiban SLA  
**Otorisasi:** `Bearer (Creator)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "storage_object_key": "vault/hires/2026/10/p01_master.jpg",
  "sha256_checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}
```
**Contoh Response Body:**
```json
{
  "fulfillment_status": "FULFILLED",
  "fulfilled_at": "2026-10-15T15:00:00Z",
  "hires_download_ready": true
}
```

#### `POST /api/v1/refunds/request`
**Deskripsi:** Pengajuan klaim refund jika fotografer gagal memenuhi master Hi-Res melewati batas SLA 48 jam atau melaporkan MASTER_LOST (OPEN-01, OPEN-02, REF-01 s/d REF-10)  
**Otorisasi:** `Bearer (Buyer)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "order_item_id": "01927c8b-5a12-7800-b611-oi01",
  "reason_code": "HIRES_SLA_BREACH",
  "destination_type": "INTERNAL_WALLET"
}
```
**Contoh Response Body:**
```json
{
  "refund_item_id": "01927c8b-5a12-7800-b611-ref01",
  "order_item_id": "01927c8b-5a12-7800-b611-oi01",
  "eligible_amount_idr": 45000,
  "buyer_credit_amount_idr": 45000,
  "creator_clawback_idr": 31500,
  "host_clawback_idr": 6750,
  "platform_clawback_idr": 6750,
  "gateway_fee_idr": 0,
  "destination_type": "INTERNAL_WALLET",
  "status": "COMPLETED",
  "completed_at": "2026-10-17T15:05:00Z",
  "message": "Dana 100% sebesar Rp 45.000 telah berhasil dikreditkan ke saldo dompet internal pembeli."
}
```

### Domain 06: Finance, Ledger, Settlements & Payouts
Domain ini menangani buku besar berpasangan (*double-entry*), saldo dompet, penahanan settlement (*hold period*), dan pencairan dana ke rekening bank.

#### `GET /api/v1/finance/wallet`
**Deskripsi:** Mendapatkan ringkasan saldo pengguna (Tersedia, Tertahan/Hold, Total Dicairkan)  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "currency": "IDR",
  "available_balance_idr": 4200000,
  "pending_hold_balance_idr": 1850000,
  "total_withdrawn_idr": 12500000
}
```

#### `GET /api/v1/finance/ledger-entries`
**Deskripsi:** Mutasi rekening pembukuan double-entry lengkap untuk transparansi finansial  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "entry_id": "01927c8b-5a12-7800-b611-le01",
    "transaction_type": "ORDER_PAYMENT",
    "direction": "CREDIT",
    "amount_idr": 31500,
    "account_name": "Creator Payable - Budi",
    "posted_at": "2026-10-15T15:00:00Z"
  }
]
```

#### `GET /api/v1/finance/settlements`
**Deskripsi:** Daftar status penahanan dana penjualan (Hold Period T+7 hari)  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "settlement_id": "01927c8b-5a12-7800-b611-set01",
    "gross_idr": 315000,
    "net_payable_idr": 315000,
    "status": "HELD",
    "hold_until": "2026-10-22T15:00:00Z"
  }
]
```

#### `POST /api/v1/finance/payouts/request`
**Deskripsi:** Permintaan penarikan saldo tersedia ke rekening bank terdaftar  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "amount_idr": 2000000,
  "bank_account_id": "01927c8b-5a12-7800-b611-ba01"
}
```
**Contoh Response Body:**
```json
{
  "payout_id": "01927c8b-5a12-7800-b611-po01",
  "payout_reference": "PO-20261015-001",
  "amount_idr": 2000000,
  "status": "PROCESSING",
  "destination_bank": "BCA - 1234567890"
}
```

---

### Domain 07: Storage, Quota & S3 Direct Multipart Upload
Domain ini menangani reservasi kuota atomik (DEC-Q03), sesi upload massal 1-2000 file, presigned URL S3, dan rekonsiliasi kuota.

#### `GET /api/v1/storage/quota`
**Deskripsi:** Mendapatkan status kuota akun penyimpanan (Limit, Terpakai, Direservasi)  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "quota_limit_bytes": 107374182400,
  "billable_bytes": 34827100120,
  "reserved_bytes": 1258291200,
  "remaining_bytes": 71288791080
}
```

#### `POST /api/v1/storage/upload-batches/initiate`
**Deskripsi:** Inisiasi batch upload massal (1 s/d 2000 berkas) dan reservasi kuota atomik (DEC-Q03)  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `201 Created`

**Contoh Request Body:**
```json
{
  "event_id": "01927c8b-5a12-7800-b611-d1029471abcd",
  "total_files": 150,
  "total_bytes": 2250000000,
  "files": [
    {
      "client_file_token": "cli_tok_001",
      "original_filename": "DSC_0001.JPG",
      "expected_bytes": 15000000
    }
  ]
}
```
**Contoh Response Body:**
```json
{
  "upload_batch_id": "01927c8b-5a12-7800-b611-ub01",
  "status": "RESERVED",
  "reserved_bytes": 2250000000,
  "reservation_expires_at": "2026-10-16T15:00:00Z"
}
```

#### `POST /api/v1/storage/multipart/initiate`
**Deskripsi:** Inisiasi sesi upload multipart S3 untuk berkas foto tertentu  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "upload_batch_id": "01927c8b-5a12-7800-b611-ub01",
  "client_file_token": "cli_tok_001"
}
```
**Contoh Response Body:**
```json
{
  "upload_item_id": "01927c8b-5a12-7800-b611-ui01",
  "provider_upload_id": "mp_upload_id_9921bc...",
  "s3_object_key": "raw-uploads/2026/10/ub01/DSC_0001.JPG"
}
```

#### `POST /api/v1/storage/multipart/sign-part`
**Deskripsi:** Mendapatkan Presigned PUT URL S3 langsung untuk bagian part tertentu (Direct-to-S3)  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "upload_item_id": "01927c8b-5a12-7800-b611-ui01",
  "part_number": 1
}
```
**Contoh Response Body:**
```json
{
  "part_number": 1,
  "presigned_url": "https://vault.galeriku.com/raw-uploads/2026/10/ub01/DSC_0001.JPG?partNumber=1&uploadId=...&X-Amz-Signature=..."
}
```

#### `POST /api/v1/storage/multipart/complete`
**Deskripsi:** Menutup sesi upload multipart setelah seluruh part berhasil dikirim ke S3  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "upload_item_id": "01927c8b-5a12-7800-b611-ui01",
  "parts": [
    {
      "part_number": 1,
      "etag": "\"d41d8cd98f00b204e9800998ecf8427e\""
    }
  ],
  "sha256_checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}
```
**Contoh Response Body:**
```json
{
  "upload_item_id": "01927c8b-5a12-7800-b611-ui01",
  "status": "COMPLETED",
  "photo_id": "01927c8b-5a12-7800-b611-p101"
}
```

#### `POST /api/v1/storage/upload-batches/{id}/finalize`
**Deskripsi:** Finalisasi sesi batch unggahan massal & rekonsiliasi pemakaian kuota aktual  
**Otorisasi:** `Bearer (Creator/Host)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
None (Empty POST)
```
**Contoh Response Body:**
```json
{
  "upload_batch_id": "01927c8b-5a12-7800-b611-ub01",
  "status": "COMPLETED",
  "total_files_uploaded": 150,
  "actual_bytes": 2248900120,
  "quota_reconciled": true
}
```

---

### Domain 08: Platform Ops, System Settings & Admin Portal
Domain ini menangani verifikasi persetujuan Host oleh Super Admin, audit log platform, dan pengaturan sistem global.

#### `GET /api/v1/admin/audit-logs`
**Deskripsi:** Mendapatkan log audit keamanan dan mutasi sistem  
**Otorisasi:** `Bearer (Platform Admin)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
[
  {
    "id": "01927c8b-5a12-7800-b611-al01",
    "action_code": "HOST_APPLICATION_APPROVED",
    "actor_user_id": "01927c8b-5a12-7800-b611-admin01",
    "ip_address": "103.24.50.12",
    "occurred_at": "2026-10-15T15:10:00Z"
  }
]
```

#### `POST /api/v1/admin/hosts/{id}/approve`
**Deskripsi:** Menyetujui aplikasi Host Organization dan menerbitkan hak Host Leader  
**Otorisasi:** `Bearer (Platform Admin)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "notes": "Dokumen NIB dan Akta Perusahaan valid"
}
```
**Contoh Response Body:**
```json
{
  "organization_id": "01927c8b-5a12-7800-b611-aaaa11112222",
  "verification_status": "APPROVED",
  "approved_at": "2026-10-15T15:11:00Z"
}
```

#### `POST /api/v1/admin/hosts/{id}/reject`
**Deskripsi:** Menolak pengajuan Host Organization dengan alasan terdokumentasi  
**Otorisasi:** `Bearer (Platform Admin)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "rejection_reason": "Dokumen NPWP tidak sesuai dengan nama badan hukum yang didaftarkan"
}
```
**Contoh Response Body:**
```json
{
  "organization_id": "01927c8b-5a12-7800-b611-aaaa11112222",
  "verification_status": "REJECTED"
}
```

#### `GET /api/v1/admin/settings`
**Deskripsi:** Mendapatkan konfigurasi sistem operasional tingkat platform  
**Otorisasi:** `Bearer (Platform Admin)` | **Status Kode:** `200 OK`

**Contoh Response Body:**
```json
{
  "SLA_HOURS_HIRES": 48,
  "SETTLEMENT_HOLD_DAYS": 7,
  "DEFAULT_PLATFORM_FEE_PCT": 15.00,
  "DEFAULT_HOST_FEE_PCT": 15.00,
  "MAX_BULK_UPLOAD_FILES": 2000
}
```

#### `PUT /api/v1/admin/settings`
**Deskripsi:** Memperbarui konfigurasi sistem operasional platform  
**Otorisasi:** `Bearer (Platform Admin)` | **Status Kode:** `200 OK`

**Contoh Request Body:**
```json
{
  "setting_key": "SETTLEMENT_HOLD_DAYS",
  "value": 7
}
```
**Contoh Response Body:**
```json
{
  "setting_key": "SETTLEMENT_HOLD_DAYS",
  "value": 7,
  "updated_at": "2026-10-15T15:12:00Z"
}
```

---

## 3. Katalog Kode Galat Resmi (System Error Catalog)

| Kode Error (Error Code) | HTTP Status | Penjelasan & Tindakan Pemulihan Klien |
|---|:---:|---|
| `AUTH_UNAUTHORIZED` | 401 | Token JWT tidak ada, kedaluwarsa, atau tidak valid. Klien harus memperbarui token via `/auth/refresh` atau login ulang. |
| `AUTH_FORBIDDEN` | 403 | Pengguna tidak memiliki kapabilitas (*capability*) atau peran yang dipersyaratkan untuk mengakses resource. |
| `BIB_VERIFICATION_REQUIRED` | 403 | Foto memerlukan pembuktian kepemilikan BIB (DEC-Q02). Klien harus meminta verifikasi OTP. |
| `RESOURCE_NOT_FOUND` | 404 | Entitas data yang dicari (event, foto, pesanan, akun) tidak ditemukan di database. |
| `IDEMPOTENCY_CONFLICT` | 409 | Permintaan dengan Idempotency-Key yang sama sedang berjalan atau menghasilkan payload berbeda. |
| `INSUFFICIENT_STORAGE_QUOTA` | 422 | Kapasitas kuota penyimpanan akun tidak mencukupi untuk melakukan reservasi upload batch baru (DEC-Q03). |
| `SLA_NOT_BREACHED` | 422 | Pengajuan komplain refund ditolak karena batas waktu SLA pemenuhan 48 jam belum terlampaui. |
| `ORDER_ALREADY_PAID` | 422 | Upaya pembayaran pesanan yang statusnya sudah berstatus `PAID` atau `SETTLED`. |
| `TOO_MANY_REQUESTS` | 429 | Batas kuota laju pemanggilan API terlampaui. Klien harus menunggu durasi yang tercantum pada header `Retry-After`. |
| `INTERNAL_SERVER_ERROR` | 500 | Terjadi galat komputasi pada server. ID pelacakan (*request_id*) dicatat untuk investigasi tim engineer. |

---

## 4. Kebijakan Rate Limiting & Keamanan API

| Kategori Endpoint | Batas Frekuensi (Rate Limit) | Algoritma & Kebijakan |
|---|:---:|---|
| **Publik / Penjelajahan Katalog** | 60 request / menit per IP | Leaky Bucket dengan token burst 20 |
| **Pengguna Terotentikasi (User)** | 300 request / menit per User ID | Token Bucket berbasis Redis |
| **AI Pencarian Selfie (Face Search)** | 5 request / menit per User ID / IP | Mencegah eksploitasi berlebihan worker komputasi GPU/CPU |
| **Pengiriman Tantangan OTP** | 3 request / 10 menit per Nomor HP | Mencegah pembengkakan biaya SMS/WhatsApp gateway & brute force |
| **Presigned Part Direct-to-S3** | 2000 request / batch unggahan | Diizinkan burst tinggi untuk kelancaran unggahan massal 2000 file |

---
*Dokumen ini disetujui sebagai acuan kontrak API resmi pembangunan Backend dan Klien Galeriku.*