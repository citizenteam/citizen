-- Migration: 007_citizenauth_multitenancy.sql
-- Description: Add organization scoping and CitizenAuth instance configuration tables
-- Created: 2025-10-27

-- Add organization scoping to citizenauth_user_mapping
ALTER TABLE citizenauth_user_mapping
    ADD COLUMN IF NOT EXISTS organization_id UUID;

-- Drop legacy unique constraints
ALTER TABLE citizenauth_user_mapping
    DROP CONSTRAINT IF EXISTS citizenauth_user_mapping_citizenauth_user_id_key,
    DROP CONSTRAINT IF EXISTS citizenauth_user_mapping_citizenauth_user_id_local_user_id_key,
    DROP CONSTRAINT IF EXISTS citizenauth_user_mapping_user_org_unique,
    DROP CONSTRAINT IF EXISTS citizenauth_user_mapping_local_org_unique;

-- Add new uniqueness constraints with organization scope
ALTER TABLE citizenauth_user_mapping
    ADD CONSTRAINT citizenauth_user_mapping_user_org_unique UNIQUE (citizenauth_user_id, organization_id),
    ADD CONSTRAINT citizenauth_user_mapping_local_org_unique UNIQUE (organization_id, local_user_id);

CREATE INDEX IF NOT EXISTS idx_citizenauth_user_mapping_org
    ON citizenauth_user_mapping(organization_id);

-- Add organization scoping to app_permissions
ALTER TABLE app_permissions
    ADD COLUMN IF NOT EXISTS organization_id UUID;

ALTER TABLE app_permissions
    DROP CONSTRAINT IF EXISTS app_permissions_user_id_app_id_key,
    DROP CONSTRAINT IF EXISTS app_permissions_user_app_org_unique;

ALTER TABLE app_permissions
    ADD CONSTRAINT app_permissions_user_app_org_unique UNIQUE (user_id, app_id, organization_id);

CREATE INDEX IF NOT EXISTS idx_app_permissions_org
    ON app_permissions(organization_id);

