# Migration Guide: From GORM AutoMigrate to BFM

This guide explains how to migrate from the existing GORM AutoMigrate system to the new BFM migration system.

## Overview

The existing system uses:
- GORM AutoMigrate for PostgreSQL tables
- GreptimeDB AutoMigrate for time-series tables
- Etcd for metadata storage

The new BFM system uses:
- SQL migration scripts embedded in Go files
- Centralized migration execution via HTTP/Protobuf APIs
- State tracking in PostgreSQL/MySQL

## Step 1: Extract Table Definitions

### PostgreSQL Tables

For each GORM model in the dashboard project, extract the table definition:

1. Locate the model file (e.g., `backend/internal/domains/.../models/`)
2. Extract the struct definition and GORM tags
3. Convert to SQL CREATE TABLE statement
4. Create migration script following naming convention

Example:
```go
// Original GORM model
type User struct {
    ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    Email     string    `gorm:"type:varchar(255);unique;not null"`
    Name      string    `gorm:"type:varchar(255)"`
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

Convert to SQL:
```sql
CREATE TABLE IF NOT EXISTS core.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### GreptimeDB Tables

For GreptimeDB tables:

1. Locate table schemas in `backend/internal/database/greptime/auto_migrate.go`
2. Extract SQL from `GetTableSchemas()` function
3. Create migration script with GreptimeDB-specific syntax (TIME INDEX, etc.)

Example:
```sql
CREATE TABLE IF NOT EXISTS apm_metrics (
    ts TIMESTAMP TIME INDEX,
    service_name STRING,
    metric_name STRING,
    value DOUBLE,
    tags MAP(STRING, STRING),
    PRIMARY KEY (ts, service_name, metric_name)
);
```

### Etcd Metadata

For Etcd metadata:

1. Document existing key structure
2. Create JSON migration files with key-value operations
3. Use prefix structure: `/{prefix}/{schema}/{table}/{key}`

Example:
```json
[
  {
    "operation": "put",
    "key": "system.version",
    "value": "1.0.0"
  }
]
```

## Step 2: Create Migration Scripts

### PostgreSQL Migration

Create files:
- `sfm/postgresql/core/core_users_20250101120000_create_users.go`
- `sfm/postgresql/core/core_users_20250101120000_create_users.sql`
- `sfm/postgresql/core/core_users_20250101120000_create_users_down.sql`

The `.go` file:
```go
package core

import (
    _ "embed"
    "bfm/api/internal/executor"
    "bfm/api/internal/registry"
)

//go:embed core_users_20250101120000_create_users.sql
var upSQL string

//go:embed core_users_20250101120000_create_users_down.sql
var downSQL string

func init() {
    migration := &executor.MigrationScript{
        Schema:     "core",
        Table:      "users",
        Version:    "20250101120000",
        Name:       "create_users",
        Connection: "core",
        Backend:    "postgresql",
        UpSQL:      upSQL,
        DownSQL:    downSQL,
    }
    registry.GlobalRegistry.Register(migration)
}
```

### GreptimeDB Migration

Create files:
- `sfm/greptimedb/logs/logs_apm_metrics_20250101120000_create_apm_metrics.go`
- `sfm/greptimedb/logs/logs_apm_metrics_20250101120000_create_apm_metrics.sql`
- `sfm/greptimedb/logs/logs_apm_metrics_20250101120000_create_apm_metrics_down.sql`

Note: Schema is dynamic (per environment), so leave it empty in the migration script.

### Etcd Migration

Create files:
- `sfm/etcd/metadata/metadata_config_20250101120000_init_config.go`
- `sfm/etcd/metadata/metadata_config_20250101120000_init_config.json`
- `sfm/etcd/metadata/metadata_config_20250101120000_init_config_down.json`

## Step 3: Organize by Backend

### Core Backend (PostgreSQL)
- Schema: `core`
- Tables: regions, organizations, plans, plan_features, organization_plans, plan_history, environments
- Location: `sfm/postgresql/core/`

### Guard Backend (PostgreSQL)
- Schema: `guard`
- Tables: users, user_organizations, user_two_factors, backup_codes, api_keys, subscriptions, cloud_credentials, credential_audit_logs, storage_usage_snapshots
- Location: `sfm/postgresql/guard/`

### Organization Backend (PostgreSQL)
- Schema: Dynamic (`cli_{environment_id}`)
- Tables: All other tables (alerts, APM, observability, cost, analytics, etc.)
- Location: `sfm/postgresql/organization/`

### Traces Backend (GreptimeDB)
- Schema: `opentelemetry_traces`
- Tables: Trace data tables
- Location: `sfm/greptimedb/traces/`

### Logs Backend (GreptimeDB)
- Schema: Dynamic (`cli_{environment_id}`)
- Tables: opentelemetry_logs, APM, RUM, Logs, Events, etc.
- Location: `sfm/greptimedb/logs/`

### Metadata Backend (Etcd)
- Prefix: `/opsview/metadata/` or configurable
- Keys: Global static system and user configurations
- Location: `sfm/etcd/metadata/`

## Step 4: Migration Execution

### Initial Setup

1. Set up BFM server with proper configuration
2. Ensure state database is accessible
3. Configure all connection settings

### Running Migrations

#### Core Schema Migration
```bash
curl -X POST http://localhost:7070/api/v1/migrate \
  -H "Authorization: Bearer $BFM_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "target": {
      "backend": "postgresql",
      "schema": "core",
      "connection": "core"
    },
    "connection": "core",
    "schema": "core"
  }'
```

#### Guard Schema Migration
```bash
curl -X POST http://localhost:7070/api/v1/migrate \
  -H "Authorization: Bearer $BFM_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "target": {
      "backend": "postgresql",
      "schema": "guard",
      "connection": "guard"
    },
    "connection": "guard",
    "schema": "guard"
  }'
```

#### Environment-Specific Migration
```bash
curl -X POST http://localhost:7070/api/v1/migrate \
  -H "Authorization: Bearer $BFM_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "target": {
      "backend": "postgresql",
      "connection": "organization"
    },
    "connection": "organization",
    "schema": "cli_environment_id_here",
    "environment": "environment_id_here"
  }'
```

## Step 5: Verification

1. Check migration state:
   ```sql
   SELECT * FROM bfm_migrations ORDER BY applied_at DESC;
   ```

2. Verify tables exist:
   ```sql
   SELECT tablename FROM pg_tables WHERE schemaname = 'core';
   ```

3. Test application functionality to ensure migrations worked correctly

## Best Practices

1. **Versioning**: Use timestamp format: `YYYYMMDDHHMMSS`
2. **Idempotency**: Always use `IF NOT EXISTS` and `IF EXISTS`
3. **Rollback**: Always provide down migrations
4. **Testing**: Test migrations in a development environment first
5. **Documentation**: Document complex migrations with comments
6. **Ordering**: Migrations are executed in version order

## Troubleshooting

### Migration Already Applied
If a migration shows as already applied but tables don't exist:
1. Check state table: `SELECT * FROM bfm_migrations WHERE migration_id = '...'`
2. If needed, delete the record and re-run migration

### Schema Creation Issues
If schema creation fails:
1. Verify database user has CREATE SCHEMA permissions
2. Check if schema already exists manually
3. Verify connection configuration

### Connection Errors
If backend connection fails:
1. Verify environment variables are set correctly
2. Test connection manually (psql, etc.)
3. Check network connectivity and firewall rules

