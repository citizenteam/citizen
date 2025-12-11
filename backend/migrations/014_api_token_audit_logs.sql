-- Migration: 014_api_token_audit_logs
-- Description: Add audit logging for API token usage and failed attempts

-- Create api_token_audit_logs table
CREATE TABLE IF NOT EXISTS api_token_audit_logs (
    id SERIAL PRIMARY KEY,
    token_prefix VARCHAR(15),           -- Token prefix for identification (ct_xxxxxxxx)
    local_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,  -- Local user ID
    citizenauth_user_id VARCHAR(255),   -- CitizenAuth UUID (for permission check)
    organization_id UUID,               -- Organization context
    event_type VARCHAR(20) NOT NULL,    -- 'success', 'failed', 'revoked'
    failure_reason VARCHAR(50),         -- 'invalid_format', 'not_found', 'expired', 'inactive'
    ip_address VARCHAR(45) NOT NULL,    -- IPv4 or IPv6
    user_agent TEXT,                    -- Browser/client info
    app_name VARCHAR(100),              -- Target app if applicable
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_audit_logs_token_prefix ON api_token_audit_logs(token_prefix);
CREATE INDEX IF NOT EXISTS idx_audit_logs_local_user_id ON api_token_audit_logs(local_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_citizenauth_user_id ON api_token_audit_logs(citizenauth_user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_id ON api_token_audit_logs(organization_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_event_type ON api_token_audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_ip_address ON api_token_audit_logs(ip_address);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON api_token_audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_app_name ON api_token_audit_logs(app_name);

-- Composite index for admin queries
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_type_time ON api_token_audit_logs(organization_id, event_type, created_at DESC);

COMMENT ON TABLE api_token_audit_logs IS 'Audit log for API token usage and security events';

-- =====================
-- Row Level Security (RLS)
-- =====================

-- Enable RLS on api_token_audit_logs
ALTER TABLE api_token_audit_logs ENABLE ROW LEVEL SECURITY;

-- Note: Super admin check (is_super_admin from JWT) is done at application level
-- RLS handles org-based permissions

-- Policy 1: Org owners (role=admin, app_id='__all__') can see all logs in their org
CREATE POLICY audit_logs_org_owner ON api_token_audit_logs
    FOR SELECT
    TO PUBLIC
    USING (
        organization_id = current_setting('app.current_org_id', true)::uuid
        AND EXISTS (
            SELECT 1 FROM app_permissions 
            WHERE user_id = current_setting('app.current_citizenauth_user_id', true)
            AND organization_id = current_setting('app.current_org_id', true)::uuid
            AND app_id = '__all__'
            AND role = 'admin'
        )
    );

-- Policy 2: Server admins can see logs for their specific apps
CREATE POLICY audit_logs_server_admin ON api_token_audit_logs
    FOR SELECT
    TO PUBLIC
    USING (
        organization_id = current_setting('app.current_org_id', true)::uuid
        AND app_name IS NOT NULL
        AND EXISTS (
            SELECT 1 FROM app_permissions 
            WHERE user_id = current_setting('app.current_citizenauth_user_id', true)
            AND organization_id = current_setting('app.current_org_id', true)::uuid
            AND app_id = api_token_audit_logs.app_name
            AND role = 'admin'
        )
    );

-- Policy 3: Users can see their OWN token audit logs
CREATE POLICY audit_logs_owner_only ON api_token_audit_logs
    FOR SELECT
    TO PUBLIC
    USING (
        local_user_id = current_setting('app.current_local_user_id', true)::integer
    );

-- Policy 4: Anyone can INSERT (for logging from app)
CREATE POLICY audit_logs_insert ON api_token_audit_logs
    FOR INSERT
    TO PUBLIC
    WITH CHECK (true);

-- =====================
-- Enable RLS on api_tokens table
-- =====================

-- Enable RLS on api_tokens  
ALTER TABLE api_tokens ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only manage their OWN tokens
CREATE POLICY api_tokens_owner_only ON api_tokens
    FOR ALL
    TO PUBLIC
    USING (user_id = current_setting('app.current_local_user_id', true)::integer);

-- Record migration
INSERT INTO schema_migrations (version) VALUES ('014_api_token_audit_logs')
ON CONFLICT (version) DO NOTHING;
