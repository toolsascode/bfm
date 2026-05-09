package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/logger"
	"github.com/toolsascode/bfm/api/internal/state"
)

// Tracker implements StateTracker for PostgreSQL
type Tracker struct {
	pool   *pgxpool.Pool
	schema string
}

// NewTracker creates a new PostgreSQL state tracker
func NewTracker(connStr string, schema string) (*Tracker, error) {
	// Parse connection config
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PostgreSQL connection string: %w", err)
	}

	// Configure connection pool settings
	configureConnectionPool(poolConfig)

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL connection pool: %w", err)
	}

	tracker := &Tracker{
		pool:   pool,
		schema: schema,
	}

	// Initialize the tracker (create table if needed)
	if err := tracker.Initialize(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to initialize tracker: %w", err)
	}

	return tracker, nil
}

// Initialize creates the migration state tables
func (t *Tracker) Initialize(ctx interface{}) error {
	ctxVal := ctx.(context.Context)

	// Ensure schema exists
	if t.schema != "" && t.schema != "public" {
		schemaQuery := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(t.schema))
		if _, err := t.pool.Exec(ctxVal, schemaQuery); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	// Create migrations_list table
	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	createListTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			migration_id VARCHAR(255) PRIMARY KEY,
			schema VARCHAR(255) NOT NULL,
			version VARCHAR(50) NOT NULL,
			name VARCHAR(255) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			backend VARCHAR(50) NOT NULL,
			up_sql VARCHAR(255),
			down_sql VARCHAR(255),
			dependencies TEXT[],
			structured_dependencies JSONB,
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, listTableName)

	if _, err := t.pool.Exec(ctxVal, createListTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_list table: %w", err)
	}

	// Create indexes for migrations_list
	// Note: migration_id is PRIMARY KEY so already indexed, but explicit index is kept for consistency
	// All tables with migration_id column must have an index on it for performance and foreign key constraints
	indexSQL1 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_migration_id ON %s (migration_id)", listTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL1)

	indexSQL2 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_connection_backend ON %s (connection, backend)", listTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL2)

	indexSQL3 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_status ON %s (status)", listTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL3)

	// Create migrations_history table
	historyTableName := "migrations_history"
	if t.schema != "" && t.schema != "public" {
		historyTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_history"))
	}

	createHistoryTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			migration_id VARCHAR(255) NOT NULL,
			schema VARCHAR(255) NOT NULL,
			version VARCHAR(50) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			backend VARCHAR(50) NOT NULL,
			status VARCHAR(20) NOT NULL,
			error_message TEXT,
			executed_by VARCHAR(255),
			execution_method VARCHAR(20) NOT NULL DEFAULT 'api',
			execution_context TEXT,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (migration_id) REFERENCES %s(migration_id) ON DELETE CASCADE
		)
	`, historyTableName, listTableName)

	if _, err := t.pool.Exec(ctxVal, createHistoryTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_history table: %w", err)
	}

	// Create indexes for migrations_history
	// Index on migration_id is required for foreign key performance and to avoid using migration names that don't exist in migrations_list
	indexSQL4 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_migration_id ON %s (migration_id)", historyTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL4)

	indexSQL5 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_applied_at ON %s (applied_at DESC)", historyTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL5)

	indexSQL6 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_status ON %s (status)", historyTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL6)

	// Create migrations_executions table
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	createExecutionsTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			migration_id VARCHAR(255) NOT NULL,
			schema VARCHAR(255) NOT NULL,
			version VARCHAR(50) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			backend VARCHAR(50) NOT NULL,
			status VARCHAR(20) NOT NULL,
			applied BOOLEAN NOT NULL DEFAULT FALSE,
			applied_at TIMESTAMP,
			actions TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (migration_id) REFERENCES %s(migration_id) ON DELETE CASCADE,
			PRIMARY KEY (migration_id, schema, version, connection, backend)
		)
	`, executionsTableName, listTableName)

	if _, err := t.pool.Exec(ctxVal, createExecutionsTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_executions table: %w", err)
	}

	// Migrate existing schema if needed (handle databases with old id column)
	// Try to drop id column if it exists (for existing databases)
	// This is safe because CREATE TABLE IF NOT EXISTS won't recreate the column
	dropIDColumnSQL := fmt.Sprintf(`
		ALTER TABLE %s DROP COLUMN IF EXISTS id
	`, executionsTableName)
	_, _ = t.pool.Exec(ctxVal, dropIDColumnSQL)

	// Drop old unique constraint if it exists (will be replaced by PRIMARY KEY)
	dropUniqueSQL := fmt.Sprintf(`
		ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s_migration_id_schema_version_connection_backend_key
	`, executionsTableName, executionsTableName)
	_, _ = t.pool.Exec(ctxVal, dropUniqueSQL)

	// Ensure composite primary key exists (CREATE TABLE already created it, but this handles existing tables)
	// First check if a primary key on these columns already exists
	// Use oid to check for primary key constraint
	var schemaNameForCheck string
	if t.schema != "" && t.schema != "public" {
		schemaNameForCheck = t.schema
	} else {
		schemaNameForCheck = "public"
	}
	checkPKSQL := `
		SELECT COUNT(*) FROM pg_constraint c
		JOIN pg_class t ON c.conrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		WHERE n.nspname = $1
		AND t.relname = $2
		AND c.contype = 'p'
		AND array_length(c.conkey, 1) = 5
	`
	var pkCount int
	if err := t.pool.QueryRow(ctxVal, checkPKSQL, schemaNameForCheck, "migrations_executions").Scan(&pkCount); err == nil && pkCount == 0 {
		// Drop any existing primary key first
		dropOldPKSQL := fmt.Sprintf(`
			ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s_pkey
		`, executionsTableName, executionsTableName)
		_, _ = t.pool.Exec(ctxVal, dropOldPKSQL)

		// Create composite primary key
		createPKSQL := fmt.Sprintf(`
			ALTER TABLE %s ADD PRIMARY KEY (migration_id, schema, version, connection, backend)
		`, executionsTableName)
		if _, err := t.pool.Exec(ctxVal, createPKSQL); err != nil {
			// Log warning but don't fail - table might already have the constraint
			fmt.Printf("Note: Could not create composite primary key on %s (may already exist): %v\n", executionsTableName, err)
		}
	}

	// Create indexes for migrations_executions
	// Index on migration_id is required for foreign key performance and to avoid using migration names that don't exist in migrations_list
	indexSQL7 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_executions_migration_id ON %s (migration_id)", executionsTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL7)

	indexSQL8 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_executions_status ON %s (status)", executionsTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL8)

	indexSQL9 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_executions_created_at ON %s (created_at DESC)", executionsTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL9)

	// Ensure foreign key constraint exists on migrations_executions.migration_id
	// This constraint prevents invalid migration IDs from being inserted
	var fkCount int
	checkFKSQL := `
		SELECT COUNT(*) FROM pg_constraint c
		JOIN pg_class t ON c.conrelid = t.oid
		JOIN pg_namespace n ON t.relnamespace = n.oid
		JOIN pg_class r ON c.confrelid = r.oid
		WHERE n.nspname = $1
		AND t.relname = $2
		AND r.relname = $3
		AND c.contype = 'f'
		AND c.conname LIKE '%migration_id%'
	`
	if err := t.pool.QueryRow(ctxVal, checkFKSQL, schemaNameForCheck, "migrations_executions", "migrations_list").Scan(&fkCount); err == nil && fkCount == 0 {
		// Foreign key constraint doesn't exist, create it
		createFKSQL := fmt.Sprintf(`
			ALTER TABLE %s
			ADD CONSTRAINT migrations_executions_migration_id_fkey
			FOREIGN KEY (migration_id) REFERENCES %s(migration_id) ON DELETE CASCADE
		`, executionsTableName, listTableName)
		if _, err := t.pool.Exec(ctxVal, createFKSQL); err != nil {
			// Log warning but don't fail - constraint might already exist with different name
			fmt.Printf("Note: Could not create foreign key constraint on %s (may already exist): %v\n", executionsTableName, err)
		} else {
			fmt.Printf("✓ Foreign key constraint created on %s.migration_id -> %s.migration_id\n", executionsTableName, listTableName)
		}
	}

	// Create migrations_dependencies table
	dependenciesTableName := "migrations_dependencies"
	if t.schema != "" && t.schema != "public" {
		dependenciesTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_dependencies"))
	}

	createDependenciesTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			migration_id VARCHAR(255) NOT NULL,
			dependency_id VARCHAR(255) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			schema TEXT[] NOT NULL,
			target VARCHAR(255) NOT NULL,
			target_type VARCHAR(20) NOT NULL DEFAULT 'name',
			requires_table VARCHAR(255),
			requires_schema VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (migration_id) REFERENCES %s(migration_id) ON DELETE CASCADE,
			FOREIGN KEY (dependency_id) REFERENCES %s(migration_id) ON DELETE CASCADE
		)
	`, dependenciesTableName, listTableName, listTableName)

	if _, err := t.pool.Exec(ctxVal, createDependenciesTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_dependencies table: %w", err)
	}

	// Create indexes for migrations_dependencies
	// Index on migration_id is required for foreign key performance and to avoid using migration names that don't exist in migrations_list
	indexSQL10 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_dependencies_migration_id ON %s (migration_id)", dependenciesTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL10)

	indexSQL11 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_dependencies_dependency_id ON %s (dependency_id)", dependenciesTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL11)

	// Create migrations_skipped table
	skippedTableName := "migrations_skipped"
	if t.schema != "" && t.schema != "public" {
		skippedTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_skipped"))
	}

	createSkippedTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			migration_id VARCHAR(255) NOT NULL,
			schema VARCHAR(255) NOT NULL,
			version VARCHAR(50) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			backend VARCHAR(50) NOT NULL,
			executed_by VARCHAR(255),
			execution_method VARCHAR(20) NOT NULL DEFAULT 'api',
			execution_context TEXT,
			skipped_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (migration_id) REFERENCES %s(migration_id) ON DELETE CASCADE
		)
	`, skippedTableName, listTableName)

	if _, err := t.pool.Exec(ctxVal, createSkippedTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_skipped table: %w", err)
	}

	// Create indexes for migrations_skipped
	indexSQL12 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_skipped_migration_id ON %s (migration_id)", skippedTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL12)

	indexSQL13 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_skipped_skipped_at ON %s (skipped_at DESC)", skippedTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL13)

	indexSQL14 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_skipped_connection_backend ON %s (connection, backend)", skippedTableName)
	_, _ = t.pool.Exec(ctxVal, indexSQL14)

	// Migrate existing data from old tables if they exist
	executionsTableNameForMigration := executionsTableName
	dependenciesTableNameForMigration := dependenciesTableName
	if err := t.migrateExistingData(ctxVal, listTableName, historyTableName, executionsTableNameForMigration, dependenciesTableNameForMigration); err != nil {
		// Log warning but don't fail initialization
		fmt.Printf("Warning: Failed to migrate existing data: %v\n", err)
	}

	return nil
}

