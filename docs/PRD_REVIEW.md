# PRD_REVIEW.md
## Galeriku — Catatan Review PRD dan Keputusan Produk

* **Sumber utama:** `PRD.md`
* **Versi sumber:** 2.2 Enhanced Master Blueprint
* **Tanggal keputusan:** 15 Oktober 2026
* **Status:** Review dan tiga keputusan produk tercatat; asumsi teknis masih memerlukan validasi.
* **Aturan integritas:** `PRD.md` adalah dokumen sumber dan tidak diubah oleh catatan ini. Dokumen ini merupakan catatan turunan.

---

### 1. Ringkasan produk

Galeriku adalah marketplace foto dan konten visual berbasis AI dengan lima kanal utama:
1. Aplikasi Flutter Android dan iOS.
2. Web Storefront publik tanpa kewajiban memasang aplikasi.
3. Creator Web Studio untuk upload massal dan pemenuhan master Hi-Res.
4. Host B2B Management Portal.
5. Super Admin Dashboard.

Konsep produk meliputi FotoTree untuk event/venue, pencarian wajah dan nomor BIB, feed GaleriGram, upload massal hingga target 2.000 foto, watermark server-side, checkout, fulfillment Hi-Res dengan target SLA 48 jam, refund, ledger double-entry, dan pembagian pendapatan tiga pihak.

Model akun adalah satu akun multi-kemampuan: pembeli, fotografer/penjual, dan Host apabila memenuhi persyaratan. Core backend menjadi sumber kebenaran untuk otorisasi, harga, pembayaran, saldo, refund, dan akses download.

---

### 2. Keputusan resmi yang disetujui

#### DEC-Q01 — Download Non-Hi-Res
* **Status:** DISETUJUI
* **Keputusan:** Versi Non-Hi-Res yang diberikan setelah pembayaran memakai watermark mikro sampai master Hi-Res tersedia.
* **Dampak produk dan UI:**
  * Halaman detail foto harus menjelaskan perbedaan Non-Hi-Res dan Hi-Res.
  * Halaman pesanan harus menunjukkan status master Hi-Res dan tenggat yang berlaku.
  * Preview/download Non-Hi-Res harus diberi label yang jelas bahwa file memakai watermark mikro.
  * Ketika master Hi-Res sudah tersedia dan syarat pemenuhan terpenuhi, UI menyediakan akses ke file Hi-Res sesuai order.
  * Notifikasi harus membedakan pembayaran berhasil, Non-Hi-Res tersedia, master menunggu, dan master tersedia.
* **Catatan yang belum diputuskan:** Kebijakan refund 100% jika SLA 48 jam gagal masih harus diselaraskan dengan hak penggunaan file Non-Hi-Res yang sudah diunduh. Watermark mikro mengurangi nilai penggunaan ulang, tetapi tidak dengan sendirinya menghapus risiko moral hazard. Jangan menerapkan potongan refund atau mengubah jumlah refund tanpa keputusan bisnis dan review hukum.

#### DEC-Q02 — Checkout berbasis BIB
* **Status:** DISETUJUI
* **Keputusan:** Nomor BIB dapat digunakan untuk pencarian dan penemuan kandidat foto. Checkout memerlukan OTP atau kode verifikasi event.
* **Dampak produk dan UI:**
  * Pencarian BIB tersedia di Web Storefront.
  * Preview hasil pencarian tetap mengikuti kebijakan publikasi, watermark, dan privasi.
  * Sebelum checkout, tampilkan langkah verifikasi OTP/kode event.
  * Status verifikasi, masa berlaku kode, pengiriman ulang, batas percobaan, dan error harus terlihat.
  * Kode harus diverifikasi server-side dan terikat pada konteks yang benar, seperti event/pendaftaran atau sesi checkout.
  * Jangan menganggap OTP sebagai bukti otomatis bahwa semua subjek di foto adalah pembeli. Validasi hubungan antara kode, pendaftaran, dan hak pembelian harus dirancang secara eksplisit.
  * Jika provider event tidak mendukung integrasi verifikasi, flow fallback harus ditetapkan sebelum implementasi.
* **Catatan yang belum diputuskan:** Mekanisme penerbitan kode, kanal pengiriman, integrasi dengan sistem pendaftaran race, masa berlaku, recovery, dan validasi foto dengan banyak subjek masih perlu spesifikasi.

