-- Example: fixed-schema migration (Schema: "core" in .go). Suitable for BFM_AUTO_MIGRATE.
CREATE TABLE IF NOT EXISTS core_schema_example_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
