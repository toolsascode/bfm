package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"bfm/api/internal/backends"
	"bfm/api/internal/queue"
	"bfm/api/internal/registry"
	"bfm/api/internal/state"
)

// Context keys for execution metadata
type contextKey string

const (
	executedByKey       contextKey = "executed_by"
	executionMethodKey  contextKey = "execution_method"
	executionContextKey contextKey = "execution_context"
)

// SetExecutionContext sets execution context in the context
func SetExecutionContext(ctx context.Context, executedBy, executionMethod string, executionContext map[string]interface{}) context.Context {
	ctx = context.WithValue(ctx, executedByKey, executedBy)
	ctx = context.WithValue(ctx, executionMethodKey, executionMethod)
	if executionContext != nil {
		ctxBytes, _ := json.Marshal(executionContext)
		ctx = context.WithValue(ctx, executionContextKey, string(ctxBytes))
	}
	return ctx
}

// GetExecutionContext extracts execution context from context
func GetExecutionContext(ctx context.Context) (executedBy, executionMethod, executionContext string) {
	executedBy = "system"
	executionMethod = "api"
	executionContext = ""

	if val := ctx.Value(executedByKey); val != nil {
		if s, ok := val.(string); ok {
			executedBy = s
		}
	}
	if val := ctx.Value(executionMethodKey); val != nil {
		if s, ok := val.(string); ok {
			executionMethod = s
		}
	}
	if val := ctx.Value(executionContextKey); val != nil {
		if s, ok := val.(string); ok {
			executionContext = s
		}
	}
	return executedBy, executionMethod, executionContext
}

// Executor executes migrations
type Executor struct {
	registry     registry.Registry
	stateTracker state.StateTracker
	backends     map[string]backends.Backend
	connections  map[string]*backends.ConnectionConfig
	queue        queue.Queue // Optional queue for async execution
	mu           sync.Mutex
}

// NewExecutor creates a new migration executor
func NewExecutor(reg registry.Registry, tracker state.StateTracker) *Executor {
	return &Executor{
		registry:     reg,
		stateTracker: tracker,
		backends:     make(map[string]backends.Backend),
		connections:  make(map[string]*backends.ConnectionConfig),
	}
}

// SetConnections sets the connection configurations
func (e *Executor) SetConnections(connections map[string]*backends.ConnectionConfig) error {
	if connections == nil {
		return fmt.Errorf("connections map cannot be nil")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.connections = connections
	return nil
}

// SetQueue sets the queue for async execution
func (e *Executor) SetQueue(q queue.Queue) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.queue = q
}

// RegisterBackend registers a backend for use in migrations
func (e *Executor) RegisterBackend(name string, backend backends.Backend) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.backends[name] = backend
}

// GetRegistry returns the migration registry
func (e *Executor) GetRegistry() registry.Registry {
	return e.registry
}

// GetBackend returns a backend by name
func (e *Executor) GetBackend(name string) backends.Backend {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.backends[name]
}

// GetConnectionConfig returns a connection config by name
func (e *Executor) GetConnectionConfig(name string) (*backends.ConnectionConfig, error) {
	return e.getConnectionConfig(name)
}

// ExecuteSync executes migrations synchronously (bypasses queue, used by worker)
func (e *Executor) ExecuteSync(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool) (*ExecuteResult, error) {
	return e.executeSync(ctx, target, connectionName, schemaName, dryRun)
}

// Execute executes migrations based on a target specification
// If queue is configured, it will queue the job instead of executing directly
func (e *Executor) Execute(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool) (*ExecuteResult, error) {
	// If queue is enabled, queue the job instead of executing
	e.mu.Lock()
	hasQueue := e.queue != nil
	e.mu.Unlock()

	if hasQueue {
		return e.queueJob(ctx, target, connectionName, schemaName, dryRun)
	}

	// Otherwise, execute synchronously
	return e.executeSync(ctx, target, connectionName, schemaName, dryRun)
}

