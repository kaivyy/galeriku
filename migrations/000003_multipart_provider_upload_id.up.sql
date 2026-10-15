-- ============================================================================
-- Migration 000003: Correct the multipart upload session provider identifier
--
-- Object storage providers return multipart upload identifiers as opaque strings
-- (MinIO returns a 124-character base64 value), not as UUIDs. Migration 000001
-- declared multipart_upload_sessions.provider_upload_id as UUID, which cannot
-- hold a real provider identifier. This migration widens the column to TEXT so a
-- genuine multipart session can be recorded and later completed.
-- ============================================================================

ALTER TABLE multipart_upload_sessions
    ALTER COLUMN provider_upload_id TYPE TEXT;
