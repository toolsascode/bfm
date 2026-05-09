# Running migrations over the API

**Scope:** How to **execute up migrations** via **HTTP** and **gRPC**, including **dependency behavior**, **fixed vs dynamic schema**, and **batch ordering**. **No CLI** in this guide. For **tag filters**, see [TAGS.md](./TAGS.md). For **authoring** dependencies in code, see [MIGRATION_DEPENDENCIES.md](./MIGRATION_DEPENDENCIES.md).

**Canonical types:**

- HTTP JSON: [`api/internal/api/http/dto/migrations.go`](../api/internal/api/http/dto/migrations.go) (`MigrateUpRequest`, `MigrationTarget`, …)
- Protobuf: [`api/internal/api/protobuf/migration.proto`](../api/internal/api/protobuf/migration.proto)

---

## Prerequisites

- `BFM_API_TOKEN` on the server; HTTP clients use `Authorization: Bearer <token>`.
- Target **connection** is configured (env / config) and reachable.
- Migration scripts are **compiled into** the running server (registry). If `GET /api/v1/migrations/{id}` returns empty `up_sql` / `down_sql`, the migration is not in the binary—rebuild and redeploy, then optionally `POST /api/v1/migrations/reindex` for listing.

---

## HTTP: `POST /api/v1/migrations/up`

**Body** (`MigrateUpRequest`):

| Field | Role |
|-------|------|
| `connection` | **Required.** Connection name (e.g. `core`). |
| `target` | Filters which **registered** scripts run (`backend`, `connection`, optional `schema`, `tables`, `version`, optional `tags`). |
| `schemas` | **Runtime schema(s)** for dynamic migrations or per-schema runs; see below. |
| `dry_run` | If true, no SQL/JSON executed; useful for CI checks. |
| `ignore_dependencies` | If true, **skip** dependency expansion/validation and sort by **version only** (dangerous). |

**Response:** `success`, `applied[]`, `skipped[]`, `errors[]` (and optional `queued` / `job_id` if async queue is enabled).

---

## Fixed vs dynamic schema

### Fixed-schema migration

The migration is registered with a **non-empty** `schema` matching a real database schema.

- You may set `target.schema` to that same schema to narrow selection.
- `schemas` is often empty `[]` or a single schema aligned with execution context.

### Dynamic-schema migration

The migration is registered with **empty** `schema` (tenant / per-schema execution).

- **Do not** set `target.schema` to a tenant name if that filters **out** dynamic rows (registry filter is equality on `migration.Schema`).
- Pass the tenant (or logical schema) in **`schemas`**: e.g. `["tenant_123"]`. For multiple tenants in one request, use multiple entries: `["tenant_a","tenant_b"]` (server runs the selected set once per schema).

---

## Dependencies (default behavior)

When `ignore_dependencies` is **false** (default):

1. BfM selects migrations matching `target`.
2. It **expands** the set with **pending** migrations required by structured/simple dependencies.
3. It **orders** runs (topological sort / resolver) so dependents run after dependencies.
4. On PostgreSQL, it may **validate** schemas/tables and dependency state before executing.

**Declaring** dependencies happens in migration source (Go / metadata)—not in this API. See [MIGRATION_DEPENDENCIES.md](./MIGRATION_DEPENDENCIES.md).

### Opt out: `ignore_dependencies: true`

- No dependency expansion/validation (per server logic); **version order** only for the selected set.
- Use only when you accept breakage risk (missing tables, wrong order).

---

## HTTP: dependency-safe order for a known ID set

**Endpoint:** `POST /api/v1/migrations/order-batch`

**Body:**

```json
{
  "migration_ids": ["20240101120000_first_core_postgresql_core", "20240101120001_second_core_postgresql_core"],
  "connection": "core"
}
```

**Response:**

```json
{
  "ordered_migration_ids": ["20240101120000_first_core_postgresql_core", "20240101120001_second_core_postgresql_core"]
}
```

Use this when a client loads a **subset** of IDs (e.g. UI selection) and must run them in dependency order: call `order-batch`, then call **`migrations/up`** once per ID with `target.version` set, or run a broader `target` if appropriate.

**gRPC:** This endpoint **does not** exist on `MigrationService` in [`migration.proto`](../api/internal/api/protobuf/migration.proto). gRPC clients should either:

- Issue a single **`Migrate`** with a `target` that already includes all needed scripts (server orders internally), or
- Order IDs using an HTTP client to `order-batch`, or
- Implement ordering out-of-band (not recommended unless IDs are independent).

---

## HTTP examples

Set `BASE` and `BFM_API_TOKEN`.

### One migration by version (fixed schema)

```bash
curl -s -X POST "${BASE}/api/v1/migrations/up" \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
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
  }'
```

### Dynamic schema (single tenant)

```bash
curl -s -X POST "${BASE}/api/v1/migrations/up" \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
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
  }'
```

### All pending for connection (empty version, careful)

```bash
curl -s -X POST "${BASE}/api/v1/migrations/up" \
  -H "Authorization: Bearer ${BFM_API_TOKEN}" \
  -H "Content-Type: application/json" \
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
  }'
```

Prefer **explicit** `target` filters (and [TAGS.md](./TAGS.md) if using tags) instead of overly broad runs.

---

## gRPC: `Migrate`

**RPC:** `migration.MigrationService/Migrate`

**Request** (`MigrateRequest`):

- `target` — same fields as HTTP `MigrationTarget` (including optional `tags`).
- `connection` — required string.
- `schema` / `schema_name` — execution schema for this call (see proto comments). If `schema` is empty, `schema_name` can supply the dynamic schema name.
- `dry_run`, `ignore_dependencies` — same meaning as HTTP.

**Response:** `MigrateResponse` — `success`, `applied`, `skipped`, `errors`.

**Multi-schema:** Unlike HTTP’s `schemas` array, a single `Migrate` carries **one** schema context. Repeat **`Migrate`** per tenant/schema or use HTTP for batch `schemas`.

### Protobuf sketch (reference)

```protobuf
rpc Migrate(MigrateRequest) returns (MigrateResponse);

message MigrateRequest {
  MigrationTarget target = 1;
  string connection = 2;
  string schema = 3;
  string schema_name = 4;
  bool dry_run = 5;
  bool ignore_dependencies = 6;
}
```

---

## Agent quick reference

| Goal | HTTP | gRPC |
|------|------|------|
| Run with deps | `ignore_dependencies: false` | `ignore_dependencies: false` |
| Force version order | `ignore_dependencies: true` | `ignore_dependencies: true` |
| Dynamic tenant schema | `schemas: ["tenant"]`, `target.schema` often `""` | `schema` or `schema_name` |
| Tag AND filter | `target.tags` | `target.tags` |
| Reorder known IDs | `POST .../order-batch` | Not in proto—use HTTP or broad `Migrate` |

---

## See also

- [TAGS.md](./TAGS.md) — `target.tags` only.
- [EXECUTING_MIGRATIONS.md](./EXECUTING_MIGRATIONS.md) — troubleshooting, migration IDs, reindex.
- [DEPLOYMENT.md](./DEPLOYMENT.md) — auto-migrate, env vars.
