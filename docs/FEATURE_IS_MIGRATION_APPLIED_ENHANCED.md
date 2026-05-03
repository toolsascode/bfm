# Feature: Enhanced IsMigrationApplied with Explicit Parameters

## Overview

Enhance the `IsMigrationApplied` feature to accept explicit parameters (`migration_id`, `schema`, `connection`, `version`, `backend`) instead of only parsing a `migrationID` string. This makes the API more explicit, flexible, and allows direct queries without database lookups for metadata.

## Current Implementation

Currently, `IsMigrationApplied`:
- Accepts only `migrationID` as a parameter
- Extracts `schema` from the `migrationID` prefix (if present)
- Queries `migrations_list` to get `version`, `connection`, and `backend`
- Checks `migrations_executions` using all 5 fields internally

**Current API:**
- HTTP: `GET /api/v1/migrations/{id}/applied`
- gRPC: `IsMigrationApplied(migration_id: string)`

## Requirements

### 1. Update Function Signatures

**State Interface** (`api/internal/state/interface.go`):
- Change `IsMigrationApplied(ctx interface{}, migrationID string) (bool, error)`
- To: `IsMigrationApplied(ctx interface{}, migrationID, schema, connection, version, backend string) (bool, error)`
- Make all parameters except `migrationID` optional (empty string means "any" or "derive from migrationID")

**Executor** (`api/internal/executor/executor.go`):
- Update `IsMigrationApplied(ctx context.Context, migrationID string) (bool, error)`
- To: `IsMigrationApplied(ctx context.Context, migrationID, schema, connection, version, backend string) (bool, error)`
- Pass parameters to state tracker

### 2. Update Implementation

**PostgreSQL Tracker** (`api/internal/state/postgresql/tracker.go`):
- Update `IsMigrationApplied` to use provided parameters
- If `schema`, `connection`, `version`, or `backend` are empty, fall back to current behavior (extract from migrationID or query database)
- If all parameters are provided, use them directly without database lookups
- Always check `migrations_executions` table using all 5 fields when available

**Logic:**
```go
// If all parameters provided, use directly
if schema != "" && connection != "" && version != "" && backend != "" {
    // Direct query to migrations_executions
    query := `SELECT EXISTS(
        SELECT 1 FROM migrations_executions
        WHERE migration_id = $1
        AND schema = $2
        AND version = $3
        AND connection = $4
        AND backend = $5
        AND (status = 'applied' OR status = 'pending')
    )`
    // Execute query with provided parameters
}

// Otherwise, fall back to current behavior (extract from migrationID or query migrations_list)
```

### 3. Update HTTP API

**Handler** (`api/internal/api/http/handler.go`):
- Change endpoint from `GET /api/v1/migrations/{id}/applied`
- To: `GET /api/v1/migrations/applied` with query parameters
- Accept query parameters: `migration_id` (required), `schema`, `connection`, `version`, `backend` (all optional)
- Keep backward compatibility: if only `migration_id` is provided, use current behavior

**New HTTP Endpoint:**
```go
// @Summary      Check if migration is applied
// @Description  Returns a boolean indicating if the migration has been applied. Accepts explicit parameters for precise checking.
// @Tags         migrations
// @Accept       json
// @Produce      json
// @Param        migration_id query string true "Migration ID"
// @Param        schema query string false "Schema name (optional, for precise checking)"
// @Param        connection query string false "Connection name (optional, for precise checking)"
// @Param        version query string false "Version (optional, for precise checking)"
// @Param        backend query string false "Backend type (optional, for precise checking)"
// @Success      200 {object} map[string]bool "Success"
// @Failure      400 {object} map[string]interface{} "Bad request"
// @Failure      401 {object} map[string]interface{} "Unauthorized"
// @Failure      500 {object} map[string]interface{} "Internal server error"
// @Security     Bearer
// @Router       /migrations/applied [get]
func (h *Handler) isMigrationApplied(c *gin.Context) {
    migrationID := c.Query("migration_id")
    if migrationID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "migration_id is required"})
        return
    }

    schema := c.Query("schema")
    connection := c.Query("connection")
    version := c.Query("version")
    backend := c.Query("backend")

    applied, err := h.executor.IsMigrationApplied(
        c.Request.Context(),
        migrationID,
        schema,
        connection,
        version,
        backend,
    )
    // ... rest of handler
}
```

**Route Registration:**
```go
api.GET("/migrations/applied", h.authenticate, h.isMigrationApplied)
```

**Backward Compatibility:**
- Keep old route `GET /api/v1/migrations/{id}/applied` for compatibility
- Or redirect it to new endpoint with `migration_id` query parameter

### 4. Update gRPC API

**Protobuf Definition** (`api/internal/api/protobuf/migration.proto`):
```protobuf
// IsMigrationAppliedRequest represents a request to check if a migration is applied
message IsMigrationAppliedRequest {
  string migration_id = 1;   // Required: ID of migration to check
  string schema = 2;         // Optional: Schema name for precise checking
  string connection = 3;     // Optional: Connection name for precise checking
  string version = 4;        // Optional: Version for precise checking
  string backend = 5;        // Optional: Backend type for precise checking
}

// IsMigrationAppliedResponse represents the result of checking if a migration is applied
message IsMigrationAppliedResponse {
  bool applied = 1;           // True if migration is applied, false otherwise
}
```

