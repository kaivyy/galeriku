# Galeriku

Platform foto event olahraga Indonesia: marketplace foto maraton dengan pencarian wajah (AI), pencarian nomor dada (BIB), pembayaran QRIS, dan pengiriman master Hi-Res dengan SLA 48 jam.

## Struktur Proyek

```
galeriku/
  backend/         Go API (Go 1.22+, pgxpool, stdlib mux, TDD)
  ai-worker/       Python AI worker (RabbitMQ, InsightFace, EasyOCR, Qdrant)
  web/             Astro storefront + React SPA internal channels
  mobile/          Flutter app (Android + iOS)
  migrations/      PostgreSQL migrations (000001-000005)
  docker-compose.yml
  Makefile
  CHANGELOG.md
  docs/            Semua dokumentasi
```

## Dokumentasi

| Dokumen | Isi |
|---|---|
| [PRD.md](docs/PRD.md) | Master Blueprint v2.2: keputusan produk, requirement, timeline |
| [UI_UX_SPEC.md](docs/UI_UX_SPEC.md) | Spesifikasi layar semua kanal (M/W/CS/H/A) |
| [API_SPEC.md](docs/API_SPEC.md) | Kontrak REST 78 endpoint per 8 domain |
| [DATABASE_SPEC.md](docs/DATABASE_SPEC.md) | Kamus data 81 tabel PostgreSQL |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Arsitektur hexagonal + diagram dataflow |
| [GALERIKU_API_ARCHITECTURE_SPEC.md](docs/GALERIKU_API_ARCHITECTURE_SPEC.md) | Kontrak API tingkat tinggi antar kanal |
| [ROADMAP.md](docs/ROADMAP.md) | 9 fase development (semua selesai) |
| [IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) | Rincian teknis per fase per task |
| [FOTOYU_BENCHMARK_ANALYSIS.md](docs/FOTOYU_BENCHMARK_ANALYSIS.md) | Benchmark kompetitor FotoYu v1.11.11 |
| [PRD_REVIEW.md](docs/PRD_REVIEW.md) | Review PRD dan keputusan terbuka |
| [CHANGELOG.md](CHANGELOG.md) | Riwayat perubahan per fase |

## Cara Mulai

```bash
docker compose up -d          # PostgreSQL, Redis, MinIO, RabbitMQ, Qdrant
make migrate-up              # Terapkan semua migrasi
make test                    # Backend Go + web vitest + tsc + lint-copy
make test-integration        # Backend full integration dengan Docker
make openapi-validate       # Validasi OpenAPI 3.1
```

## Status

Semua 9 fase selesai dan terpush ke [github.com/kaivyy/galeriku](https://github.com/kaivyy/galeriku).

| Fase | Status |
|---|---|
| 1. Core Foundation | Selesai |
| 2. Identity & Host B2B | Selesai |
| 3. FotoTree & S3 Upload | Selesai |
| 4. Python AI Worker | Selesai |
| 5. Discovery & Checkout | Selesai |
| 6. Payment & Fulfillment | Selesai |
| 7. Ledger & Settlements | Selesai |
| 8. Frontend Contracts | Selesai |
| 9. Hardening & Launch | Selesai |