// extractBaseMigrationID removes prefixes (organization ID, schema, etc.) to get base migration_id
// Migration ID can have multiple prefixes: {org_id}_{schema}_{version}_{name}_{backend}_{connection}
// Base format: {version}_{name}_{backend}_{connection}
// Version is typically 14 digits (YYYYMMDDHHMMSS), so we keep removing prefixes until we find a version
func extractBaseMigrationID(migrationID string) string {
	// Remove rollback suffix if present
	id := migrationID
	if strings.Contains(id, "_rollback") {
		id = strings.TrimSuffix(id, "_rollback")
	}

	parts := strings.Split(id, "_")
	if len(parts) < 4 {
		// Not enough parts, return as-is
		return id
	}

	// Find the first part that looks like a version (14 digits)
	// Keep removing prefixes until we find a version
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		// Check if this part is a version (14 digits, YYYYMMDDHHMMSS)
		if len(part) == 14 {
			allDigits := true
			for _, r := range part {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				// Found the version, this is the start of the base migration ID
				return strings.Join(parts[i:], "_")
			}
		}
	}

	// If no version found, return original (might be a legacy format)
	return id
}

// RecordMigration records a migration execution
func (t *Tracker) RecordMigration(ctx interface{}, migration *state.MigrationRecord) error {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	historyTableName := "migrations_history"
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
		historyTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_history"))
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	appliedAt := time.Now()
	if migration.AppliedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, migration.AppliedAt); err == nil {
			appliedAt = parsed
		}
	}

	// Extract base migration_id
	// Migration ID can be in formats:
	// - Base: {version}_{name}_{backend}_{connection}
	// - Schema-specific: {schema}_{version}_{name}_{backend}_{connection}
	// - With organization/tenant prefix: {org_id}_{schema}_{version}_{name}_{backend}_{connection}
	// - With rollback suffix: ..._rollback
	// migrations_list should always use the base ID (without prefixes)
	migrationID := migration.MigrationID
	isRollback := strings.Contains(migrationID, "_rollback")
	baseMigrationID := extractBaseMigrationID(migrationID)

	executedBy := migration.ExecutedBy
	if executedBy == "" {
		executedBy = "system"
	}
	executionMethod := migration.ExecutionMethod
	if executionMethod == "" {
		executionMethod = "api"
	}

	// Map status values
	status := migration.Status
	if status == "success" {
		status = "applied"
	}

	// Extract connection type from execution context for logging
	connectionType := "unknown"
	if migration.ExecutionContext != "" {
		var execCtx map[string]interface{}
		if err := json.Unmarshal([]byte(migration.ExecutionContext), &execCtx); err == nil {
			if ct, ok := execCtx["connection_type"].(string); ok && ct != "" {
				connectionType = ct
			}
		}
	}

	// Log migration execution with connection type
	logger.Infof("Recording migration: id=%s, status=%s, connection=%s, backend=%s, connection_type=%s, execution_method=%s",
		baseMigrationID, status, migration.Connection, migration.Backend, connectionType, executionMethod)

	// Convert schema to array
	schemas := []string{}
	if migration.Schema != "" {
		schemas = []string{migration.Schema}
	}

	// Debug logging for schema-specific migrations
	logger.Debug("RecordMigration: migrationID=%s, baseMigrationID=%s, migration.Schema=%s, schemas=%v, status=%s",
		migration.MigrationID, baseMigrationID, migration.Schema, schemas, status)

	// Ensure migration exists in migrations_list before inserting history
	// Use INSERT ... ON CONFLICT DO UPDATE to create if missing, update if exists
	// This ensures the foreign key constraint is satisfied before inserting into migrations_history
	listStatus := status
	if isRollback {
		listStatus = "rolled_back"
	}
	if status == "success" {
		listStatus = "applied"
	}

	// Extract name from baseMigrationID if needed
	// Format: {version}_{name}_{backend}_{connection}
	migrationName := ""
	baseParts := strings.Split(baseMigrationID, "_")
	if len(baseParts) >= 4 {
		// Version is first part, backend is second-to-last, connection is last
		// Name is everything in between
		migrationName = strings.Join(baseParts[1:len(baseParts)-2], "_")
	}

	// Use empty string for schema if not provided (migrations_list allows empty schema)
	schemaValue := ""
	if len(schemas) > 0 {
		schemaValue = schemas[0]
	}

	// Only update status if it's not already 'applied' to prevent overwriting successful migrations
	// Reference the existing row using the table name in the CASE expression
	upsertListSQL := fmt.Sprintf(`
		INSERT INTO %s AS ml (migration_id, schema, version, name, connection, backend, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (migration_id) DO UPDATE SET
			status = CASE
				WHEN ml.status = 'applied' THEN ml.status
				ELSE EXCLUDED.status
			END,
			updated_at = CURRENT_TIMESTAMP
	`, listTableName)

	_, err := t.pool.Exec(ctxVal, upsertListSQL,
		baseMigrationID, schemaValue, migration.Version, migrationName,
		migration.Connection, migration.Backend, listStatus)
	if err != nil {
		return fmt.Errorf("failed to upsert migration in migrations_list: %w", err)
	}

	// Always record history, even if schema is empty
	// Insert one record per schema into migrations_history
	insertHistorySQL := fmt.Sprintf(`
		INSERT INTO %s (migration_id, schema, version, connection, backend,
		                status, error_message, executed_by, execution_method, execution_context, applied_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, historyTableName)

	// If no schemas specified, use empty string to record history anyway
	historySchemas := schemas
	if len(historySchemas) == 0 {
		historySchemas = []string{""}
	}

	// Insert history for each schema (or empty string if no schema)
	for _, schema := range historySchemas {
		var historyID int
		logger.Debug("RecordMigration: Inserting into migrations_history: migration_id=%s, schema=%s, version=%s, connection=%s, backend=%s, status=%s",
			baseMigrationID, schema, migration.Version, migration.Connection, migration.Backend, status)
		err = t.pool.QueryRow(ctxVal, insertHistorySQL,
			baseMigrationID, schema, migration.Version,
			migration.Connection, migration.Backend, status, migration.ErrorMessage,
			executedBy, executionMethod, migration.ExecutionContext, appliedAt, appliedAt).Scan(&historyID)
		if err != nil {
			logger.Errorf("RecordMigration: Failed to insert into migrations_history: migration_id=%s, schema=%s, error=%v",
				baseMigrationID, schema, err)
			return fmt.Errorf("failed to insert into migrations_history: %w", err)
		}
		logger.Debug("RecordMigration: Successfully inserted into migrations_history: id=%d, migration_id=%s, schema=%s",
			historyID, baseMigrationID, schema)
	}

	// Skip migrations_executions if no schemas specified (this table requires schema)
	if len(schemas) == 0 {
		return nil
	}

	// Insert one record per schema into migrations_executions
	applied := status == "applied"
	var appliedAtPtr *time.Time
	if applied {
		appliedAtPtr = &appliedAt
	}

	execStatus := "pending"
	if applied {
		execStatus = "applied"
	} else if status == "failed" {
		execStatus = "failed"
	}

	// Validate that baseMigrationID exists in migrations_list before inserting into migrations_executions
	// This ensures the foreign key constraint is satisfied and provides clear error messages
	checkExistsSQL := fmt.Sprintf(`
		SELECT COUNT(*) FROM %s WHERE migration_id = $1
	`, listTableName)
	var count int
	err = t.pool.QueryRow(ctxVal, checkExistsSQL, baseMigrationID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to validate migration_id %s: %w", baseMigrationID, err)
	}
	if count == 0 {
		// This should not happen since we upsert into migrations_list above, but log as warning
		fmt.Fprintf(os.Stderr, "⚠️  WARNING: Migration ID '%s' (extracted from '%s') does not exist in migrations_list. This may indicate an invalid migration ID format.\n", baseMigrationID, migration.MigrationID)
		// Try to upsert again to ensure it exists
		_, err = t.pool.Exec(ctxVal, upsertListSQL,
			baseMigrationID, schemaValue, migration.Version, migrationName,
			migration.Connection, migration.Backend, listStatus)
		if err != nil {
			return fmt.Errorf("failed to upsert migration in migrations_list (retry): %w", err)
		}
	}

	insertExecutionSQL := fmt.Sprintf(`
		INSERT INTO %s (migration_id, schema, version, connection, backend, status, applied, applied_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (migration_id, schema, version, connection, backend) DO UPDATE SET
			status = EXCLUDED.status,
			applied = EXCLUDED.applied,
			applied_at = EXCLUDED.applied_at,
			updated_at = CURRENT_TIMESTAMP
	`, executionsTableName)

	// Create one record per schema
	for _, schema := range schemas {
		// Insert into migrations_executions with foreign key validation
		logger.Debug("RecordMigration: Upserting into migrations_executions: migration_id=%s, schema=%s, version=%s, connection=%s, backend=%s, status=%s, applied=%v",
			baseMigrationID, schema, migration.Version, migration.Connection, migration.Backend, execStatus, applied)
		_, err = t.pool.Exec(ctxVal, insertExecutionSQL,
			baseMigrationID, schema, migration.Version,
			migration.Connection, migration.Backend, execStatus, applied, appliedAtPtr)
		if err != nil {
			// Check if this is a foreign key violation
			errStr := err.Error()
			if strings.Contains(errStr, "foreign key") || strings.Contains(errStr, "violates foreign key constraint") {
				fmt.Fprintf(os.Stderr, "❌ ERROR: Foreign key violation detected!\n")
				fmt.Fprintf(os.Stderr, "   Migration ID: %s (extracted from: %s)\n", baseMigrationID, migration.MigrationID)
				fmt.Fprintf(os.Stderr, "   Schema: %s, Version: %s, Connection: %s, Backend: %s\n", schema, migration.Version, migration.Connection, migration.Backend)
				fmt.Fprintf(os.Stderr, "   This migration_id does not exist in migrations_list table.\n")
				fmt.Fprintf(os.Stderr, "   Please ensure the migration is registered in migrations_list first.\n")
				return fmt.Errorf("foreign key violation: migration_id '%s' does not exist in migrations_list: %w", baseMigrationID, err)
			}
			return fmt.Errorf("failed to insert into migrations_executions: %w", err)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to insert into migrations_executions: %w", err)
	}

	return nil
}

// RecordDependencyMigration records a dependency migration as applied without creating history entries.
// Requirement: Dependencies should only be recorded in the execution history of the migration that depends on them.
// This method marks the dependency as applied in migrations_list and migrations_executions but skips migrations_history.
func (t *Tracker) RecordDependencyMigration(ctx interface{}, migration *state.MigrationRecord) error {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	appliedAt := time.Now()
	if migration.AppliedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, migration.AppliedAt); err == nil {
			appliedAt = parsed
		}
	}

	migrationID := migration.MigrationID
	baseMigrationID := extractBaseMigrationID(migrationID)

	// Map status values
	status := migration.Status
	if status == "success" {
		status = "applied"
	}

	listStatus := status

	// Extract name from baseMigrationID
	migrationName := ""
	baseParts := strings.Split(baseMigrationID, "_")
	if len(baseParts) >= 4 {
		migrationName = strings.Join(baseParts[1:len(baseParts)-2], "_")
	}

	// Convert schema to array
	schemas := []string{}
	if migration.Schema != "" {
		schemas = []string{migration.Schema}
	}

	schemaValue := ""
	if len(schemas) > 0 {
		schemaValue = schemas[0]
	}

	// Update migrations_list to mark dependency as applied (but don't create history)
	upsertListSQL := fmt.Sprintf(`
		INSERT INTO %s AS ml (migration_id, schema, version, name, connection, backend, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (migration_id) DO UPDATE SET
			status = CASE
				WHEN ml.status = 'applied' THEN ml.status
				ELSE EXCLUDED.status
			END,
			updated_at = CURRENT_TIMESTAMP
	`, listTableName)

	_, err := t.pool.Exec(ctxVal, upsertListSQL,
		baseMigrationID, schemaValue, migration.Version, migrationName,
		migration.Connection, migration.Backend, listStatus)
	if err != nil {
		return fmt.Errorf("failed to upsert dependency migration in migrations_list: %w", err)
	}

	// Update migrations_executions (but skip migrations_history - requirement 4)
	if len(schemas) == 0 {
		return nil
	}

	applied := status == "applied"
	var appliedAtPtr *time.Time
	if applied {
		appliedAtPtr = &appliedAt
	}

	execStatus := "pending"
	if applied {
		execStatus = "applied"
	} else if status == "failed" {
		execStatus = "failed"
	}

	insertExecutionSQL := fmt.Sprintf(`
		INSERT INTO %s (migration_id, schema, version, connection, backend, status, applied, applied_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (migration_id, schema, version, connection, backend) DO UPDATE SET
			status = EXCLUDED.status,
			applied = EXCLUDED.applied,
			applied_at = EXCLUDED.applied_at,
			updated_at = CURRENT_TIMESTAMP
	`, executionsTableName)

	for _, schema := range schemas {
		_, err = t.pool.Exec(ctxVal, insertExecutionSQL,
			baseMigrationID, schema, migration.Version,
			migration.Connection, migration.Backend, execStatus, applied, appliedAtPtr)
		if err != nil {
			return fmt.Errorf("failed to insert dependency execution state for %s: %w", baseMigrationID, err)
		}
	}

	logger.Debug("Recorded dependency migration %s as applied (no history entry created)", baseMigrationID)
	return nil
}

// GetMigrationHistory retrieves migration history with optional filters
func (t *Tracker) GetMigrationHistory(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	ctxVal := ctx.(context.Context)

	historyTableName := "migrations_history"
	if t.schema != "" && t.schema != "public" {
		historyTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_history"))
	}

	query := fmt.Sprintf(`
		SELECT id, migration_id, schema, version, connection, backend,
		       applied_at, status, error_message, executed_by, execution_method, execution_context
		FROM %s WHERE 1=1
	`, historyTableName)

	args := []interface{}{}
	argIndex := 1

	if filters != nil {
		if filters.Schema != "" {
			// For VARCHAR schema column, check if schema is in comma-separated string
			// Match exact schema or schema in comma-separated list
			query += fmt.Sprintf(" AND (schema = $%d OR schema LIKE $%d || ',%%' OR schema LIKE '%%,' || $%d || ',%%' OR schema LIKE '%%,' || $%d)", argIndex, argIndex, argIndex, argIndex)
			args = append(args, filters.Schema)
			argIndex++
		}
		if filters.Connection != "" {
			query += fmt.Sprintf(" AND connection = $%d", argIndex)
			args = append(args, filters.Connection)
			argIndex++
		}
		if filters.Backend != "" {
			query += fmt.Sprintf(" AND backend = $%d", argIndex)
			args = append(args, filters.Backend)
			argIndex++
		}
		if filters.Status != "" {
			query += fmt.Sprintf(" AND status = $%d", argIndex)
			args = append(args, filters.Status)
			argIndex++
		}
		if filters.Version != "" {
			query += fmt.Sprintf(" AND version = $%d", argIndex)
			args = append(args, filters.Version)
		}
	}

	query += " ORDER BY applied_at DESC, id DESC"

	rows, err := t.pool.Query(ctxVal, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	var records []*state.MigrationRecord
	for rows.Next() {
		var record state.MigrationRecord
		var appliedAt time.Time
		var id int

		err := rows.Scan(
			&id,
			&record.MigrationID,
			&record.Schema,
			&record.Version,
			&record.Connection,
			&record.Backend,
			&appliedAt,
			&record.Status,
			&record.ErrorMessage,
			&record.ExecutedBy,
			&record.ExecutionMethod,
			&record.ExecutionContext,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}

		record.ID = fmt.Sprintf("%d", id)
		record.AppliedAt = appliedAt.Format(time.RFC3339)
		records = append(records, &record)
	}

	return records, rows.Err()
}

// GetMigrationList retrieves the list of migrations with their last status
func (t *Tracker) GetMigrationList(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	query := fmt.Sprintf(`
		SELECT migration_id, schema, version, name, connection, backend,
		       status, created_at, updated_at
		FROM %s WHERE 1=1
	`, listTableName)

	args := []interface{}{}
	argIndex := 1

	if filters != nil {
		if filters.Schema != "" {
			// For VARCHAR schema column, check if schema is in comma-separated string
			// Match exact schema or schema in comma-separated list
			query += fmt.Sprintf(" AND (schema = $%d OR schema LIKE $%d || ',%%' OR schema LIKE '%%,' || $%d || ',%%' OR schema LIKE '%%,' || $%d)", argIndex, argIndex, argIndex, argIndex)
			args = append(args, filters.Schema)
			argIndex++
		}
		if filters.Connection != "" {
			query += fmt.Sprintf(" AND connection = $%d", argIndex)
			args = append(args, filters.Connection)
			argIndex++
		}
		if filters.Backend != "" {
			query += fmt.Sprintf(" AND backend = $%d", argIndex)
			args = append(args, filters.Backend)
			argIndex++
		}
		if filters.Status != "" {
			query += fmt.Sprintf(" AND status = $%d", argIndex)
			args = append(args, filters.Status)
			argIndex++
		}
		if filters.Version != "" {
			query += fmt.Sprintf(" AND version = $%d", argIndex)
			args = append(args, filters.Version)
		}
	}

	rows, err := t.pool.Query(ctxVal, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations list: %w", err)
	}
	defer rows.Close()

	var items []*state.MigrationListItem
	for rows.Next() {
		var item state.MigrationListItem
		var createdAt, updatedAt *time.Time

		err := rows.Scan(
			&item.MigrationID,
			&item.Schema,
			&item.Version,
			&item.Name,
			&item.Connection,
			&item.Backend,
			&item.LastStatus,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration list item: %w", err)
		}

		// Map status values for compatibility
		if item.LastStatus == "applied" {
			item.Applied = true
		} else {
			item.Applied = false
		}

		// Use updated_at as last_applied_at if status is applied
		if item.Applied && updatedAt != nil {
			item.LastAppliedAt = updatedAt.Format(time.RFC3339)
		}

		items = append(items, &item)
	}

	return items, rows.Err()
}

// GetMigrationDetail retrieves detailed information about a single migration from migrations_list
func (t *Tracker) GetMigrationDetail(ctx interface{}, migrationID string) (*state.MigrationDetail, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	// Remove prefixes to get base migration_id
	baseMigrationID := extractBaseMigrationID(migrationID)

	query := fmt.Sprintf(`
		SELECT migration_id, schema, version, name, connection, backend,
		       up_sql, down_sql, dependencies, structured_dependencies, status, created_at, updated_at
		FROM %s WHERE migration_id = $1
	`, listTableName)

	var detail state.MigrationDetail
	var schemaStr *string
	var upSQL, downSQL *string
	var dependencies []string
	var structuredDepsJSON *string
	var createdAt, updatedAt *time.Time

	err := t.pool.QueryRow(ctxVal, query, baseMigrationID).Scan(
		&detail.MigrationID,
		&schemaStr,
		&detail.Version,
		&detail.Name,
		&detail.Connection,
		&detail.Backend,
		&upSQL,
		&downSQL,
		&dependencies,
		&structuredDepsJSON,
		&detail.Status,
		&createdAt,
		&updatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query migration detail: %w", err)
	}

	if schemaStr != nil {
		detail.Schema = *schemaStr
	}
	if upSQL != nil {
		detail.UpSQL = *upSQL
	}
	if downSQL != nil {
		detail.DownSQL = *downSQL
	}
	if dependencies != nil {
		detail.Dependencies = dependencies
	}
	if structuredDepsJSON != nil && *structuredDepsJSON != "" {
		var structuredDeps []backends.Dependency
		if err := json.Unmarshal([]byte(*structuredDepsJSON), &structuredDeps); err == nil {
			detail.StructuredDependencies = structuredDeps
		}
	}

	return &detail, nil
}

// GetMigrationExecutions retrieves all execution records for a migration, ordered by created_at DESC
func (t *Tracker) GetMigrationExecutions(ctx interface{}, migrationID string) ([]*state.MigrationExecution, error) {
	ctxVal := ctx.(context.Context)

	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	// Remove prefixes to get base migration_id
	baseMigrationID := extractBaseMigrationID(migrationID)

	query := fmt.Sprintf(`
		SELECT migration_id, schema, version, connection, backend,
		       status, applied, applied_at, created_at, updated_at
		FROM %s WHERE migration_id = $1
		ORDER BY created_at DESC
	`, executionsTableName)

	rows, err := t.pool.Query(ctxVal, query, baseMigrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query migration executions: %w", err)
	}
	defer rows.Close()

	var executions []*state.MigrationExecution
	for rows.Next() {
		var exec state.MigrationExecution
		var schemaStr *string
		var appliedAt, createdAt, updatedAt *time.Time

		err := rows.Scan(
			&exec.MigrationID,
			&schemaStr,
			&exec.Version,
			&exec.Connection,
			&exec.Backend,
			&exec.Status,
			&exec.Applied,
			&appliedAt,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration execution: %w", err)
		}

		if schemaStr != nil {
			exec.Schema = *schemaStr
		}
		if appliedAt != nil {
			exec.AppliedAt = appliedAt.Format(time.RFC3339)
		}
		if createdAt != nil {
			exec.CreatedAt = createdAt.Format(time.RFC3339)
		}
		if updatedAt != nil {
			exec.UpdatedAt = updatedAt.Format(time.RFC3339)
		}

		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// GetRecentExecutions retrieves recent execution records across all migrations, ordered by created_at DESC
func (t *Tracker) GetRecentExecutions(ctx interface{}, limit int) ([]*state.MigrationExecution, error) {
	ctxVal := ctx.(context.Context)

	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	query := fmt.Sprintf(`
		SELECT migration_id, schema, version, connection, backend,
		       status, applied, applied_at, created_at, updated_at
		FROM %s
		ORDER BY created_at DESC
		LIMIT $1
	`, executionsTableName)

	rows, err := t.pool.Query(ctxVal, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent executions: %w", err)
	}
	defer rows.Close()

	var executions []*state.MigrationExecution
	for rows.Next() {
		var exec state.MigrationExecution
		var schemaStr *string
		var appliedAt, createdAt, updatedAt *time.Time

		err := rows.Scan(
			&exec.MigrationID,
			&schemaStr,
			&exec.Version,
			&exec.Connection,
			&exec.Backend,
			&exec.Status,
			&exec.Applied,
			&appliedAt,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration execution: %w", err)
		}

		if schemaStr != nil {
			exec.Schema = *schemaStr
		}
		if appliedAt != nil {
			exec.AppliedAt = appliedAt.Format(time.RFC3339)
		}
		if createdAt != nil {
			exec.CreatedAt = createdAt.Format(time.RFC3339)
		}
		if updatedAt != nil {
			exec.UpdatedAt = updatedAt.Format(time.RFC3339)
		}

		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// RecordSkippedMigrations records skipped migrations for a given execution context
func (t *Tracker) RecordSkippedMigrations(ctx interface{}, skippedMigrationIDs []string, executedBy, executionMethod, executionContext string) error {
	if len(skippedMigrationIDs) == 0 {
		return nil
	}

	ctxVal := ctx.(context.Context)

	skippedTableName := "migrations_skipped"
	if t.schema != "" && t.schema != "public" {
		skippedTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_skipped"))
	}

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	// Get migration details for each skipped migration ID
	// We need to extract schema, version, connection, backend from migrations_list
	for _, migrationID := range skippedMigrationIDs {
		// Extract base migration ID (remove schema prefix if present)
		// migrations_list stores base IDs, and migrations_skipped foreign key references base IDs
		baseMigrationID := extractBaseMigrationID(migrationID)

		// Extract schema from the original migrationID if it has a prefix
		var schema string
		if baseMigrationID != migrationID {
			// There was a prefix, extract it
			parts := strings.Split(migrationID, "_")
			baseParts := strings.Split(baseMigrationID, "_")
			if len(parts) > len(baseParts) {
				// The difference is the prefix, which could be organization ID + schema
				// For simplicity, use the first part as schema
				schema = parts[0]
			}
		}

		// Query migrations_list to get migration details using base migration ID
		query := fmt.Sprintf(`
			SELECT schema, version, connection, backend
			FROM %s
			WHERE migration_id = $1
		`, listTableName)

		var dbSchema, version, connection, backend string
		err := t.pool.QueryRow(ctxVal, query, baseMigrationID).Scan(&dbSchema, &version, &connection, &backend)
		if err != nil {
			// If migration not found in list, skip it (it might not be registered yet)
			// Log warning but continue with other migrations
			fmt.Printf("Warning: Skipped migration %s (base: %s) not found in migrations_list, skipping record\n", migrationID, baseMigrationID)
			continue
		}

		// Use schema from migrationID if available, otherwise use schema from migrations_list
		if schema == "" {
			schema = dbSchema
		}

		// Insert into migrations_skipped using base migration ID (required for foreign key)
		insertSQL := fmt.Sprintf(`
			INSERT INTO %s (migration_id, schema, version, connection, backend, executed_by, execution_method, execution_context, skipped_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, skippedTableName)

		_, err = t.pool.Exec(ctxVal, insertSQL,
			baseMigrationID, schema, version, connection, backend,
			executedBy, executionMethod, executionContext)
		if err != nil {
			// Log error but continue with other migrations
			fmt.Printf("Warning: Failed to record skipped migration %s: %v\n", migrationID, err)
			continue
		}

		// Keep only the 5 most recent records for this migration_id + schema combination
		// Delete records older than the 5th most recent
		// Use baseMigrationID for consistency with the insert above
		deleteSQL := fmt.Sprintf(`
			DELETE FROM %s
			WHERE migration_id = $1 AND schema = $2
			AND id NOT IN (
				SELECT id FROM %s
				WHERE migration_id = $1 AND schema = $2
				ORDER BY skipped_at DESC
				LIMIT 5
			)
		`, skippedTableName, skippedTableName)

		_, deleteErr := t.pool.Exec(ctxVal, deleteSQL, baseMigrationID, schema)
		if deleteErr != nil {
			// Log error but don't fail - this is cleanup
			fmt.Printf("Warning: Failed to cleanup old skipped migration records for %s (base: %s, schema: %s): %v\n", migrationID, baseMigrationID, schema, deleteErr)
		}
	}

	return nil
}

// GetSkippedMigrations retrieves skipped migrations, optionally filtered by migration_id or recent limit
func (t *Tracker) GetSkippedMigrations(ctx interface{}, migrationID string, limit int) ([]*state.SkippedMigration, error) {
	ctxVal := ctx.(context.Context)

	skippedTableName := "migrations_skipped"
	if t.schema != "" && t.schema != "public" {
		skippedTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_skipped"))
	}

	var query string
	var args []interface{}

	if migrationID != "" {
		query = fmt.Sprintf(`
			SELECT id, migration_id, schema, version, connection, backend,
			       executed_by, execution_method, execution_context, skipped_at, created_at
			FROM %s
			WHERE migration_id = $1
			ORDER BY skipped_at DESC
			LIMIT $2
		`, skippedTableName)
		args = []interface{}{migrationID, limit}
	} else {
		query = fmt.Sprintf(`
			SELECT id, migration_id, schema, version, connection, backend,
			       executed_by, execution_method, execution_context, skipped_at, created_at
			FROM %s
			ORDER BY skipped_at DESC
			LIMIT $1
		`, skippedTableName)
		args = []interface{}{limit}
	}

	rows, err := t.pool.Query(ctxVal, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query skipped migrations: %w", err)
	}
	defer rows.Close()

	var skippedMigrations []*state.SkippedMigration
	for rows.Next() {
		var skipped state.SkippedMigration
		var executedBy, executionMethod, executionContext *string
		var skippedAt, createdAt *time.Time

		err := rows.Scan(
			&skipped.ID,
			&skipped.MigrationID,
			&skipped.Schema,
			&skipped.Version,
			&skipped.Connection,
			&skipped.Backend,
			&executedBy,
			&executionMethod,
			&executionContext,
			&skippedAt,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan skipped migration: %w", err)
		}

		if executedBy != nil {
			skipped.ExecutedBy = *executedBy
		}
		if executionMethod != nil {
			skipped.ExecutionMethod = *executionMethod
		}
		if executionContext != nil {
			skipped.ExecutionContext = *executionContext
		}
		if skippedAt != nil {
			skipped.SkippedAt = skippedAt.Format(time.RFC3339)
		}
		if createdAt != nil {
			skipped.CreatedAt = createdAt.Format(time.RFC3339)
		}

		skippedMigrations = append(skippedMigrations, &skipped)
	}

	return skippedMigrations, rows.Err()
}

// IsMigrationApplied checks if a migration has been successfully applied.
// This only returns true for migrations with status 'applied', not 'pending'.
// For concurrency control (checking if a migration is pending or applied),
// use IsMigrationPendingOrApplied instead.
func (t *Tracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	// Extract base migration_id and detect schema prefix
	baseMigrationID := extractBaseMigrationID(migrationID)
	var schemaName string
	// If baseMigrationID is different from migrationID, there was a prefix
	if baseMigrationID != migrationID {
		// Extract schema name from the prefix
		parts := strings.Split(migrationID, "_")
		baseParts := strings.Split(baseMigrationID, "_")
		if len(parts) > len(baseParts) {
			// The difference is the prefix, which could be organization ID + schema
			// For simplicity, use the first part as schema (could be org_id or schema)
			// This will be validated when querying migrations_executions
			schemaName = parts[0]
		}
	}

	// For dynamic schemas (with schema prefix), check migrations_executions table
	// This tracks per-schema executions and is more accurate for dynamic schemas
	// For dynamic schemas, we MUST check executions table per-schema and NOT fall back to migrations_list
	// because migrations_list tracks globally, not per-schema
	if schemaName != "" {
		// First, get version, connection, and backend from migrations_list
		// We need these to check all 5 fields in migrations_executions
		var version, connection, backend string
		getMetadataQuery := fmt.Sprintf(`
			SELECT version, connection, backend
			FROM %s
			WHERE migration_id = $1
			LIMIT 1
		`, listTableName)
		err := t.pool.QueryRow(ctxVal, getMetadataQuery, baseMigrationID).Scan(&version, &connection, &backend)
		if err != nil {
			// If migration not found in migrations_list, it's not applied
			if err == pgx.ErrNoRows {
				return false, nil
			}
			return false, fmt.Errorf("failed to get migration metadata: %w", err)
		}

		// Check migrations_executions with all 5 fields: migration_id, schema, version, connection, backend
		// CRITICAL: Only check for 'applied' status, not 'pending'
		// 'pending' means the migration hasn't executed yet, so it shouldn't be considered applied
		query := fmt.Sprintf(`
			SELECT EXISTS(
				SELECT 1 FROM %s
				WHERE migration_id = $1
				AND schema = $2
				AND version = $3
				AND connection = $4
				AND backend = $5
				AND status = 'applied'
			)`, executionsTableName)
		var exists bool
		err = t.pool.QueryRow(ctxVal, query, baseMigrationID, schemaName, version, connection, backend).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("failed to check migration status in executions table: %w", err)
		}
		// For dynamic schemas, only return the result from executions table
		// Do NOT fall back to migrations_list as it's not schema-specific
		return exists, nil
	}

	// For fixed-schema migrations (no schema prefix), check migrations_list table
	// This handles migrations with fixed schemas defined in the migration itself
	// CRITICAL: Only check for 'applied' status, not 'pending'
	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM %s
			WHERE migration_id IN ($1, $2)
			AND status = 'applied'
		)`, listTableName)
	var exists bool
	err := t.pool.QueryRow(ctxVal, query, migrationID, baseMigrationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check migration status: %w", err)
	}

	return exists, nil
}

// IsMigrationPendingOrApplied checks if a migration is pending or applied.
// For base migration IDs, migrations_list "pending" means registered-not-applied, not in-flight; this
// matches IsMigrationApplied (applied only). For schema-specific IDs, migrations_executions may hold
// status pending while a run is in progress.
func (t *Tracker) IsMigrationPendingOrApplied(ctx interface{}, migrationID string) (bool, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	// Extract base migration_id and detect schema prefix
	baseMigrationID := extractBaseMigrationID(migrationID)
	var schemaName string
	if baseMigrationID != migrationID {
		parts := strings.Split(migrationID, "_")
		baseParts := strings.Split(baseMigrationID, "_")
		if len(parts) > len(baseParts) {
			schemaName = parts[0]
		}
	}

	// Fixed-schema / base ID: list "pending" is not an execution lock; align with applied-only check.
	if schemaName == "" {
		return t.IsMigrationApplied(ctx, migrationID)
	}

	// Schema-specific: check migrations_executions for applied or in-flight pending.
	var version, connection, backend string
	getMetadataQuery := fmt.Sprintf(`
		SELECT version, connection, backend
		FROM %s
		WHERE migration_id = $1
		LIMIT 1
	`, listTableName)
	err := t.pool.QueryRow(ctxVal, getMetadataQuery, baseMigrationID).Scan(&version, &connection, &backend)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to get migration metadata: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM %s
			WHERE migration_id = $1
			AND schema = $2
			AND version = $3
			AND connection = $4
			AND backend = $5
			AND (status = 'applied' OR status = 'pending')
		)`, executionsTableName)
	var exists bool
	err = t.pool.QueryRow(ctxVal, query, baseMigrationID, schemaName, version, connection, backend).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check migration status in executions table: %w", err)
	}
	return exists, nil
}

