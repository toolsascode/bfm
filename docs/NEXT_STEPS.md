# Next Steps for BFM Migration System

## 1. Testing & Validation

### Unit Tests
- [ ] Write unit tests for each backend (PostgreSQL, GreptimeDB, Etcd)
- [ ] Test state tracker operations (record, query, check applied)
- [ ] Test executor logic (migration selection, ordering, execution)
- [ ] Test registry operations (register, find, filter)
- [ ] Test configuration loading and validation

### Integration Tests
- [ ] Test end-to-end migration flow with real databases
- [ ] Test schema creation for all backends
- [ ] Test migration state tracking
- [ ] Test error handling and rollback scenarios
- [ ] Test concurrent migration execution (should be prevented)

### Test Setup
```bash
# Create test databases
# Set up test environment variables
# Run test suite
cd bfm
go test ./...
```

## 2. Migration Script Generation

### Extract Existing Schemas
- [ ] Extract all GORM models from dashboard project
- [ ] Convert GORM models to SQL CREATE TABLE statements
- [ ] Extract GreptimeDB table schemas from `GetTableSchemas()`
- [ ] Document existing Etcd key structure

### Create Migration Scripts
- [ ] Create Core backend migrations (regions, organizations, plans, environments)
- [ ] Create Guard backend migrations (users, api_keys, subscriptions, etc.)
- [ ] Create Organization backend migrations (alerts, APM, cost, analytics)
- [ ] Create GreptimeDB Traces backend migrations
- [ ] Create GreptimeDB Logs backend migrations (APM, RUM, Events)
- [ ] Create Etcd Metadata backend migrations

### Migration Script Template
Use the example migrations as templates:
- `sfm/postgresql/core/core_users_20250101120000_create_users.go`
- `sfm/greptimedb/logs/logs_apm_metrics_20250101120000_create_apm_metrics.go`
- `sfm/etcd/metadata/metadata_config_20250101120000_init_config.go`

## 3. Protobuf Code Generation

### Generate gRPC Code
```bash
# Install protoc and Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate code from .proto file
cd bfm/internal/api/protobuf
protoc --go_out=. --go-grpc_out=. migration.proto
```

### Implement gRPC Handler
- [ ] Complete the Protobuf handler implementation
- [ ] Add gRPC server to main.go
- [ ] Test gRPC API endpoints
- [ ] Add authentication middleware for gRPC

## 4. Enhanced Features

### Migration Locking
- [ ] Implement distributed locking (Redis/Etcd) to prevent concurrent migrations
- [ ] Add lock timeout and cleanup
- [ ] Handle lock acquisition failures gracefully

### Migration Rollback
- [ ] Implement rollback functionality using DownSQL
- [ ] Add rollback endpoint to HTTP API
- [ ] Test rollback scenarios

### Migration Status & History
- [ ] Add GET endpoint to query migration status
- [ ] Add endpoint to get migration history
- [ ] Add filtering and pagination

### Dry-Run Improvements
- [ ] Enhance dry-run to show what would be executed
- [ ] Add validation checks in dry-run mode
- [ ] Show estimated execution time

## 5. Configuration & Deployment

### Environment Setup
- [ ] Create `.env.example` file with all required variables
- [ ] Document all configuration options
- [ ] Add configuration validation on startup

### Docker Support
- [ ] Create Dockerfile for BFM server
- [ ] Create docker-compose.yml for local development
- [ ] Document deployment process

### CI/CD Integration
- [ ] Add GitHub Actions / GitLab CI for testing
- [ ] Add automated migration script validation
- [ ] Add version tagging and releases

## 6. Documentation

### API Documentation
- [ ] Add OpenAPI/Swagger documentation for HTTP API
- [ ] Document all request/response formats
- [ ] Add example curl commands

### Migration Guide
- [ ] Complete migration guide with step-by-step instructions
- [ ] Add troubleshooting section
- [ ] Add best practices guide

### Developer Guide
- [ ] Document how to create new migration scripts
- [ ] Document naming conventions
- [ ] Add code examples

## 7. Integration with Dashboard Project

### Trigger Integration
- [ ] Add HTTP client in dashboard to call BFM API
- [ ] Integrate with environment creation flow
- [ ] Add migration trigger on environment creation
- [ ] Add error handling and retry logic

### Migration from GORM AutoMigrate
- [ ] Create migration plan for transitioning from GORM
- [ ] Test migration in staging environment
- [ ] Schedule production migration
- [ ] Monitor and validate after migration

## 8. Monitoring & Observability

### Logging
- [ ] Add structured logging (logrus/zap)
- [ ] Add log levels and filtering
- [ ] Log all migration operations

### Metrics
- [ ] Add Prometheus metrics
- [ ] Track migration execution time
- [ ] Track success/failure rates
- [ ] Track migration counts per backend

### Health Checks
- [ ] Enhance health check endpoint
- [ ] Add database connectivity checks
- [ ] Add backend health status

## 9. Security Enhancements

### Authentication
- [ ] Support multiple API tokens
- [ ] Add token rotation
- [ ] Add rate limiting

### Authorization
- [ ] Add role-based access control
- [ ] Restrict migrations by connection/backend
- [ ] Audit logging for all operations

## 10. Performance Optimizations

### Connection Pooling
- [ ] Implement connection pooling for all backends
- [ ] Add connection pool configuration
- [ ] Monitor connection usage

### Batch Operations
- [ ] Support batch migration execution
- [ ] Optimize state tracking queries
- [ ] Add migration caching

## Priority Order

1. **High Priority** (Before Production):
   - Testing & Validation
   - Migration Script Generation
   - Configuration & Deployment
   - Integration with Dashboard Project

2. **Medium Priority** (Post-Launch):
   - Protobuf Code Generation
   - Enhanced Features (rollback, status endpoints)
   - Monitoring & Observability
   - Documentation improvements

3. **Low Priority** (Future Enhancements):
   - Security enhancements
   - Performance optimizations
   - Advanced features

## Quick Start Checklist

Before using BFM in production:

- [ ] All unit tests pass
- [ ] Integration tests pass with real databases
- [ ] All migration scripts created and tested
- [ ] Configuration documented and validated
- [ ] Health checks working
- [ ] Logging configured
- [ ] Error handling tested
- [ ] Rollback tested
- [ ] Documentation complete
- [ ] Deployment process documented

