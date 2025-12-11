-- Migration: 009_app_public_settings_domains.sql
-- Description: Add custom_domain and ssl_enabled columns to app_public_settings
-- Created: 2025-11-15

ALTER TABLE app_public_settings
    ADD COLUMN IF NOT EXISTS custom_domain VARCHAR(255),
    ADD COLUMN IF NOT EXISTS ssl_enabled BOOLEAN DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_app_public_settings_domain
    ON app_public_settings(custom_domain);