// GetLastMigrationVersion gets the last applied version for a schema/table
func (t *Tracker) GetLastMigrationVersion(ctx interface{}, schema, table string) (string, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	query := fmt.Sprintf(`
		SELECT version
		FROM %s
		WHERE (schema = $1 OR schema LIKE $1 || ',%%' OR schema LIKE '%%,' || $1 || ',%%' OR schema LIKE '%%,' || $1) AND status = 'applied'
		ORDER BY version DESC
		LIMIT 1
	`, listTableName)

	var version string
	err := t.pool.QueryRow(ctxVal, query, schema).Scan(&version)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get last migration version: %w", err)
	}

	return version, nil
}

// RegisterScannedMigration registers a scanned migration in migrations_list (status: pending)
func (t *Tracker) RegisterScannedMigration(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = quoteIdentifier(t.schema) + "." + quoteIdentifier("migrations_list")
	}

	// migrations_list should always be inserted (even with empty schema) for dependency resolution
	// Use empty string if schema is not provided
	schemaValue := schema
	if schemaValue == "" {
		schemaValue = "" // Empty string is allowed for migrations_list
	}

	insertListSQL := `INSERT INTO ` + listTableName + ` (migration_id, schema, version, name, connection, backend, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (migration_id) DO NOTHING`

	now := time.Now()
	_, err := t.pool.Exec(ctxVal, insertListSQL,
		migrationID, schemaValue, version, name, connection, backend,
		"pending", now, now)
	if err != nil {
		return fmt.Errorf("failed to register scanned migration: %w", err)
	}

	return nil
}

