-- Migration: 013_build_logs_jsonb.sql
-- Description: Add JSONB column for real-time build logs storage
-- Created: 2024-11-28

-- Add JSONB column for storing build logs as array of log entries
ALTER TABLE deployment_runs ADD COLUMN IF NOT EXISTS build_logs_json JSONB DEFAULT '[]'::jsonb;

-- Add index for faster queries on log entries
CREATE INDEX IF NOT EXISTS idx_deployment_runs_build_logs_json ON deployment_runs USING GIN (build_logs_json);

-- Comment
COMMENT ON COLUMN deployment_runs.build_logs_json IS 'Array of log entries: [{timestamp, step, log}]';

