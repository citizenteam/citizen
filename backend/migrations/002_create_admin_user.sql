-- Migration: 002_create_admin_user.sql
-- Description: Reserved for admin user creation (handled by API via environment variables)
-- Created: 2024-12-19

-- Admin user will be created by API using environment variables for security
-- This migration is kept for version tracking but doesn't create static users

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('002_create_admin_user') 
ON CONFLICT (version) DO NOTHING; 