// UpdateMigrationInfo updates migration metadata (schema, version, name, connection, backend) without affecting status/history
func (t *Tracker) UpdateMigrationInfo(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = quoteIdentifier(t.schema) + "." + quoteIdentifier("migrations_list")
	}

	// migrations_list should always be updated (even with empty schema) for dependency resolution
	// Use empty string if schema is not provided
	schemaValue := schema
	if schemaValue == "" {
		schemaValue = "" // Empty string is allowed for migrations_list
	}

	updateSQL := fmt.Sprintf(`
		UPDATE %s
		SET schema = $1,
		    version = $2,
		    name = $3,
		    connection = $4,
		    backend = $5,
		    updated_at = CURRENT_TIMESTAMP
		WHERE migration_id = $6
	`, listTableName)

	result, err := t.pool.Exec(ctxVal, updateSQL,
		schemaValue, version, name, connection, backend, migrationID)
	if err != nil {
		return fmt.Errorf("failed to update migration info: %w", err)
	}

	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("migration %s not found", migrationID)
	}

	return nil
}

// DeleteMigration deletes a migration from migrations_list (cascades to history via foreign key)
func (t *Tracker) DeleteMigration(ctx interface{}, migrationID string) error {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = quoteIdentifier(t.schema) + "." + quoteIdentifier("migrations_list")
	}

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE migration_id = $1", listTableName)
	_, err := t.pool.Exec(ctxVal, deleteSQL, migrationID)
	if err != nil {
		return fmt.Errorf("failed to delete migration: %w", err)
	}

	return nil
}

