# UI_UX_SPEC.md
## Galeriku — UI/UX Specification

* **Sumber Kebutuhan:** `PRD.md` Versi 2.2 Enhanced Master Blueprint
* **Keputusan Resmi Terkait:** `PRD_REVIEW.md` (`DEC-Q01`, `DEC-Q02`, `DEC-Q03`)
* **Status Dokumen:** Spesifikasi Tahap Desain Arsitektur Interaksi & Antarmuka (Belum merupakan implementasi grafis final)
* **Prinsip Utama:** Semua state, validasi izin, harga, saldo, status transaksi, dan hak unduh bersumber dari backend (*Zero Client Trust*). UI tidak boleh mengarang status transaksi atau menyimpulkan hak akses dari data lokal.

---

## 1. TUJUAN DAN CAKUPAN

Dokumen ini mendefinisikan arsitektur informasi, peta navigasi, alur pengguna (*user flows*), daftar layar komprehensif, variasi state komponen (*loading, empty, error, success, disabled*), sistem token desain, serta *acceptance criteria* untuk **5 kanal akses resmi Galeriku**:
1. **Aplikasi Mobile (Flutter Android & iOS):** Klien lapangan pelari/pengunjung, aktivasi liveness wajah, feed visual GaleriGram, dan proteksi tangkapan layar `FLAG_SECURE`.
2. **Web Storefront Publik (Astro + React Islands):** Portal pembeli tanpa aplikasi (*No-App Buyer Portal*) untuk penelusuran katalog, pencarian nomor BIB, keranjang belanja, checkout QRIS desktop, dan unduhan koleksi langsung via browser.
3. **Creator Web Studio (React + TypeScript SPA):** Studio kerja profesional fotografer dari laptop/PC dengan fitur *Drag-and-Drop* unggah massal hingga **2.000 foto per sesi** via *Direct S3 Multipart Presigned URLs*, ekstraksi EXIF sisi klien, dan pemenuhan batch master Hi-Res (SLA 48 jam).
4. **Host B2B Management Portal (React SPA):** Portal manajemen bagi pengelola tempat/event berbadan hukum (NIB/PT), pendelegasian hingga **5 Host Admin**, rekrutmen fotografer resmi, pengaturan komisi (*Host Fee*), dan pencairan saldo bulanan.
5. **Super Admin Dashboard (React SPA):** Portal internal operasional tim Galeriku untuk verifikasi legalitas Host, moderasi konten, audit kepatuhan UU PDP, penanganan sengketa (*dispute*), dan rekonsiliasi finansial.

---

## 2. PRINSIP FUNDAMENTAL DESAIN UI/UX

1. **Foto Sebagai Subjek Utama:** Antarmuka visual bersifat minimalis, bersih, dan kontras tinggi; elemen UI tidak boleh menutupi subjek wajah atau nomor dada pelari secara berlebihan.
2. **Mobile-First untuk Penemuan, Desktop-First untuk Produksi:**
   * Pengalaman mencari dan membeli foto dioptimalkan untuk perangkat mobile layar sentuh (60–120 fps).
   * Pengalaman fotografer mengunggah ribuan foto (1–2.000 berkas) dan manajemen B2B Host dioptimalkan untuk monitor desktop/laptop.
3. **Zero Biometric Friction pada Onboarding:** Tamu (*Guest*) dapat menjelajahi feed, kalender, peta FotoTree, dan melihat foto pratinjau ber-watermark tanpa dipaksa login atau menyerahkan data biometrik.
4. **Pemisahan Tegas Pencarian BIB vs Checkout:** Nomor BIB dapat digunakan bebas untuk pencarian foto. Namun tahap *Checkout* mewajibkan verifikasi OTP/Kode Event atau verifikasi wajah demi mencegah *stalking*.
5. **Transparansi Status File (Keputusan `DEC-Q01`):**
   * Non-Hi-Res yang langsung diunduh setelah bayar diberi label eksplisit: *"Versi Non-Hi-Res — Memakai Watermark Mikro sampai Master Hi-Res Tersedia"*.
   * Master Hi-Res berstatus privat dan hanya dapat diunduh melalui *Signed URL* berbatas waktu 60 menit setelah fotografer memenuhi pesanan.
6. **Integritas Kuota Dinamis (Keputusan `DEC-Q03`):**
   * Web Studio menghitung ukuran batch via *preflight* browser untuk kenyamanan pengguna, namun backend Go yang menjadi penentu otoritatif reservasi kuota storage.
7. **Kejelasan Aksesibilitas:** Status tidak boleh disampaikan melalui warna saja; wajib disertai ikon dan label teks yang terbaca pembaca layar (*screen reader*).
8. **Kejujuran Keamanan:** UI tidak boleh memberikan klaim palsu bahwa tangkapan layar (*screenshot*) atau penyalinan file dapat dicegah 100% secara mutlak di semua sistem operasi.

---

## 3. STATUS REQUIREMENT

