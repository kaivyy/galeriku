-- ============================================================================
-- Galeriku: Migration 000002: Identity, Sessions & UUIDv7 Runtime Fix
-- Target Engine: PostgreSQL 16
-- Scope: Task 2.1 (Critical UUIDv7 Fix, Credentials, Sessions, Bank Accounts & FKs)
-- Date: 15 Oktober 2026
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. Fix UUIDv7 Generator Runtime Overflow (P0 Blocker)
-- ----------------------------------------------------------------------------
-- Root cause: Casting v_time_ms (BIGINT epoch milliseconds ~1.79T+) to INTEGER
-- without bitwise masking causes "integer out of range" error in PostgreSQL.
-- Fix: Apply bitwise mask `& 255` on all byte positions before conversion.
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

    -- 48-bit timestamp (epoch ms) split into 6 bytes with bitwise masking (& 255)
    v_bytes := SET_BYTE(v_bytes, 0, ((v_time_ms >> 40) & 255)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 1, ((v_time_ms >> 32) & 255)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 2, ((v_time_ms >> 24) & 255)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 3, ((v_time_ms >> 16) & 255)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 4, ((v_time_ms >> 8) & 255)::INTEGER);
    v_bytes := SET_BYTE(v_bytes, 5, (v_time_ms & 255)::INTEGER);

    -- Version 7: 0111xxxx (mask higher 4 bits with 0x70 / 112)
    v_bytes := SET_BYTE(v_bytes, 6, (GET_BYTE(v_bytes, 6) & 15) | 112);

    -- Variant 10xx: 10xxxxxx (mask higher 2 bits with 0x80 / 128)
    v_bytes := SET_BYTE(v_bytes, 8, (GET_BYTE(v_bytes, 8) & 63) | 128);

    RETURN ENCODE(v_bytes, 'hex')::UUID;
END;
$$ LANGUAGE plpgsql VOLATILE;

CREATE OR REPLACE FUNCTION uuid_generate_v7()
RETURNS UUID AS $$
BEGIN
    RETURN generate_uuid_v7();
END;
$$ LANGUAGE plpgsql VOLATILE;

