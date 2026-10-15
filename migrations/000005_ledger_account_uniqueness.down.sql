-- ============================================================================
-- Migration 000005 rollback: drop the unique index.
-- ============================================================================

DROP INDEX IF EXISTS uq_ledger_account_identity;