#### DEC-Q03 — Kuota upload fotografer
* **Status:** DISETUJUI
* **Keputusan:** Kuota bersifat dinamis berdasarkan ukuran batch dan kapasitas tersisa.
* **Dampak produk dan UI:**
  * Sebelum upload, Web Studio menjalankan preflight untuk menghitung total ukuran file dan memeriksa kuota tersisa dari backend.
  * Pengguna melihat jumlah file valid, total ukuran, kapasitas tersisa, dan estimasi kebutuhan ruang.
  * Jika batch melebihi kuota, upload penuh tidak dimulai sampai pengguna menghapus file, membagi batch, atau memperoleh kapasitas tambahan sesuai kebijakan.
  * Backend melakukan pemeriksaan kuota lagi ketika membuat upload session. Hasil perhitungan browser tidak dianggap otoritatif.
  * Reservasi kuota untuk upload aktif harus mencegah dua batch bersamaan melampaui kuota.
  * Setelah upload selesai atau gagal, reservasi diselesaikan atau dilepas secara idempoten.
  * Kapasitas harus membedakan master, preview, thumbnail, temporary multipart parts, dan file fulfillment jika semuanya dihitung terhadap kuota.
* **Catatan yang belum diputuskan:** Kuota dasar 10 GB, batas paket maksimum, bonus referral, apakah preview dihitung, dan masa retensi master belum diubah dari PRD sumber. Perlu keputusan pricing/storage terpisah.

---


#### DEC-REF — Kebijakan Refund & Pembalikan Pendapatan (Clawback) Tiga Pihak
- **Status**: DISETUJUI
- **Sumber Evaluasi**: Audit Reverse-Engineering FotoYu v1.11.11 & Kepatuhan Akuntansi Galeriku.
- **Rincian Keputusan**:
  1. **SLA Pemenuhan 48 Jam (`REF-02`)**: Dihitung dari timestamp pembayaran lunas (`payments.paid_at`).
  2. **100% Harga Dasar Dikembalikan (`REF-03`)**: Pembeli memperoleh pengembalian penuh atas harga dasar foto (`unit_price_idr`) tanpa pemotongan biaya payment gateway dari hak konsumen.
  3. **Tujuan Pengembalian (`REF-01`)**: Default ke Saldo Internal Dompet Pembeli (*Buyer Wallet*) untuk kredit instan dan pengalaman belanja mulus, dengan opsi pencairan tunai (*payout*) ke rekening bank terdaftar (minimal Rp 50.000, `REF-08`).
  4. **Pembalikan Alokasi 3 Pihak (`REF-05`)**: Dilakukan via jurnal pembalikan berpasangan (`creator_clawback_idr`, `host_clawback_idr`, `platform_clawback_idr`). Terlindungi oleh masa penahanan settlement **T + 7 hari** pasca-fulfillment, sehingga dana belum ditransfer ke luar platform saat SLA 48 jam dievaluasi.
  5. **Kelayakan Pasca Unduh Non-Hi-Res (`REF-06`)**: Pembeli tetap berhak memperoleh refund 100% karena file Non-Hi-Res memuat watermark mikro destruktif pada tingkat piksel (DEC-Q01).
  6. **Granularitas Parsial Multi-Item (`REF-07`)**: Evaluasi SLA dan refund dihitung secara independen per baris item (`order_items`).
  7. **Early Failure Report & Larangan Generative Upscale (`REF-10`)**: Fotografer dapat melaporkan kerusakan kartu memori lebih awal (`MASTER_LOST`) untuk membuka hak refund instan tanpa menunggu 48 jam. Pengunggahan master hasil generative AI upscaling dilarang keras dan dikenakan sanksi reputasi.

### 3. Matriks requirement, decision, assumption, risk

