# BFM Deployment Guide

## Quick Start

### Using Docker Compose

1. **Copy environment file:**

```bash
cp .env.example .env
```

2. **Edit `.env` file with your configuration:**
   - Set `BFM_API_TOKEN` to a secure token
   - Configure database connections
   - Set backend connection details

3. **Start services:**

```bash
docker-compose up -d
```

4. **Check health:**

```bash
curl http://localhost:7070/health
```

### Manual Deployment

1. **Build the binary:**

```bash
go build -o bfm-server ./cmd/server
```

2. **Set environment variables:**

```bash
export BFM_API_TOKEN=your-token
export BFM_STATE_DB_PASSWORD=your-password
# ... other variables
```

3. **Run the server:**

```bash
./bfm-server
```

## Configuration

### Required Environment Variables

- `BFM_API_TOKEN` - API token for authentication (required)
- `BFM_STATE_DB_PASSWORD` - State database password (required)
- Connection-specific variables for each backend you use

### Optional Environment Variables

- `BFM_HTTP_PORT` - HTTP server port (default: 7070)
- `BFM_GRPC_PORT` - gRPC server port (default: 9090)
- `BFM_STATE_SCHEMA` - State database schema (default: public)
- `BFM_LOG_LEVEL` - Logging level: DEBUG, INFO, WARN, ERROR, FATAL (default: INFO)

## Production Deployment

### Security Considerations

1. **API Token:**
   - Use a strong, randomly generated token
   - Store in secret management system
   - Rotate regularly

2. **Database Credentials:**
   - Use separate credentials for each connection
   - Store in secret management system
   - Use SSL/TLS for database connections

3. **Network Security:**
   - Run BFM in a private network
   - Use firewall rules to restrict access
   - Enable HTTPS/TLS for HTTP API (use reverse proxy)

### High Availability

1. **State Database:**
   - Use PostgreSQL with replication
   - Configure connection pooling
   - Monitor database health

2. **BFM Instances:**
   - Run multiple instances behind a load balancer
   - Use distributed locking (Redis/Etcd) to prevent concurrent migrations
   - Monitor instance health

3. **Backend Connections:**
   - Use connection pooling
   - Configure timeouts and retries
   - Monitor connection health

### Monitoring

1. **Health Checks:**
   - HTTP: `GET /health`
   - gRPC: Health check service (if implemented)

2. **Logging:**
   - Structured logging to stdout
   - Collect logs with centralized logging system
   - Set appropriate log levels

3. **Metrics:**
   - Track migration execution time
   - Track success/failure rates
   - Track migration counts per backend

### Scaling

- **Horizontal Scaling:** Run multiple BFM instances
- **Vertical Scaling:** Increase resources for state database
- **Connection Pooling:** Configure appropriate pool sizes

## Integration with Dashboard

### Environment Variables in Dashboard

Add to dashboard `.env`:

```bash
BFM_API_URL=http://bfm:7070
BFM_API_TOKEN=your-bfm-api-token
```

### Migration Triggers

Migrations are automatically triggered when:

- A new environment is created
- Core/Guard schemas need initialization

### BfM server startup auto-migrate

The API server (`cmd/server`) runs pending **up** migrations per configured connection shortly after startup, then **retries in rounds** until there are no remaining **fixed-schema** migrations to apply, a stall is detected (no progress with no errors), or limits are reached.

- **`BFM_AUTO_MIGRATE`**: enabled by default when unset. Set to `false`, `0`, `off`, or `no` to disable.
- **`BFM_AUTO_MIGRATE_CONNECTIONS`**: optional comma-separated list of connection names (e.g. `core,guard`). If unset, every connection defined in config is considered, subject to the readiness rules below.
- **`BFM_AUTO_MIGRATE_RETRY_INTERVAL`**: duration between full rounds over all ready connections (default `5s`). Go duration syntax (e.g. `10s`, `1m`). Set to **`0`** or **`0s`** for **legacy single-pass** behavior (one round only, after the initial startup delay).
- **`BFM_AUTO_MIGRATE_RETRY_MAX_ROUNDS`**: maximum number of rounds when the retry interval is positive (default `24`). Ignored when the retry interval is zero (only one round runs).

**Readiness (incomplete connections are skipped):** Auto-migrate does not call `ExecuteUp` for a connection if its env config is obviously incomplete for the backend, to avoid useless dials (e.g. etcd logging retries to `:2379` when no endpoints or host+port are set).

- **postgresql**: requires non-empty `Host` (`{CONN}_DB_HOST`).
- **greptimedb**: requires non-empty `Host`.
- **etcd**: requires non-empty `{CONN}_ENDPOINTS` (or any extra key whose name matches `endpoints`, case-insensitive), **or** both `Host` and `Port` non-empty.
- **Other backends**: no extra check (forward compatible).

If you declare `METADATA_BACKEND=etcd` but do not configure etcd endpoints in this environment, that connection is skipped until you set `METADATA_ENDPOINTS` (or host+port) or list only ready connections via `BFM_AUTO_MIGRATE_CONNECTIONS`.

This uses the same `ExecuteUp` path as the HTTP API (synchronous, not the async queue). **Dynamic-schema migrations** (empty schema in the migration definition) still need an explicit schema in a manual/API run; auto-migrate passes an empty schema, so those migrations are **skipped** (info log) until you run migrate up with `schemas` set. They are also **excluded from the pending count** that drives retries, so the loop can finish while the migration list UI still shows those rows as pending. Fixed-schema migrations on the same connection still apply during auto-migrate.

If every round applies nothing, reports no errors, and the fixed-schema pending count does not drop, auto-migrate **stops with a warning** (e.g. backend/connection name mismatch between config and registered migrations); fix configuration and restart or run migrations via the API.

**PostgreSQL naming:** The registry treats **`postgres`** and **`postgresql`** as the same backend when matching config to registered migrations (e.g. config `postgresql` with migration metadata `postgres`). Migration IDs still use whatever backend string is stored on each script.

### Manual Migration

You can also trigger migrations manually via API:

```bash
curl -X POST http://bfm:7070/api/v1/migrate \
  -H "Authorization: Bearer $BFM_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "target": {
      "backend": "postgresql",
      "connection": "core"
    },
    "connection": "core",
    "schema": "core"
  }'
```

## Troubleshooting

### Common Issues

1. **Connection Errors:**
   - Verify database credentials
   - Check network connectivity
   - Verify firewall rules

2. **Migration Failures:**
   - Check migration logs
   - Verify migration scripts are correct
   - Check database permissions

3. **State Tracking Issues:**
   - Verify state database is accessible
   - Check schema exists
   - Verify state table was created

### Debug Mode

Enable debug logging:

```bash
export BFM_LOG_LEVEL=DEBUG
```

## Backup and Recovery

1. **State Database:**
   - Regular backups of state database
   - Test restore procedures
   - Keep migration history

2. **Migration Scripts:**
   - Version control all migration scripts
   - Keep backups of SQL files
   - Document migration dependencies
