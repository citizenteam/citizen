-- Add GitHub App manifest support fields
ALTER TABLE github_config
    ADD COLUMN IF NOT EXISTS app_id BIGINT,
    ADD COLUMN IF NOT EXISTS app_slug TEXT,
    ADD COLUMN IF NOT EXISTS app_name TEXT,
    ADD COLUMN IF NOT EXISTS private_key TEXT,
    ADD COLUMN IF NOT EXISTS installation_id BIGINT;