| ID | Jenis | Topik | Status / Catatan |
|---|---|---|---|
| **DEC-Q01** | Decision | Watermark mikro Non-Hi-Res sampai Hi-Res tersedia | Disetujui |
| **DEC-Q02** | Decision | BIB untuk pencarian; OTP/kode event wajib sebelum checkout | Disetujui |
| **DEC-Q03** | Decision | Kuota dinamis berdasarkan ukuran batch dan kapasitas tersisa | Disetujui |
| **REQ-001** | Requirement | Satu akun multi-kemampuan | Dipertahankan dari PRD |
| **REQ-002** | Requirement | Guest dapat menjelajah konten publik | Dipertahankan, dengan akses publik mengikuti aturan privasi |
| **REQ-003** | Requirement | Upload langsung ke object storage dengan multipart presigned URL | Dipertahankan |
| **REQ-004** | Requirement | Master Hi-Res privat | Dipertahankan |
| **REQ-005** | Requirement | Backend menentukan status payment, harga, saldo, dan hak download | Dipertahankan |
| **ASSUMP-001** | Assumption | OTP dapat diintegrasikan dengan sistem pendaftaran event | Belum terbukti; perlu validasi provider/event |
| **ASSUMP-002** | Assumption | Target 2.000 file dan target waktu upload dapat dicapai pada kondisi jaringan yang ditentukan | Perlu benchmark nyata |
| **ASSUMP-003** | Assumption | Akurasi OCR BIB ≥94% pada kondisi lapangan | Belum terbukti |
| **ASSUMP-004** | Assumption | Target face matching ≥96,5% dapat dicapai pada dataset dan metrik evaluasi yang disepakati | Belum terbukti; definisikan metrik dan dataset |
| **ASSUMP-005** | Assumption | Penghapusan embedding di Qdrant kurang dari 60 detik dapat dipenuhi end-to-end | Perlu uji termasuk cache, worker, replika, dan backup |
| **ASSUMP-006** | Assumption | Proteksi screenshot lintas platform dapat diterapkan setara | Tidak benar sebagai jaminan mutlak; kemampuan Android dan iOS berbeda |
| **RISK-001** | Risk | Refund 100% setelah file Non-Hi-Res diunduh dapat menimbulkan moral hazard | Belum diselesaikan oleh watermark mikro saja |
| **RISK-002** | Risk | Nomor BIB dapat diketahui pihak lain | Checkout perlu verifikasi dan hak akses server-side |
| **RISK-003** | Risk | Batch upload melebihi kuota atau membuat penggunaan storage tak terduga | Preflight, reservation, quota accounting, lifecycle |
| **RISK-004** | Risk | Pemrosesan wajah dan anak merupakan data berisiko tinggi | Perlu review legal, consent, retensi, akses, dan penghapusan |
| **RISK-005** | Risk | Nilai revenue share dapat tidak konsisten jika biaya dan pembulatan tidak ditetapkan | Aturan finansial dan ledger harus formal |
| **RISK-006** | Risk | Pernyataan teknis spesifik seperti ambang similarity 0,68 belum tentu valid lintas model/dataset | Harus ditetapkan lewat evaluasi, bukan asumsi |
| **RISK-007** | Risk | Klaim “mustahil dihapus” atau proteksi screenshot mutlak tidak akurat | Hindari janji keamanan absolut |
| **RISK-008** | Risk | Versi framework/dependency di PRD mungkin perlu diperbarui saat implementasi | Verifikasi kompatibilitas dan dukungan sebelum lock |

---

### 4. Kontradiksi dan keputusan lanjutan

#### 4.1 Anti-stalker, BIB, dan checkout
Keputusan OTP/kode event memperkuat kontrol, tetapi belum sepenuhnya membuktikan bahwa subjek foto adalah pembeli. Spesifikasi harus menentukan hubungan antara identitas pendaftar, BIB, kode, hasil pencarian, dan hak pembelian. Untuk foto yang memuat banyak orang, jangan menyatakan seluruh foto hanya berisi pembeli.

#### 4.2 Non-Hi-Res, SLA, dan refund
Keputusan watermark mikro dicatat. Aturan refund tetap harus menjelaskan apakah refund otomatis mematikan akses selanjutnya, bagaimana status file yang sudah diunduh ditangani, dan bagaimana mencegah klaim refund ganda. Jangan mengklaim dapat menarik kembali file lokal yang telah diunduh.

#### 4.3 Kuota dan batch upload
Preflight wajib dilakukan di browser untuk UX dan di backend untuk otoritas. Gunakan ukuran byte aktual, batas file, reservation, multipart cleanup, dan perhitungan storage turunan. Jumlah 2.000 file bukan jaminan bahwa batch muat dalam kuota.