**gRPC Handler** (`api/internal/api/protobuf/handler.go`):
```go
func (s *Server) IsMigrationApplied(ctx context.Context, req *IsMigrationAppliedRequest) (*IsMigrationAppliedResponse, error) {
    if req == nil || req.MigrationId == "" {
        return nil, status.Error(codes.InvalidArgument, "request and migration_id are required")
    }

    applied, err := s.executor.IsMigrationApplied(
        ctx,
        req.MigrationId,
        req.Schema,
        req.Connection,
        req.Version,
        req.Backend,
    )
    // ... rest of handler
}
```

### 5. Update All Mock Implementations

Update all mock `StateTracker` implementations in test files to match the new signature:

**Files to update:**
- `api/internal/registry/dependency_resolver_test.go` - `mockStateTracker`
- `api/internal/executor/executor_test.go` - `mockStateTracker`
- `api/internal/executor/executor_cross_connection_test.go` - `fakeStateTracker`
- `api/internal/api/http/handler_test.go` - `mockStateTracker`
- `api/internal/backends/postgresql/validator_test.go` - `mockStateTrackerForValidator`

**New signature for all mocks:**
```go
func (m *mockStateTracker) IsMigrationApplied(ctx interface{}, migrationID, schema, connection, version, backend string) (bool, error) {
    // Implementation
}
```

### 6. Update All Call Sites

Find and update all places where `IsMigrationApplied` is called:

**Files to check:**
- `api/internal/executor/executor.go` - Multiple call sites
- `api/internal/backends/postgresql/validator.go` - Call sites
- Any other files that call this method

**Update pattern:**
```go
// Old:
applied, err := e.stateTracker.IsMigrationApplied(ctx, migrationID)

// New (if you have explicit values):
applied, err := e.stateTracker.IsMigrationApplied(ctx, migrationID, schema, connection, version, backend)

// New (if you want to use current behavior - pass empty strings):
applied, err := e.stateTracker.IsMigrationApplied(ctx, migrationID, "", "", "", "")
```

### 7. Regenerate Protobuf Code

After updating `migration.proto`:
```bash
make generate-protobuf
# or
cd api/internal/api/protobuf && bash generate.sh
```

### 8. Update Documentation

**README.md:**
- Update HTTP API section with new endpoint and query parameters
- Update gRPC API section with new request message fields
- Add examples showing both usage patterns (with and without explicit parameters)

**Example:**
```markdown
### Check if Migration is Applied

Check if a specific migration has been applied:

**Simple check (using migrationID only):**
```bash
GET /api/v1/migrations/applied?migration_id=20250115120000_create_users_postgresql_core
Authorization: Bearer {BFM_API_TOKEN}
```

**Precise check (with explicit parameters):**
```bash
GET /api/v1/migrations/applied?migration_id=20250115120000_create_users_postgresql_core&schema=core&connection=core&version=20250115120000&backend=postgresql
Authorization: Bearer {BFM_API_TOKEN}
```

Response:
```json
{
  "applied": true
}
```
```

## Implementation Steps

1. **Update State Interface** - Change function signature in `api/internal/state/interface.go`
2. **Update PostgreSQL Tracker** - Implement new logic in `api/internal/state/postgresql/tracker.go`
3. **Update Executor** - Change signature and pass parameters in `api/internal/executor/executor.go`
4. **Update HTTP Handler** - Change endpoint and add query parameters in `api/internal/api/http/handler.go`
5. **Update Protobuf Definition** - Add new fields in `api/internal/api/protobuf/migration.proto`
6. **Update gRPC Handler** - Use new fields in `api/internal/api/protobuf/handler.go`
7. **Update All Mocks** - Fix all test mocks to match new signature
8. **Update All Call Sites** - Find and update all `IsMigrationApplied` calls
9. **Regenerate Protobuf** - Run `make generate-protobuf`
10. **Update Documentation** - Update README.md and other docs
11. **Test** - Verify both old behavior (backward compatibility) and new explicit parameter behavior

## Testing Checklist

- [ ] Test with only `migration_id` (backward compatibility)
- [ ] Test with all parameters provided (new explicit behavior)
- [ ] Test with partial parameters (some empty, some provided)
- [ ] Test HTTP endpoint with query parameters
- [ ] Test gRPC endpoint with new message fields
- [ ] Test with dynamic schemas
- [ ] Test with fixed schemas
- [ ] Verify all unit tests pass
- [ ] Verify all integration tests pass

## Backward Compatibility

**Important:** Maintain backward compatibility by:
- Accepting empty strings for optional parameters (treat as "not specified")
- Falling back to current behavior when parameters are not provided
- Keeping old HTTP route if needed, or redirecting to new one

## Benefits

1. **More Explicit API** - Users can specify exactly what to check
2. **No Database Lookups** - When all parameters provided, no need to query `migrations_list`
3. **More Flexible** - Can check specific combinations without parsing migrationID
4. **Better Performance** - Direct queries when all parameters known
5. **Clearer Intent** - API clearly shows what fields are being checked