// getMigrationID generates a migration ID (same format as executor)
func (t *Tracker) getMigrationID(migration *backends.MigrationScript) string {
	return fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
}

// ReindexMigrations reloads the BfM migration list and updates the database state
// This should be called asynchronously in the background
func (t *Tracker) ReindexMigrations(ctx interface{}, registry interface{}) error {
	ctxVal := ctx.(context.Context)

	// Type assert registry to get GetAll method
	type Registry interface {
		GetAll() []*backends.MigrationScript
	}
	reg, ok := registry.(Registry)
	if !ok {
		return fmt.Errorf("registry does not implement GetAll() method")
	}

	listTableName := "migrations_list"
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	// Step 1: Get all migrations from BfM registry
	bfmMigrations := reg.GetAll()
	bfmMigrationMap := make(map[string]*backends.MigrationScript)

	for _, migration := range bfmMigrations {
		migrationID := t.getMigrationID(migration)
		bfmMigrationMap[migrationID] = migration
	}

	// Step 2: Get all migrations from database
	dbMigrations, err := t.GetMigrationList(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get database migrations: %w", err)
	}

	dbMigrationMap := make(map[string]*state.MigrationListItem)
	for _, migration := range dbMigrations {
		dbMigrationMap[migration.MigrationID] = migration
	}

	// Step 3: For each BfM migration, update or insert into migrations_list
	for migrationID, migration := range bfmMigrationMap {
		// Convert schema to array (handle single schema or multiple)
		schemas := []string{}
		if migration.Schema != "" {
			schemas = []string{migration.Schema}
		}

		// Convert dependencies to array
		dependencies := migration.Dependencies
		if dependencies == nil {
			dependencies = []string{}
		}

		// Convert structured dependencies to JSONB
		structuredDepsJSON, err := json.Marshal(migration.StructuredDependencies)
		if err != nil {
			return fmt.Errorf("failed to marshal structured dependencies: %w", err)
		}

		// Construct filenames for up_sql and down_sql
		// Filename pattern: {version}_{name}.up.{sql|json} and {version}_{name}.down.{sql|json}
		var upExt, downExt string
		if migration.Backend == "etcd" || migration.Backend == "mongodb" {
			upExt = ".up.json"
			downExt = ".down.json"
		} else {
			upExt = ".up.sql"
			downExt = ".down.sql"
		}
		upSQLFilename := fmt.Sprintf("%s_%s%s", migration.Version, migration.Name, upExt)
		downSQLFilename := fmt.Sprintf("%s_%s%s", migration.Version, migration.Name, downExt)

		// Check if migration exists in database
		dbMigration, exists := dbMigrationMap[migrationID]

		// Determine status based on execution state
		status := "pending"
		if exists {
			// Check if migration has been executed
			executed, err := t.IsMigrationApplied(ctx, migrationID)
			if err == nil && executed {
				status = "applied"
			} else if exists && dbMigration.LastStatus == "failed" {
				status = "failed"
			} else if exists && dbMigration.LastStatus == "rolled_back" {
				status = "rolled_back"
			} else if exists {
				// Map old status values
				if dbMigration.LastStatus == "success" {
					status = "applied"
				} else {
					status = dbMigration.LastStatus
				}
			}
		}

		// Upsert into migrations_list
		upsertSQL := fmt.Sprintf(`
			INSERT INTO %s (
				migration_id, schema, version, name, connection, backend,
				up_sql, down_sql, dependencies, structured_dependencies, status, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CURRENT_TIMESTAMP)
			ON CONFLICT (migration_id) DO UPDATE SET
				schema = EXCLUDED.schema,
				version = EXCLUDED.version,
				name = EXCLUDED.name,
				connection = EXCLUDED.connection,
				backend = EXCLUDED.backend,
				up_sql = EXCLUDED.up_sql,
				down_sql = EXCLUDED.down_sql,
				dependencies = EXCLUDED.dependencies,
				structured_dependencies = EXCLUDED.structured_dependencies,
				status = EXCLUDED.status,
				updated_at = CURRENT_TIMESTAMP
		`, listTableName)

		// migrations_list should always be inserted (even with empty schema) for dependency resolution
		// Use empty string if no schema is specified
		schemaValue := ""
		if len(schemas) > 0 {
			schemaValue = schemas[0]
		}

		// Insert/update migrations_list (always, even with empty schema)
		_, err = t.pool.Exec(ctxVal, upsertSQL,
			migrationID,
			schemaValue,
			migration.Version,
			migration.Name,
			migration.Connection,
			migration.Backend,
			upSQLFilename,
			downSQLFilename,
			dependencies,
			string(structuredDepsJSON),
			status,
		)
		if err != nil {
			return fmt.Errorf("failed to upsert migration %s: %w", migrationID, err)
		}

		// Skip migrations_executions if no schemas specified
		if len(schemas) == 0 {
			// Still update dependencies even if no schema
			if err := t.updateMigrationDependencies(ctxVal, migrationID, migration, listTableName); err != nil {
				return fmt.Errorf("failed to update dependencies for %s: %w", migrationID, err)
			}
			continue
		}

		// Insert into migrations_executions table - one record per schema
		applied := status == "applied"
		var appliedAt *time.Time
		if applied && exists && dbMigration.LastAppliedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, dbMigration.LastAppliedAt); err == nil {
				appliedAt = &parsed
			}
		}

		execStatus := "pending"
		if applied {
			execStatus = "applied"
		} else if status == "failed" {
			execStatus = "failed"
		}

		insertExecutionSQL := fmt.Sprintf(`
			INSERT INTO %s (migration_id, schema, version, connection, backend, status, applied, applied_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (migration_id, schema, version, connection, backend) DO UPDATE SET
				status = EXCLUDED.status,
				applied = EXCLUDED.applied,
				applied_at = EXCLUDED.applied_at,
				updated_at = CURRENT_TIMESTAMP
		`, executionsTableName)

		// Create one record per schema
		for _, schema := range schemas {
			_, err = t.pool.Exec(ctxVal, insertExecutionSQL,
				migrationID,
				schema,
				migration.Version,
				migration.Connection,
				migration.Backend,
				execStatus,
				applied,
				appliedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to insert execution state for %s: %w", migrationID, err)
			}
		}

		// Update dependencies table
		if err := t.updateMigrationDependencies(ctxVal, migrationID, migration, listTableName); err != nil {
			return fmt.Errorf("failed to update dependencies for %s: %w", migrationID, err)
		}
	}

	// Step 4: Delete migrations that no longer exist in BfM
	for migrationID := range dbMigrationMap {
		if _, exists := bfmMigrationMap[migrationID]; !exists {
			if err := t.DeleteMigration(ctx, migrationID); err != nil {
				// Log but continue
				fmt.Printf("Warning: Failed to delete migration %s: %v\n", migrationID, err)
			}
		}
	}

	return nil
}