* `P0`: **Wajib untuk MVP** (Peluncuran awal, transaksi aman, privasi patuh).
* `P1`: **Penting** setelah alur utama P0 stabil (Peningkatan operasional dan automasi).
* `P2`: **Dapat Ditunda** (Fitur sosial lanjutan, gamifikasi, merchandise fisik).
* `TBD`: **Memerlukan Keputusan Bisnis/Legal Lanjutan**.

---

## 4. ARSITEKTUR INFORMASI & PETA NAVIGASI (NAVIGATION MAP)

```mermaid
graph TD
    subgraph Kanal 1: Mobile App (Flutter)
        M_Nav[Bottom Navigation 5 Tab]
        M_Nav --> M_Tab1[1. GaleriGram Feed]
        M_Nav --> M_Tab2[2. Cari Foto AI]
        M_Nav --> M_Tab3[3. FotoTree Explorer]
        M_Nav --> M_Tab4[4. Koleksi & Unduhan]
        M_Nav --> M_Tab5[5. Akun, Profil & Dompet]
    end

    subgraph Kanal 2: Web Storefront
        W_Nav[Header Desktop Nav]
        W_Nav --> W_Home[Katalog FotoTree & Event]
        W_Nav --> W_BIB[Pencarian Nomor BIB]
        W_Nav --> W_Cart[Keranjang Belanja]
        W_Nav --> W_Order[Pesanan & Unduh Koleksi]
        W_Nav --> W_Auth[Login / Profil]
    end

    subgraph Kanal 3: Creator Web Studio
        CS_Nav[Sidebar Studio Fotografer]
        CS_Nav --> CS_Overview[Dashboard Performa]
        CS_Nav --> CS_Upload[Unggah Massal 2.000 Foto]
        CS_Nav --> CS_Pending[Fulfillment SLA 48 Jam]
        CS_Nav --> CS_Gallery[Manajemen Galeri & Harga]
        CS_Nav --> CS_Wallet[Saldo & Payout]
    end

    subgraph Kanal 4: Host B2B Portal
        H_Nav[Sidebar Host Portal]
        H_Nav --> H_Dashboard[Ringkasan Venue / Event]
        H_Nav --> H_Tree[Manajemen FotoTree & Rute]
        H_Nav --> H_Admin[Delegasi 5 Host Admin]
        H_Nav --> H_Creator[Perekrutan Fotografer Resmi]
        H_Nav --> H_Finance[Bagi Hasil & Tarik Saldo PT]
    end

    subgraph Kanal 5: Super Admin Dashboard
        A_Nav[Sidebar Super Admin]
        A_Nav --> A_KYC[Verifikasi Legalitas Host & User]
        A_Nav --> A_Mod[Moderasi Konten & Hak Cipta]
        A_Nav --> A_Finance[Audit Double-Entry & Payout]
        A_Nav --> A_Privacy[Audit Log Hapus Biometrik PDP]
    end
```

---

## 5. MODEL AKUN DAN CAPABILITY UI

### 5.1. Prinsip Multi-Capability
* Identitas akun tunggal (`User`) dapat memiliki capability `BUYER`, `SELLER`, dan `HOST` secara bersamaan.
* Menu UI ditampilkan secara adaptif berdasarkan capability yang diverifikasi oleh backend (bukan sekadar flag statis pada storage lokal).

### 5.2. Alur Aktivasi Mode Fotografer (Kreator)
1. Pengguna membuka tab **Akun** -> memilih **"Mulai Menjual"** atau **"Aktifkan Mode Fotografer"**.
2. Sistem menampilkan ringkasan hak cipta, skema komisi platform (15%), biaya host (10–20%), dan net fotografer (65–75%).
3. Pengguna mengisi profil fotografer (nama brand, media sosial, portofolio).
4. Pengguna menyelesaikan kuis kurikulum etika pemotretan publik dan privasi.
5. Pengguna mendaftarkan nomor rekening bank untuk penarikan dana.
6. **Siklus Status UI:** `NOT_APPLIED` -> `PENDING_VERIFICATION` -> `ACTIVE` <-> `SUSPENDED` / `REJECTED`.
7. Begitu status `ACTIVE`, menu **Web Studio** dan dashboard penjualan otomatis terbuka di akun yang sama.

### 5.3. Alur Pendaftaran Host B2B
* Host mengajukan profil organisasi, nama badan hukum (PT/CV), Nomor Induk Berusaha (NIB), NPWP, dan rekening atas nama perusahaan.
* UI menampilkan status: `MENUNGGU_VERIFIKASI_LEGALITAS` dengan estimasi waktu review 1–2 hari kerja oleh Super Admin.

---

## 6. SPESIFIKASI LAYAR KANAL MOBILE (FLUTTER)

