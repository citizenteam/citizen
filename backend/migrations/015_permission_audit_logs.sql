-- Migration: 015_permission_audit_logs
-- Description: Add audit logging for permission changes (grant/revoke)

-- Create permission_audit_logs table
CREATE TABLE IF NOT EXISTS permission_audit_logs (
    id SERIAL PRIMARY KEY,
    event_type VARCHAR(30) NOT NULL,           -- 'permission.granted', 'permission.revoked'
    organization_id UUID NOT NULL,
    app_id VARCHAR(100) NOT NULL,
    role VARCHAR(20),                          -- Role granted (null for revoke)
    performed_by VARCHAR(255),                 -- Who performed the action
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_perm_audit_org ON permission_audit_logs(organization_id);
CREATE INDEX IF NOT EXISTS idx_perm_audit_app ON permission_audit_logs(app_id);
CREATE INDEX IF NOT EXISTS idx_perm_audit_type ON permission_audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_perm_audit_time ON permission_audit_logs(created_at DESC);

COMMENT ON TABLE permission_audit_logs IS 'Audit log for permission changes (grant/revoke operations)';

-- Record migration
INSERT INTO schema_migrations (version) VALUES ('015_permission_audit_logs')
ON CONFLICT (version) DO NOTHING;