// updateMigrationDependencies updates the migrations_dependencies table
func (t *Tracker) updateMigrationDependencies(ctx context.Context, migrationID string, migration *backends.MigrationScript, listTableName string) error {
	dependenciesTableName := "migrations_dependencies"
	if t.schema != "" && t.schema != "public" {
		dependenciesTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_dependencies"))
	}

	// Delete existing dependencies for this migration
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE migration_id = $1", dependenciesTableName)
	_, err := t.pool.Exec(ctx, deleteSQL, migrationID)
	if err != nil {
		return fmt.Errorf("failed to delete existing dependencies: %w", err)
	}

	// Insert structured dependencies
	for _, dep := range migration.StructuredDependencies {
		// Find dependency_id by resolving the dependency target
		dependencyID, err := t.resolveDependencyID(ctx, dep, listTableName)
		if err != nil {
			// Log but continue - dependency might be in a different connection/backend or not yet registered
			// This is expected for cross-connection dependencies or when dependencies haven't been scanned yet
			// The dependency will be resolved at execution time via the registry
			continue
		}

		schemas := []string{}
		if dep.Schema != "" {
			schemas = []string{dep.Schema}
		}

		insertSQL := fmt.Sprintf(`
			INSERT INTO %s (
				migration_id, dependency_id, connection, schema, target, target_type,
				requires_table, requires_schema
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, dependenciesTableName)

		targetType := dep.TargetType
		if targetType == "" {
			targetType = "name"
		}

		_, err = t.pool.Exec(ctx, insertSQL,
			migrationID,
			dependencyID,
			dep.Connection,
			schemas,
			dep.Target,
			targetType,
			dep.RequiresTable,
			dep.RequiresSchema,
		)
		if err != nil {
			return fmt.Errorf("failed to insert dependency: %w", err)
		}
	}

	// Insert simple dependencies (convert to structured format)
	for _, depName := range migration.Dependencies {
		// Find dependency_id by name
		dependencyID, err := t.findMigrationIDByName(ctx, depName, listTableName)
		if err != nil {
			// Skip if dependency not found - it might be in a different connection/backend or not yet registered
			// This is expected for cross-connection dependencies or when dependencies haven't been scanned yet
			// The dependency will be resolved at execution time via the registry
			continue
		}

		schemas := []string{}
		if migration.Schema != "" {
			schemas = []string{migration.Schema}
		}

		insertSQL := fmt.Sprintf(`
			INSERT INTO %s (
				migration_id, dependency_id, connection, schema, target, target_type
			)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, dependenciesTableName)

		_, err = t.pool.Exec(ctx, insertSQL,
			migrationID,
			dependencyID,
			migration.Connection,
			schemas,
			depName,
			"name",
		)
		if err != nil {
			return fmt.Errorf("failed to insert simple dependency: %w", err)
		}
	}

	return nil
}

