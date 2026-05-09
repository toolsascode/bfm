# Tag-filtered migration execution (API)

**Scope:** How to **select and run** migrations using `target.tags` over **HTTP** and **gRPC** only. This document is written for humans and **AI agents** implementing clients: copy-paste examples, explicit request shapes, and common failure modes.

**Related:** General migrate semantics (dependencies, `schemas`, fixed vs dynamic schema) are in [MIGRATION.md](./MIGRATION.md). Declaring tags on migration source (SQL header / Go) is summarized under [Declaring tags in source](#declaring-tags-in-source) below.

---

## Prerequisites

- Running BfM server with HTTP (default `7070`) and gRPC (default `9090`) as configured.
- `BFM_API_TOKEN` set; clients send `Authorization: Bearer <token>` on HTTP.
- Migrations are **registered in the server binary** (empty `up_sql` on `GET /api/v1/migrations/{id}` means not in registry—rebuild/redeploy first).
- Canonical Protobuf: [`api/internal/api/protobuf/migration.proto`](../api/internal/api/protobuf/migration.proto) — `MigrationTarget.tags` (field 6).

---

## Concepts (agent checklist)

| Rule | Detail |
|------|--------|
| Wire format | Each tag is one string `"key=value"`. Use JSON array `target.tags` (HTTP) or `repeated string tags` (proto). |
| Parsing | Split on the **first** `=` only; trim key and value; keys normalized to **lowercase** on the server. |
| Semantics | **AND**: every requested pair must match the migration’s tag map. |
| Untagged migrations | If the request includes any tags, migrations **without** tags **do not** match. |
| Invalid tags | HTTP **400**; gRPC **`InvalidArgument`**. |
| Dependencies | Tag filter applies to the **initial** selection; pending **dependency** migrations may still be added and run without those tags. |
| Reading tags | `GET /api/v1/migrations/{id}` and gRPC `GetMigration` return `tags` from the registry when present. |

---

## Dynamic schema and tags

If a migration is defined with an **empty** logical schema (dynamic / tenant schema):

- Do **not** set `target.schema` to a non-empty value to “mean” the tenant—dynamic rows usually have **empty** `schema` in the registry; a wrong `target.schema` filters them out.
- Pass the runtime schema via HTTP **`schemas`** (e.g. `["tenant_123"]`) or gRPC **`schema`** / **`schema_name`** (one schema per `Migrate` call—repeat the RPC for multiple tenants).

See [MIGRATION.md](./MIGRATION.md) for the full fixed vs dynamic rules.

---

## HTTP examples

Replace `BASE` (e.g. `http://localhost:7070`) and `BFM_API_TOKEN`.

### 1) Single tag, fixed schema (illustrative)

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
      "version": "",
      "tags": ["env=prod"]
    },
    "schemas": [],
    "dry_run": false,
    "ignore_dependencies": false
  }'
```

### 2) Multiple tags (AND)

All pairs must match the same migration’s declared tags:

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
      "version": "",
      "tags": ["env=prod", "feature=billing"]
    },
    "schemas": [],
    "dry_run": false,
    "ignore_dependencies": false
  }'
```

### 3) Dynamic schema + tags

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
      "version": "",
      "tags": ["env=prod", "feature=billing"]
    },
    "schemas": ["tenant_123"],
    "dry_run": false,
    "ignore_dependencies": false
  }'
```

### 4) Run all pending for a connection that match tags (empty version)

Use **`version: ""`** and connection/backend filters; keep `target.schema` **empty** for dynamic-schema migrations.

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
      "version": "",
      "tags": ["env=prod"]
    },
    "schemas": ["tenant_123"],
    "dry_run": false,
    "ignore_dependencies": false
  }'
```

---

## gRPC / Protobuf examples

**Service:** `migration.MigrationService` / **`Migrate`**.

**Messages** (see [`migration.proto`](../api/internal/api/protobuf/migration.proto)):

```protobuf
message MigrationTarget {
  string backend = 1;
  string schema = 2;
  repeated string tables = 3;
  string version = 4;
  string connection = 5;
  repeated string tags = 6;  // each: "key=value"
}

message MigrateRequest {
  MigrationTarget target = 1;
  string connection = 2;
  string schema = 3;
  string schema_name = 4;  // alternative to schema for dynamic naming
  bool dry_run = 5;
  bool ignore_dependencies = 6;
}
```

### Equivalent: single tag + dynamic schema

- Set `target.connection`, `target.backend`, `target.tags = ["env=prod"]`.
- For tenant execution, set **`schema`** or **`schema_name`** to the tenant schema (e.g. `tenant_123`), not `target.schema` if that would exclude dynamic migrations (same HTTP rules).

Pseudo **grpcurl** (adjust `-d` to your tool):

```json
{
  "target": {
    "backend": "postgresql",
    "connection": "core",
    "tags": ["env=prod", "feature=billing"]
  },
  "connection": "core",
  "schema_name": "tenant_123",
  "dry_run": false,
  "ignore_dependencies": false
}
```

**Note:** One gRPC `Migrate` call applies **one** execution schema (`schema` / `schema_name`). For multiple tenants, call `Migrate` once per schema or use HTTP `schemas: ["a","b"]` which loops per schema.

---

## Declaring tags in source

Not required for calling the API; migrations may have **no** tags.

- **SQL:** optional first-line style header in `.up.sql` consumed at **build** time:
  `-- bfm-tags: env=prod, feature=billing`
- **Go:** `MigrationScript.Tags = []string{"env=prod", ...}`

For build pipeline details, see [DEVELOPMENT.md](./DEVELOPMENT.md).

---

## Common mistakes (agents)

1. **Malformed tag** (missing `=`, empty key) → 400 / `InvalidArgument`.
2. **AND too strict** — combining tags that no single migration has → no matches; **nothing applied** (success with empty applied list is possible).
3. **Dynamic schema** — setting `target.schema` incorrectly so dynamic migrations never match; use empty `target.schema` and `schemas` / `schema_name` instead.
4. **Expecting OR between tags** — BfM uses **AND** only for `target.tags`.
