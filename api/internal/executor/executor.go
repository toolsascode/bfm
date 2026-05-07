package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/backends/postgresql"
	"github.com/toolsascode/bfm/api/internal/logger"
	"github.com/toolsascode/bfm/api/internal/queue"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"
)

// Context keys for execution metadata
type contextKey string

const (
	executedByKey         contextKey = "executed_by"
	executionMethodKey    contextKey = "execution_method"
	executionContextKey   contextKey = "execution_context"
	autoMigrateContextKey contextKey = "bfm_auto_migrate"
)

// WithAutoMigrateContext marks ctx so executeSync skips migrations with empty Schema
// when no schema was provided in the request (startup auto-migrate). Manual/API runs
// without this value still get a clear error for dynamic-schema migrations.
func WithAutoMigrateContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, autoMigrateContextKey, true)
}

func isAutoMigrateContext(ctx context.Context) bool {
	v, ok := ctx.Value(autoMigrateContextKey).(bool)
	return ok && v
}

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

// GetSkippedMigrations retrieves skipped migrations from the state tracker
func (e *Executor) GetSkippedMigrations(ctx context.Context, migrationID string, limit int) ([]*state.SkippedMigration, error) {
	return e.stateTracker.GetSkippedMigrations(ctx, migrationID, limit)
}

// ExecuteSync executes migrations synchronously (bypasses queue, used by worker)
func (e *Executor) ExecuteSync(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool, ignoreDependencies bool) (*ExecuteResult, error) {
	return e.executeSync(ctx, target, connectionName, schemaName, dryRun, ignoreDependencies)
}