### M-01: Splash & Bootstrap (P0)
* **Tujuan:** Inisialisasi token non-secret, verifikasi integritas sesi JWT, dan sinkronisasi capability pengguna.
* **Komponen:** Logo animasi Galeriku, indikator pemuatan halus.
* **States:** `Loading`, `Session Valid -> Home`, `Session Expired -> Guest Mode`, `Offline`, `Service Unavailable`.
* **Acceptance Criteria:**
  * Tidak menampilkan data privat sebelum sesi diverifikasi backend.
  * Tidak menyimpan secret atau API key backend di memori aplikasi.

### M-02: Guest Onboarding (P0)
* **Tujuan:** Edukasi nilai produk dan navigasi tanpa hambatan pendaftaran.
* **Komponen:** Ilustrasi visual, carousel pengenalan fitur, tombol `[ Jelajahi Tanpa Login ]`, tombol `[ Masuk / Daftar ]`.
* **Acceptance Criteria:** Pengguna dapat menjelajahi feed dan katalog event tanpa dipaksa membuat akun. Consent biometrik tidak diminta pada onboarding umum.

### M-03: Login & Registrasi (P0)
* **Tujuan:** Autentikasi aman berbasis Nomor HP (WhatsApp OTP), Email Magic Link, atau Google/Apple SSO.
* **Komponen:** Input field nomor telepon, selector negara (+62), tombol kirim OTP, opsi SSO, persetujuan Syarat Layanan & Kebijakan Privasi.
* **States:** `Default`, `Invalid Input`, `Loading`, `Rate Limited (Hitung Mundur 60s)`, `Success`.

### M-04: GaleriGram Feed (P0)
* **Tujuan:** Eksplorasi visual vertikal video aksi dan foto sorotan publik bergaya platform modern.
* **Layout:**
  * Foto potret menggunakan `BoxFit.cover` (tanpa distorsi).
  * Foto lanskap menggunakan `BoxFit.contain` dengan latar belakang blur estetis.
  * Overlay minimalis: nama event, nama fotografer, badge harga, tombol Like, Share, dan tombol cepat `[ Beli Foto ]`.
* **States:** `Skeleton Loading`, `Feed Kosong`, `Pagination Loading`, `Network Error`.
* **Acceptance Criteria:**
  * Hanya menampilkan foto terkompresi ber-watermark diagonal server-side. Berkas master privat tidak pernah dimuat di feed.
  * Terdapat menu `Laporkan Konten` pada setiap postingan.

### M-05: Cari Foto AI — Entry Point (P0)
* **Tujuan:** Titik masuk pencarian foto terarah.
* **Komponen:** Pemilih FotoTree target, kartu opsi **Mode Wajah (Selfie)**, kartu opsi **Mode Nomor Dada (BIB)**, penjelasan privasi biometrik.
* **Acceptance Criteria:** Pencarian wajah mengarahkan pengguna ke *Biometric Consent Gate* jika belum disetujui.

### M-06: Persetujuan Biometrik (Biometric Consent Gate) (P0)
* **Tujuan:** Kepatuhan terhadap UU PDP No. 27/2022 atas data biometrik spesifik.
* **Komponen:** Naskah tujuan pemrosesan data wajah, dasar hukum, masa retensi, hak penarikan consent kapan saja, tautan kebijakan privasi, *checkbox* eksplisit tidak dicentang otomatis, tombol `[ Setuju & Lanjutkan ]` (disabled sebelum dicentang), tombol `[ Batal ]`.
* **Acceptance Criteria:**
  * Checkbox wajib dicentang manual oleh pengguna.
  * Versi naskah persetujuan dan timestamp dicatat di server.
  * Menolak persetujuan tidak membatasi hak akses penelusuran publik.

### M-07: Kamera Selfie & Panduan Oval (P0)
* **Tujuan:** Pengambilan foto wajah acuan berkualitas tinggi dengan verifikasi keaslian hidup (*liveness*).
* **Komponen:** Lingkaran panduan oval wajah (*Face Oval Guide*), indikator pencahayaan real-time, tombol jepret, pratinjau hasil, tombol `[ Ambil Ulang ]` dan `[ Gunakan Foto Ini ]`.
* **States:** `Izin Kamera Ditolak`, `Wajah Tidak Terdeteksi`, `Banyak Wajah dalam Frame`, `Wajah Terlalu Buram/Miring`, `Memproses Embedding`.
* **Acceptance Criteria:**
  * Foto selfie acuan tidak dipublikasikan ke feed umum.
  * Berkas sementara dienkripsi di secure sandbox aplikasi.

### M-08: Pencarian Nomor Dada (BIB Search) (P0)
* **Tujuan:** Penemuan foto pelari berdasarkan angka nomor dada.
* **Komponen:** Pemilih event, input field numerik BIB (3–6 digit), tombol cari, galeri kandidat ber-watermark.
* **Acceptance Criteria:** UI tidak membocorkan nama pendaftar atau biodata pribadi pemilik nomor BIB.

