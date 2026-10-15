-- ============================================================================
-- Migration 000004 rollback: drop the catalog and idempotency tables.
-- ============================================================================

DROP TABLE IF EXISTS order_idempotency_keys;
DROP TABLE IF EXISTS photo_prices;