// resolveDependencyID resolves a dependency to a migration_id
func (t *Tracker) resolveDependencyID(ctx context.Context, dep backends.Dependency, listTableName string) (string, error) {
	var query string
	var args []interface{}

	if dep.TargetType == "version" {
		query = fmt.Sprintf(`
			SELECT migration_id FROM %s
			WHERE connection = $1 AND version = $2
			LIMIT 1
		`, listTableName)
		args = []interface{}{dep.Connection, dep.Target}
	} else {
		query = fmt.Sprintf(`
			SELECT migration_id FROM %s
			WHERE connection = $1 AND name = $2
			LIMIT 1
		`, listTableName)
		args = []interface{}{dep.Connection, dep.Target}
	}

	var migrationID string
	err := t.pool.QueryRow(ctx, query, args...).Scan(&migrationID)
	if err != nil {
		return "", fmt.Errorf("dependency not found: %w", err)
	}

	return migrationID, nil
}

// findMigrationIDByName finds a migration_id by name
func (t *Tracker) findMigrationIDByName(ctx context.Context, name string, listTableName string) (string, error) {
	query := fmt.Sprintf(`
		SELECT migration_id FROM %s
		WHERE name = $1
		LIMIT 1
	`, listTableName)

	var migrationID string
	err := t.pool.QueryRow(ctx, query, name).Scan(&migrationID)
	if err != nil {
		return "", fmt.Errorf("migration not found: %w", err)
	}

	return migrationID, nil
}

// Close closes the database connection
func (t *Tracker) Close() error {
	if t.pool != nil {
		t.pool.Close()
		t.pool = nil
	}
	return nil
}