### M-09: Status Antrean Pencarian AI (P0)
* **Tujuan:** Memberikan transparansi proses asinkron saat ribuan foto sedang dipindai.
* **Komponen:** Animasi pemindaian, label status riil dari backend (*"RoboYu sedang memindai 12.450 foto di FotoTree..."*), estimasi waktu tunggu, tombol `[ Beritahu Saya Jika Selesai ]` (menutup layar ke background).
* **Acceptance Criteria:** Status mencerminkan antrean worker RabbitMQ riil di server, bukan timer palsu di klien.

### M-10: Galeri Hasil Pencarian (P0)
* **Tujuan:** Menampilkan foto-foto yang cocok dengan pengguna.
* **Komponen:** *Staggered masonry grid* 2-kolom, filter sudut kamera/spot KM, tombol feedback `[ Ini Saya ]` / `[ Bukan Saya ]`, tombol multi-select untuk borong foto, tombol *Beli Paket Semua Foto*.
* **Acceptance Criteria:** Foto yang skor kemiripannya ambigu ($0.62 \le \text{Cosine} < 0.68$) diberi label hati-hati: *"Mungkin Kamu?"*.

### M-11: FotoTree Explorer & Peta GPS (P0)
* **Tujuan:** Navigasi geografis rute lomba dan destinasi wisata.
* **Komponen:** Peta interaktif Mapbox GL, pin FotoTree, rute GPS maraton, pin posisi fotografer di rute (*Spot KM 10, KM 21, Finish*), kalender event tahunan di Indonesia.

### M-12: Detail Foto & Inspeksi Pratinjau (P0)
* **Tujuan:** Penilaian kualitas foto sebelum melakukan pembelian.
* **Komponen:** Viewer foto pratinjau zoomable (maks. zoom 2.5x), watermark diagonal jelas, rincian fotografer, harga satuan, tombol `[ Tambah ke Keranjang ]`, tombol `[ Beli Sekarang ]`.
* **Keamanan Layanan:** Android mengaktifkan `FLAG_SECURE`; iOS memantau `UIScreen.capturedDidChangeNotification` untuk mencegah tangkapan layar.

### M-13: Keranjang Belanja & Diskon Grosir (P0)
* **Tujuan:** Review daftar belanjaan dan kalkulasi diskon otomatis.
* **Komponen:** Daftar foto terpilih, thumbnail, nama kreator, breakdown harga:
  * Subtotal kotor
  * Diskon grosir bertingkat (5%, 10%, 25%) yang dihitung backend
  * Total akhir
* **Acceptance Criteria:** Total harga dan diskon bersumber dari respons Go API, bukan kalkulasi sepihak di JavaScript/Dart.

### M-14: Verifikasi OTP / Kode Event Checkout BIB (P0; Keputusan `DEC-Q02`)
* **Tujuan:** Verifikasi hak akses pembeli sebelum checkout dari hasil pencarian nomor BIB.
* **Komponen:** Informasi nomor BIB target, penjelasan verifikasi, input 6-digit OTP/kode pendaftaran race, tombol kirim ulang kode (cooldown 60s), tombol `[ Verifikasi & Lanjut Checkout ]`.
* **States:** `Kode Terkirim`, `Kode Salah`, `Kode Kedaluwarsa`, `Terlalu Banyak Percobaan (Rate Limited)`.
* **Acceptance Criteria:**
  * Checkout tidak dapat dilanjutkan sebelum backend memvalidasi keabsahan kode.
  * Mencegah orang asing membeli foto pelari lain tanpa izin.

### M-15: Checkout & Pembayaran Inbound (P0)
* **Tujuan:** Eksekusi pembayaran resmi melalui payment gateway.
* **Komponen:** Ringkasan pesanan, pilihan metode pembayaran (Dynamic QRIS, Virtual Account BCA/Mandiri/BRI/BNI, GoPay, OVO), hitung mundur kedaluwarsa invoice (15–60 menit), tombol deeplink aplikasi pembayaran.
* **States:** `Membuat Invoice`, `Menunggu Pembayaran`, `Pembayaran Berhasil Terverifikasi`, `Invoice Kedaluwarsa`.
* **Acceptance Criteria:** Status pembayaran lunas hanya diakui setelah Go API menerima dan memvalidasi webhook resmi dari Xendit/Midtrans.

### M-16: Koleksi & Status Pemenuhan Master (P0)
* **Tujuan:** Memantau pesanan dan mengunduh berkas yang telah dibeli.
* **Komponen:**
  * Kartu pesanan foto
  * Penanda status terpisah: **Status Pembayaran** vs **Status Pemenuhan Master Hi-Res**
  * Hitung mundur SLA 48 Jam fotografer
  * Tombol unduh instan Non-Hi-Res
  * Tombol unduh Master Hi-Res (aktif jika status `COMPLETED`)
  * Tombol `[ Ajukan Revisi ]` jika master salah/buram.

