-- Depends on core_schema_example_settings (same fixed schema "core").
CREATE TABLE IF NOT EXISTS core_schema_example_audit (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_core_schema_example_audit_created_at
    ON core_schema_example_audit (created_at DESC);
