-- Migration: 012_deployment_runs.sql
-- Description: Deployment runs tracking for K3s adapter with real-time logging support
-- Created: 2024-11-28

-- Create deployment_runs table for tracking individual deployments
CREATE TABLE IF NOT EXISTS deployment_runs (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(100) NOT NULL,
    run_id VARCHAR(100) NOT NULL UNIQUE, -- Unique identifier for this deployment run
    
    -- Git information
    git_url VARCHAR(500),
    git_branch VARCHAR(100) DEFAULT 'main',
    git_commit VARCHAR(100),
    commit_message TEXT,
    
    -- Build information
    builder VARCHAR(50) DEFAULT 'auto', -- auto, nixpacks, dockerfile
    image_ref VARCHAR(500),
    
    -- Status tracking
    status VARCHAR(50) DEFAULT 'pending', -- pending, initializing, cloning, building, pushing, deploying, completed, failed, cancelled
    
    -- Timing
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_seconds INTEGER,
    
    -- Logs
    build_logs TEXT,
    error_message TEXT,
    
    -- Trigger info
    trigger_type VARCHAR(50) DEFAULT 'manual', -- manual, webhook, auto-deploy
    triggered_by INTEGER, -- user_id
    
    -- K3s specific
    job_name VARCHAR(255),
    namespace VARCHAR(100),
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create deployment_steps table for tracking individual steps
CREATE TABLE IF NOT EXISTS deployment_steps (
    id SERIAL PRIMARY KEY,
    run_id VARCHAR(100) NOT NULL REFERENCES deployment_runs(run_id) ON DELETE CASCADE,
    
    step_name VARCHAR(100) NOT NULL, -- initializing, cloning, building, pushing, deploying, cleanup
    step_order INTEGER NOT NULL,
    status VARCHAR(50) DEFAULT 'pending', -- pending, running, completed, failed, skipped
    
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_ms INTEGER,
    
    logs TEXT,
    error_message TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for deployment_runs
CREATE INDEX IF NOT EXISTS idx_deployment_runs_app_name ON deployment_runs(app_name);
CREATE INDEX IF NOT EXISTS idx_deployment_runs_run_id ON deployment_runs(run_id);
CREATE INDEX IF NOT EXISTS idx_deployment_runs_status ON deployment_runs(status);
CREATE INDEX IF NOT EXISTS idx_deployment_runs_started_at ON deployment_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployment_runs_app_started ON deployment_runs(app_name, started_at DESC);

-- Indexes for deployment_steps
CREATE INDEX IF NOT EXISTS idx_deployment_steps_run_id ON deployment_steps(run_id);
CREATE INDEX IF NOT EXISTS idx_deployment_steps_status ON deployment_steps(status);

-- Triggers for updated_at
DROP TRIGGER IF EXISTS update_deployment_runs_updated_at ON deployment_runs;
DROP TRIGGER IF EXISTS update_deployment_steps_updated_at ON deployment_steps;

CREATE TRIGGER update_deployment_runs_updated_at 
    BEFORE UPDATE ON deployment_runs 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_deployment_steps_updated_at 
    BEFORE UPDATE ON deployment_steps 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Record this migration
INSERT INTO schema_migrations (version) VALUES ('012_deployment_runs') 
ON CONFLICT (version) DO NOTHING;

