-- ============================================================================
-- Migration 000003 rollback: restore the UUID column type.
--
-- This rollback is deliberately restrictive. UUID cannot represent a real
-- provider upload identifier, so rows holding a genuine identifier cannot be
-- converted back; the rollback fails rather than silently corrupting them.
-- Rows must be cleared first if the type must be reverted.
-- ============================================================================

ALTER TABLE multipart_upload_sessions
    ALTER COLUMN provider_upload_id TYPE UUID
    USING provider_upload_id::uuid;
