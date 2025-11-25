package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"bfm/api/internal/backends"
	"bfm/api/internal/backends/postgresql"
	"bfm/api/internal/logger"
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

// topologicalSort sorts migrations based on their dependencies using topological sort
// Returns sorted migrations and any errors (circular dependencies, missing dependencies)
func (e *Executor) topologicalSort(migrations []*backends.MigrationScript) ([]*backends.MigrationScript, error) {
	if len(migrations) == 0 {
		return migrations, nil
	}

	// Build a map of migration name to migration(s) for quick lookup
	// Since dependencies are by name, we need to handle multiple migrations with same name
	nameToMigrations := make(map[string][]*backends.MigrationScript)
	for _, migration := range migrations {
		nameToMigrations[migration.Name] = append(nameToMigrations[migration.Name], migration)
	}

	// Build dependency graph: migration ID -> list of dependency migration IDs
	// Also build reverse graph for topological sort
	graph := make(map[string][]string)        // migration -> dependencies
	reverseGraph := make(map[string][]string) // dependency -> dependents
	inDegree := make(map[string]int)          // in-degree count for each migration

	// Create a unique ID for each migration (using the same format as getMigrationID)
	getID := func(m *backends.MigrationScript) string {
		return e.getMigrationID(m)
	}

	// Initialize all migrations in the graph
	migrationMap := make(map[string]*backends.MigrationScript)
	for _, migration := range migrations {
		migrationID := getID(migration)
		migrationMap[migrationID] = migration
		graph[migrationID] = []string{}
		reverseGraph[migrationID] = []string{}
		inDegree[migrationID] = 0
	}

	// Build the dependency graph
	var missingDeps []string
	for _, migration := range migrations {
		migrationID := getID(migration)
		for _, depName := range migration.Dependencies {
			// Find all migrations with this name (can be across different connections/backends)
			depMigrations := e.registry.GetMigrationByName(depName)
			if len(depMigrations) == 0 {
				missingDeps = append(missingDeps, fmt.Sprintf("%s depends on %s (not found)", migrationID, depName))
				continue
			}

			// For each dependency, check if it's in our current set of migrations
			// If yes, add edge; if no, it's an external dependency (we'll treat as satisfied)
			for _, depMigration := range depMigrations {
				depID := getID(depMigration)
				// Only add edge if dependency is in our current migration set
				if _, exists := migrationMap[depID]; exists {
					graph[migrationID] = append(graph[migrationID], depID)
					reverseGraph[depID] = append(reverseGraph[depID], migrationID)
					inDegree[migrationID]++
				}
			}
		}
	}

	// Report missing dependencies
	if len(missingDeps) > 0 {
		return nil, fmt.Errorf("missing dependencies: %s", strings.Join(missingDeps, "; "))
	}

	// Detect circular dependencies and perform topological sort using Kahn's algorithm
	// Start with migrations that have no dependencies (in-degree = 0)
	queue := make([]string, 0)
	for migrationID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, migrationID)
		}
	}

	// Sort initial queue by version for deterministic ordering
	sort.Slice(queue, func(i, j int) bool {
		return migrationMap[queue[i]].Version < migrationMap[queue[j]].Version
	})

	sorted := make([]*backends.MigrationScript, 0, len(migrations))
	processed := make(map[string]bool)

	// Process queue
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if processed[currentID] {
			continue
		}

		processed[currentID] = true
		sorted = append(sorted, migrationMap[currentID])

		// Reduce in-degree of dependents and add to queue
		newQueueItems := make([]string, 0)
		for _, dependentID := range reverseGraph[currentID] {
			inDegree[dependentID]--
			if inDegree[dependentID] == 0 {
				newQueueItems = append(newQueueItems, dependentID)
			}
		}
		// Sort new queue items by version before adding to maintain deterministic order
		sort.Slice(newQueueItems, func(i, j int) bool {
			return migrationMap[newQueueItems[i]].Version < migrationMap[newQueueItems[j]].Version
		})
		queue = append(queue, newQueueItems...)
	}

	// Check for circular dependencies (if not all migrations were processed)
	if len(sorted) < len(migrations) {
		var circular []string
		for migrationID := range migrationMap {
			if !processed[migrationID] {
				circular = append(circular, migrationID)
			}
		}
		return nil, fmt.Errorf("circular dependency detected involving migrations: %s", strings.Join(circular, ", "))
	}

	// The sorted list is already in topological order with version-based tiebreaking
	// No need for additional sorting

	return sorted, nil
}

