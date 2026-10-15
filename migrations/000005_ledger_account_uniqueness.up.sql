-- ============================================================================
-- Migration 000005: Ledger account uniqueness
--
-- The double-entry engine resolves accounts by (owner_type, owner_id,
-- account_code, currency_code). Without a unique constraint, a concurrent
-- posting could create duplicate accounts and split one wallet's balance
-- across rows. The NULL owner_id (platform accounts) is normalized to empty
-- text inside the index expression so platform rows stay unique too.
-- ============================================================================

-- Deduplicate existing rows: keep the earliest id per identity key.
DELETE FROM ledger_accounts a
USING ledger_accounts b
WHERE a.owner_type = b.owner_type
  AND COALESCE(a.owner_id::text, '') = COALESCE(b.owner_id::text, '')
  AND a.account_code = b.account_code
  AND a.currency_code = b.currency_code
  AND a.id > b.id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_ledger_account_identity
    ON ledger_accounts (owner_type, COALESCE(owner_id::text, ''), account_code, currency_code);