// Execute executes migrations based on a target specification
// If queue is configured, it will queue the job instead of executing directly
func (e *Executor) Execute(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool, ignoreDependencies bool) (*ExecuteResult, error) {
	// If queue is enabled, queue the job instead of executing
	e.mu.Lock()
	hasQueue := e.queue != nil
	e.mu.Unlock()

	if hasQueue {
		return e.queueJob(ctx, target, connectionName, schemaName, dryRun)
	}

	// Otherwise, execute synchronously
	return e.executeSync(ctx, target, connectionName, schemaName, dryRun, ignoreDependencies)
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

// expandWithPendingDependencies takes an initial set of migrations and expands it by
// including any pending dependency migrations referenced via structured dependencies.
// It uses the state tracker to ensure already-applied dependencies are not re-executed.
// Returns the expanded migrations, a map indicating which migrations are dependencies,
// and a map of dependency ID -> parent migration ID.
func (e *Executor) expandWithPendingDependencies(ctx context.Context, migrations []*backends.MigrationScript) ([]*backends.MigrationScript, map[string]bool, map[string]string, error) {
	if len(migrations) == 0 {
		return migrations, make(map[string]bool), make(map[string]string), nil
	}

	// Build quick lookup of already selected migrations by ID so we don't duplicate.
	selected := make(map[string]*backends.MigrationScript)
	for _, m := range migrations {
		selected[e.getMigrationID(m)] = m
	}

	resolver := registry.NewDependencyResolver(e.registry, e.stateTracker)

	// Collect additional migrations to include and track which are dependencies
	var toInclude []*backends.MigrationScript
	dependencyMap := make(map[string]bool)         // Maps migration ID -> true if it's a dependency
	dependencyParentMap := make(map[string]string) // Maps dependency ID -> parent migration ID

	for _, migration := range migrations {
		if len(migration.StructuredDependencies) > 0 {
			logger.Debug("Migration %s_%s has %d structured dependency(ies), expanding...", migration.Version, migration.Name, len(migration.StructuredDependencies))
		}
		for _, dep := range migration.StructuredDependencies {
			logger.Debug("Resolving dependency: connection=%s, schema=%s, target=%s, type=%s", dep.Connection, dep.Schema, dep.Target, dep.TargetType)
			// Resolve targets for this dependency (may be cross-connection).
			targetMigrations, err := resolver.ResolveDependencyTargets(dep)
			if err != nil {
				// Requirement 1: If dependencies don't exist, proceed with migration (don't fail)
				logger.Warnf("Dependency not found for %s_%s: connection=%s, schema=%s, target=%s, type=%s: %v. Proceeding with migration.", migration.Version, migration.Name, dep.Connection, dep.Schema, dep.Target, dep.TargetType, err)
				continue
			}

			logger.Debug("Found %d target migration(s) for dependency: connection=%s, schema=%s, target=%s", len(targetMigrations), dep.Connection, dep.Schema, dep.Target)

			if len(targetMigrations) == 0 {
				logger.Warnf("No target migrations found for dependency: connection=%s, schema=%s, target=%s, type=%s. Proceeding with migration.", dep.Connection, dep.Schema, dep.Target, dep.TargetType)
				continue
			}

			for _, target := range targetMigrations {
				targetID := e.getMigrationID(target)
				logger.Debug("Checking dependency migration %s (connection=%s, schema=%s, version=%s, name=%s)", targetID, target.Connection, target.Schema, target.Version, target.Name)

				// If it's already in the initial set, nothing to do.
				if _, exists := selected[targetID]; exists {
					logger.Debug("Dependency migration %s already in execution set, skipping", targetID)
					continue
				}

				// Only include if the dependency migration is not yet applied.
				applied, err := e.stateTracker.IsMigrationApplied(ctx, targetID)
				if err != nil {
					logger.Errorf("Error checking if migration %s is applied: %v", targetID, err)
					return nil, make(map[string]bool), make(map[string]string), fmt.Errorf("failed to check dependency migration status for %s: %w", targetID, err)
				}
				logger.Debug("Migration %s applied status: %v", targetID, applied)
				if applied {
					logger.Debug("Dependency migration %s already applied, skipping", targetID)
					continue
				}

				logger.Infof("Auto-including pending dependency migration: %s (connection=%s, schema=%s) for %s_%s", targetID, target.Connection, target.Schema, migration.Version, migration.Name)
				selected[targetID] = target
				toInclude = append(toInclude, target)
				dependencyMap[targetID] = true // Mark as dependency
				parentID := e.getMigrationID(migration)
				dependencyParentMap[targetID] = parentID // Track parent migration
			}
		}
	}

	// If nothing extra was found, return original slice.
	if len(toInclude) == 0 {
		logger.Infof("No pending dependency migrations to auto-include (all dependencies already applied or not found)")
		return migrations, make(map[string]bool), make(map[string]string), nil
	}

	logger.Infof("Expanded migration set: %d initial + %d auto-included dependencies = %d total", len(migrations), len(toInclude), len(migrations)+len(toInclude))

	// Merge initial migrations + newly included ones.
	expanded := make([]*backends.MigrationScript, 0, len(migrations)+len(toInclude))
	expanded = append(expanded, migrations...)
	expanded = append(expanded, toInclude...)

	return expanded, dependencyMap, dependencyParentMap, nil
}

// runSingleMigrationUp records pending state, runs the migration backend, and records the outcome.
// Caller must hold WithMigrationExecutionLock for the same (migrationID, schema, connection).
func (e *Executor) runSingleMigrationUp(
	ctx context.Context,
	migration *backends.MigrationScript,
	migrationID string,
	schema string,
	schemaName string,
	dependencyMap map[string]bool,
	dependencyParentMap map[string]string,
	executedDependencies map[string][]string,
	result *ExecuteResult,
) {
	// Check if this is a dependency migration
	// NOTE: Only override migrationID for dependencies if schemaName was NOT provided
	// If schemaName was provided, we need to track per-schema even for dependencies
	isDependency := dependencyMap != nil && dependencyMap[migrationID]
	baseMigrationID := e.getMigrationID(migration)
	isDependency = isDependency || (dependencyMap != nil && dependencyMap[baseMigrationID])
	if isDependency && schemaName == "" {
		// Only use base ID for dependencies if no specific schema was requested
		// This preserves per-schema tracking when schemaName is provided
		migrationID = baseMigrationID
		logger.Debug("Migration is a dependency, using base ID: %s", migrationID)
	}

	logger.Debug("Recording migration with ID: %s (schema: %s, isDependency: %v)", migrationID, schema, isDependency)

	// Extract execution context
	executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

	// Record migration start IMMEDIATELY to prevent concurrent execution
	// Use migration.Connection (not connectionName) since this migration may be from a different connection
	record := &state.MigrationRecord{
		MigrationID:      migrationID,
		Schema:           schema,
		Table:            "",
		Version:          migration.Version,
		Connection:       migration.Connection,
		Backend:          migration.Backend,
		Status:           "pending",
		AppliedAt:        time.Now().Format(time.RFC3339),
		ErrorMessage:     "",
		ExecutedBy:       executedBy,
		ExecutionMethod:  executionMethod,
		ExecutionContext: executionContext,
	}

	// Record as pending immediately to prevent race conditions
	// For dependencies, use RecordDependencyMigration (requirement 4: no history)
	// If this fails because another process already marked it as pending/applied, skip execution
	var recordErr error
	if isDependency {
		recordErr = e.stateTracker.RecordDependencyMigration(ctx, record)
	} else {
		logger.Debug("Recording migration as pending: migrationID=%s, schema=%s, status=%s", record.MigrationID, record.Schema, record.Status)
		recordErr = e.stateTracker.RecordMigration(ctx, record)
		if recordErr == nil {
			logger.Debug("Successfully recorded migration as pending: migrationID=%s, schema=%s - history should be in migrations_history", record.MigrationID, record.Schema)
		}
	}
	if recordErr != nil {
		// Re-check if migration was applied by another process (concurrency control)
		// Use IsMigrationApplied (not IsMigrationPendingOrApplied) because we want to skip only if actually applied
		applied, checkErr := e.stateTracker.IsMigrationApplied(ctx, migrationID)
		if checkErr == nil && applied {
			result.Skipped = append(result.Skipped, migrationID)
			return
		}
		logger.Errorf("Failed to record migration start for %s (schema=%s): %v", migrationID, record.Schema, recordErr)
		result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration start for %s: %v", migrationID, recordErr))
		return
	}

	// Double-check after recording to ensure we didn't race with another process (concurrency control)
	// Use IsMigrationApplied (not IsMigrationPendingOrApplied) because we just recorded it as pending ourselves
	// We only want to skip if another process marked it as APPLIED while we were recording
	applied, checkErr := e.stateTracker.IsMigrationApplied(ctx, migrationID)
	if checkErr == nil && applied {
		// Another process marked it as applied, skip
		result.Skipped = append(result.Skipped, migrationID)
		return
	}

	// Get backend for this migration's connection (may differ from target connection for cross-connection dependencies)
	migrationConnectionConfig, err := e.getConnectionConfig(migration.Connection)
	if err != nil {
		record.Status = "failed"
		record.ErrorMessage = fmt.Sprintf("failed to get connection config for %s: %v", migration.Connection, err)
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", migrationID, err))
		if err := e.stateTracker.RecordMigration(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration %s: %v", migrationID, err))
		}
		return
	}

	migrationBackend, ok := e.backends[migrationConnectionConfig.Backend]
	if !ok {
		record.Status = "failed"
		record.ErrorMessage = fmt.Sprintf("backend %s not registered for connection %s", migrationConnectionConfig.Backend, migration.Connection)
		result.Errors = append(result.Errors, fmt.Sprintf("%s: backend %s not registered", migrationID, migrationConnectionConfig.Backend))
		if err := e.stateTracker.RecordMigration(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration %s: %v", migrationID, err))
		}
		return
	}

	// Connect to the migration's backend (may be different from target backend)
	if err := migrationBackend.Connect(migrationConnectionConfig); err != nil {
		record.Status = "failed"
		record.ErrorMessage = fmt.Sprintf("failed to connect to backend for %s: %v", migration.Connection, err)
		result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to connect: %v", migrationID, err))
		if err := e.stateTracker.RecordMigration(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration %s: %v", migrationID, err))
		}
		return
	}

	// Apply template variable replacement
	upSQL, err := replaceTemplateVariables(migration.UpSQL, migration, schema)
	if err != nil {
		// Migration was already marked as pending, update to failed
		record.Status = "failed"
		record.ErrorMessage = fmt.Sprintf("failed to replace template variables in UpSQL: %v", err)
		result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to replace template variables in UpSQL: %v", migrationID, err))
		// Record the failure
		if isDependency {
			if recordErr := e.stateTracker.RecordDependencyMigration(ctx, record); recordErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to record dependency migration failure %s: %v", migrationID, recordErr))
			}
		} else {
			if recordErr := e.stateTracker.RecordMigration(ctx, record); recordErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration failure %s: %v", migrationID, recordErr))
			}
		}
		return
	}

	downSQL := migration.DownSQL
	if downSQL != "" {
		var err error
		downSQL, err = replaceTemplateVariables(migration.DownSQL, migration, schema)
		if err != nil {
			// Migration was already marked as pending, update to failed
			record.Status = "failed"
			record.ErrorMessage = fmt.Sprintf("failed to replace template variables in DownSQL: %v", err)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to replace template variables in DownSQL: %v", migrationID, err))
			// Record the failure
			if isDependency {
				if recordErr := e.stateTracker.RecordDependencyMigration(ctx, record); recordErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to record dependency migration failure %s: %v", migrationID, recordErr))
				}
			} else {
				if recordErr := e.stateTracker.RecordMigration(ctx, record); recordErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration failure %s: %v", migrationID, recordErr))
				}
			}
			return
		}
	}

	// Convert executor.MigrationScript to backends.MigrationScript
	// Use provided schema instead of migration.Schema for dynamic schemas
	backendMigration := &backends.MigrationScript{
		Schema:     schema,
		Version:    migration.Version,
		Name:       migration.Name,
		Connection: migration.Connection,
		Backend:    migration.Backend,
		UpSQL:      upSQL,
		DownSQL:    downSQL,
	}

	// Execute the migration using its own backend
	err = migrationBackend.ExecuteMigration(ctx, backendMigration)
	_ = migrationBackend.Close() // Close after execution
	if err != nil {
		record.Status = "failed"
		record.ErrorMessage = err.Error()
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", migrationID, err))
	} else {
		record.Status = "success"
		// Fresh completion time so history ordering is deterministic (pending row may share the pre-exec timestamp).
		record.AppliedAt = time.Now().Format(time.RFC3339)
		result.Applied = append(result.Applied, migrationID)

		// Requirement 3: Track executed dependencies for parent migration
		if isDependency && dependencyParentMap != nil {
			parentID := dependencyParentMap[baseMigrationID]
			if parentID == "" {
				// Try with full migrationID
				parentID = dependencyParentMap[migrationID]
			}
			if parentID != "" {
				executedDependencies[parentID] = append(executedDependencies[parentID], migrationID)
				logger.Debug("Tracked dependency %s for parent migration %s", migrationID, parentID)
			}
		}
	}

	// Record migration in state tracker
	// Requirement 4: Dependencies use RecordDependencyMigration (no history)
	// CRITICAL: Ensure record.Schema is set correctly for schema-specific migrations
	// The schema must match the schema used in migrations_executions for ON CONFLICT to work
	if isDependency {
		if err := e.stateTracker.RecordDependencyMigration(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to record dependency migration %s: %v", migrationID, err))
		}
	} else {
		// For non-dependencies, add executed dependencies to execution context
		if len(executedDependencies[migrationID]) > 0 {
			// Parse existing execution context and add dependencies
			var execCtx map[string]interface{}
			if executionContext != "" {
				if err := json.Unmarshal([]byte(executionContext), &execCtx); err != nil {
					execCtx = make(map[string]interface{})
				}
			} else {
				execCtx = make(map[string]interface{})
			}
			execCtx["executed_dependencies"] = executedDependencies[migrationID]
			if updatedCtx, err := json.Marshal(execCtx); err == nil {
				record.ExecutionContext = string(updatedCtx)
			}
		}
		// Ensure schema is set correctly for the update (should already be set from initial record creation)
		logger.Debug("Updating migration record: migrationID=%s, schema=%s, status=%s", record.MigrationID, record.Schema, record.Status)
		if err := e.stateTracker.RecordMigration(ctx, record); err != nil {
			logger.Errorf("Failed to record migration %s (status=%s, schema=%s): %v", migrationID, record.Status, record.Schema, err)
			result.Errors = append(result.Errors, fmt.Sprintf("failed to record migration %s: %v", migrationID, err))
		} else {
			logger.Debug("Successfully recorded migration %s (status=%s, schema=%s) - history should be in migrations_history", migrationID, record.Status, record.Schema)
		}
	}
}