// queueJob queues a migration job for async execution
func (e *Executor) queueJob(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool) (*ExecuteResult, error) {
	// Create job from target
	job := &queue.Job{
		ID:         fmt.Sprintf("job_%d", time.Now().UnixNano()),
		Target:     convertTarget(target),
		Connection: connectionName,
		Schema:     schemaName,
		DryRun:     dryRun,
		Metadata:   make(map[string]interface{}),
	}

	// Publish job to queue
	e.mu.Lock()
	q := e.queue
	e.mu.Unlock()

	if err := q.PublishJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to queue migration job: %w", err)
	}

	// Return queued result
	return &ExecuteResult{
		Success: true,
		Applied: []string{},
		Skipped: []string{},
		Errors:  []string{},
		Queued:  true,
		JobID:   job.ID,
	}, nil
}

// convertTarget converts registry.MigrationTarget to queue.MigrationTarget
func convertTarget(target *registry.MigrationTarget) *queue.MigrationTarget {
	if target == nil {
		return nil
	}
	return &queue.MigrationTarget{
		Backend:    target.Backend,
		Schema:     target.Schema,
		Tables:     target.Tables,
		Version:    target.Version,
		Connection: target.Connection,
	}
}

// executeSync executes migrations synchronously
func (e *Executor) executeSync(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool) (*ExecuteResult, error) {
	// Find migrations matching the target
	migrations, err := e.registry.FindByTarget(target)
	if err != nil {
		return nil, fmt.Errorf("failed to find migrations: %w", err)
	}

	if len(migrations) == 0 {
		return &ExecuteResult{
			Success: true,
			Applied: []string{},
			Skipped: []string{},
			Errors:  []string{},
		}, nil
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	result := &ExecuteResult{
		Applied: []string{},
		Skipped: []string{},
		Errors:  []string{},
	}

	// Get backend for the connection
	connectionConfig, err := e.getConnectionConfig(connectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection config: %w", err)
	}

	backend, ok := e.backends[connectionConfig.Backend]
	if !ok {
		return nil, fmt.Errorf("backend %s not registered", connectionConfig.Backend)
	}

	// Ensure backend is connected
	if err := backend.Connect(connectionConfig); err != nil {
		return nil, fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backend.Close() }()

	// Process each migration
	for _, migration := range migrations {
		migrationID := e.getMigrationID(migration)

		// Check if already applied
		applied, err := e.stateTracker.IsMigrationApplied(ctx, migrationID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to check migration status for %s: %v", migrationID, err))
			continue
		}

		if applied {
			result.Skipped = append(result.Skipped, migrationID)
			continue
		}

		// Resolve schema name (use provided or from migration)
		schema := schemaName
		if schema == "" {
			schema = migration.Schema
		}

		// Execute migration
		if dryRun {
			result.Applied = append(result.Applied, fmt.Sprintf("%s (dry-run)", migrationID))
			continue
		}

		// Extract execution context
		executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

		// Record migration start
		record := &state.MigrationRecord{
			MigrationID:      migrationID,
			Schema:           schema,
			Table:            "",
			Version:          migration.Version,
			Connection:       connectionName,
			Backend:          migration.Backend,
			Status:           "pending",
			AppliedAt:        time.Now().Format(time.RFC3339),
			ErrorMessage:     "",
			ExecutedBy:       executedBy,
			ExecutionMethod:  executionMethod,
			ExecutionContext: executionContext,
		}

		// Convert executor.MigrationScript to backends.MigrationScript
		// Use provided schema instead of migration.Schema for dynamic schemas
		backendMigration := &backends.MigrationScript{
			Schema:     schema,
			Version:    migration.Version,
			Name:       migration.Name,
			Connection: migration.Connection,
			Backend:    migration.Backend,
			UpSQL:      migration.UpSQL,
			DownSQL:    migration.DownSQL,
		}

		// Execute the migration
		err = backend.ExecuteMigration(ctx, backendMigration)
		if err != nil {
			record.Status = "failed"
			record.ErrorMessage = err.Error()
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", migrationID, err))
		} else {
			record.Status = "success"
			result.Applied = append(result.Applied, migrationID)
		}

		// Record migration in state tracker
		if err := e.stateTracker.RecordMigration(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration %s: %v", migrationID, err))
		}
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// GetAllMigrations returns all registered migrations
func (e *Executor) GetAllMigrations() []*backends.MigrationScript {
	return e.registry.GetAll()
}

// GetMigrationByID finds a migration by its ID
func (e *Executor) GetMigrationByID(migrationID string) *backends.MigrationScript {
	allMigrations := e.registry.GetAll()
	for _, migration := range allMigrations {
		id := e.getMigrationID(migration)
		if id == migrationID {
			return migration
		}
	}
	return nil
}

// GetMigrationHistory retrieves migration history
func (e *Executor) GetMigrationHistory(ctx context.Context, filters *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	return e.stateTracker.GetMigrationHistory(ctx, filters)
}

// GetMigrationList retrieves the list of migrations with their last status
func (e *Executor) GetMigrationList(ctx context.Context, filters *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	return e.stateTracker.GetMigrationList(ctx, filters)
}

// RegisterScannedMigration registers a scanned migration in migrations_list
func (e *Executor) RegisterScannedMigration(ctx context.Context, migrationID, schema, table, version, name, connection, backend string) error {
	return e.stateTracker.RegisterScannedMigration(ctx, migrationID, schema, table, version, name, connection, backend)
}

// IsMigrationApplied checks if a migration has been applied
func (e *Executor) IsMigrationApplied(ctx context.Context, migrationID string) (bool, error) {
	return e.stateTracker.IsMigrationApplied(ctx, migrationID)
}

// ExecuteUp executes up migrations for the given schemas
func (e *Executor) ExecuteUp(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemas []string, dryRun bool) (*ExecuteResult, error) {
	result := &ExecuteResult{
		Applied: []string{},
		Skipped: []string{},
		Errors:  []string{},
	}

	// If no schemas provided, use empty string (single execution)
	if len(schemas) == 0 {
		schemas = []string{""}
	}

	// Execute for each schema
	for _, schema := range schemas {
		schemaResult, err := e.executeSync(ctx, target, connectionName, schema, dryRun)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: %v", schema, err))
			continue
		}

		result.Applied = append(result.Applied, schemaResult.Applied...)
		result.Skipped = append(result.Skipped, schemaResult.Skipped...)
		result.Errors = append(result.Errors, schemaResult.Errors...)
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// ExecuteDown executes down migrations for the given schemas
func (e *Executor) ExecuteDown(ctx context.Context, migrationID string, schemas []string, dryRun bool) (*ExecuteResult, error) {
	result := &ExecuteResult{
		Applied: []string{},
		Skipped: []string{},
		Errors:  []string{},
	}

	// Get migration from registry
	migration := e.GetMigrationByID(migrationID)
	if migration == nil {
		return nil, fmt.Errorf("migration not found: %s", migrationID)
	}

	// If no schemas provided, try to get schema from migration or use empty string
	if len(schemas) == 0 {
		if migration.Schema != "" {
			schemas = []string{migration.Schema}
		} else {
			schemas = []string{""}
		}
	}

	// Get connection config
	connectionConfig, err := e.getConnectionConfig(migration.Connection)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection config: %w", err)
	}

	// Get backend
	backend, ok := e.backends[connectionConfig.Backend]
	if !ok {
		return nil, fmt.Errorf("backend %s not registered", connectionConfig.Backend)
	}

	// Connect to backend
	if err := backend.Connect(connectionConfig); err != nil {
		return nil, fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backend.Close() }()

	// Execute down migration for each schema
	for _, schema := range schemas {
		// Check if migration is applied for this schema
		schemaMigrationID := e.getMigrationIDWithSchema(migration, schema)
		applied, err := e.stateTracker.IsMigrationApplied(ctx, schemaMigrationID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: failed to check migration status: %v", schema, err))
			continue
		}

		if !applied {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s (not applied)", schemaMigrationID))
			continue
		}

		if dryRun {
			result.Applied = append(result.Applied, fmt.Sprintf("%s (dry-run)", schemaMigrationID))
			continue
		}

		// Execute down migration
		if migration.DownSQL == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: migration does not have rollback SQL", schema))
			continue
		}

		// Create a down migration script with schema
		downMigration := &backends.MigrationScript{
			Schema:     schema,
			Version:    migration.Version,
			Name:       migration.Name + "_down",
			Connection: migration.Connection,
			Backend:    migration.Backend,
			UpSQL:      migration.DownSQL, // Use DownSQL as UpSQL for down migration
			DownSQL:    migration.UpSQL,   // Use UpSQL as DownSQL
		}

		err = backend.ExecuteMigration(ctx, downMigration)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: %v", schema, err))

			// Extract execution context
			executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

			// Record failed down migration
			record := &state.MigrationRecord{
				MigrationID:      schemaMigrationID + "_down",
				Schema:           schema,
				Table:            "",
				Version:          migration.Version,
				Connection:       migration.Connection,
				Backend:          migration.Backend,
				Status:           "failed",
				AppliedAt:        time.Now().Format(time.RFC3339),
				ErrorMessage:     err.Error(),
				ExecutedBy:       executedBy,
				ExecutionMethod:  executionMethod,
				ExecutionContext: executionContext,
			}
			_ = e.stateTracker.RecordMigration(ctx, record)
			continue
		}

		// Extract execution context
		executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

		// Record successful down migration
		record := &state.MigrationRecord{
			MigrationID:      schemaMigrationID + "_down",
			Schema:           schema,
			Table:            "",
			Version:          migration.Version,
			Connection:       migration.Connection,
			Backend:          migration.Backend,
			Status:           "rolled_back",
			AppliedAt:        time.Now().Format(time.RFC3339),
			ErrorMessage:     "",
			ExecutedBy:       executedBy,
			ExecutionMethod:  executionMethod,
			ExecutionContext: executionContext,
		}
		if err := e.stateTracker.RecordMigration(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: failed to record migration: %v", schema, err))
		} else {
			result.Applied = append(result.Applied, schemaMigrationID)
		}
	}

	result.Success = len(result.Errors) == 0
	return result, nil
}

// getMigrationIDWithSchema generates a migration ID with a specific schema
func (e *Executor) getMigrationIDWithSchema(migration *backends.MigrationScript, schema string) string {
	if schema != "" {
		return fmt.Sprintf("%s_%s_%s_%s", schema, migration.Connection, migration.Version, migration.Name)
	}
	return fmt.Sprintf("%s_%s_%s", migration.Connection, migration.Version, migration.Name)
}

// Rollback rolls back a migration
func (e *Executor) Rollback(ctx context.Context, migrationID string) (*RollbackResult, error) {
	// Get migration from registry
	migration := e.GetMigrationByID(migrationID)
	if migration == nil {
		return nil, fmt.Errorf("migration not found: %s", migrationID)
	}

	// Check if migration is applied
	applied, err := e.IsMigrationApplied(ctx, migrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to check migration status: %w", err)
	}

	if !applied {
		return &RollbackResult{
			Success: false,
			Message: "migration is not applied",
			Errors:  []string{"migration is not applied"},
		}, nil
	}

	// Get connection config
	connectionConfig, err := e.getConnectionConfig(migration.Connection)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection config: %w", err)
	}

	// Get backend
	backend, ok := e.backends[connectionConfig.Backend]
	if !ok {
		return nil, fmt.Errorf("backend %s not registered", connectionConfig.Backend)
	}

	// Connect to backend
	if err := backend.Connect(connectionConfig); err != nil {
		return nil, fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backend.Close() }()

	// Execute rollback SQL
	if migration.DownSQL == "" {
		return &RollbackResult{
			Success: false,
			Message: "migration does not have rollback SQL",
			Errors:  []string{"migration does not have rollback SQL"},
		}, nil
	}

	// Create a rollback migration script
	rollbackMigration := &backends.MigrationScript{
		Schema:     migration.Schema,
		Version:    migration.Version,
		Name:       migration.Name + "_rollback",
		Connection: migration.Connection,
		Backend:    migration.Backend,
		UpSQL:      migration.DownSQL, // Use DownSQL as UpSQL for rollback
		DownSQL:    migration.UpSQL,   // Use UpSQL as DownSQL for rollback
	}

	// Execute rollback
	err = backend.ExecuteMigration(ctx, rollbackMigration)
	if err != nil {
		// Extract execution context
		executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

		// Record failed rollback
		record := &state.MigrationRecord{
			MigrationID:      migrationID + "_rollback",
			Schema:           migration.Schema,
			Table:            "",
			Version:          migration.Version,
			Connection:       migration.Connection,
			Backend:          migration.Backend,
			Status:           "failed",
			AppliedAt:        time.Now().Format(time.RFC3339),
			ErrorMessage:     err.Error(),
			ExecutedBy:       executedBy,
			ExecutionMethod:  executionMethod,
			ExecutionContext: executionContext,
		}
		_ = e.stateTracker.RecordMigration(ctx, record)

		return &RollbackResult{
			Success: false,
			Message: "rollback failed",
			Errors:  []string{err.Error()},
		}, nil
	}

	// Extract execution context
	executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

	// Remove migration from state (mark as not applied)
	// We'll delete the record or mark it as rolled back
	// For now, we'll create a rollback record
	record := &state.MigrationRecord{
		MigrationID:      migrationID + "_rollback",
		Schema:           migration.Schema,
		Table:            "",
		Version:          migration.Version,
		Connection:       migration.Connection,
		Backend:          migration.Backend,
		Status:           "rolled_back",
		AppliedAt:        time.Now().Format(time.RFC3339),
		ErrorMessage:     "",
		ExecutedBy:       executedBy,
		ExecutionMethod:  executionMethod,
		ExecutionContext: executionContext,
	}
	_ = e.stateTracker.RecordMigration(ctx, record)

	return &RollbackResult{
		Success: true,
		Message: "rollback completed successfully",
		Errors:  []string{},
	}, nil
}

