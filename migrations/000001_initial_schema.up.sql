-- ============================================================================
-- Galeriku — Migration 000001: Initial Schema (76 Tables across 8 Domains)
-- Target Engine: PostgreSQL 16
-- Author: Galeriku Senior Engineering Team
-- Date: 15 Oktober 2026
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ----------------------------------------------------------------------------
-- UUIDv7 Generator Function for PostgreSQL 16
-- ----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION generate_uuid_v7()
RETURNS UUID AS $$
DECLARE
    v_time DOUBLE PRECISION;
    v_time_ms BIGINT;
    v_bytes BYTEA;
BEGIN
    v_time := EXTRACT(EPOCH FROM CLOCK_TIMESTAMP());
    v_time_ms := FLOOR(v_time * 1000)::BIGINT;
    v_bytes := gen_random_bytes(16);
    v_bytes := SET_BYTE(v_bytes, 0, (v_time_ms >> 40)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 1, (v_time_ms >> 32)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 2, (v_time_ms >> 24)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 3, (v_time_ms >> 16)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 4, (v_time_ms >> 8)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 5, (v_time_ms)::INTEGER);
    -- Version 7
    v_bytes := SET_BYTE(v_bytes, 6, (GET_BYTE(v_bytes, 6) & 15) | 112);
    -- Variant 10xx
    v_bytes := SET_BYTE(v_bytes, 8, (GET_BYTE(v_bytes, 8) & 63) | 128);
    RETURN ENCODE(v_bytes, 'hex')::UUID;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- ============================================================================
