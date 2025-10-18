-- Migration: 003_add_api_token_system.sql
-- Description: Add API token system with app-based access control
-- Created: 2024-12-20

-- Create api_tokens table
CREATE TABLE IF NOT EXISTS api_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,        -- SHA-256 hash of the actual token
    token_prefix VARCHAR(12) NOT NULL,              -- First 8-10 chars for identification (ct_abc12345)
    name VARCHAR(100) NOT NULL,                     -- User-defined token name
    description TEXT,                               -- Optional description
    expires_at TIMESTAMP WITH TIME ZONE,            -- Optional expiration date
    last_used_at TIMESTAMP WITH TIME ZONE,          -- Last usage timestamp
    last_used_ip INET,                              -- Last used IP address
    usage_count INTEGER DEFAULT 0,                  -- Total usage count
    is_active BOOLEAN DEFAULT true,                 -- Token active status
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create app_api_access table for per-app API access control
CREATE TABLE IF NOT EXISTS app_api_access (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,                 -- App name (matches Dokku app names)
    api_access_enabled BOOLEAN DEFAULT false,       -- Whether API access is enabled for this app
    allowed_operations TEXT[] DEFAULT ARRAY['*'],   -- Always full access when enabled
    rate_limit_per_minute INTEGER DEFAULT 60,       -- Rate limit per minute for this app
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(app_name)
);

-- Indexes for api_tokens
CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_active ON api_tokens(is_active);
CREATE INDEX IF NOT EXISTS idx_api_tokens_expires_at ON api_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_api_tokens_prefix ON api_tokens(token_prefix);

-- Indexes for app_api_access
CREATE INDEX IF NOT EXISTS idx_app_api_access_app_name ON app_api_access(app_name);
CREATE INDEX IF NOT EXISTS idx_app_api_access_enabled ON app_api_access(api_access_enabled);

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add triggers for updated_at
CREATE TRIGGER update_api_tokens_updated_at 
    BEFORE UPDATE ON api_tokens 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_app_api_access_updated_at 
    BEFORE UPDATE ON app_api_access 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Insert default API access settings for existing apps (if any)
-- This will be handled by the application logic when apps are created

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('003_add_api_token_system') 
ON CONFLICT (version) DO NOTHING;

-- Example data (commented out - will be managed via API)
-- INSERT INTO app_api_access (app_name, api_access_enabled, allowed_operations) VALUES 
-- ('myapp', true, ARRAY['read', 'deploy', 'restart']),
-- ('webapp', false, ARRAY['read']);
