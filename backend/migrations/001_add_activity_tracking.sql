-- Migration: 001_add_activity_tracking.sql
-- Description: Add activity tracking tables and enhancements for failed deployments
-- Created: 2024-12-19

-- Create app_activities table for tracking all app-related activities
CREATE TABLE IF NOT EXISTS app_activities (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,
    activity_type VARCHAR(50) NOT NULL, -- deploy, restart, domain, config, env, build
    activity_status VARCHAR(50) NOT NULL, -- success, error, warning, info, pending
    message TEXT NOT NULL,
    details JSONB, -- Additional details like commit_hash, branch, etc.
    user_id INTEGER, -- User who triggered the activity (nullable for webhook)
    trigger_type VARCHAR(50) DEFAULT 'manual', -- manual, webhook, automatic
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration INTEGER, -- Duration in seconds
    error_message TEXT, -- Error details for failed activities
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for app_activities
CREATE INDEX IF NOT EXISTS idx_app_activities_app_name ON app_activities(app_name);
CREATE INDEX IF NOT EXISTS idx_app_activities_type ON app_activities(activity_type);
CREATE INDEX IF NOT EXISTS idx_app_activities_status ON app_activities(activity_status);
CREATE INDEX IF NOT EXISTS idx_app_activities_started_at ON app_activities(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_activities_trigger_type ON app_activities(trigger_type);
CREATE INDEX IF NOT EXISTS idx_app_activities_user_id ON app_activities(user_id);

-- Add deployment failure tracking to existing github_deployment_logs
ALTER TABLE github_deployment_logs 
ADD COLUMN IF NOT EXISTS failure_reason TEXT,
ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_retry_at TIMESTAMP WITH TIME ZONE;

-- Add activity tracking to app_deployments
ALTER TABLE app_deployments
ADD COLUMN IF NOT EXISTS deployment_logs TEXT, -- Store deployment output/logs
ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- Create index for last_activity_at
CREATE INDEX IF NOT EXISTS idx_app_deployments_last_activity_at ON app_deployments(last_activity_at DESC);

-- Add restart tracking
CREATE TABLE IF NOT EXISTS app_restart_logs (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,
    triggered_by INTEGER, -- user_id who triggered restart
    trigger_type VARCHAR(50) DEFAULT 'manual', -- manual, automatic, scheduled
    status VARCHAR(50) NOT NULL, -- success, failed, pending
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration INTEGER, -- Duration in seconds
    output TEXT, -- Restart command output
    error_message TEXT, -- Error if restart failed
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for app_restart_logs
CREATE INDEX IF NOT EXISTS idx_app_restart_logs_app_name ON app_restart_logs(app_name);
CREATE INDEX IF NOT EXISTS idx_app_restart_logs_status ON app_restart_logs(status);
CREATE INDEX IF NOT EXISTS idx_app_restart_logs_started_at ON app_restart_logs(started_at DESC);

-- Domain change tracking
CREATE TABLE IF NOT EXISTS app_domain_logs (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    action VARCHAR(50) NOT NULL, -- add, remove, update
    triggered_by INTEGER, -- user_id who triggered action
    status VARCHAR(50) NOT NULL, -- success, failed, pending
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    output TEXT, -- Command output
    error_message TEXT, -- Error if action failed
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for app_domain_logs
CREATE INDEX IF NOT EXISTS idx_app_domain_logs_app_name ON app_domain_logs(app_name);
CREATE INDEX IF NOT EXISTS idx_app_domain_logs_action ON app_domain_logs(action);
CREATE INDEX IF NOT EXISTS idx_app_domain_logs_status ON app_domain_logs(status);

-- Environment variable change tracking
CREATE TABLE IF NOT EXISTS app_env_logs (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,
    env_key VARCHAR(255) NOT NULL,
    action VARCHAR(50) NOT NULL, -- set, update, remove
    triggered_by INTEGER, -- user_id who triggered action
    status VARCHAR(50) NOT NULL, -- success, failed, pending
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    output TEXT, -- Command output
    error_message TEXT, -- Error if action failed
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for app_env_logs
CREATE INDEX IF NOT EXISTS idx_app_env_logs_app_name ON app_env_logs(app_name);
CREATE INDEX IF NOT EXISTS idx_app_env_logs_action ON app_env_logs(action);
CREATE INDEX IF NOT EXISTS idx_app_env_logs_status ON app_env_logs(status);

-- Migration tracking table
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('001_add_activity_tracking') 
ON CONFLICT (version) DO NOTHING;

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add triggers for updated_at (drop existing first to avoid conflicts)
DROP TRIGGER IF EXISTS update_app_activities_updated_at ON app_activities;
DROP TRIGGER IF EXISTS update_app_deployments_updated_at ON app_deployments;

CREATE TRIGGER update_app_activities_updated_at BEFORE UPDATE ON app_activities FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_app_deployments_updated_at BEFORE UPDATE ON app_deployments FOR EACH ROW EXECUTE FUNCTION update_updated_at_column(); 