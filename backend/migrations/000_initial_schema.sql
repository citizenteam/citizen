-- Migration: 000_initial_schema.sql
-- Description: Initial database schema with all tables
-- Created: 2024-12-19

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    email VARCHAR(100) UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add GitHub OAuth columns to existing users table
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS github_id INTEGER UNIQUE,
ADD COLUMN IF NOT EXISTS github_username VARCHAR(100),
ADD COLUMN IF NOT EXISTS github_access_token VARCHAR(255),
ADD COLUMN IF NOT EXISTS github_connected BOOLEAN DEFAULT false;

-- Indexes for users table
CREATE INDEX IF NOT EXISTS idx_users_github_id ON users(github_id);
CREATE INDEX IF NOT EXISTS idx_users_github_connected ON users(github_connected);

-- Create app_custom_domains table
CREATE TABLE IF NOT EXISTS app_custom_domains (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,
    domain VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(app_name, domain)
);

-- Indexes for app_custom_domains
CREATE INDEX IF NOT EXISTS idx_app_custom_domains_app_name ON app_custom_domains(app_name);
CREATE INDEX IF NOT EXISTS idx_app_custom_domains_domain ON app_custom_domains(domain);
CREATE INDEX IF NOT EXISTS idx_app_custom_domains_active ON app_custom_domains(is_active);