### M-17: Unduh Non-Hi-Res Instan (P0; Keputusan `DEC-Q01`)
* **Tujuan:** Memberikan kepuasan unduh cepat 1080p sesaat setelah bayar.
* **Komponen:** Tombol `[ Unduh Versi Non-Hi-Res ]`, label wajib: *"Versi Non-Hi-Res — Memakai Watermark Mikro sampai Master Hi-Res Tersedia"*, progres unduh.
* **Acceptance Criteria:** File biner yang diunduh benar-benar telah memuat watermark mikro di level piksel gambar dari server, bukan sekadar overlay tampilan aplikasi.

### M-18: Unduh Master Hi-Res (P0)
* **Tujuan:** Pengunduhan berkas resolusi penuh asli tanpa kompresi.
* **Komponen:** Informasi dimensi asli (misal: 6000x4000 piksel, 15 MB), tombol unduh, tombol unduh semua zip (jika beli paket).
* **Acceptance Criteria:** Akses unduh menggunakan *Presigned URL* privat bertanda tangan kriptografis dengan TTL 60 menit.

### M-19: Akun, Profil & Dompet (P0)
* **Tujuan:** Manajemen identitas, riwayat belanja, dan aktivasi kemampuan kreator.
* **Komponen:** Info pengguna, tombol mode beralih fotografer (`Switch to Creator`), ringkasan saldo, menu manajemen akun anak, menu pengaturan privasi & data biometrik.

### M-20: Dompet Finansial Kreator & Payout (P0 untuk Penjual)
* **Tujuan:** Pengelolaan pendapatan fotografer dan penarikan saldo.
* **Komponen:** Tampilan saldo terpisah ganda (**Pending Settlement** vs **Available Balance**), mutasi ledger *double-entry*, riwayat pencairan, tombol `[ Tarik Dana ]`, input nomor rekening bank + verifikasi PIN/TOTP.

### M-21: Pengaturan Privasi & Penghapusan Biometrik (P0)
* **Tujuan:** Kepatuhan penuh atas *Right to Erasure* UU PDP No. 27/2022.
* **Komponen:** Status izin biometrik aktif, tombol `[ Hapus Data Biometrik Wajah Saya ]`, penjelasan konsekuensi penghapusan, dialog konfirmasi keamanan, status riil proses penghapusan di backend.

---

## 7. SPESIFIKASI LAYAR KANAL WEB STOREFRONT (PUBLIK)

### W-01: Landing Page & Event Discovery (P0)
* **Tujuan:** Pintu gerbang utama SEO penemuan di Google dan eksplorasi event tanpa aplikasi.
* **Komponen:** Hero search bar, carousel FotoTree unggulan, kalender race nasional, filter cabang olahraga, tombol cepat `[ Cari Foto Saya ]`.

### W-02: Katalog FotoTree & Detail Event (P0)
* **Tujuan:** Menampilkan profil acara/venue, daftar fotografer bertugas, rute lomba, dan statistik dokumentasi.
* **Komponen:** Header branding event/venue, informasi tanggal & lokasi, tab galeri pratinjau publik ber-watermark tebal.

### W-03: Pencarian Nomor BIB di Web (P0)
* **Tujuan:** Pelari mencari foto langsung dari browser komputer/laptop.
* **Komponen:** Field input BIB, pemilihan event, grid pratinjau hasil pencocokan. Bebas diakses tanpa login.

### W-04: Detail Foto Web (P0)
* **Tujuan:** Inspeksi pratinjau foto sebelum dimasukkan ke keranjang belanja web.
* **Komponen:** Viewer foto ber-watermark diagonal, harga dasar dari backend, nama fotografer, tombol `[ + Keranjang ]`, tombol `[ Beli Langsung ]`.

### W-05: Keranjang Belanja Web (Cart) (P0)
* **Tujuan:** Pengelolaan daftar foto belanjaan web dan penerapan diskon kuantitas otomatis (5%, 10%, 25%).
* **Komponen:** Daftar foto, kalkulasi subtotal, potongan diskon volume, tombol `[ Lanjut Pembayaran ]`.

### W-06: Verifikasi OTP / Kode Event di Web (P0; Keputusan `DEC-Q02`)
* **Tujuan:** Memvalidasi hak pembelian nomor BIB di web sebelum pembayaran diterbitkan.
* **Komponen:** Input kode OTP verifikasi pendaftaran race, hitung mundur kedaluwarsa kode, tombol verifikasi.

### W-07: Checkout Desktop Dynamic QRIS (P0)
* **Tujuan:** Pembayaran mudah dari layar monitor PC/laptop.
* **Komponen:** Menampilkan **kode QRIS dinamis berukuran besar di layar komputer** untuk dipindai oleh mobile banking pembeli, hitung mundur pembayaran 15–30 menit, status otomatis berubah ke sukses saat webhook diterima.

### W-08: Halaman Koleksi & Riwayat Pesanan Web (`/collection`) (P0)
* **Tujuan:** Mengakses foto yang sudah dibeli langsung dari browser.
* **Komponen:** Daftar pesanan, status pembayaran lunas, status pemenuhan master Hi-Res fotografer.