#### 4.4 Keuangan tiga pihak
Perlu spesifikasi tentang urutan pengurangan biaya gateway, basis persentase Host/Platform/Kreator, pembulatan, refund, chargeback, pajak, waktu vesting, dan saldo pending/available. Contoh angka di PRD harus diuji ulang dengan aturan final.

#### 4.5 Privasi dan data anak
Jangan mengasumsikan consent generik cukup untuk semua pemrosesan biometrik. Lakukan peninjauan hukum terkait dasar pemrosesan, verifikasi wali, retensi, penghapusan, transfer data, dan kebutuhan penilaian dampak. Data transaksi yang wajib dipertahankan harus dipisahkan dari biometrik yang tidak lagi diperlukan.

---

### 5. Matriks benchmark sebelum rilis

| Area | Target di PRD | Cara Validasi |
|---|:---:|---|
| **Face matching** | TPR ≥96,5% pada kondisi yang disebutkan | Dataset berizin dan representatif; laporkan false positive/negative dan threshold |
| **OCR BIB** | ≥94% | Dataset lapangan dengan nomor terlipat, kotor, bergerak, dan pencahayaan beragam |
| **Bulk upload** | 2.000 foto, 10–20 GB, target <15 menit pada 50 Mbps | Uji browser, storage, concurrency, retry, dan batas jaringan; jangan hanya uji mock |
| **AI search latency** | <1,8 detik per sesi | Definisikan ukuran katalog, hardware, p95/p99, dan apakah antrean termasuk |
| **SLA Hi-Res** | <48 jam | Uji timer, zona waktu, notifikasi, eskalasi, dan refund |
| **Biometric deletion** | <60 detik pada target sistem | Uji Qdrant, cache, queue, storage, replika dan laporan status |
| **Uptime** | 99,9% | Tetapkan periode ukur, pengecualian provider, dan monitoring |
| **Screenshot** | Tidak dapat dijamin absolut | Uji kemampuan platform; dokumentasikan keterbatasan Android/iOS |

---

### 6. Rekomendasi prioritas

#### P0 — Wajib sebelum transaksi publik
* Backend authorization untuk semua resource privat.
* Upload preflight dan kuota server-side.
* Master privat dan preview ber-watermark.
* Verifikasi payment webhook dan idempotency.
* Hak download terikat pada order.
* OTP/kode event yang terverifikasi server.
* Consent dan prosedur penghapusan biometrik.
* State machine refund dan fulfillment.
* Ledger dan rekonsiliasi finansial.

#### P1 — Sebelum ekspansi
* Dashboard Host lengkap.
* Analitik lanjutan.
* Optimasi rekomendasi feed.
* Paket storage berbayar.
* Automasi moderasi yang lebih luas.
* Optimasi multi-region jika dibutuhkan.

#### P2 — Setelah validasi produk
* Fitur sosial yang lebih kompleks.
* Gamifikasi.
* Integrasi venue tambahan.
* Ekspansi regional.

---

### 7. Decision backlog berikutnya

| ID | Keputusan yang Diperlukan | Pemilik yang Disarankan |
|---|---|---|
| **DB-01** | Aturan refund ketika Non-Hi-Res sudah diunduh | Product + Legal |
| **DB-02** | Penerbitan dan validasi OTP/kode event | Product + Backend |
| **DB-03** | Apakah OTP hanya memverifikasi pendaftaran atau juga relasi terhadap foto | Product + Privacy/Legal |
| **DB-04** | Kuota dasar, maksimum, dan perhitungan preview/master | Product + Finance + Engineering |
| **DB-05** | Formula final revenue share dan pembulatan | Finance + Backend |
| **DB-06** | Retensi biometrik, selfie, embedding, dan backup | Privacy/Legal + Security |
| **DB-07** | Metrik benchmark AI dan dataset evaluasi | AI + QA |
| **DB-08** | SLA operasional serta kebijakan eskalasi | Operations + Product |

---

### 8. Batas dokumen

Catatan ini tidak menyatakan bahwa target performa, akurasi, kepatuhan, atau kontrol keamanan telah diuji. Semua angka target tetap berstatus persyaratan sampai benchmark atau audit membuktikannya. `PRD.md` asli dipertahankan tanpa perubahan.
