-- CREATE SCHEMA IF NOT EXISTS core;

CREATE TABLE IF NOT EXISTS solution_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id UUID NOT NULL,
    feature_flag TEXT NOT NULL,
    applied_by TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_solution_runs_environment_feature
    ON solution_runs (environment_id, feature_flag);