// RollbackResult represents the result of a rollback operation
type RollbackResult struct {
	Success bool
	Message string
	Errors  []string
}

// HealthCheck performs health checks on the executor
func (e *Executor) HealthCheck(ctx context.Context) error {
	// Check state tracker
	if err := e.stateTracker.Initialize(ctx); err != nil {
		return fmt.Errorf("state tracker health check failed: %w", err)
	}
	return nil
}

// GetStateTracker returns the state tracker
func (e *Executor) GetStateTracker() state.StateTracker {
	return e.stateTracker
}

// ExecuteResult represents the result of migration execution
type ExecuteResult struct {
	Success bool
	Applied []string
	Skipped []string
	Errors  []string
	Queued  bool   // Whether the job was queued instead of executed
	JobID   string // Job ID if queued
}

// getMigrationID generates a unique migration ID
func (e *Executor) getMigrationID(migration *backends.MigrationScript) string {
	// If schema is provided, include it in the ID for uniqueness
	// Format: {schema}_{connection}_{version}_{name} or {connection}_{version}_{name}
	if migration.Schema != "" {
		return fmt.Sprintf("%s_%s_%s_%s", migration.Schema, migration.Connection, migration.Version, migration.Name)
	}
	return fmt.Sprintf("%s_%s_%s", migration.Connection, migration.Version, migration.Name)
}

// getConnectionConfig gets connection config
func (e *Executor) getConnectionConfig(connectionName string) (*backends.ConnectionConfig, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	config, ok := e.connections[connectionName]
	if !ok {
		return nil, fmt.Errorf("connection %s not found", connectionName)
	}

	return config, nil
}