-- Tabel: users (Identitas global user; user dapat memiliki beberapa capability.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS users (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    email_normalized VARCHAR(128) NULL UNIQUE,
    phone_e164 VARCHAR(128) NULL UNIQUE,
    account_status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: user_profiles (Profil publik/non-sensitif. Jangan menaruh rekening atau rahasia autentikasi di sini.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS user_profiles (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    avatar_asset_key TEXT NULL,
    locale TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: capabilities (Katalog capability seperti BUYER, CREATOR, HOST_LEADER, PLATFORM_ADMIN.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS capabilities (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL
);

-- ============================================================================
-- Tabel: user_capabilities (Capability global. Hak pada organisasi/event tetap harus diperiksa lewat membership.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS user_capabilities (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    capability_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    granted_by UUID NULL,
    granted_at TIMESTAMPTZ DEFAULT NULL NULL,
    revoked_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: host_organizations (Entitas bisnis Host.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_organizations (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    legal_name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    business_field TEXT NOT NULL,
    verification_status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: host_applications (Siklus pengajuan dan approval Host.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_applications (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    submitted_by UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    reviewed_by UUID NULL,
    review_reason_code TEXT NULL,
    submitted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NULL,
    reviewed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NULL
);

-- ============================================================================
-- Tabel: host_legal_documents (Dokumen tetap privat di object storage; database menyimpan metadata.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_legal_documents (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_application_id UUID NOT NULL,
    object_key VARCHAR(255) NOT NULL,
    document_type TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    review_status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: host_locations (Lokasi/venue organisasi; koordinat/geofence perlu validasi geospasial.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_locations (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    address JSONB NOT NULL,
    latitude NUMERIC(10, 7) NULL,
    longitude NUMERIC(10, 7) NULL,
    geofence JSONB NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: host_memberships (Host Leader/Admin terikat organisasi; tidak memberi akses finansial secara implisit.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_memberships (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    user_id UUID NOT NULL,
    membership_role VARCHAR(64) NOT NULL,
    status VARCHAR(64) NOT NULL,
    invited_by UUID NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    revoked_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: host_invitations (Token undangan hanya disimpan dalam bentuk hash; penerimaan harus atomik.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_invitations (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    invited_by UUID NOT NULL,
    invitee_user_id UUID NULL,
    invitee_email TEXT NULL,
    role TEXT NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    status VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

-- ============================================================================
-- Tabel: host_admin_limits (Batas admin dikonfigurasi, bukan ditanam permanen dalam kode.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_admin_limits (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL UNIQUE,
    max_admins INTEGER NOT NULL,
    policy_version TEXT NOT NULL,
    updated_by UUID NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: events (FotoTree/event yang dikelola organisasi Host.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    slug VARCHAR(128) NOT NULL,
    name TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    starts_at TIMESTAMPTZ DEFAULT NULL NULL,
    ends_at TIMESTAMPTZ DEFAULT NULL NULL,
    created_by UUID NOT NULL
);

-- ============================================================================
-- Tabel: event_sessions (Sesi/tahapan dalam event.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS event_sessions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    event_id UUID NOT NULL,
    name TEXT NOT NULL,
    starts_at TIMESTAMPTZ DEFAULT NULL NULL,
    ends_at TIMESTAMPTZ DEFAULT NULL NULL,
    venue_id UUID NULL,
    status VARCHAR(64) NOT NULL
);

-- ============================================================================
-- Tabel: event_access_codes (Kode akses event. Simpan hash, bukan kode mentah bila cukup.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS event_access_codes (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    event_id UUID NOT NULL,
    code_hash VARCHAR(255) NOT NULL,
    code_type TEXT NOT NULL,
    expires_at TIMESTAMPTZ DEFAULT NULL NULL,
    max_uses INTEGER NULL,
    use_count INTEGER NOT NULL,
    status VARCHAR(64) NOT NULL
);

-- ============================================================================
-- Tabel: event_creator_memberships (Jenis INTERNAL/INDEPENDENT berlaku per event, bukan role global.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS event_creator_memberships (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    event_id UUID NOT NULL,
    creator_user_id UUID NOT NULL,
    creator_type TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    approved_by UUID NULL,
    effective_from TIMESTAMPTZ DEFAULT NULL NULL,
    effective_until TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: event_locations (Titik rute, area pemotretan atau lokasi event.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS event_locations (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    event_id UUID NOT NULL,
    name TEXT NOT NULL,
    latitude NUMERIC(10, 7) NOT NULL,
    longitude NUMERIC(10, 7) NOT NULL,
    route_order INTEGER NULL,
    geofence JSONB NULL,
    status VARCHAR(64) NOT NULL
);

-- ============================================================================
-- Tabel: photos (Entitas foto logis, bukan bytes gambar.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photos (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    uploader_user_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    captured_at TIMESTAMPTZ NULL,
    width_px INTEGER NULL,
    height_px INTEGER NULL,
    moderation_status TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: photo_event_links (Join table agar satu foto dapat terkait lebih dari satu event jika produk mengizinkan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photo_event_links (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    photo_id UUID NOT NULL,
    event_id UUID NOT NULL,
    linked_by UUID NULL,
    status VARCHAR(64) NOT NULL,
    linked_at TIMESTAMPTZ NOT NULL
);

-- ============================================================================
-- Tabel: photo_assets (Master, preview ber-watermark, thumbnail, derivatif, objek sementara.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photo_assets (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    photo_id UUID NOT NULL,
    asset_type VARCHAR(64) NOT NULL,
    object_key VARCHAR(255) NOT NULL UNIQUE,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    checksum_sha256 TEXT NULL,
    processing_status VARCHAR(64) NOT NULL,
    watermark_status TEXT NOT NULL
);

-- ============================================================================
-- Tabel: photo_bib_tags (Tag BIB dari OCR, kreator atau admin; hasil OCR dapat perlu review.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photo_bib_tags (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    photo_id UUID NOT NULL,
    bib_normalized TEXT NOT NULL,
    source VARCHAR(64) NOT NULL,
    confidence NUMERIC(5, 4) NULL,
    review_status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: photo_moderation_reviews (Histori moderasi; jangan menimpa keputusan lama.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photo_moderation_reviews (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    photo_id UUID NOT NULL,
    reviewer_user_id UUID NULL,
    decision VARCHAR(64) NOT NULL,
    reason_code TEXT NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: photo_collections (Koleksi pengguna jika fitur koleksi pribadi diterapkan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photo_collections (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    owner_user_id UUID NOT NULL,
    name TEXT NOT NULL,
    visibility TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: photo_collection_items (Item koleksi; unique pasangan collection/photo.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS photo_collection_items (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    photo_collection_id UUID NOT NULL,
    photo_id UUID NOT NULL,
    added_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: biometric_consents (Riwayat consent; buat record/version baru, jangan menulis ulang histori.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS biometric_consents (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    subject_user_id UUID NOT NULL,
    consent_purpose TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    granted_at TIMESTAMPTZ DEFAULT NULL NULL,
    withdrawn_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: biometric_subjects (Referensi subjek; bukan tempat menyimpan vektor atau selfie mentah.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS biometric_subjects (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NULL,
    processing_status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deletion_requested_at TIMESTAMPTZ NULL,
    deleted_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: face_vector_refs (Referensi ke Qdrant; vector payload disimpan di vector store.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS face_vector_refs (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    biometric_subject_id UUID NOT NULL,
    photo_id UUID NOT NULL,
    vector_store TEXT NOT NULL,
    collection_name TEXT NOT NULL,
    vector_point_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: ai_jobs (OCR, embedding, match, thumbnail atau job AI lain.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_jobs (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    requested_by UUID NULL,
    job_type VARCHAR(64) NOT NULL,
    subject_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    idempotency_key TEXT NULL UNIQUE,
    attempt_count INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: ai_job_results (Simpan metadata minimum; hindari menyimpan biometrik mentah dalam hasil.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_job_results (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    ai_job_id UUID NOT NULL,
    result_type VARCHAR(64) NOT NULL,
    result_reference JSONB NULL,
    confidence NUMERIC(5, 4) NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: ai_search_sessions (Sesi pencarian AI/BIB dengan lifetime terbatas.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_search_sessions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NULL,
    event_id UUID NULL,
    search_type TEXT NOT NULL,
    consent_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    expires_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: ai_search_results (Hasil pencarian sementara; perlu kebijakan retensi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_search_results (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    search_session_id UUID NOT NULL,
    photo_id UUID NOT NULL,
    rank INTEGER NOT NULL,
    score TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: biometric_deletion_jobs (Orkestrasi penghapusan yang dapat diulang.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS biometric_deletion_jobs (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    biometric_subject_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL,
    attempt_count INTEGER NOT NULL,
    last_error_code TEXT NULL
);

-- ============================================================================
-- Tabel: biometric_deletion_tasks (Lacak PostgreSQL/Qdrant/cache/queue secara independen.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS biometric_deletion_tasks (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    deletion_job_id UUID NOT NULL,
    system_name VARCHAR(64) NOT NULL,
    target_reference TEXT NULL,
    status VARCHAR(64) NOT NULL,
    attempt_count INTEGER NOT NULL,
    last_attempt_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: consent_audit_logs (Audit minimum tanpa menyimpan raw biometric atau token.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS consent_audit_logs (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    subject_ref UUID NULL,
    event_type VARCHAR(64) NOT NULL,
    policy_version TEXT NULL,
    request_id UUID NULL,
    occurred_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    metadata JSONB NOT NULL
);

-- ============================================================================
-- Tabel: carts (Keranjang aktif/terbengkalai; checkout menghasilkan snapshot order.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS carts (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    buyer_user_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    currency_code TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: cart_items (Unique cart/photo/product agar tidak duplikat jika aturan produk mengizinkan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS cart_items (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    cart_id UUID NOT NULL,
    photo_id UUID NOT NULL,
    product_type TEXT NOT NULL,
    added_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: orders (Snapshot total saat order; uang IDR integer rupiah.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS orders (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    buyer_user_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    currency_code TEXT NOT NULL,
    subtotal_idr BIGINT NOT NULL,
    discount_idr BIGINT NOT NULL,
    total_idr BIGINT NOT NULL,
    placed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: order_items (Snapshot harga, seller, event dan produk saat pembelian.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS order_items (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_id UUID NOT NULL,
    photo_id UUID NOT NULL,
    seller_user_id UUID NOT NULL,
    event_id UUID NULL,
    product_type TEXT NOT NULL,
    unit_price_idr BIGINT NOT NULL,
    quantity INTEGER NOT NULL,
    line_total_idr BIGINT NOT NULL
);

-- ============================================================================
-- Tabel: order_status_events (Audit transisi status order.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS order_status_events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_id UUID NOT NULL,
    from_status TEXT NULL,
    to_status TEXT NOT NULL,
    reason_code TEXT NULL,
    actor_user_id UUID NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: checkout_verifications (Verifikasi OTP/kode pendaftaran sebelum checkout.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS checkout_verifications (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    event_id UUID NULL,
    verification_type TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    verified_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ DEFAULT NULL NULL,
    attempt_count INTEGER NOT NULL
);

-- ============================================================================
-- Tabel: verification_challenges (Challenge OTP; kode tidak disimpan plaintext.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS verification_challenges (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    checkout_verification_id UUID NOT NULL,
    destination_hash TEXT NULL,
    code_hash VARCHAR(255) NOT NULL,
    status VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempt_count INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: checkout_verification_grants (Grant verifikasi terbatas dan terikat cakupan; checkout divalidasi backend.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS checkout_verification_grants (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    event_id UUID NULL,
    order_id UUID NULL,
    verification_id UUID NOT NULL,
    scope VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: payments (Sumber kebenaran status final tetap hasil verifikasi backend/provider.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS payments (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_id UUID NOT NULL,
    provider TEXT NOT NULL,
    provider_payment_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    amount_idr BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    captured_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: payment_events (Webhook inbox/payment event; simpan payload sensitif secara minim.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS payment_events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    payment_id UUID NULL,
    provider_event_id UUID NOT NULL UNIQUE,
    event_type VARCHAR(64) NOT NULL,
    signature_valid BOOLEAN NOT NULL,
    payload_hash TEXT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: refund_requests (Eligible SLA bukan berarti provider sudah mengembalikan dana.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS refund_requests (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_id UUID NOT NULL,
    requested_by UUID NULL,
    reason_code TEXT NOT NULL,
    eligibility_status TEXT NOT NULL,
    decision_status TEXT NOT NULL,
    exception_code TEXT NULL,
    decision_by UUID NULL,
    decision_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: refund_items (Detail item refund, nominal clawback 3 pihak, alokasi biaya gateway, dan status eksekusi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS refund_items (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    refund_request_id UUID NOT NULL,
    order_item_id UUID NOT NULL,
    reason_code TEXT NOT NULL,
    eligible_amount_idr BIGINT NOT NULL,
    buyer_credit_amount_idr BIGINT NOT NULL,
    gateway_fee_idr BIGINT NOT NULL,
    creator_clawback_idr BIGINT NOT NULL,
    host_clawback_idr BIGINT NOT NULL,
    platform_clawback_idr BIGINT NOT NULL,
    destination_type TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: fulfillment_jobs (SLA dilacak per item untuk mendukung partial fulfillment.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS fulfillment_jobs (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_item_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    hires_due_at TIMESTAMPTZ NOT NULL,
    hires_ready_at TIMESTAMPTZ NULL,
    failure_code TEXT NULL,
    attempt_count INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: fulfillment_events (Riwayat transisi fulfillment.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS fulfillment_events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    fulfillment_job_id UUID NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    from_status TEXT NULL,
    to_status TEXT NOT NULL,
    reason_code TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: download_grants (Grant spesifik aset dan order item; signed URL harus berumur terbatas.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS download_grants (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_item_id UUID NOT NULL,
    user_id UUID NOT NULL,
    asset_id UUID NOT NULL,
    grant_type TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    revoked_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: download_events (Audit event pemberian/akses/download; bukan bukti sempurna bahwa file tersimpan di perangkat.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS download_events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    download_grant_id UUID NULL,
    user_id UUID NULL,
    asset_id UUID NULL,
    event_type VARCHAR(64) NOT NULL,
    request_id UUID NULL,
    occurred_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: host_fee_rules (Rule family untuk INTERNAL/INDEPENDENT; precedence org vs event harus ditentukan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_fee_rules (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    event_id UUID NULL,
    creator_type TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: host_fee_rule_versions (Versi tidak diubah setelah aktif; persen basis points adalah usulan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS host_fee_rule_versions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_fee_rule_id UUID NOT NULL,
    version_number INTEGER NOT NULL,
    fee_rate_bps INTEGER NOT NULL,
    calculation_base TEXT NOT NULL,
    rounding_policy TEXT NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NULL,
    created_by UUID NOT NULL
);

-- ============================================================================
-- Tabel: order_revenue_splits (Snapshot nominal per pihak; beneficiary_id polimorfik perlu desain lebih kuat.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS order_revenue_splits (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    order_id UUID NOT NULL,
    order_item_id UUID NULL,
    beneficiary_type VARCHAR(64) NOT NULL,
    beneficiary_id UUID NULL,
    rule_version_id UUID NULL,
    basis_amount_idr BIGINT NOT NULL,
    rate_bps INTEGER NULL,
    amount_idr BIGINT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: ledger_accounts (Pertimbangkan party/beneficiary registry untuk foreign key yang kuat.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ledger_accounts (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    owner_type VARCHAR(64) NOT NULL,
    owner_id UUID NULL,
    account_code TEXT NOT NULL,
    currency_code TEXT NOT NULL,
    status VARCHAR(64) NOT NULL
);

-- ============================================================================
-- Tabel: ledger_transactions (Jurnal append-only; koreksi dengan transaksi kompensasi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ledger_transactions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    transaction_type TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_id UUID NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    status VARCHAR(64) NOT NULL,
    posted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: ledger_entries (Double-entry; total debit dan kredit per transaksi wajib seimbang.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ledger_entries (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    ledger_transaction_id UUID NOT NULL,
    ledger_account_id UUID NOT NULL,
    direction VARCHAR(64) NOT NULL,
    amount_idr BIGINT NOT NULL CHECK (amount_idr > 0),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: settlement_batches (Batch per periode bila settlement agregat diperlukan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS settlement_batches (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    approved_by UUID NULL
);

-- ============================================================================
-- Tabel: settlements (Available hanya setelah fulfillment Hi-Res dan hold period dipenuhi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS settlements (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    settlement_batch_id UUID NULL,
    beneficiary_type VARCHAR(64) NOT NULL,
    beneficiary_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    hold_until TIMESTAMPTZ NOT NULL,
    amount_idr BIGINT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    released_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: payouts (Pembayaran payout eksternal harus dapat direkonsiliasi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS payouts (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    settlement_id UUID NOT NULL,
    host_bank_account_id UUID NULL,
    provider_payout_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    amount_idr BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    submitted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NULL,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: payout_events (Riwayat status payout/provider.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS payout_events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    payout_id UUID NOT NULL,
    provider_event_id UUID NULL,
    event_type VARCHAR(64) NOT NULL,
    status VARCHAR(64) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: storage_accounts (Akun penyimpanan; lebih fleksibel daripada langsung melekat ke user.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS storage_accounts (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    owner_type VARCHAR(64) NOT NULL,
    owner_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: quota_policy_versions (Membedakan inventaris fisik dan kategori yang ditagihkan.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS quota_policy_versions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    policy_code TEXT NOT NULL,
    version_number INTEGER NOT NULL,
    billable_categories JSONB NOT NULL,
    limit_bytes BIGINT NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: storage_quotas (Ringkasan kuota; transaksi atomik mengendalikan reservasi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS storage_quotas (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    storage_account_id UUID NOT NULL,
    policy_version_id UUID NOT NULL,
    quota_limit_bytes BIGINT NOT NULL,
    billable_bytes BIGINT NOT NULL,
    reserved_bytes BIGINT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: quota_reservations (Reservasi sebelum upload URL diberikan, lalu rekonsiliasi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS quota_reservations (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    storage_quota_id UUID NOT NULL,
    upload_batch_id UUID NOT NULL UNIQUE,
    reserved_bytes BIGINT NOT NULL,
    committed_bytes BIGINT NOT NULL,
    status VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    released_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: upload_batches (Satu batch sampai 2.000 file sesuai policy; validasi backend wajib.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS upload_batches (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    storage_account_id UUID NOT NULL,
    owner_user_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    expected_file_count INTEGER NOT NULL,
    expected_bytes BIGINT NOT NULL,
    actual_bytes BIGINT NOT NULL,
    expires_at TIMESTAMPTZ DEFAULT NULL NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: upload_items (Unique batch/client token untuk idempotensi item.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS upload_items (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    upload_batch_id UUID NOT NULL,
    client_file_token TEXT NOT NULL,
    expected_bytes BIGINT NOT NULL,
    actual_bytes BIGINT NULL,
    object_key VARCHAR(255) NULL,
    checksum_sha256 TEXT NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: multipart_upload_sessions (Jangan simpan credential/object-storage secret di database.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS multipart_upload_sessions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    upload_item_id UUID NOT NULL,
    provider_upload_id UUID NOT NULL,
    status VARCHAR(64) NOT NULL,
    part_count INTEGER NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: storage_objects (Inventaris semua objek fisik termasuk derivatif/temporary sesuai observasi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS storage_objects (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    object_key VARCHAR(255) NOT NULL UNIQUE,
    asset_id UUID NULL,
    storage_account_id UUID NULL,
    object_category VARCHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL,
    storage_status VARCHAR(64) NOT NULL,
    checksum_sha256 TEXT NULL,
    observed_at TIMESTAMPTZ NULL,
    deleted_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: storage_usage_entries (Append-only usage record untuk rekonsiliasi dan audit.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS storage_usage_entries (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    storage_account_id UUID NOT NULL,
    storage_object_id UUID NULL,
    event_type VARCHAR(64) NOT NULL,
    physical_delta_bytes BIGINT NOT NULL,
    billable_delta_bytes BIGINT NOT NULL,
    source_type TEXT NOT NULL,
    source_id UUID NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: audit_logs (Audit administratif dan aksi sensitif; tanpa secret/token/raw biometric.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    actor_user_id UUID NULL,
    action_code VARCHAR(64) NOT NULL,
    target_type TEXT NOT NULL,
    target_id UUID NULL,
    reason_code TEXT NULL,
    request_id UUID NULL,
    metadata JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: admin_actions (Aksi admin dengan alasan dan status eksekusi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS admin_actions (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    admin_user_id UUID NOT NULL,
    action_type VARCHAR(64) NOT NULL,
    target_type TEXT NOT NULL,
    target_id UUID NOT NULL,
    reason_code TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    completed_at TIMESTAMPTZ DEFAULT NULL NULL
);

-- ============================================================================
-- Tabel: idempotency_keys (Unique (scope,key), deteksi penggunaan ulang key dengan payload berbeda.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS idempotency_keys (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    scope VARCHAR(64) NOT NULL,
    key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    response_reference TEXT NULL,
    expires_at TIMESTAMPTZ DEFAULT NULL NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: outbox_events (Durable event delivery untuk pekerja/asynchronous systems.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB DEFAULT '{}'::jsonb NOT NULL,
    status VARCHAR(64) NOT NULL,
    attempt_count INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    published_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: webhook_inbox (Dedup webhook; raw payload disimpan hanya bila diperlukan dan terproteksi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS webhook_inbox (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_event_id UUID NOT NULL,
    payload_hash TEXT NULL,
    signature_valid BOOLEAN NOT NULL,
    status VARCHAR(64) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: notifications (Notifikasi yang ditujukan kepada user.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS notifications (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    channel VARCHAR(64) NOT NULL,
    template_code TEXT NOT NULL,
    status VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- ============================================================================
-- Tabel: notification_deliveries (Status pengiriman/retry channel notifikasi.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS notification_deliveries (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    notification_id UUID NOT NULL,
    provider TEXT NOT NULL,
    provider_message_id UUID NULL,
    status VARCHAR(64) NOT NULL,
    attempt_count INTEGER NOT NULL,
    last_attempt_at TIMESTAMPTZ NULL,
    delivered_at TIMESTAMPTZ NULL
);

-- ============================================================================
-- Tabel: system_settings (Config non-secret; secret disimpan di secret manager.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS system_settings (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    setting_key TEXT NOT NULL UNIQUE,
    value JSONB DEFAULT '{}'::jsonb NOT NULL,
    version INTEGER NOT NULL,
    updated_by UUID NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