// resolveDependencies resolves dependencies using DependencyResolver for structured dependencies,
// or falls back to topologicalSort for simple string dependencies
func (e *Executor) resolveDependencies(migrations []*backends.MigrationScript) ([]*backends.MigrationScript, error) {
	if len(migrations) == 0 {
		return migrations, nil
	}

	// Check if any migration has structured dependencies
	hasStructuredDeps := false
	for _, migration := range migrations {
		if len(migration.StructuredDependencies) > 0 {
			hasStructuredDeps = true
			break
		}
	}

	// If structured dependencies exist, use DependencyResolver
	if hasStructuredDeps {
		resolver := registry.NewDependencyResolver(e.registry, e.stateTracker)
		getMigrationID := func(m *backends.MigrationScript) string {
			return e.getMigrationID(m)
		}
		return resolver.ResolveDependencies(migrations, getMigrationID)
	}

	// Otherwise, use the existing topologicalSort for backward compatibility
	return e.topologicalSort(migrations)
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

	// Get backend for the connection (needed for validation)
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

	// Validate dependencies before execution (for PostgreSQL backend)
	if connectionConfig.Backend == "postgresql" {
		pgBackend, ok := backend.(*postgresql.Backend)
		if ok {
			validator := postgresql.NewDependencyValidator(pgBackend, e.stateTracker, e.registry)
			for _, migration := range migrations {
				validationErrors := validator.ValidateDependencies(ctx, migration, schemaName)
				if len(validationErrors) > 0 {
					var errorMsgs []string
					for _, err := range validationErrors {
						errorMsgs = append(errorMsgs, err.Error())
					}
					return nil, fmt.Errorf("dependency validation failed: %s", strings.Join(errorMsgs, "; "))
				}
			}
		}
	}

	// Sort migrations topologically based on dependencies
	// Use DependencyResolver for structured dependencies, fall back to simple topologicalSort for backward compatibility
	sortedMigrations, err := e.resolveDependencies(migrations)
	if err != nil {
		// If dependency resolution fails, fall back to version-based sort and report error
		logger.Warnf("Dependency resolution failed: %v, falling back to version-based sort", err)
		sort.Slice(migrations, func(i, j int) bool {
			return migrations[i].Version < migrations[j].Version
		})
		sortedMigrations = migrations
		// Add error to result but continue execution
	}

	result := &ExecuteResult{
		Applied: []string{},
		Skipped: []string{},
		Errors:  []string{},
	}

	// If dependency resolution had errors, add them to result
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("dependency resolution: %v", err))
	}

	// Process each migration
	for _, migration := range sortedMigrations {
		migrationID := e.getMigrationID(migration)

		// Resolve schema name (use provided or from migration)
		schema := schemaName
		if schema == "" {
			schema = migration.Schema
		}

		// Check if already applied (use base migration ID for checking)
		applied, err := e.stateTracker.IsMigrationApplied(ctx, migrationID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to check migration status for %s: %v", migrationID, err))
			continue
		}

		if applied {
			result.Skipped = append(result.Skipped, migrationID)
			continue
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
// Migration ID format: {version}_{name}_{backend}_{connection}
// Also supports legacy formats for backward compatibility
func (e *Executor) GetMigrationByID(migrationID string) *backends.MigrationScript {
	allMigrations := e.registry.GetAll()
	for _, migration := range allMigrations {
		// Primary format: {version}_{name}_{backend}_{connection}
		id := e.getMigrationID(migration)
		if id == migrationID {
			return migration
		}
		// Legacy format: {version}_{name} (old format without backend/connection)
		legacyID := fmt.Sprintf("%s_%s", migration.Version, migration.Name)
		if legacyID == migrationID {
			return migration
		}
		// Legacy format: {connection}_{version}_{name}
		legacyIDWithConnection := fmt.Sprintf("%s_%s_%s", migration.Connection, migration.Version, migration.Name)
		if legacyIDWithConnection == migrationID {
			return migration
		}
		// Legacy format with schema: {schema}_{connection}_{version}_{name}
		if migration.Schema != "" {
			legacyIDWithSchema := fmt.Sprintf("%s_%s_%s_%s", migration.Schema, migration.Connection, migration.Version, migration.Name)
			if legacyIDWithSchema == migrationID {
				return migration
			}
			// Legacy format with sanitized schema
			sanitizedSchema := strings.ReplaceAll(migration.Schema, "/", "_")
			sanitizedSchema = strings.Trim(sanitizedSchema, "_")
			for strings.Contains(sanitizedSchema, "__") {
				sanitizedSchema = strings.ReplaceAll(sanitizedSchema, "__", "_")
			}
			legacyIDWithSanitizedSchema := fmt.Sprintf("%s_%s_%s_%s", sanitizedSchema, migration.Connection, migration.Version, migration.Name)
			if legacyIDWithSanitizedSchema == migrationID {
				return migration
			}
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

// UpdateMigrationInfo updates migration metadata without affecting status/history
func (e *Executor) UpdateMigrationInfo(ctx context.Context, migrationID, schema, table, version, name, connection, backend string) error {
	return e.stateTracker.UpdateMigrationInfo(ctx, migrationID, schema, table, version, name, connection, backend)
}

// ReindexResult represents the result of a reindex operation
type ReindexResult struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Updated []string `json:"updated"`
	Total   int      `json:"total"`
}

// ReindexMigrations scans the filesystem and synchronizes the database with existing migration files
func (e *Executor) ReindexMigrations(ctx context.Context, sfmPath string) (*ReindexResult, error) {
	result := &ReindexResult{
		Added:   []string{},
		Removed: []string{},
		Updated: []string{},
	}

	if sfmPath == "" {
		return nil, fmt.Errorf("SFM path is required for reindexing")
	}

	// Check if directory exists
	if _, err := os.Stat(sfmPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SFM directory does not exist: %s", sfmPath)
	}

	// Scan all migration files from filesystem
	// Structure: sfm/{backend}/{connection}/{version}_{name}.go
	fileMigrations := make(map[string]struct {
		backend    string
		connection string
		version    string
		name       string
		filePath   string
		schema     string
	})

	err := filepath.Walk(sfmPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Verify directory structure: sfm/{backend}/{connection}/{version}_{name}.go
		relPath, err := filepath.Rel(sfmPath, path)
		if err != nil {
			return nil // Skip files we can't process
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) < 3 {
			return nil // Not in expected structure
		}

		filename := parts[len(parts)-1]
		filenameWithoutExt := strings.TrimSuffix(filename, ".go")

		// Verify filename format: {version}_{name}.go where version is 14 digits
		versionRegex := regexp.MustCompile(`^(\d{14})_(.+)$`)
		matches := versionRegex.FindStringSubmatch(filenameWithoutExt)
		if len(matches) != 3 {
			return nil // Skip files that don't match expected format
		}

		version := matches[1]
		name := matches[2]
		backend := parts[0]
		connection := parts[1]

		// Extract schema from .go file (for reference, not used in ID)
		schema := extractSchemaFromGoFile(path)

		// Generate migration ID using the same format as getMigrationID
		// Format: {version}_{name}_{backend}_{connection}
		migrationID := fmt.Sprintf("%s_%s_%s_%s", version, name, backend, connection)

		fileMigrations[migrationID] = struct {
			backend    string
			connection string
			version    string
			name       string
			filePath   string
			schema     string
		}{backend, connection, version, name, path, schema}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error scanning SFM directory: %w", err)
	}

	// Get all migrations from database
	dbMigrations, err := e.stateTracker.GetMigrationList(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get migrations from database: %w", err)
	}

	dbMigrationMap := make(map[string]*state.MigrationListItem)
	for _, migration := range dbMigrations {
		dbMigrationMap[migration.MigrationID] = migration
	}

	// Find migrations to add or update
	for migrationID, fileMigration := range fileMigrations {
		dbMigration, exists := dbMigrationMap[migrationID]
		if !exists {
			// Register this migration with schema from .go file
			if err := e.stateTracker.RegisterScannedMigration(ctx, migrationID, fileMigration.schema, "", fileMigration.version, fileMigration.name, fileMigration.connection, fileMigration.backend); err != nil {
				// Log error but continue
				fmt.Printf("Warning: Failed to register migration %s: %v\n", migrationID, err)
			} else {
				result.Added = append(result.Added, migrationID)
			}
		} else {
			// Migration exists - check if schema or other fields need updating
			needsUpdate := false
			updateSchema := dbMigration.Schema

			// Check if schema differs (file schema takes precedence if non-empty)
			if fileMigration.schema != "" && dbMigration.Schema != fileMigration.schema {
				needsUpdate = true
				updateSchema = fileMigration.schema
			}

			// Check if other fields differ (version, name, connection, backend)
			if dbMigration.Version != fileMigration.version ||
				dbMigration.Name != fileMigration.name ||
				dbMigration.Connection != fileMigration.connection ||
				dbMigration.Backend != fileMigration.backend {
				needsUpdate = true
			}

			if needsUpdate {
				// Update the migration metadata without affecting status/history
				if err := e.UpdateMigrationInfo(ctx, migrationID, updateSchema, dbMigration.Table, fileMigration.version, fileMigration.name, fileMigration.connection, fileMigration.backend); err != nil {
					fmt.Printf("Warning: Failed to update migration %s: %v\n", migrationID, err)
				} else {
					result.Updated = append(result.Updated, migrationID)
				}
			}
		}
	}

	// Find migrations to remove (in database but not in filesystem)
	for migrationID := range dbMigrationMap {
		if _, exists := fileMigrations[migrationID]; !exists {
			// Delete this migration from database
			if err := e.stateTracker.DeleteMigration(ctx, migrationID); err != nil {
				// Log error but continue
				fmt.Printf("Warning: Failed to delete migration %s: %v\n", migrationID, err)
			} else {
				result.Removed = append(result.Removed, migrationID)
			}
		}
	}

	// Get updated count
	updatedMigrations, err := e.stateTracker.GetMigrationList(ctx, nil)
	if err == nil {
		result.Total = len(updatedMigrations)
	}

	return result, nil
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
// This is used when checking if a migration is applied to a specific schema
// Base migration ID is {version}_{name}_{backend}_{connection}, but for schema-specific checks we include schema
func (e *Executor) getMigrationIDWithSchema(migration *backends.MigrationScript, schema string) string {
	baseID := e.getMigrationID(migration)
	if schema != "" {
		// For schema-specific checks, prefix with schema
		// This allows the same migration to be tracked separately per schema
		return fmt.Sprintf("%s_%s", schema, baseID)
	}
	return baseID
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
// Migration ID format: {version}_{name}_{backend}_{connection}
func (e *Executor) getMigrationID(migration *backends.MigrationScript) string {
	return fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
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
