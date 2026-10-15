-- ============================================================================
-- Galeriku: Migration 000002 Rollback: Identity, Sessions & UUIDv7
-- Target Engine: PostgreSQL 16
-- Scope: Rollback Task 2.1
-- ============================================================================

-- Drop indexes
DROP INDEX IF EXISTS uq_active_host_leader;
DROP INDEX IF EXISTS idx_auth_otp_challenges_email_purpose;

-- Drop foreign keys
ALTER TABLE IF EXISTS payouts DROP CONSTRAINT IF EXISTS fk_payouts_host_bank_account;
ALTER TABLE IF EXISTS host_admin_limits DROP CONSTRAINT IF EXISTS fk_host_admin_limits_org;
ALTER TABLE IF EXISTS host_invitations DROP CONSTRAINT IF EXISTS fk_host_invitations_inviter;
ALTER TABLE IF EXISTS host_invitations DROP CONSTRAINT IF EXISTS fk_host_invitations_org;
ALTER TABLE IF EXISTS host_memberships DROP CONSTRAINT IF EXISTS fk_host_memberships_user;
ALTER TABLE IF EXISTS host_memberships DROP CONSTRAINT IF EXISTS fk_host_memberships_org;
ALTER TABLE IF EXISTS host_legal_documents DROP CONSTRAINT IF EXISTS fk_host_legal_documents_application;
ALTER TABLE IF EXISTS host_applications DROP CONSTRAINT IF EXISTS fk_host_applications_submitter;
ALTER TABLE IF EXISTS host_applications DROP CONSTRAINT IF EXISTS fk_host_applications_org;
ALTER TABLE IF EXISTS user_capabilities DROP CONSTRAINT IF EXISTS fk_user_capabilities_capability;
ALTER TABLE IF EXISTS user_capabilities DROP CONSTRAINT IF EXISTS fk_user_capabilities_user;
ALTER TABLE IF EXISTS user_profiles DROP CONSTRAINT IF EXISTS fk_user_profiles_user;

-- Drop newly added tables
DROP TABLE IF EXISTS host_bank_accounts CASCADE;
DROP TABLE IF EXISTS auth_otp_challenges CASCADE;
DROP TABLE IF EXISTS auth_refresh_tokens CASCADE;
DROP TABLE IF EXISTS user_credentials CASCADE;

-- Deregister migration from tracking table
DELETE FROM schema_migrations WHERE version = '000002';
