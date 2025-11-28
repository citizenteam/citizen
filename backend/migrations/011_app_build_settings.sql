-- Migration: 011_app_build_settings.sql
-- Description: Add app build settings table for builder type configuration
-- Created: 2025-11-28

-- Table to store build configuration per app
CREATE TABLE IF NOT EXISTS app_build_settings (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL UNIQUE,
    builder_type VARCHAR(50) NOT NULL DEFAULT 'auto',  -- 'auto', 'dockerfile', 'nixpacks'
    dockerfile_path VARCHAR(255) DEFAULT 'Dockerfile',
    build_env JSONB DEFAULT '{}',  -- Build-time environment variables
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Index for fast lookup by app_name
CREATE INDEX IF NOT EXISTS idx_app_build_settings_app_name 
    ON app_build_settings(app_name);

-- Comment on columns
COMMENT ON TABLE app_build_settings IS 'Stores build configuration for each app';
COMMENT ON COLUMN app_build_settings.builder_type IS 'Build method: auto (nixpacks with fallback), dockerfile, nixpacks';
COMMENT ON COLUMN app_build_settings.dockerfile_path IS 'Path to Dockerfile relative to repo root';
COMMENT ON COLUMN app_build_settings.build_env IS 'Build-time environment variables as JSON';

