<!-- markdownlint-disable MD041 -->

**Documentation map:** For **API-first** how-tos, prefer **[TAGS.md](./TAGS.md)** (tag-filtered migrate) and **[MIGRATION.md](./MIGRATION.md)** (migrate up, dependencies, dynamic schema, `order-batch`). This file is a **checklist** (IDs, registry vs DB, curl patterns, troubleshooting).

## Executing migrations (one, many, or all)

This guide focuses on **running a specific migration** (or a subset / all), and on the
common gotchas that make a migration “not run” even when it exists in the database/UI.

## Key concepts (why a migration might not execute)

BfM has **two “sources of truth”** involved when you execute migrations:

- **Registry (in-memory, compiled into the server binary)**: what the executor can *actually run*.
  - Migrations are registered via `init()` functions in generated `*.go` migration files.
- **State DB (`migrations_list`, `migrations_executions`, `migrations_history`)**: what the
  API/UI can *list*, track, and display.
  - `POST /api/v1/migrations/reindex` scans the filesystem and syncs `migrations_list`,
    but it does **not** magically load code into the running server.

### Quick check: is this migration executable?

Call `GET /api/v1/migrations/{id}`. If the response has **empty `up_sql` / `down_sql`**, it usually means:

- the migration exists in the **DB listing**, but
- it is **not registered in the server registry** (i.e., not compiled into the running binary),
- so it **cannot be executed** until the server is rebuilt with that migration code.

## Migration IDs (important for “specific migration”)

The primary migration ID format used by the executor/registry is:

- **Base ID**: `{version}_{name}_{backend}_{connection}`

When you execute migrations **for a specific schema** (dynamic schemas, or explicit
per-schema execution), BfM tracks them separately by prefixing the base ID:

- **Schema-specific ID**: `{schema}_{version}_{name}_{backend}_{connection}`

You’ll see schema-specific IDs in execution/history records when per-schema execution is used.

## Prerequisites

- BfM server running (HTTP by default on `:7070`)
- `BFM_API_TOKEN` set in the server environment
- Connection configuration set for the target connection
  (e.g. `CORE_DB_HOST`, `CORE_DB_PASSWORD`, etc.)

For local/dev startup options, see `docs/DEVELOPMENT.md`.

## Step 1: Find the migration you want to run

### List migrations (with filters)

```bash
curl -s \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  "http://localhost:7070/api/v1/migrations?connection=core&backend=postgresql" | jq .
```

Useful filters:

- `connection`: limit to one connection (recommended)
- `backend`: e.g. `postgresql`, `greptimedb`, `etcd`
- `schema`: schema recorded in `migrations_list` (note: **dynamic-schema migrations often have empty schema**)
- `version`: 14-digit version timestamp

### Get details for one migration

```bash
curl -s \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  "http://localhost:7070/api/v1/migrations/${MIGRATION_ID}" | jq .
```

If `up_sql` is empty here, fix the “not compiled into server” issue first (see Troubleshooting).

## Step 2: Execute migrations (up)

The HTTP endpoint for “up” is:

- `POST /api/v1/migrations/up`

The request has two separate knobs:

- `target`: **filters which migration scripts are selected from the registry**
- `schemas`: **executes the selected scripts once per schema** (used for dynamic schemas, or explicit per-schema runs)

### A) Execute ONE migration (recommended pattern)

Because `target` does not include a “name” filter, the safest way to run “one migration” is typically:

- filter by `connection` + `backend`
- filter by an exact `version` (usually unique)
- optionally filter by `tables` if you have multiple scripts sharing a version

#### Fixed-schema migration (migration has `Schema != ""`)

```bash
curl -s -X POST \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:7070/api/v1/migrations/up" \
  -d '{
    "connection": "core",
    "target": {
      "backend": "postgresql",
      "connection": "core",
      "schema": "core",
      "tables": [],
      "version": "20250101120000"
    },
    "schemas": [],
    "dry_run": false,
    "ignore_dependencies": false
  }' | jq .
```

Notes:

- `target.schema` **filters registry migrations by their declared schema**.
- If you omit `target.schema`, you may select migrations from multiple schemas (if you have them).

#### Dynamic-schema migration (migration has `Schema == ""`)

**Critical:** do **not** set `target.schema`, because the registry filter is
`migration.Schema == target.Schema`, and dynamic migrations have empty schema.