### W-09: Unduh Non-Hi-Res di Web (P0; Keputusan `DEC-Q01`)
* **Tujuan:** Pengunduhan langsung file bersih 1080p ber-watermark mikro dari browser web detik itu juga setelah bayar.

### W-10: Profil Publik Fotografer di Web (P1)
* **Tujuan:** Portofolio profesional fotografer yang dapat dibagikan di media sosial.

---

## 8. SPESIFIKASI LAYAR CREATOR WEB STUDIO (FOTOGRAFER)

### CS-01: Login & Capability Gate (P0)
* **Tujuan:** Memastikan hanya akun dengan kemampuan penjual aktif (`CAN_SELL`) yang dapat membuka Web Studio.

### CS-02: Dashboard Ringkasan Performa (P0)
* **Tujuan:** Memantau indikator performa penjualan fotografer.
* **Komponen:** Kartu metrik (Total Foto Diunggah, Foto Terjual, Pesanan Menunggu Hi-Res, Saldo Pending, Saldo Siap Tarik, Utilisasi Kuota Storage).

### CS-03: Pemilih FotoTree Target (P0)
* **Tujuan:** Mengasosiasikan batch foto dengan FotoTree yang tepat.
* **Komponen:** Pencarian FotoTree terdaftar, verifikasi izin motret di lokasi/event terkait.

### CS-04: Dropzone Unggah Massal (P0)
* **Tujuan:** Area *Drag-and-Drop* profesional berkapasitas besar.
* **Komponen:** Kotak dropzone luas, tombol pemilihan folder/berkas dari hard disk komputer, filter format berkas (JPEG, PNG, HEIC/HEIF).

### CS-05: Preflight Check & Alokasi Kuota (P0; Keputusan `DEC-Q03`)
* **Tujuan:** Mencegah tab browser crash dan memvalidasi kapasitas storage sebelum upload biner dimulai.
* **Alur Interaksi:**
  1. Browser memindai daftar berkas yang di-drop (hingga 2.000 file).
  2. Ekstraksi metadata EXIF di browser (`DateTimeOriginal`, `GPSLatitude`, `GPSLongitude`, `CameraModel`).
  3. Panel ringkasan menampilkan:
     * Jumlah berkas valid
     * Berkas ditolak (rusak/format salah)
     * Total ukuran batch dalam GB (misal: 14.8 GB)
     * Kuota penyimpanan tersisa di backend (misal: 10.0 GB)
  4. **Pencegahan Over-Quota:** Jika ukuran batch > kuota tersisa, tombol upload dinonaktifkan dengan panduan: *"Kapasitas tidak cukup. Silakan kurangi jumlah berkas atau ajukan penambahan kuota."*
  5. Go API melakukan verifikasi ulang otoritatif dan menerbitkan *quota reservation* saat sesi multipart dibuka.

### CS-06: Monitoring Progres Upload Massal (P0)
* **Tujuan:** Visualisasi transmisi data paralel ke S3 Storage.
* **Komponen:** Bar progres global (%), progres bar individu per file, status koneksi (5–10 worker paralel), tombol `[ Jeda / Lanjutkan ]`, tombol `[ Batalkan ]`, penanganan auto-retry jika paket data drop.

### CS-07: Antrean Pemrosesan Server (*Processing Queue*) (P0)
* **Tujuan:** Fotografer memantau progres pembuatan thumbnail, watermark diagonal, dan ekstraksi AI wajah & OCR BIB di server.

### CS-08: Manajemen Galeri & Edit Harga (P0)
* **Tujuan:** Mengelola foto-foto yang telah terbit.
* **Komponen:** Tabel galeri foto, filter tanggal/event, penyesuaian harga dasar per foto atau paket, tombol arsip/hapus.

### CS-09: Pemenuhan Master Hi-Res SLA 48 Jam (`Pending Hi-Res Fulfillment`) (P0)
* **Tujuan:** Memenuhi kewajiban pengiriman berkas asli sebelum batas waktu 48 jam berakhir.
* **Komponen:**
  * Tabel pesanan masuk yang menunggu master
  * Hitung mundur sisa waktu SLA per pesanan (warna hijau >24h, kuning 12-24h, merah <12h)
  * **Batch Drag-and-Drop Hi-Res:** Fotografer men-drop folder berisi master editan; sistem mencocokkan nama file secara otomatis (`filename auto-matching`) ke ID pesanan pembeli
  * Tombol `[ Kirim Semua Master Terpilih ]`.

### CS-10: Pembukuan Penjualan & Ledger Finansial (P0)
* **Tujuan:** Transparansi perhitungan komisi 3-pihak (Platform 15%, Host 10–20%, Fotografer 65–75%).
* **Komponen:** Rincian riwayat transaksi, snapshot harga saat checkout, pemotongan fee, dan pencatatan mutasi *double-entry*.

