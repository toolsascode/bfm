-- bfm-tags: example=demo, tier=optional
-- Example: fixed-schema migration with labels for tag-filtered migrate (see docs/TAGS.md).
CREATE TABLE IF NOT EXISTS core_schema_tagged_example (
    id BIGSERIAL PRIMARY KEY,
    note TEXT NOT NULL DEFAULT ''
);
