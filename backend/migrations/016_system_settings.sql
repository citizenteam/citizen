-- System Settings Table
-- Stores global configuration for the Citizen instance

CREATE TABLE IF NOT EXISTS system_settings (
    id SERIAL PRIMARY KEY,
    key VARCHAR(100) UNIQUE NOT NULL,
    value TEXT NOT NULL,
    description TEXT,
    value_type VARCHAR(20) DEFAULT 'string', -- string, int, bool, json
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create index for fast key lookup
CREATE INDEX IF NOT EXISTS idx_system_settings_key ON system_settings(key);

-- Insert default settings
INSERT INTO system_settings (key, value, description, value_type) VALUES
    ('deployment_queue_enabled', 'true', 'Enable deployment queue for managing concurrent builds', 'bool'),
    ('deployment_queue_workers', '3', 'Number of concurrent deployment workers', 'int'),
    ('build_timeout_minutes', '30', 'Maximum time for a build before timeout', 'int'),
    ('auto_cleanup_old_builds', 'true', 'Automatically cleanup old build jobs', 'bool'),
    ('max_builds_per_app', '10', 'Maximum number of build history entries per app', 'int')
ON CONFLICT (key) DO NOTHING;

-- Add comment
COMMENT ON TABLE system_settings IS 'Global system configuration settings';