### CS-11: Manajemen Kuota Penyimpanan (*Storage Meter*) (P0)
* **Tujuan:** Pemantauan konsumsi ruang penyimpanan awan.
* **Komponen:** Gauge meter utilisasi storage, rincian komponen terpakai (master vs preview), riwayat bonus kuota referral (+1 GB per pembeli baru).

---

## 9. SPESIFIKASI LAYAR KANAL HOST B2B PORTAL

### H-01: Registrasi Legalitas Badan Usaha (P0)
* **Tujuan:** Onboarding entitas bisnis pengelola tempat/event.
* **Komponen:** Input data perusahaan, nomor NIB, NPWP badan usaha, upload dokumen SIUP/NIB, nomor rekening bank atas nama PT/CV, persetujuan syarat kerjasama Host.

### H-02: Dashboard Operasional Host (P0)
* **Tujuan:** Ringkasan performa seluruh FotoTree yang dinaungi.
* **Komponen:** Metrik omzet total, jumlah fotografer bertugas, total foto aktif, saldo Host terkumpul.

### H-03: Manajemen FotoTree, Lokasi & Rute (P0)
* **Tujuan:** Pembuatan dan konfigurasi wadah dokumentasi.
* **Komponen:** Form pembuatan FotoTree (Event atau Venue Tetap), penetapan jadwal, penetapan geofence koordinat GPS rute/lokasi, upload logo banner.

### H-04: Pendelegasian 5 Host Admin (P0)
* **Tujuan:** Membagi wewenang kepada staf panitia lapangan.
* **Komponen:** Form undang admin via email/no HP, batasan kuota ketat maksimal 5 akun, pengaturan izin akses operasional, tombol cabut akses admin.

### H-05: Perekrutan & Manajemen Fotografer Resmi (P0)
* **Tujuan:** Mengatur fotografer yang diizinkan memotret di venue/event.
* **Komponen:** Daftar pelamar fotografer, verifikasi portofolio, tombol setujui/tolak (*approval*), pencatatan nomor rompi resmi event.

### H-06: Konfigurasi Pembagian Hasil (*Revenue Sharing*) (P0)
* **Tujuan:** Menetapkan persentase bagi hasil *Host Fee*.
* **Komponen:** Slider persentase Host (10% s.d. 20%), simulator pendapatan real-time per transaksi foto, riwayat audit perubahan tarif komisi.

### H-07: Penarikan Saldo Bulanan Host (P0)
* **Tujuan:** Pencairan pendapatan resmi Host ke rekening perusahaan.
* **Komponen:** Saldo tersedia, jadwal pencairan bulanan (tanggal 30/31), histori transfer bank, bukti potong pajak/faktur komisi.

---

## 10. SPESIFIKASI LAYAR KANAL SUPER ADMIN DASHBOARD

### A-01: Ringkasan Kesehatan Sistem & Metrik (P0)
* **Tujuan:** Pemantauan operasional real-time seluruh sistem.
* **Komponen:** Volume transaksi, status payment gateway, latensi antrean RabbitMQ, error rate inferensi AI, peringatan lonjakan refund.

### A-02: Manajemen Pengguna & Hak Akses Capability (P0)
* **Tujuan:** Audit identitas dan penegakan sanksi akun pelanggar.
* **Komponen:** Pencarian pengguna, penyesuaian capability (`CAN_BUY`, `CAN_SELL`, `CAN_HOST`), tombol bekukan akun (*suspend*), audit log tindakan admin.

### A-03: Verifikasi Legalitas Host (P0)
* **Tujuan:** Verifikasi dokumen hukum NIB/NPWP pendaftar Host baru sebelum diizinkan menerbitkan FotoTree komersial.

### A-04: Moderasi Konten & Penanganan Hak Cipta (P0)
* **Tujuan:** Mengadili foto/video yang dilaporkan melanggar pedoman komunitas atau hak cipta fotografer lain (*DMCA takedown*).

### A-05: Monitoring Pesanan, Pembayaran & Refund (P0)
* **Tujuan:** Investigasi kegagalan webhook payment dan persetujuan refund sengketa.

### A-06: Rekonsiliasi Finansial & Ledger Pembukuan (P0)
* **Tujuan:** Pencocokan mutasi kas masuk payment gateway vs saldo penampungan *Pending Settlement* dan pengeluaran *Payout*.

### A-07: Penanganan Permintaan Privasi UU PDP (P0)
* **Tujuan:** Pemantauan antrean penghapusan data biometrik (*Right to Erasure*) dan penarikan consent pengguna.

### A-08: Log Audit Sistem (*Immutable Audit Logs*) (P0)
* **Tujuan:** Rekam jejak seluruh operasi sensitif platform (perubahan saldo, akses file master, penghapusan data, eskalasi izin).

---

## 11. SISTEM DESAIN DAN TOKEN UI