```bash
curl -s -X POST \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:7070/api/v1/migrations/up" \
  -d '{
    "connection": "core",
    "target": {
      "backend": "postgresql",
      "connection": "core",
      "schema": "",
      "tables": [],
      "version": "20250101120000"
    },
    "schemas": ["tenant_123"],
    "dry_run": false,
    "ignore_dependencies": false
  }' | jq .
```

This will:

- select the matching migration(s) from the registry (by backend/connection/version[/tables])
- execute them **for `tenant_123`**
- track execution under the **schema-specific migration ID**: `tenant_123_{version}_{name}_{backend}_{connection}`

### B) Execute a SUBSET of migrations

#### Run everything for a connection/backend (all pending)

```bash
curl -s -X POST \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:7070/api/v1/migrations/up" \
  -d '{
    "connection": "core",
    "target": {
      "backend": "postgresql",
      "connection": "core",
      "schema": "",
      "tables": [],
      "version": ""
    },
    "schemas": [],
    "dry_run": false,
    "ignore_dependencies": false
  }' | jq .
```

If you want to execute that subset for multiple schemas (multi-tenant), pass `schemas`:

```json
{
  "schemas": ["tenant_123", "tenant_456"]
}
```

### C) Execute ALL migrations (everything BfM knows about)

This is generally **not recommended** unless you have a single connection. If you omit
`target.connection`, the registry filter will not restrict by connection and you can
select scripts from multiple connections.

If you really want “all”, do it intentionally:

- pick one connection at a time (multiple calls), or
- ensure your target filter is explicit.

## Tag-filtered execution (`target.tags`)

See **[TAGS.md](./TAGS.md)** for HTTP/gRPC examples, AND semantics, dynamic schema + tags, and declaring tags in source. The FFM UI supports tag input and **Execute by tags** when Backend and Connection filters are set.

## Dry-run and dependency behavior

- **Dry run**: set `dry_run: true` (BfM will report what would be applied, without executing SQL/JSON).
- **Dependencies**:
  - default: dependencies are expanded/resolved and validated (PostgreSQL has additional dependency validation).
  - force execution: set `ignore_dependencies: true` (sorts by version only; use with caution).

## Rollback / down (execute migrations “down”)

There are two ways you’ll commonly “undo” a migration:

### A) Rollback endpoint (recommended)

- `POST /api/v1/migrations/{id}/rollback`
- Optional body: `{ "schemas": ["tenant_123"] }`

```bash
curl -s -X POST \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:7070/api/v1/migrations/${MIGRATION_ID}/rollback" \
  -d '{ "schemas": ["tenant_123"] }' | jq .
```

### B) Down endpoint (explicit “down” execution)

- `POST /api/v1/migrations/down`
- Body requires `migration_id` and optional `schemas`

```bash
curl -s -X POST \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:7070/api/v1/migrations/down" \
  -d '{
    "migration_id": "'"${MIGRATION_ID}"'",
    "schemas": ["tenant_123"],
    "dry_run": false,
    "ignore_dependencies": false
  }' | jq .
```

## Verify what happened

Common verification calls:

- **Applied?**: `GET /api/v1/migrations/{id}/applied`
- **Status**: `GET /api/v1/migrations/{id}/status`
- **History**: `GET /api/v1/migrations/{id}/history`
- **Executions**: `GET /api/v1/migrations/{id}/executions`
- **Recent executions**: `GET /api/v1/migrations/executions/recent?limit=20`

## Troubleshooting checklist (common causes of “it didn’t run”)

### 1) You filtered out the migration (dynamic schema gotcha)

If the migration is dynamic-schema (`migration.Schema == ""`), then:

- setting `target.schema` to a non-empty value will make it **never match**.
- the correct approach is: keep `target.schema` empty and pass the real schema(s) via `schemas`.

### 2) Migration is in the DB/UI but not executable (not in registry)

Symptoms:

- `GET /api/v1/migrations/{id}` returns empty `up_sql` / `down_sql`
- executing “up” doesn’t find/apply it (or applies nothing)

Fix:

- ensure the generated migration `*.go` file is compiled into the running server binary
- then run `POST /api/v1/migrations/reindex` to sync `migrations_list` if needed

### 3) Wrong connection/backend selected

Ensure:

- request-level `connection` matches your configured connection name
- `target.connection` matches the same connection (recommended for safety)
- `target.backend` matches the backend for that connection

### 4) It was already applied (or is pending from another run)

Check:

- `GET /api/v1/migrations/{id}/executions`
- `GET /api/v1/migrations/{id}/status`

BfM records a **pending** execution immediately to avoid concurrent runs; if another
process raced you, it may appear as skipped/pending.
