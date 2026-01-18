-- Migration: 008_fix_revoke_permission.sql
-- Description: Fix revoke_app_permission function type mismatch
-- Created: 2026-01-18

-- Fix the type mismatch: ROW_COUNT returns INTEGER, not BOOLEAN
CREATE OR REPLACE FUNCTION revoke_app_permission(
    p_user_id VARCHAR,
    p_organization_id UUID,
    p_app_id VARCHAR
) RETURNS BOOLEAN AS $$
DECLARE
    v_deleted INTEGER;  -- Fixed: was BOOLEAN, should be INTEGER
BEGIN
    DELETE FROM app_permissions
    WHERE user_id = p_user_id
      AND organization_id = p_organization_id
      AND app_id = p_app_id;

    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RETURN v_deleted > 0;
END;
$$ LANGUAGE plpgsql;

-- Record migration
INSERT INTO schema_migrations (version) VALUES ('008_fix_revoke_permission')
ON CONFLICT (version) DO NOTHING;