```
Tokens Dasar:
- Spacing: 4px, 8px, 12px, 16px, 24px, 32px, 48px, 64px
- Radius:  4px (sm), 8px (md), 16px (lg), 24px (xl), 9999px (full/pill)
- Elevation: Level 0 (flat), Level 1 (cards), Level 2 (dropdown/popover), Level 3 (modals/sheets)
```

### 11.1. Palet Warna Fungsional (*Functional Colors*)
* **Primary (Brand Athletic):** Biru Elektrik `#0066FF` (Aksi utama, tombol aktif, identitas visual).
* **Secondary:** Cyan Neon `#00D2FF` (Aksen dinamis, progress indicator).
* **Neutral Dark (Background Dark Mode & Inspeksi Foto):** `#0D1117` (Mengurangi pantulan cahaya saat melihat foto).
* **Neutral Light (Surface & Cards):** `#F8FAFC` / `#FFFFFF` (Tampilan bersih katalog).
* **Success:** Emerald `#10B981` (Transaksi lunas, master Hi-Res terkirim, liveness valid).
* **Warning:** Amber `#F59E0B` (Timer SLA < 24 jam, foto berlabel "Mungkin Kamu?").
* **Danger:** Rose `#EF4444` (SLA kedaluwarsa, auto-refund, pelanggaran privasi).

### 11.2. Aksesibilitas (WCAG 2.1 AA Standard)
* Rasio kontras teks utama terhadap background minimal $4.5:1$.
* Kontrol interaktif di layar mobile memiliki ukuran area sentuh minimal $48 \times 48$ dp.
* Setiap pesan error pada form input wajib terhubung dengan atribut `aria-describedby` (Web) atau `Semantics` (Flutter).
* Seluruh navigasi web studio dan storefront dapat dioperasikan penuh menggunakan keyboard (*Tab, Enter, Escape, Arrow keys*).

---

## 12. STANDAR STATE DAN PENANGANAN ERROR GLOBAL

Setiap layar data-driven wajib mengimplementasikan 7 state standar:
1. **Initial Skeleton Loading:** Menampilkan kerangka tata letak beranimasi shimmer lembut.
2. **Empty State Bersahabat:** Menjelaskan ketiadaan data beserta tombol aksi pemulihan (misal: *"Belum ada foto yang cocok. Coba cari dengan nomor dada atau upload selfie baru"*).
3. **Partial Data State:** Menampilkan data yang berhasil dimuat sambil menampilkan indikator peringatan untuk bagian yang gagal.
4. **Offline State:** Deteksi putusnya jaringan lokal dengan opsi mencoba kembali (*retry*).
5. **Timeout State:** Penjelasan ramah jika koneksi server lambat tanpa membuat transaksi ganda.
6. **Rate Limited State:** Tampilan hitung mundur penalti jika pengguna memicu spam request OTP atau kueri API.
7. **Idempotency Guard:** Tombol aksi finansial (*Bayar*, *Tarik Dana*, *Submit Batch*) otomatis masuk ke kondisi *disabled* dan menampilkan spinner pemrosesan sesaat setelah ditekan sekali untuk mencegah duplikasi eksekusi.

---

## 13. REKAPITULASI ACCEPTANCE CRITERIA LINTAS KANAL

* [x] **Guest Browsing:** Tamu dapat menjelajahi seluruh konten FotoTree dan melihat foto pratinjau ber-watermark tanpa paksaan login atau penyerahan data biometrik.
* [x] **Kepatuhan Privasi Biometrik:** Kamera selfie acuan hanya dapat diaktifkan setelah persetujuan eksplisit dicentang secara sadar oleh pengguna terdaftar.
* [x] **Integritas Checkout BIB:** Pencarian nomor dada dibuka bebas di web, namun checkout wajib lolos verifikasi OTP pendaftaran race untuk mencegah pembelian foto orang lain oleh pihak asing.
* [x] **Transparansi Non-Hi-Res:** Unduhan Non-Hi-Res instan beresolusi 1080p memuat watermark mikro sampai master Hi-Res bersih diunggah fotografer.
* [x] **Proteksi Master Privat:** Master foto resolusi tinggi terisolasi mutlak di bucket privat terenkripsi dan hanya dapat diakses melalui signed URL ber-TTL 60 menit setelah otorisasi transaksi valid.
* [x] **Preflight & Reservasi Kuota 2.000 Foto:** Browser Web Studio memvalidasi ukuran batch sebelum upload dimulai, dan Go API mengelola reservasi kuota storage secara otoritatif untuk mencegah tab crash dan kehabisan memori.
* [x] **Identitas Tunggal Multi-Capability:** Satu akun dapat berfungsi sebagai pembeli, fotografer, dan host tanpa memerlukan akun kedua.
* [x] **Pemisahan Finansial Mutlak:** Dana penjualan tertahan di pos *Pending Settlement* dan hanya berpindah ke *Available Balance* setelah pesanan berstatus `COMPLETED`.
