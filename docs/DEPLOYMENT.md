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
