-- Migration: 005_app_permissions.sql
-- Description: App-level permission system for multi-user access control
-- Created: 2025-10-21

-- ============================================================================
-- APP PERMISSIONS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS app_permissions (
    id SERIAL PRIMARY KEY,
    
    -- User identification (from CitizenAuth JWT)
    user_id VARCHAR(255) NOT NULL,  -- UUID from CitizenAuth
    
    -- App identification (local app)
    app_id VARCHAR(255) NOT NULL,   -- App name or ID
    
    -- Permission level
    role VARCHAR(50) NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
    
    -- Audit trail
    granted_by VARCHAR(255),         -- Admin user ID who granted permission
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Ensure unique permission per user per app
    UNIQUE(user_id, app_id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_app_perms_user ON app_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_app_perms_app ON app_permissions(app_id);
CREATE INDEX IF NOT EXISTS idx_app_perms_user_app ON app_permissions(user_id, app_id);
CREATE INDEX IF NOT EXISTS idx_app_perms_role ON app_permissions(role);

COMMENT ON TABLE app_permissions IS 'App-level permissions for users (synced from CitizenAuth)';
COMMENT ON COLUMN app_permissions.user_id IS 'User UUID from CitizenAuth JWT';
COMMENT ON COLUMN app_permissions.app_id IS 'Local app identifier (app name)';
COMMENT ON COLUMN app_permissions.role IS 'Permission level: admin (full), member (deploy), viewer (read-only)';

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Check if user has permission for app
CREATE OR REPLACE FUNCTION check_app_permission(
    p_user_id VARCHAR,
    p_app_id VARCHAR,
    p_required_role VARCHAR
) RETURNS BOOLEAN AS $$
DECLARE
    v_user_role VARCHAR;
    v_role_level INT;
    v_required_level INT;
BEGIN
    -- Get user's role for this app
    SELECT role INTO v_user_role
    FROM app_permissions
    WHERE user_id = p_user_id AND app_id = p_app_id;
    
    -- No permission found
    IF v_user_role IS NULL THEN
        RETURN FALSE;
    END IF;
    
    -- Role hierarchy: viewer=1, member=2, admin=3
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

COMMENT ON FUNCTION check_app_permission IS 'Check if user has sufficient permission level for app';

-- Get user's permissions for all apps
CREATE OR REPLACE FUNCTION get_user_app_permissions(
    p_user_id VARCHAR
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
    ORDER BY ap.granted_at DESC;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_user_app_permissions IS 'Get all app permissions for a user';

-- Grant or update app permission
CREATE OR REPLACE FUNCTION grant_app_permission(
    p_user_id VARCHAR,
    p_app_id VARCHAR,
    p_role VARCHAR,
    p_granted_by VARCHAR
) RETURNS VOID AS $$
BEGIN
    INSERT INTO app_permissions (
        user_id,
        app_id,
        role,
        granted_by
    ) VALUES (
        p_user_id,
        p_app_id,
        p_role,
        p_granted_by
    )
    ON CONFLICT (user_id, app_id)
    DO UPDATE SET
        role = EXCLUDED.role,
        granted_by = EXCLUDED.granted_by,
        updated_at = NOW();
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION grant_app_permission IS 'Grant or update user permission for app (upsert)';

-- Revoke app permission
CREATE OR REPLACE FUNCTION revoke_app_permission(
    p_user_id VARCHAR,
    p_app_id VARCHAR
) RETURNS BOOLEAN AS $$
DECLARE
    v_deleted BOOLEAN;
BEGIN
    DELETE FROM app_permissions
    WHERE user_id = p_user_id AND app_id = p_app_id;
    
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RETURN v_deleted > 0;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION revoke_app_permission IS 'Revoke user permission for app';

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Update updated_at on permission changes
CREATE OR REPLACE FUNCTION update_app_permission_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS update_app_permissions_updated_at ON app_permissions;
CREATE TRIGGER update_app_permissions_updated_at
    BEFORE UPDATE ON app_permissions
    FOR EACH ROW
    EXECUTE FUNCTION update_app_permission_updated_at();

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('005_app_permissions') 
ON CONFLICT (version) DO NOTHING;