// migrateExistingData migrates data from old bfm_migrations table to new tables
func (t *Tracker) migrateExistingData(ctx context.Context, listTableName, historyTableName, executionsTableName, dependenciesTableName string) error {
	oldTableName := "bfm_migrations"
	if t.schema != "" && t.schema != "public" {
		oldTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("bfm_migrations"))
	}

	// Check if old table exists
	checkTableSQL := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = 'bfm_migrations'
		)
	`

	var tableExists bool
	schemaName := "public"
	if t.schema != "" && t.schema != "public" {
		schemaName = t.schema
	}

	err := t.pool.QueryRow(ctx, checkTableSQL, schemaName).Scan(&tableExists)
	if err != nil || !tableExists {
		// Old table doesn't exist, nothing to migrate
		return nil
	}

	// Get all records from old table
	query := fmt.Sprintf(`
		SELECT migration_id, schema, table_name, version, connection, backend,
		       applied_at, status, error_message
		FROM %s
		ORDER BY applied_at DESC
	`, oldTableName)

	rows, err := t.pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query old table: %w", err)
	}
	defer rows.Close()

	// Track which migrations we've seen to avoid duplicates in list table
	seenMigrations := make(map[string]bool)
	// Track migration records by base ID to determine last status
	migrationRecords := make(map[string][]struct {
		migrationID string
		schema      string
		tableName   string
		version     string
		connection  string
		backend     string
		status      string
		appliedAt   time.Time
		errorMsg    string
	})

	// Store all records first
	for rows.Next() {
		var migrationID, schema, tableName, version, connection, backend, status, errorMessage string
		var appliedAt time.Time

		err := rows.Scan(&migrationID, &schema, &tableName, &version, &connection, &backend, &appliedAt, &status, &errorMessage)
		if err != nil {
			continue
		}

		// Extract base migration_id (remove _rollback suffix and prefixes)
		baseMigrationID := extractBaseMigrationID(migrationID)

		// Store record for later processing
		if migrationRecords[baseMigrationID] == nil {
			migrationRecords[baseMigrationID] = []struct {
				migrationID string
				schema      string
				tableName   string
				version     string
				connection  string
				backend     string
				status      string
				appliedAt   time.Time
				errorMsg    string
			}{}
		}
		migrationRecords[baseMigrationID] = append(migrationRecords[baseMigrationID], struct {
			migrationID string
			schema      string
			tableName   string
			version     string
			connection  string
			backend     string
			status      string
			appliedAt   time.Time
			errorMsg    string
		}{migrationID, schema, tableName, version, connection, backend, status, appliedAt, errorMessage})
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading rows: %w", err)
	}

	// PHASE 1: Insert all migrations into migrations_list first (required for foreign key)
	for baseMigrationID, records := range migrationRecords {
		if seenMigrations[baseMigrationID] {
			continue
		}
		seenMigrations[baseMigrationID] = true

		// Find the most recent record
		var latestRecord struct {
			migrationID string
			schema      string
			tableName   string
			version     string
			connection  string
			backend     string
			status      string
			appliedAt   time.Time
			errorMsg    string
		}
		var latestSuccessRecord *struct {
			migrationID string
			schema      string
			tableName   string
			version     string
			connection  string
			backend     string
			status      string
			appliedAt   time.Time
			errorMsg    string
		}

		for _, record := range records {
			if record.appliedAt.After(latestRecord.appliedAt) {
				latestRecord = record
			}
			// Track most recent successful, non-rollback record
			if !strings.Contains(record.migrationID, "_rollback") && record.status == "success" {
				if latestSuccessRecord == nil || record.appliedAt.After(latestSuccessRecord.appliedAt) {
					latestSuccessRecord = &record
				}
			}
		}

		// Determine last status
		lastStatus := latestRecord.status
		lastAppliedAt := latestRecord.appliedAt

		// Map old status values to new ones
		if lastStatus == "success" {
			lastStatus = "applied"
		}

		// If latest is a rollback, check if there's a more recent success
		if strings.Contains(latestRecord.migrationID, "_rollback") {
			lastStatus = "rolled_back"
			if latestSuccessRecord != nil {
				lastAppliedAt = latestSuccessRecord.appliedAt
			}
		}

		// Extract name from migration_id (format: {version}_{name}_{backend}_{connection})
		name := baseMigrationID
		parts := strings.Split(baseMigrationID, "_")
		if len(parts) >= 4 {
			// Format: {version}_{name}_{backend}_{connection}
			name = parts[1]
		}

		// Use metadata from the first record (all records for same baseMigrationID should have same metadata)
		schema := latestRecord.schema
		version := latestRecord.version
		connection := latestRecord.connection
		backend := latestRecord.backend

		// migrations_list should always be inserted (even with empty schema) for dependency resolution
		// Use empty string if schema is not provided
		schemaValue := schema
		if schemaValue == "" {
			schemaValue = "" // Empty string is allowed for migrations_list
		}

		// Insert into migrations_list (one record per migration, even with empty schema)
		insertListSQL := fmt.Sprintf(`
			INSERT INTO %s (migration_id, schema, version, name, connection, backend,
			                status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (migration_id) DO UPDATE SET
				schema = EXCLUDED.schema,
				version = EXCLUDED.version,
				name = EXCLUDED.name,
				connection = EXCLUDED.connection,
				backend = EXCLUDED.backend,
				status = EXCLUDED.status,
				updated_at = CURRENT_TIMESTAMP
		`, listTableName)

		_, err := t.pool.Exec(ctx, insertListSQL,
			baseMigrationID, schemaValue, version, name, connection, backend,
			lastStatus, lastAppliedAt, time.Now())
		if err != nil {
			return fmt.Errorf("failed to insert into migrations_list: %w", err)
		}

		// Skip migrations_executions if schema is empty
		if schema == "" {
			continue
		}

		// Populate migrations_executions table - one record per schema
		applied := lastStatus == "applied"
		var appliedAtPtr *time.Time
		if applied {
			appliedAtPtr = &lastAppliedAt
		}

		execStatus := "pending"
		if applied {
			execStatus = "applied"
		} else if lastStatus == "failed" {
			execStatus = "failed"
		}

		insertExecutionSQL := fmt.Sprintf(`
			INSERT INTO %s (migration_id, schema, version, connection, backend, status, applied, applied_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, CURRENT_TIMESTAMP), CURRENT_TIMESTAMP)
			ON CONFLICT (migration_id, schema, version, connection, backend) DO UPDATE SET
				status = EXCLUDED.status,
				applied = EXCLUDED.applied,
				applied_at = EXCLUDED.applied_at,
				updated_at = CURRENT_TIMESTAMP
		`, executionsTableName)

		// Ensure lastAppliedAt is not zero value (use current time if zero)
		createdAt := lastAppliedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		_, err = t.pool.Exec(ctx, insertExecutionSQL,
			baseMigrationID, schema, version, connection, backend, execStatus, applied, appliedAtPtr, createdAt)
		if err != nil {
			return fmt.Errorf("failed to insert into migrations_executions: %w", err)
		}
	}

	// PHASE 2: Now insert all history records (foreign key constraint is satisfied)
	for baseMigrationID, records := range migrationRecords {
		for _, record := range records {
			// Extract base migration_id (remove _rollback suffix if present)
			isRollback := strings.Contains(record.migrationID, "_rollback")

			// Skip if schema is empty
			if record.schema == "" {
				continue
			}

			// Map status values
			status := record.status
			if status == "success" {
				status = "applied"
			}

			// Insert into migrations_history (all records, including rollbacks) - one record per schema
			insertHistorySQL := fmt.Sprintf(`
				INSERT INTO %s (migration_id, schema, version, connection, backend,
				                status, error_message, executed_by, execution_method, applied_at, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			`, historyTableName)

			executionMethod := "api" // Default for migrated data
			if isRollback {
				executionMethod = "api" // Rollbacks are typically via API
			}

			_, err := t.pool.Exec(ctx, insertHistorySQL,
				baseMigrationID, record.schema, record.version, record.connection, record.backend,
				status, record.errorMsg, "system", executionMethod, record.appliedAt, record.appliedAt)
			if err != nil {
				return fmt.Errorf("failed to insert into migrations_history: %w", err)
			}
		}
	}

	return nil
}

// quoteIdentifier quotes a PostgreSQL identifier
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// configureConnectionPool configures the database connection pool with reasonable defaults
// that can be overridden via environment variables
func configureConnectionPool(config *pgxpool.Config) {
	// Max connections per pool (default: 2, reduced from 5 to prevent connection exhaustion)
	// This limits how many connections each pool instance can open
	maxConns := getEnvInt("BFM_DB_MAX_OPEN_CONNS", 2)
	config.MaxConns = int32(maxConns)

	// Max idle connections per pool (default: 1, reduced from 2)
	// This keeps some connections ready for reuse
	maxIdleConns := getEnvInt("BFM_DB_MAX_IDLE_CONNS", 1)
	config.MinConns = int32(maxIdleConns)

	// Connection max lifetime (default: 3 minutes, reduced from 5)
	// This prevents using stale connections
	connMaxLifetime := time.Duration(getEnvInt("BFM_DB_CONN_MAX_LIFETIME_MINUTES", 3)) * time.Minute
	config.MaxConnLifetime = connMaxLifetime

	// Connection max idle time (default: 30 seconds, supports both seconds and minutes for flexibility)
	// This closes idle connections after this duration
	// Check for seconds first (more granular), then fall back to minutes
	var connMaxIdleTime time.Duration
	if idleTimeSeconds := getEnvInt("BFM_DB_CONN_MAX_IDLE_TIME_SECONDS", 0); idleTimeSeconds > 0 {
		connMaxIdleTime = time.Duration(idleTimeSeconds) * time.Second
	} else {
		connMaxIdleTime = time.Duration(getEnvInt("BFM_DB_CONN_MAX_IDLE_TIME_MINUTES", 1)) * time.Minute
	}
	config.MaxConnIdleTime = connMaxIdleTime
}

// getEnvInt gets an integer environment variable or returns the default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
