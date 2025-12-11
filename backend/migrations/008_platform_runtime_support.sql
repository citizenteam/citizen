-- Migration: 008_platform_runtime_support.sql
-- Description: runtime adapter + build plan support (k3s transition groundwork)
-- Created: 2025-10-29

BEGIN;

-- ============================================================================
-- APP BUILD PLANS (stores uploaded/generated Dockerfile/compose/nixpacks plans)
-- ============================================================================
CREATE TABLE IF NOT EXISTS app_build_plans (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL,
    source VARCHAR(50) NOT NULL DEFAULT 'user', -- user | system | llm
    plan_type VARCHAR(50) NOT NULL,             -- dockerfile | docker-compose | nixpacks | custom
    plan_content TEXT NOT NULL,
    metadata JSONB,
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending | ready | failed | superseded
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_app_build_plans_app_name ON app_build_plans(app_name);
CREATE INDEX IF NOT EXISTS idx_app_build_plans_status ON app_build_plans(status);

CREATE OR REPLACE FUNCTION update_app_build_plans_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_app_build_plans_updated_at ON app_build_plans;
CREATE TRIGGER trg_app_build_plans_updated_at
    BEFORE UPDATE ON app_build_plans
    FOR EACH ROW
    EXECUTE FUNCTION update_app_build_plans_updated_at();

-- ============================================================================
-- APP ADAPTER STATE (per-app runtime adapter metadata)
-- ============================================================================
CREATE TABLE IF NOT EXISTS app_adapter_state (
    id SERIAL PRIMARY KEY,
    app_name VARCHAR(255) NOT NULL UNIQUE,
    adapter_type VARCHAR(50) NOT NULL DEFAULT 'dokku', -- dokku | k3s | docker
    context JSONB,                                     -- arbitrary adapter-specific payload
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE OR REPLACE FUNCTION update_app_adapter_state_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_app_adapter_state_updated_at ON app_adapter_state;
CREATE TRIGGER trg_app_adapter_state_updated_at
    BEFORE UPDATE ON app_adapter_state
    FOR EACH ROW
    EXECUTE FUNCTION update_app_adapter_state_updated_at();

-- ============================================================================
-- APP DEPLOYMENTS EXTENSIONS
-- ============================================================================
ALTER TABLE app_deployments
    ADD COLUMN IF NOT EXISTS image_ref TEXT,
    ADD COLUMN IF NOT EXISTS builder_type VARCHAR(100),
    ADD COLUMN IF NOT EXISTS runtime_adapter VARCHAR(50) NOT NULL DEFAULT 'dokku';

CREATE INDEX IF NOT EXISTS idx_app_deployments_runtime_adapter
    ON app_deployments(runtime_adapter);

CREATE INDEX IF NOT EXISTS idx_app_deployments_image_ref
    ON app_deployments(image_ref)
    WHERE image_ref IS NOT NULL;

COMMIT;

INSERT INTO schema_migrations (version) VALUES ('008_platform_runtime_support')
ON CONFLICT (version) DO NOTHING;