// executeSync executes migrations synchronously
func (e *Executor) executeSync(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemaName string, dryRun bool, ignoreDependencies bool) (*ExecuteResult, error) {
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

	// Log initial migrations found
	logger.Debug("Found %d migration(s) matching target (backend=%s, connection=%s, schema=%s)", len(migrations), target.Backend, target.Connection, target.Schema)
	for _, m := range migrations {
		logger.Debug("  - %s_%s (connection=%s, schema=%s)", m.Version, m.Name, m.Connection, m.Schema)
	}

	// Expand with pending dependencies (unless ignore_dependencies is true)
	var sortedMigrations []*backends.MigrationScript
	var dependencyResolutionError error
	var dependencyMap map[string]bool
	var dependencyParentMap map[string]string
	if ignoreDependencies {
		dependencyMap = make(map[string]bool)
		dependencyParentMap = make(map[string]string)
		// Skip dependency expansion and validation, just sort by version
		sort.Slice(migrations, func(i, j int) bool {
			return migrations[i].Version < migrations[j].Version
		})
		sortedMigrations = migrations
		logger.Infof("Ignoring dependencies: sorting migrations by version only")
	} else {
		// If any of the selected migrations declares structured dependencies, expand the set
		// with any pending dependency migrations (including cross-connection) so that
		// dependencies are executed automatically ahead of dependents.
		migrations, dependencyMap, dependencyParentMap, err = e.expandWithPendingDependencies(ctx, migrations)
		if err != nil {
			return nil, fmt.Errorf("failed to expand migrations with dependencies: %w", err)
		}
		logger.Infof("After dependency expansion: %d migration(s) ready for execution", len(migrations))

		// Get backend for the target connection (needed for validation)
		connectionConfig, err := e.getConnectionConfig(connectionName)
		if err != nil {
			return nil, fmt.Errorf("failed to get connection config: %w", err)
		}

		targetBackend, ok := e.backends[connectionConfig.Backend]
		if !ok {
			return nil, fmt.Errorf("backend %s not registered", connectionConfig.Backend)
		}

		// Sort migrations topologically based on dependencies
		// Use DependencyResolver for structured dependencies, fall back to simple topologicalSort for backward compatibility
		var depErr error
		sortedMigrations, depErr = e.resolveDependencies(migrations)
		if depErr != nil {
			// If dependency resolution fails, fall back to version-based sort and report error
			logger.Warnf("Dependency resolution failed: %v, falling back to version-based sort", depErr)
			sort.Slice(migrations, func(i, j int) bool {
				return migrations[i].Version < migrations[j].Version
			})
			sortedMigrations = migrations
			dependencyResolutionError = depErr
			// Add error to result but continue execution
		}

		// Ensure target backend is connected (for validation)
		if err := targetBackend.Connect(connectionConfig); err != nil {
			return nil, fmt.Errorf("failed to connect to backend: %w", err)
		}
		defer func() { _ = targetBackend.Close() }()

		// Validate dependencies after sorting (for PostgreSQL backend)
		// Pass the sorted execution set so validator knows which migrations will be executed
		// Only validate migrations that belong to the target connection
		if connectionConfig.Backend == "postgresql" {
			pgBackend, ok := targetBackend.(*postgresql.Backend)
			if ok {
				validator := postgresql.NewDependencyValidator(pgBackend, e.stateTracker, e.registry)
				for _, migration := range sortedMigrations {
					// Only validate migrations for the target connection
					if migration.Connection == connectionName {
						validationErrors := validator.ValidateDependenciesWithExecutionSet(ctx, migration, schemaName, sortedMigrations)
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
		}
	}

	// Log final sorted migration execution order
	if len(sortedMigrations) > 0 {
		logger.Infof("Final migration execution set (%d total, sorted by dependencies):", len(sortedMigrations))
		for i, m := range sortedMigrations {
			logger.Infof("  %d. %s_%s (connection=%s, schema=%s)", i+1, m.Version, m.Name, m.Connection, m.Schema)
		}
	} else {
		logger.Warnf("No migrations to execute after dependency expansion and sorting")
	}

	result := &ExecuteResult{
		Applied: []string{},
		Skipped: []string{},
		Errors:  []string{},
	}

	// If dependency resolution had errors, add them to result
	if dependencyResolutionError != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("dependency resolution: %v", dependencyResolutionError))
	}

	// Track executed dependencies to add to parent migration's execution context
	executedDependencies := make(map[string][]string) // Maps parent migration ID -> list of executed dependency IDs

	// Process each migration
	logger.Infof("Starting execution of %d migration(s)", len(sortedMigrations))
	for _, migration := range sortedMigrations {
		// Resolve schema name (use provided or from migration)
		schema := schemaName
		if schema == "" {
			schema = migration.Schema
		}

		// Determine migration ID based on schema requirements
		// CRITICAL: If schemaName was explicitly provided, ALWAYS use schema-specific ID for per-schema tracking
		// This ensures migrations are tracked per-schema in migrations_executions, not just globally in migrations_list
		var migrationID string
		if schemaName != "" && schema != "" {
			// Schema was explicitly provided in request - ALWAYS use schema-specific ID
			// This is required even if migration.Schema has a value, because the user wants to execute for a specific schema
			migrationID = e.getMigrationIDWithSchema(migration, schema)
			logger.Debug("Using schema-specific migration ID: %s (requested schema: %s, migration.Schema: %s)", migrationID, schema, migration.Schema)
		} else if migration.Schema == "" {
			// Dynamic schema mode: track per schema
			// If schema is still empty, we can't track it properly - this is an error condition
			if schema == "" {
				if isAutoMigrateContext(ctx) {
					logger.Infof("Skipping migration %s_%s: dynamic schema requires an explicit schema in the request (auto-migrate)", migration.Version, migration.Name)
					continue
				}
				result.Errors = append(result.Errors, fmt.Sprintf("migration %s_%s has dynamic schema but no schema provided in request", migration.Version, migration.Name))
				continue
			}
			migrationID = e.getMigrationIDWithSchema(migration, schema)
		} else {
			// Fixed schema mode: use base migration ID
			migrationID = e.getMigrationID(migration)
		}

		logger.Debug("Checking migration status: migrationID=%s, schema=%s, migration.Schema=%s, schemaName=%s", migrationID, schema, migration.Schema, schemaName)

		// Check if already applied using the migration ID (which is schema-specific if schemaName was provided)
		applied, err := e.stateTracker.IsMigrationApplied(ctx, migrationID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to check migration status for %s: %v", migrationID, err))
			continue
		}

		if applied {
			logger.Infof("Migration %s already applied, skipping", migrationID)
			result.Skipped = append(result.Skipped, migrationID)
			continue
		}

		// Execute migration
		if dryRun {
			result.Applied = append(result.Applied, fmt.Sprintf("%s (dry-run)", migrationID))
			continue
		}

		lockSchema := schema
		if lockSchema == "" {
			lockSchema = migration.Schema
		}

		if err := e.stateTracker.WithMigrationExecutionLock(ctx, migrationID, lockSchema, migration.Connection, func() error {
			e.runSingleMigrationUp(ctx, migration, migrationID, schema, schemaName, dependencyMap, dependencyParentMap, executedDependencies, result)
			return nil
		}); err != nil {
			if errors.Is(err, state.ErrMigrationAlreadyInProgress) {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", migrationID, err))
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("%s: migration lock: %v", migrationID, err))
			continue
		}
	}

	// Record skipped migrations if any
	if len(result.Skipped) > 0 {
		executedBy, executionMethod, executionContext := GetExecutionContext(ctx)
		if err := e.stateTracker.RecordSkippedMigrations(ctx, result.Skipped, executedBy, executionMethod, executionContext); err != nil {
			// Log error but don't fail the execution
			logger.Warnf("Failed to record skipped migrations: %v", err)
		}
	}

	result.Success = len(result.Errors) == 0

	// Log execution summary
	logger.Infof("Migration execution completed: %d applied, %d skipped, %d errors", len(result.Applied), len(result.Skipped), len(result.Errors))
	if len(result.Applied) > 0 {
		logger.Infof("Applied migrations: %v", result.Applied)
	}
	if len(result.Skipped) > 0 {
		logger.Infof("Skipped migrations (already applied): %v", result.Skipped)
	}
	if len(result.Errors) > 0 {
		logger.Errorf("Migration errors: %v", result.Errors)
	}

	return result, nil
}

// OrderMigrationBatch returns migration_ids sorted in dependency order for the given connection.
// Duplicate IDs are preserved in the output (grouped after their migration's topological position).
func (e *Executor) OrderMigrationBatch(migrationIDs []string, connection string) ([]string, error) {
	if len(migrationIDs) == 0 {
		return nil, nil
	}
	if connection == "" {
		return nil, fmt.Errorf("connection is required")
	}

	type pair struct {
		id string
		m  *backends.MigrationScript
	}
	pairs := make([]pair, 0, len(migrationIDs))
	var unknown []string
	for _, id := range migrationIDs {
		m := e.GetMigrationByID(id)
		if m == nil {
			unknown = append(unknown, id)
			continue
		}
		if m.Connection != connection {
			return nil, fmt.Errorf("migration %s belongs to connection %q, expected %q", id, m.Connection, connection)
		}
		pairs = append(pairs, pair{id: id, m: m})
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown migration_id(s): %s", strings.Join(unknown, ", "))
	}

	byBase := make(map[string][]string)
	seenBase := make(map[string]bool)
	var unique []*backends.MigrationScript
	for _, p := range pairs {
		base := e.getMigrationID(p.m)
		byBase[base] = append(byBase[base], p.id)
		if !seenBase[base] {
			seenBase[base] = true
			unique = append(unique, p.m)
		}
	}

	sorted, err := e.resolveDependencies(unique)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(migrationIDs))
	for _, m := range sorted {
		base := e.getMigrationID(m)
		out = append(out, byBase[base]...)
	}
	return out, nil
}

