-- Migration: 006_citizenauth_user_mapping.sql
-- Description: Map CitizenAuth UUIDs to local Citizen user IDs
-- Created: 2025-10-21

-- ============================================================================
-- USER MAPPING TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS citizenauth_user_mapping (
    id SERIAL PRIMARY KEY,
    
    -- CitizenAuth user (UUID from JWT)
    citizenauth_user_id VARCHAR(255) NOT NULL UNIQUE,
    
    -- Local Citizen user (existing integer ID)
    local_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- User info (cached from CitizenAuth)
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    
    -- Sync info
    first_login_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(citizenauth_user_id, local_user_id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_user_mapping_citizenauth_id ON citizenauth_user_mapping(citizenauth_user_id);
CREATE INDEX IF NOT EXISTS idx_user_mapping_local_id ON citizenauth_user_mapping(local_user_id);
CREATE INDEX IF NOT EXISTS idx_user_mapping_email ON citizenauth_user_mapping(email);

COMMENT ON TABLE citizenauth_user_mapping IS 'Maps CitizenAuth users (UUID) to local Citizen users (int ID)';

-- ============================================================================
-- HELPER FUNCTIONS
-- ============================================================================

-- Get or create local user for CitizenAuth user
CREATE OR REPLACE FUNCTION get_or_create_local_user(
    p_citizenauth_uuid VARCHAR,
    p_email VARCHAR,
    p_name VARCHAR
) RETURNS INTEGER AS $$
DECLARE
    v_local_user_id INTEGER;
    v_username VARCHAR;
BEGIN
    -- Check if mapping exists
    SELECT local_user_id INTO v_local_user_id
    FROM citizenauth_user_mapping
    WHERE citizenauth_user_id = p_citizenauth_uuid;
    
    -- Mapping exists, return it
    IF v_local_user_id IS NOT NULL THEN
        -- Update last login
        UPDATE citizenauth_user_mapping
        SET last_login_at = CURRENT_TIMESTAMP
        WHERE citizenauth_user_id = p_citizenauth_uuid;
        
        RETURN v_local_user_id;
    END IF;
    
    -- No mapping, create new local user
    -- Generate username from email
    v_username := SPLIT_PART(p_email, '@', 1);
    
    -- Check if username exists, add number if needed
    WHILE EXISTS (SELECT 1 FROM users WHERE username = v_username) LOOP
        v_username := v_username || '_' || FLOOR(RANDOM() * 1000)::TEXT;
    END LOOP;
    
    -- Create local user (no password - CitizenAuth handles auth)
    INSERT INTO users (username, password, email)
    VALUES (v_username, 'CITIZENAUTH_SSO', p_email)
    RETURNING id INTO v_local_user_id;
    
    -- Create mapping
    INSERT INTO citizenauth_user_mapping (
        citizenauth_user_id,
        local_user_id,
        email,
        name
    ) VALUES (
        p_citizenauth_uuid,
        v_local_user_id,
        p_email,
        p_name
    );
    
    RETURN v_local_user_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_or_create_local_user IS 'Get existing or create new local user for CitizenAuth UUID';

-- Get local user ID from CitizenAuth UUID
CREATE OR REPLACE FUNCTION get_local_user_id(
    p_citizenauth_uuid VARCHAR
) RETURNS INTEGER AS $$
DECLARE
    v_local_user_id INTEGER;
BEGIN
    SELECT local_user_id INTO v_local_user_id
    FROM citizenauth_user_mapping
    WHERE citizenauth_user_id = p_citizenauth_uuid;
    
    RETURN v_local_user_id;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION get_local_user_id IS 'Get local user ID from CitizenAuth UUID';

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('006_citizenauth_user_mapping') 
ON CONFLICT (version) DO NOTHING;