-- CitizenAuth instance configuration table
CREATE TABLE IF NOT EXISTS citizenauth_instances (
    id SERIAL PRIMARY KEY,
    instance_uuid UUID NOT NULL UNIQUE,
    organization_id UUID NOT NULL,
    domain VARCHAR(500),
    citizenauth_url VARCHAR(500),
    api_key_hash TEXT,
    api_key_prefix VARCHAR(20),
    api_key_encrypted TEXT,
    webhook_secret_encrypted TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    registered_at TIMESTAMPTZ,
    last_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_citizenauth_instances_org
    ON citizenauth_instances(organization_id);

CREATE INDEX IF NOT EXISTS idx_citizenauth_instances_status
    ON citizenauth_instances(status);

COMMENT ON TABLE citizenauth_instances IS 'CitizenAuth instance configuration and secrets';

ALTER TABLE citizenauth_instances
    ADD COLUMN IF NOT EXISTS api_key_hash TEXT,
    ADD COLUMN IF NOT EXISTS api_key_encrypted TEXT;

-- Update helper functions for organization scope

CREATE OR REPLACE FUNCTION get_or_create_local_user(
    p_citizenauth_uuid VARCHAR,
    p_email VARCHAR,
    p_name VARCHAR,
    p_organization_id UUID
) RETURNS INTEGER AS $$
DECLARE
    v_local_user_id INTEGER;
    v_username VARCHAR;
BEGIN
    SELECT local_user_id INTO v_local_user_id
    FROM citizenauth_user_mapping
    WHERE citizenauth_user_id = p_citizenauth_uuid
      AND organization_id = p_organization_id;

    IF v_local_user_id IS NOT NULL THEN
        UPDATE citizenauth_user_mapping
        SET last_login_at = CURRENT_TIMESTAMP,
            email = COALESCE(p_email, email),
            name = COALESCE(p_name, name)
        WHERE citizenauth_user_id = p_citizenauth_uuid
          AND organization_id = p_organization_id;

        RETURN v_local_user_id;
    END IF;

    v_username := SPLIT_PART(p_email, '@', 1);

    WHILE EXISTS (SELECT 1 FROM users WHERE username = v_username) LOOP
        v_username := v_username || '_' || FLOOR(RANDOM() * 1000)::TEXT;
    END LOOP;

    INSERT INTO users (username, password, email)
    VALUES (v_username, 'CITIZENAUTH_SSO', p_email)
    RETURNING id INTO v_local_user_id;

    INSERT INTO citizenauth_user_mapping (
        citizenauth_user_id,
        local_user_id,
        organization_id,
        email,
        name
    ) VALUES (
        p_citizenauth_uuid,
        v_local_user_id,
        p_organization_id,
        p_email,
        p_name
    );

    RETURN v_local_user_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_local_user_id(
    p_citizenauth_uuid VARCHAR,
    p_organization_id UUID
) RETURNS INTEGER AS $$
DECLARE
    v_local_user_id INTEGER;
BEGIN
    SELECT local_user_id INTO v_local_user_id
    FROM citizenauth_user_mapping
    WHERE citizenauth_user_id = p_citizenauth_uuid
      AND organization_id = p_organization_id;

    RETURN v_local_user_id;
END;
$$ LANGUAGE plpgsql;

-- Update permission helper functions with organization scope

CREATE OR REPLACE FUNCTION check_app_permission(
    p_user_id VARCHAR,
    p_organization_id UUID,
    p_app_id VARCHAR,
    p_required_role VARCHAR
) RETURNS BOOLEAN AS $$
DECLARE
    v_user_role VARCHAR;
    v_role_level INT;
    v_required_level INT;
BEGIN
    SELECT role INTO v_user_role
    FROM app_permissions
    WHERE user_id = p_user_id
      AND organization_id = p_organization_id
      AND app_id = p_app_id;

    IF v_user_role IS NULL THEN
        SELECT role INTO v_user_role
        FROM app_permissions
        WHERE user_id = p_user_id
          AND organization_id = p_organization_id
          AND app_id = '__all__';
    END IF;

    IF v_user_role IS NULL THEN
        RETURN FALSE;
    END IF;

    v_role_level := CASE v_user_role
        WHEN 'viewer' THEN 1
        WHEN 'member' THEN 2
        WHEN 'admin' THEN 3
        ELSE 0
    END;

    v_required_level := CASE p_required_role
        WHEN 'viewer' THEN 1
        WHEN 'member' THEN 2
        WHEN 'admin' THEN 3
        ELSE 999
    END;

    RETURN v_role_level >= v_required_level;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION grant_app_permission(
    p_user_id VARCHAR,
    p_organization_id UUID,
    p_app_id VARCHAR,
    p_role VARCHAR,
    p_granted_by VARCHAR
) RETURNS VOID AS $$
BEGIN
    INSERT INTO app_permissions (
        user_id,
        organization_id,
        app_id,
        role,
        granted_by
    ) VALUES (
        p_user_id,
        p_organization_id,
        p_app_id,
        p_role,
        p_granted_by
    )
    ON CONFLICT (user_id, app_id, organization_id)
    DO UPDATE SET
        role = EXCLUDED.role,
        granted_by = EXCLUDED.granted_by,
        updated_at = NOW();
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION revoke_app_permission(
    p_user_id VARCHAR,
    p_organization_id UUID,
    p_app_id VARCHAR
) RETURNS BOOLEAN AS $$
DECLARE
    v_deleted BOOLEAN;
BEGIN
    DELETE FROM app_permissions
    WHERE user_id = p_user_id
      AND organization_id = p_organization_id
      AND app_id = p_app_id;

    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RETURN v_deleted > 0;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_user_app_permissions(
    p_user_id VARCHAR,
    p_organization_id UUID
) RETURNS TABLE (
    app_id VARCHAR,
    role VARCHAR,
    granted_at TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        ap.app_id,
        ap.role,
        ap.granted_at
    FROM app_permissions ap
    WHERE ap.user_id = p_user_id
      AND ap.organization_id = p_organization_id
    ORDER BY ap.granted_at DESC;
END;
$$ LANGUAGE plpgsql;

-- Record migration
INSERT INTO schema_migrations (version) VALUES ('007_citizenauth_multitenancy')
ON CONFLICT (version) DO NOTHING;