-- Create app_public_settings table
CREATE TABLE IF NOT EXISTS app_public_settings (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL UNIQUE,
    is_public BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for app_public_settings
CREATE INDEX IF NOT EXISTS idx_app_public_settings_app_name ON app_public_settings(app_name);
CREATE INDEX IF NOT EXISTS idx_app_public_settings_public ON app_public_settings(is_public);

-- Create app_deployments table
CREATE TABLE IF NOT EXISTS app_deployments (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL UNIQUE,
    domain VARCHAR(255),
    port INTEGER,
    builder VARCHAR(50),
    buildpack VARCHAR(100),
    git_url VARCHAR(500),
    git_branch VARCHAR(100),
    git_commit VARCHAR(100),
    port_source VARCHAR(50),
    status VARCHAR(50) DEFAULT 'pending',
    last_deploy TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Add missing columns to existing app_deployments table
ALTER TABLE app_deployments 
ADD COLUMN IF NOT EXISTS deployment_logs TEXT,
ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

-- Indexes for app_deployments
CREATE INDEX IF NOT EXISTS idx_app_deployments_app_name ON app_deployments(app_name);
CREATE INDEX IF NOT EXISTS idx_app_deployments_status ON app_deployments(status);
CREATE INDEX IF NOT EXISTS idx_app_deployments_deleted_at ON app_deployments(deleted_at);
CREATE INDEX IF NOT EXISTS idx_app_deployments_last_activity_at ON app_deployments(last_activity_at DESC);

-- Create github_repositories table
CREATE TABLE IF NOT EXISTS github_repositories (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    app_name VARCHAR(100) NOT NULL UNIQUE,
    github_id BIGINT NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    owner VARCHAR(255) NOT NULL,
    clone_url VARCHAR(500) NOT NULL,
    html_url VARCHAR(500) NOT NULL,
    private BOOLEAN DEFAULT false,
    default_branch VARCHAR(100) DEFAULT 'main',
    auto_deploy_enabled BOOLEAN DEFAULT false,
    deploy_branch VARCHAR(100) DEFAULT 'main',
    webhook_id BIGINT,
    webhook_secret VARCHAR(255),
    webhook_active BOOLEAN DEFAULT false,
    connected_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_deploy TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for github_repositories
CREATE INDEX IF NOT EXISTS idx_github_repositories_user_id ON github_repositories(user_id);
CREATE INDEX IF NOT EXISTS idx_github_repositories_app_name ON github_repositories(app_name);
CREATE INDEX IF NOT EXISTS idx_github_repositories_github_id ON github_repositories(github_id);
CREATE INDEX IF NOT EXISTS idx_github_repositories_deleted_at ON github_repositories(deleted_at);

-- Create github_config table
CREATE TABLE IF NOT EXISTS github_config (
    id SERIAL PRIMARY KEY,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    webhook_secret TEXT NOT NULL,
    redirect_uri VARCHAR(500) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for github_config
CREATE INDEX IF NOT EXISTS idx_github_config_active ON github_config(is_active);

-- Create github_webhook_events table
CREATE TABLE IF NOT EXISTS github_webhook_events (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    action VARCHAR(50),
    ref VARCHAR(255),
    before_commit VARCHAR(100),
    after_commit VARCHAR(100),
    payload_size INTEGER,
    payload_hash VARCHAR(255),
    github_id VARCHAR(100),
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMP WITH TIME ZONE,
    deploy_triggered BOOLEAN DEFAULT false,
    deploy_success BOOLEAN,
    error_message TEXT,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for github_webhook_events
CREATE INDEX IF NOT EXISTS idx_github_webhook_events_repository_id ON github_webhook_events(repository_id);
CREATE INDEX IF NOT EXISTS idx_github_webhook_events_event_type ON github_webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_github_webhook_events_processed ON github_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_github_webhook_events_github_id ON github_webhook_events(github_id);

-- Create github_deployment_logs table
CREATE TABLE IF NOT EXISTS github_deployment_logs (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL,
    event_id INTEGER,
    app_name VARCHAR(100) NOT NULL,
    commit_hash VARCHAR(100) NOT NULL,
    commit_message TEXT,
    branch VARCHAR(100) NOT NULL,
    author_name VARCHAR(255),
    author_email VARCHAR(255),
    trigger_type VARCHAR(50),
    status VARCHAR(50),
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration INTEGER,
    build_output TEXT,
    error_output TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add missing columns to existing github_deployment_logs table
ALTER TABLE github_deployment_logs 
ADD COLUMN IF NOT EXISTS failure_reason TEXT,
ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_retry_at TIMESTAMP WITH TIME ZONE;

-- Indexes for github_deployment_logs
CREATE INDEX IF NOT EXISTS idx_github_deployment_logs_repository_id ON github_deployment_logs(repository_id);
CREATE INDEX IF NOT EXISTS idx_github_deployment_logs_app_name ON github_deployment_logs(app_name);
CREATE INDEX IF NOT EXISTS idx_github_deployment_logs_status ON github_deployment_logs(status);
CREATE INDEX IF NOT EXISTS idx_github_deployment_logs_event_id ON github_deployment_logs(event_id);

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Add triggers for updated_at columns (drop existing first to avoid conflicts)
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
DROP TRIGGER IF EXISTS update_app_custom_domains_updated_at ON app_custom_domains;
DROP TRIGGER IF EXISTS update_app_public_settings_updated_at ON app_public_settings;
DROP TRIGGER IF EXISTS update_app_deployments_updated_at ON app_deployments;
DROP TRIGGER IF EXISTS update_github_repositories_updated_at ON github_repositories;
DROP TRIGGER IF EXISTS update_github_config_updated_at ON github_config;
DROP TRIGGER IF EXISTS update_github_webhook_events_updated_at ON github_webhook_events;
DROP TRIGGER IF EXISTS update_github_deployment_logs_updated_at ON github_deployment_logs;

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_app_custom_domains_updated_at BEFORE UPDATE ON app_custom_domains FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_app_public_settings_updated_at BEFORE UPDATE ON app_public_settings FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_app_deployments_updated_at BEFORE UPDATE ON app_deployments FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_github_repositories_updated_at BEFORE UPDATE ON github_repositories FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_github_config_updated_at BEFORE UPDATE ON github_config FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_github_webhook_events_updated_at BEFORE UPDATE ON github_webhook_events FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_github_deployment_logs_updated_at BEFORE UPDATE ON github_deployment_logs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('000_initial_schema') 
ON CONFLICT (version) DO NOTHING; 