-- ----------------------------------------------------------------------------
-- 2. User Credentials Table (Password Hash & Account Security)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_credentials (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    password_algo VARCHAR(32) DEFAULT 'argon2id' NOT NULL,
    failed_attempts INTEGER DEFAULT 0 NOT NULL,
    locked_until TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT fk_user_credentials_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- ----------------------------------------------------------------------------
-- 3. Auth Refresh Tokens Table (Stateful Token Rotation & Revocation)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS auth_refresh_tokens (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    family_id UUID NOT NULL,
    is_revoked BOOLEAN DEFAULT FALSE NOT NULL,
    revoked_reason VARCHAR(64) NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    user_agent TEXT NULL,
    ip_address VARCHAR(45) NULL,
    CONSTRAINT fk_auth_refresh_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_user_id ON auth_refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_family_id ON auth_refresh_tokens(family_id);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_tokens_expires_at ON auth_refresh_tokens(expires_at);

-- ----------------------------------------------------------------------------
-- 4. Auth OTP Challenges Table (Phone & General Verification)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS auth_otp_challenges (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    phone_e164 VARCHAR(32) NULL,
    email_normalized VARCHAR(128) NULL,
    purpose VARCHAR(64) NOT NULL,
    code_hash VARCHAR(64) NOT NULL,
    attempt_count INTEGER DEFAULT 0 NOT NULL,
    max_attempts INTEGER DEFAULT 3 NOT NULL,
    status VARCHAR(32) DEFAULT 'PENDING' NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT chk_otp_target CHECK (phone_e164 IS NOT NULL OR email_normalized IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_auth_otp_challenges_phone_purpose ON auth_otp_challenges(phone_e164, purpose, status);
CREATE INDEX IF NOT EXISTS idx_auth_otp_challenges_email_purpose ON auth_otp_challenges(email_normalized, purpose, status);
CREATE INDEX IF NOT EXISTS idx_auth_otp_challenges_expires_at ON auth_otp_challenges(expires_at);

-- ----------------------------------------------------------------------------
-- 5. Host Bank Accounts Table (Resolves Broken Reference in payouts)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS host_bank_accounts (
    id UUID DEFAULT generate_uuid_v7() NOT NULL PRIMARY KEY,
    host_organization_id UUID NOT NULL,
    bank_code VARCHAR(32) NOT NULL,
    account_number_encrypted TEXT NOT NULL,
    account_number_masked VARCHAR(32) NOT NULL,
    account_holder_name VARCHAR(128) NOT NULL,
    verification_status VARCHAR(32) DEFAULT 'PENDING' NOT NULL,
    verified_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT fk_host_bank_accounts_org FOREIGN KEY (host_organization_id) REFERENCES host_organizations(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_host_bank_accounts_org ON host_bank_accounts(host_organization_id);

-- Connect payouts table foreign key to host_bank_accounts
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_payouts_host_bank_account'
    ) THEN
        ALTER TABLE payouts ADD CONSTRAINT fk_payouts_host_bank_account 
        FOREIGN KEY (host_bank_account_id) REFERENCES host_bank_accounts(id) ON DELETE RESTRICT;
    END IF;
END $$;

-- ----------------------------------------------------------------------------
-- 6. Domain 01 Foreign Key Constraints (Identity & Host)
-- ----------------------------------------------------------------------------
DO $$
BEGIN
    -- user_profiles -> users
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_user_profiles_user') THEN
        ALTER TABLE user_profiles ADD CONSTRAINT fk_user_profiles_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;

    -- user_capabilities -> users
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_user_capabilities_user') THEN
        ALTER TABLE user_capabilities ADD CONSTRAINT fk_user_capabilities_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;

    -- user_capabilities -> capabilities
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_user_capabilities_capability') THEN
        ALTER TABLE user_capabilities ADD CONSTRAINT fk_user_capabilities_capability FOREIGN KEY (capability_id) REFERENCES capabilities(id) ON DELETE RESTRICT;
    END IF;

    -- host_applications -> host_organizations
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_applications_org') THEN
        ALTER TABLE host_applications ADD CONSTRAINT fk_host_applications_org FOREIGN KEY (host_organization_id) REFERENCES host_organizations(id) ON DELETE RESTRICT;
    END IF;

    -- host_applications -> users (submitted_by)
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_applications_submitter') THEN
        ALTER TABLE host_applications ADD CONSTRAINT fk_host_applications_submitter FOREIGN KEY (submitted_by) REFERENCES users(id) ON DELETE RESTRICT;
    END IF;

    -- host_legal_documents -> host_applications
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_legal_documents_application') THEN
        ALTER TABLE host_legal_documents ADD CONSTRAINT fk_host_legal_documents_application FOREIGN KEY (host_application_id) REFERENCES host_applications(id) ON DELETE CASCADE;
    END IF;

    -- host_memberships -> host_organizations
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_memberships_org') THEN
        ALTER TABLE host_memberships ADD CONSTRAINT fk_host_memberships_org FOREIGN KEY (host_organization_id) REFERENCES host_organizations(id) ON DELETE CASCADE;
    END IF;

    -- host_memberships -> users
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_memberships_user') THEN
        ALTER TABLE host_memberships ADD CONSTRAINT fk_host_memberships_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;

    -- host_invitations -> host_organizations
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_invitations_org') THEN
        ALTER TABLE host_invitations ADD CONSTRAINT fk_host_invitations_org FOREIGN KEY (host_organization_id) REFERENCES host_organizations(id) ON DELETE CASCADE;
    END IF;

    -- host_invitations -> users (invited_by)
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_invitations_inviter') THEN
        ALTER TABLE host_invitations ADD CONSTRAINT fk_host_invitations_inviter FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE RESTRICT;
    END IF;

    -- host_admin_limits -> host_organizations
    IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'fk_host_admin_limits_org') THEN
        ALTER TABLE host_admin_limits ADD CONSTRAINT fk_host_admin_limits_org FOREIGN KEY (host_organization_id) REFERENCES host_organizations(id) ON DELETE CASCADE;
    END IF;
END $$;

-- ----------------------------------------------------------------------------
-- 7. Single Active Host Leader Invariant (OPEN-10 Decision)
-- ----------------------------------------------------------------------------
-- Exactly 1 active Host Leader is permitted per organization.
-- Inactive/revoked/transferred memberships are permitted historically.
-- ----------------------------------------------------------------------------
CREATE UNIQUE INDEX IF NOT EXISTS uq_active_host_leader 
ON host_memberships (host_organization_id) 
WHERE membership_role = 'LEADER' AND status = 'ACTIVE';