// GetAllMigrations returns all registered migrations
func (e *Executor) GetAllMigrations() []*backends.MigrationScript {
	return e.registry.GetAll()
}

// GetMigrationByID finds a migration by its ID
// Migration ID format: {version}_{name}_{backend}_{connection}
// Also supports schema-specific format: {schema}_{version}_{name}_{backend}_{connection}
// Also supports legacy formats for backward compatibility
func (e *Executor) GetMigrationByID(migrationID string) *backends.MigrationScript {
	allMigrations := e.registry.GetAll()

	// First, try to match against base IDs (exact match)
	// This handles base IDs even if they have 5+ parts due to underscores in names
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
	}

	// If no exact match found, try schema-specific matching
	// Check if migrationID could be schema-specific (format: {schema}_{version}_{name}_{backend}_{connection})
	parts := strings.Split(migrationID, "_")
	if len(parts) >= 5 {
		// Extract potential schema and base ID
		potentialSchema := parts[0]
		baseID := strings.Join(parts[1:], "_")

		for _, migration := range allMigrations {
			// Only match schema-specific IDs if the migration has a schema
			// Migrations without a schema should not match schema-specific IDs
			if migration.Schema != "" && migration.Schema == potentialSchema {
				// Check if the base ID matches this migration
				id := e.getMigrationID(migration)
				if id == baseID {
					// Verify the schema-specific ID matches
					schemaSpecificID := e.getMigrationIDWithSchema(migration, potentialSchema)
					if schemaSpecificID == migrationID {
						return migration
					}
				}
				// Also check legacy formats with schema
				legacyIDWithConnection := fmt.Sprintf("%s_%s_%s", migration.Connection, migration.Version, migration.Name)
				if legacyIDWithConnection == baseID {
					legacyIDWithSchema := fmt.Sprintf("%s_%s_%s_%s", migration.Schema, migration.Connection, migration.Version, migration.Name)
					if legacyIDWithSchema == migrationID {
						return migration
					}
				}
			}
		}
	}

	// Try legacy format with schema matching (for migrations that have schema)
	for _, migration := range allMigrations {
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

// GetMigrationDetail retrieves detailed information about a single migration from migrations_list
func (e *Executor) GetMigrationDetail(ctx context.Context, migrationID string) (*state.MigrationDetail, error) {
	return e.stateTracker.GetMigrationDetail(ctx, migrationID)
}

// GetMigrationExecutions retrieves all execution records for a migration, ordered by created_at DESC
func (e *Executor) GetMigrationExecutions(ctx context.Context, migrationID string) ([]*state.MigrationExecution, error) {
	return e.stateTracker.GetMigrationExecutions(ctx, migrationID)
}

// GetRecentExecutions retrieves recent execution records across all migrations, ordered by created_at DESC
func (e *Executor) GetRecentExecutions(ctx context.Context, limit int) ([]*state.MigrationExecution, error) {
	return e.stateTracker.GetRecentExecutions(ctx, limit)
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
		// First, check if the exact migration ID exists in filesystem (for base IDs)
		if _, exists := fileMigrations[migrationID]; exists {
			// Exact match found, keep this migration
			continue
		}

		// If not found, check if this is a schema-specific ID (format: {schema}_{version}_{name}_{backend}_{connection})
		// Extract base ID for comparison
		parts := strings.Split(migrationID, "_")
		var baseID string
		var isSchemaSpecific bool
		if len(parts) >= 5 {
			// Schema-specific ID: extract base ID by removing schema prefix
			baseID = strings.Join(parts[1:], "_")
			isSchemaSpecific = true
		} else {
			// Base ID format - if not found in filesystem, it should be removed
			baseID = migrationID
			isSchemaSpecific = false
		}

		// For schema-specific IDs, check if the base migration exists in filesystem
		// For base IDs, we already checked above and it doesn't exist, so remove it
		if isSchemaSpecific {
			// Schema-specific ID: only keep if base migration exists in filesystem
			if _, exists := fileMigrations[baseID]; !exists {
				// Base migration doesn't exist in filesystem, remove this schema-specific instance
				if err := e.stateTracker.DeleteMigration(ctx, migrationID); err != nil {
					// Log error but continue
					fmt.Printf("Warning: Failed to delete migration %s: %v\n", migrationID, err)
				} else {
					result.Removed = append(result.Removed, migrationID)
				}
			}
			// If baseID exists in filesystem, keep the schema-specific migration
		} else {
			// Base ID not found in filesystem, remove it
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

// CountPendingAutoMigratable returns how many registered migrations for the given
// connection and backend have a non-empty Schema (fixed-schema) and are not yet
// applied. Dynamic-schema migrations (empty Schema) are excluded — they cannot be
// applied by startup auto-migrate without an explicit schema in the request.
func (e *Executor) CountPendingAutoMigratable(ctx context.Context, connectionName, backend string) (int, error) {
	target := &registry.MigrationTarget{
		Backend:    backend,
		Connection: connectionName,
	}
	migrations, err := e.registry.FindByTarget(target)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range migrations {
		if m == nil || strings.TrimSpace(m.Schema) == "" {
			continue
		}
		id := e.getMigrationID(m)
		applied, err := e.stateTracker.IsMigrationApplied(ctx, id)
		if err != nil {
			return 0, err
		}
		if !applied {
			n++
		}
	}
	return n, nil
}

// ExecuteUp executes up migrations for the given schemas
func (e *Executor) ExecuteUp(ctx context.Context, target *registry.MigrationTarget, connectionName string, schemas []string, dryRun bool, ignoreDependencies bool) (*ExecuteResult, error) {
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
		schemaResult, err := e.executeSync(ctx, target, connectionName, schema, dryRun, ignoreDependencies)
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
func (e *Executor) ExecuteDown(ctx context.Context, migrationID string, schemas []string, dryRun bool, ignoreDependencies bool) (*ExecuteResult, error) {
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

		// Apply template variable replacement to down SQL
		downSQL, err := replaceTemplateVariables(migration.DownSQL, migration, schema)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: failed to replace template variables in DownSQL: %v", schema, err))
			continue
		}

		// Apply template variable replacement to up SQL (used as DownSQL in rollback)
		upSQL := migration.UpSQL
		if upSQL != "" {
			var err error
			upSQL, err = replaceTemplateVariables(migration.UpSQL, migration, schema)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("schema %s: failed to replace template variables in UpSQL: %v", schema, err))
				continue
			}
		}

		// Create a down migration script with schema
		downMigration := &backends.MigrationScript{
			Schema:     schema,
			Version:    migration.Version,
			Name:       migration.Name + "_down",
			Connection: migration.Connection,
			Backend:    migration.Backend,
			UpSQL:      downSQL, // Use DownSQL as UpSQL for down migration
			DownSQL:    upSQL,   // Use UpSQL as DownSQL
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
func (e *Executor) Rollback(ctx context.Context, migrationID string, schemas []string) (*RollbackResult, error) {
	// Get migration from registry
	migration := e.GetMigrationByID(migrationID)
	if migration == nil {
		return nil, fmt.Errorf("migration not found: %s", migrationID)
	}

	// Use provided schemas, or fall back to migration.Schema if empty
	// If both are empty, use empty string to process without schema
	schemasToUse := schemas
	if len(schemasToUse) == 0 {
		if migration.Schema != "" {
			schemasToUse = []string{migration.Schema}
		} else {
			schemasToUse = []string{""}
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

	// Execute rollback SQL
	if migration.DownSQL == "" {
		return &RollbackResult{
			Success: false,
			Message: "migration does not have rollback SQL",
			Errors:  []string{"migration does not have rollback SQL"},
		}, nil
	}

	result := &RollbackResult{
		Applied: []string{},
		Errors:  []string{},
	}

	// Execute rollback for each schema
	for _, schema := range schemasToUse {
		// Check if migration is applied for this schema by checking executions table
		// This is more accurate than checking migrations_list since executions table tracks per-schema
		baseMigrationID := e.getMigrationID(migration)
		executions, err := e.stateTracker.GetMigrationExecutions(ctx, baseMigrationID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: failed to check migration status: %v", schema, err))
			continue
		}

		// Find execution record matching this schema, version, connection, and backend
		var foundExecution *state.MigrationExecution
		for _, exec := range executions {
			// Check if schema matches (handle comma-separated schemas)
			schemaMatches := false
			if exec.Schema == schema {
				schemaMatches = true
			} else if exec.Schema != "" {
				// Handle comma-separated schemas
				schemas := strings.Split(exec.Schema, ",")
				for _, s := range schemas {
					if strings.TrimSpace(s) == schema {
						schemaMatches = true
						break
					}
				}
			}
			if schemaMatches && exec.Version == migration.Version &&
				exec.Connection == migration.Connection && exec.Backend == migration.Backend {
				foundExecution = exec
				break
			}
		}

		schemaMigrationID := e.getMigrationIDWithSchema(migration, schema)
		if foundExecution == nil || !foundExecution.Applied {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s (not applied)", schemaMigrationID))
			continue
		}

		// Create a rollback migration script with the specific schema
		rollbackMigration := &backends.MigrationScript{
			Schema:     schema,
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
				MigrationID:      schemaMigrationID + "_rollback",
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

			result.Errors = append(result.Errors, fmt.Sprintf("schema %s: %v", schema, err))
			continue
		}

		// Extract execution context
		executedBy, executionMethod, executionContext := GetExecutionContext(ctx)

		// Record successful rollback
		record := &state.MigrationRecord{
			MigrationID:      schemaMigrationID + "_rollback",
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

	// Success is true only if there are no errors AND at least one migration was rolled back
	result.Success = len(result.Errors) == 0 && len(result.Applied) > 0
	if len(result.Applied) > 0 {
		result.Message = fmt.Sprintf("rollback completed successfully for %d schema(s)", len(result.Applied))
	} else if len(result.Errors) > 0 {
		result.Message = "rollback failed"
	} else {
		result.Message = "no migrations to rollback"
	}

	return result, nil
}

// RollbackResult represents the result of a rollback operation
type RollbackResult struct {
	Success bool
	Message string
	Applied []string
	Skipped []string
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

// replaceTemplateVariables replaces template variables in SQL/JSON content
// Variables: {{.Connection}}, {{.Schema}}, {{.Backend}}, {{.Version}}
// Note: Variable names are case-insensitive (e.g., {{.connection}} == {{.Connection}})
func replaceTemplateVariables(content string, migration *backends.MigrationScript, schema string) (string, error) {
	// Determine schema to use
	schemaToUse := schema
	if schemaToUse == "" {
		schemaToUse = migration.Schema
	}

	// Create template data (using canonical case)
	data := map[string]string{
		"Connection": migration.Connection,
		"Schema":     schemaToUse,
		"Backend":    migration.Backend,
		"Version":    migration.Version,
	}

	// Normalize template variables to canonical case (first letter uppercase, rest lowercase)
	// This makes the template variables case-insensitive
	normalizedContent := normalizeTemplateVariables(content)

	// Parse template
	tmpl, err := template.New("migration").Parse(normalizedContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// normalizeTemplateVariables normalizes template variable names to canonical case
// Converts {{.variable}} -> {{.Variable}}, {{.VARIABLE}} -> {{.Variable}}, etc.
func normalizeTemplateVariables(content string) string {
	// Map of lowercase variable names to canonical names
	canonicalVars := map[string]string{
		"connection": "Connection",
		"schema":     "Schema",
		"backend":    "Backend",
		"version":    "Version",
	}

	// Regex to match template variables: {{.VariableName}}
	re := regexp.MustCompile(`\{\{\.([A-Za-z][A-Za-z0-9_]*)\}\}`)

	return re.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the variable name (remove {{. and }})
		varName := match[3 : len(match)-2]

		// Convert to lowercase to look up canonical name
		lowerVarName := strings.ToLower(varName)
		if canonicalName, exists := canonicalVars[lowerVarName]; exists {
			return "{{." + canonicalName + "}}"
		}

		// If not in our canonical list, return as-is
		return match
	})
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
