-- ============================================================================
-- Migration 000004: Photo product catalog and idempotent order receipts
--
-- Migration 000001 has no catalog table for per-photo pricing. Discovery and
-- checkout need one source of truth for price_idr per photo so the cart and
-- order snapshots can carry a concrete integer rupiah value. The idempotency
-- table lets POST /orders return the same order for a replayed Idempotency-Key.
-- ============================================================================

CREATE TABLE IF NOT EXISTS photo_prices (
    photo_id UUID NOT NULL PRIMARY KEY,
    price_idr BIGINT NOT NULL CHECK (price_idr >= 0),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS order_idempotency_keys (
    idempotency_key TEXT NOT NULL PRIMARY KEY,
    buyer_user_id UUID NOT NULL,
    order_id UUID NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);
