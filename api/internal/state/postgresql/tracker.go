package postgresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/state"
)

// Tracker implements StateTracker for PostgreSQL
type Tracker struct {
	db     *sql.DB
	schema string
}

// NewTracker creates a new PostgreSQL state tracker
func NewTracker(connStr string, schema string) (*Tracker, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool settings
	configureConnectionPool(db)

	tracker := &Tracker{
		db:     db,
		schema: schema,
	}

	// Initialize the tracker (create table if needed)
	if err := tracker.Initialize(context.Background()); err != nil {
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
		if _, err := t.db.ExecContext(ctxVal, schemaQuery); err != nil {
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

	if _, err := t.db.ExecContext(ctxVal, createListTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_list table: %w", err)
	}

	// Create indexes for migrations_list
	// Note: migration_id is PRIMARY KEY so already indexed, but explicit index is kept for consistency
	// All tables with migration_id column must have an index on it for performance and foreign key constraints
	indexSQL1 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_migration_id ON %s (migration_id)", listTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL1)

	indexSQL2 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_connection_backend ON %s (connection, backend)", listTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL2)

	indexSQL3 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_status ON %s (status)", listTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL3)

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

	if _, err := t.db.ExecContext(ctxVal, createHistoryTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_history table: %w", err)
	}

	// Create indexes for migrations_history
	// Index on migration_id is required for foreign key performance and to avoid using migration names that don't exist in migrations_list
	indexSQL4 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_migration_id ON %s (migration_id)", historyTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL4)

	indexSQL5 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_applied_at ON %s (applied_at DESC)", historyTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL5)

	indexSQL6 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_status ON %s (status)", historyTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL6)

	// Create migrations_executions table
	executionsTableName := "migrations_executions"
	if t.schema != "" && t.schema != "public" {
		executionsTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_executions"))
	}

	createExecutionsTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
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
			UNIQUE (migration_id, schema, version, connection, backend)
		)
	`, executionsTableName, listTableName)

	if _, err := t.db.ExecContext(ctxVal, createExecutionsTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_executions table: %w", err)
	}

	// Create indexes for migrations_executions
	// Index on migration_id is required for foreign key performance and to avoid using migration names that don't exist in migrations_list
	indexSQL7 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_executions_migration_id ON %s (migration_id)", executionsTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL7)

	indexSQL8 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_executions_status ON %s (status)", executionsTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL8)

	indexSQL9 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_executions_created_at ON %s (created_at DESC)", executionsTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL9)

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

	if _, err := t.db.ExecContext(ctxVal, createDependenciesTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_dependencies table: %w", err)
	}

	// Create indexes for migrations_dependencies
	// Index on migration_id is required for foreign key performance and to avoid using migration names that don't exist in migrations_list
	indexSQL10 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_dependencies_migration_id ON %s (migration_id)", dependenciesTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL10)

	indexSQL11 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_dependencies_dependency_id ON %s (dependency_id)", dependenciesTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL11)

	// Migrate existing data from old tables if they exist
	executionsTableNameForMigration := executionsTableName
	dependenciesTableNameForMigration := dependenciesTableName
	if err := t.migrateExistingData(ctxVal, listTableName, historyTableName, executionsTableNameForMigration, dependenciesTableNameForMigration); err != nil {
		// Log warning but don't fail initialization
		fmt.Printf("Warning: Failed to migrate existing data: %v\n", err)
	}

	return nil
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
	// - With rollback suffix: ..._rollback
	// migrations_list should always use the base ID (without schema prefix)
	migrationID := migration.MigrationID
	isRollback := strings.Contains(migrationID, "_rollback")
	if isRollback {
		migrationID = strings.TrimSuffix(migrationID, "_rollback")
	}

	// Remove schema prefix if present to get base migration_id
	// Schema-specific format: {schema}_{version}_{name}_{backend}_{connection}
	// Base format: {version}_{name}_{backend}_{connection}
	// Version is typically 14 digits (YYYYMMDDHHMMSS), so we check if first part is a version
	baseMigrationID := migrationID
	parts := strings.Split(migrationID, "_")
	if len(parts) >= 5 {
		// Check if first part looks like a schema name (not a version number)
		// Versions are 14 digits, so if first part is not all digits, it might be a schema prefix
		firstPart := parts[0]
		isVersion := len(firstPart) >= 10 && len(firstPart) <= 20
		if isVersion {
			allDigits := true
			for _, r := range firstPart {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			isVersion = allDigits
		}
		// If first part is not a version, it's likely a schema prefix - remove it
		if !isVersion {
			baseMigrationID = strings.Join(parts[1:], "_")
		}
	}

	executedBy := migration.ExecutedBy
	if executedBy == "" {
		executedBy = "system"
	}
	executionMethod := migration.ExecutionMethod
	if executionMethod == "" {
		executionMethod = "api"
	}

	// Convert schema to array
	schemas := []string{}
	if migration.Schema != "" {
		schemas = []string{migration.Schema}
	}

	// Map status values
	status := migration.Status
	if status == "success" {
		status = "applied"
	}

	// Only update status in migrations_list if migration exists (populated from sfm folder)
	// migrations_list should only be populated via ReindexMigrations() or RegisterScannedMigration()
	// This UPDATE will affect 0 rows if migration doesn't exist, which is acceptable
	// The foreign key constraint will prevent history insert if migration doesn't exist in list
	updateListSQL := fmt.Sprintf(`
		UPDATE %s
		SET status = $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE migration_id = $2
	`, listTableName)

	listStatus := status
	if isRollback {
		listStatus = "rolled_back"
	}
	if status == "success" {
		listStatus = "applied"
	}

	_, err := t.db.ExecContext(ctxVal, updateListSQL, listStatus, baseMigrationID)
	// Don't error if 0 rows affected - migration might not be in list yet (should be indexed from sfm first)

	// Skip insertion if no schemas specified
	if len(schemas) == 0 {
		return nil
	}

	// Insert one record per schema into migrations_history
	insertHistorySQL := fmt.Sprintf(`
		INSERT INTO %s (migration_id, schema, version, connection, backend,
		                status, error_message, executed_by, execution_method, execution_context, applied_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, historyTableName)

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
		// Insert into migrations_history
		var historyID int
		err = t.db.QueryRowContext(ctxVal, insertHistorySQL,
			baseMigrationID, schema, migration.Version,
			migration.Connection, migration.Backend, status, migration.ErrorMessage,
			executedBy, executionMethod, migration.ExecutionContext, appliedAt, appliedAt).Scan(&historyID)
		if err != nil {
			return fmt.Errorf("failed to insert into migrations_history: %w", err)
		}

		// Insert into migrations_executions
		_, err = t.db.ExecContext(ctxVal, insertExecutionSQL,
			baseMigrationID, schema, migration.Version,
			migration.Connection, migration.Backend, execStatus, applied, appliedAtPtr)
		if err != nil {
			return fmt.Errorf("failed to insert into migrations_executions: %w", err)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to insert into migrations_executions: %w", err)
	}

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

	query += " ORDER BY applied_at DESC"

	rows, err := t.db.QueryContext(ctxVal, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

	rows, err := t.db.QueryContext(ctxVal, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*state.MigrationListItem
	for rows.Next() {
		var item state.MigrationListItem
		var createdAt sql.NullTime
		var updatedAt sql.NullTime

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
		if item.Applied && updatedAt.Valid {
			item.LastAppliedAt = updatedAt.Time.Format(time.RFC3339)
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

	// Remove schema prefix if present to get base migration_id
	baseMigrationID := migrationID
	parts := strings.Split(migrationID, "_")
	if len(parts) >= 5 {
		// Check if first part is a version (all digits, 10-20 chars)
		firstPart := parts[0]
		isVersion := len(firstPart) >= 10 && len(firstPart) <= 20
		if isVersion {
			allDigits := true
			for _, r := range firstPart {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			isVersion = allDigits
		}
		// If first part is not a version, it's likely a schema prefix - remove it
		if !isVersion {
			baseMigrationID = strings.Join(parts[1:], "_")
		}
	}

	query := fmt.Sprintf(`
		SELECT migration_id, schema, version, name, connection, backend,
		       up_sql, down_sql, dependencies, structured_dependencies, status, created_at, updated_at
		FROM %s WHERE migration_id = $1
	`, listTableName)

	var detail state.MigrationDetail
	var schemaStr sql.NullString
	var upSQL, downSQL sql.NullString
	var dependencies pq.StringArray
	var structuredDepsJSON sql.NullString
	var createdAt, updatedAt sql.NullTime

	err := t.db.QueryRowContext(ctxVal, query, baseMigrationID).Scan(
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
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query migration detail: %w", err)
	}

	if schemaStr.Valid {
		detail.Schema = schemaStr.String
	}
	if upSQL.Valid {
		detail.UpSQL = upSQL.String
	}
	if downSQL.Valid {
		detail.DownSQL = downSQL.String
	}
	if dependencies != nil {
		detail.Dependencies = []string(dependencies)
	}
	if structuredDepsJSON.Valid && structuredDepsJSON.String != "" {
		var structuredDeps []backends.Dependency
		if err := json.Unmarshal([]byte(structuredDepsJSON.String), &structuredDeps); err == nil {
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

	// Remove schema prefix if present to get base migration_id
	baseMigrationID := migrationID
	parts := strings.Split(migrationID, "_")
	if len(parts) >= 5 {
		// Check if first part is a version (all digits, 10-20 chars)
		firstPart := parts[0]
		isVersion := len(firstPart) >= 10 && len(firstPart) <= 20
		if isVersion {
			allDigits := true
			for _, r := range firstPart {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			isVersion = allDigits
		}
		// If first part is not a version, it's likely a schema prefix - remove it
		if !isVersion {
			baseMigrationID = strings.Join(parts[1:], "_")
		}
	}

	query := fmt.Sprintf(`
		SELECT id, migration_id, schema, version, connection, backend,
		       status, applied, applied_at, created_at, updated_at
		FROM %s WHERE migration_id = $1
		ORDER BY created_at DESC
	`, executionsTableName)

	rows, err := t.db.QueryContext(ctxVal, query, baseMigrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query migration executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var executions []*state.MigrationExecution
	for rows.Next() {
		var exec state.MigrationExecution
		var schemaStr sql.NullString
		var appliedAt, createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&exec.ID,
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

		if schemaStr.Valid {
			exec.Schema = schemaStr.String
		}
		if appliedAt.Valid {
			exec.AppliedAt = appliedAt.Time.Format(time.RFC3339)
		}
		if createdAt.Valid {
			exec.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		if updatedAt.Valid {
			exec.UpdatedAt = updatedAt.Time.Format(time.RFC3339)
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
		SELECT id, migration_id, schema, version, connection, backend,
		       status, applied, applied_at, created_at, updated_at
		FROM %s
		ORDER BY created_at DESC
		LIMIT $1
	`, executionsTableName)

	rows, err := t.db.QueryContext(ctxVal, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var executions []*state.MigrationExecution
	for rows.Next() {
		var exec state.MigrationExecution
		var schemaStr sql.NullString
		var appliedAt, createdAt, updatedAt sql.NullTime

		err := rows.Scan(
			&exec.ID,
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

		if schemaStr.Valid {
			exec.Schema = schemaStr.String
		}
		if appliedAt.Valid {
			exec.AppliedAt = appliedAt.Time.Format(time.RFC3339)
		}
		if createdAt.Valid {
			exec.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		if updatedAt.Valid {
			exec.UpdatedAt = updatedAt.Time.Format(time.RFC3339)
		}

		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// IsMigrationApplied checks if a migration has been applied
func (t *Tracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE migration_id = $1 AND status = 'applied')", listTableName)
	var exists bool
	err := t.db.QueryRowContext(ctxVal, query, migrationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check migration status: %w", err)
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
	err := t.db.QueryRowContext(ctxVal, query, schema).Scan(&version)
	if err == sql.ErrNoRows {
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
	_, err := t.db.ExecContext(ctxVal, insertListSQL,
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

	result, err := t.db.ExecContext(ctxVal, updateSQL,
		schemaValue, version, name, connection, backend, migrationID)
	if err != nil {
		return fmt.Errorf("failed to update migration info: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

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
	_, err := t.db.ExecContext(ctxVal, deleteSQL, migrationID)
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
		_, err = t.db.ExecContext(ctxVal, upsertSQL,
			migrationID,
			schemaValue,
			migration.Version,
			migration.Name,
			migration.Connection,
			migration.Backend,
			upSQLFilename,
			downSQLFilename,
			pq.Array(dependencies),
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
			_, err = t.db.ExecContext(ctxVal, insertExecutionSQL,
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
	_, err := t.db.ExecContext(ctx, deleteSQL, migrationID)
	if err != nil {
		return fmt.Errorf("failed to delete existing dependencies: %w", err)
	}

	// Insert structured dependencies
	for _, dep := range migration.StructuredDependencies {
		// Find dependency_id by resolving the dependency target
		dependencyID, err := t.resolveDependencyID(ctx, dep, listTableName)
		if err != nil {
			// Log but continue - dependency might not exist yet
			fmt.Printf("Warning: Failed to resolve dependency for %s: %v\n", migrationID, err)
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

		_, err = t.db.ExecContext(ctx, insertSQL,
			migrationID,
			dependencyID,
			dep.Connection,
			pq.Array(schemas),
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
			// Log but continue
			fmt.Printf("Warning: Failed to find dependency %s for %s: %v\n", depName, migrationID, err)
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

		_, err = t.db.ExecContext(ctx, insertSQL,
			migrationID,
			dependencyID,
			migration.Connection,
			pq.Array(schemas),
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
	err := t.db.QueryRowContext(ctx, query, args...).Scan(&migrationID)
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
	err := t.db.QueryRowContext(ctx, query, name).Scan(&migrationID)
	if err != nil {
		return "", fmt.Errorf("migration not found: %w", err)
	}

	return migrationID, nil
}

// Close closes the database connection
func (t *Tracker) Close() error {
	if t.db != nil {
		return t.db.Close()
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

	err := t.db.QueryRowContext(ctx, checkTableSQL, schemaName).Scan(&tableExists)
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

	rows, err := t.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query old table: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

		// Extract base migration_id (remove _rollback suffix if present)
		baseMigrationID := migrationID
		isRollback := strings.Contains(migrationID, "_rollback")
		if isRollback {
			baseMigrationID = strings.TrimSuffix(migrationID, "_rollback")
		}

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

		_, err := t.db.ExecContext(ctx, insertListSQL,
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

		_, err = t.db.ExecContext(ctx, insertExecutionSQL,
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

			_, err := t.db.ExecContext(ctx, insertHistorySQL,
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
func configureConnectionPool(db *sql.DB) {
	// Max open connections per pool (default: 5)
	// This limits how many connections each sql.DB instance can open
	maxOpenConns := getEnvInt("BFM_DB_MAX_OPEN_CONNS", 5)
	db.SetMaxOpenConns(maxOpenConns)

	// Max idle connections per pool (default: 2)
	// This keeps some connections ready for reuse
	maxIdleConns := getEnvInt("BFM_DB_MAX_IDLE_CONNS", 2)
	db.SetMaxIdleConns(maxIdleConns)

	// Connection max lifetime (default: 5 minutes)
	// This prevents using stale connections
	connMaxLifetime := time.Duration(getEnvInt("BFM_DB_CONN_MAX_LIFETIME_MINUTES", 5)) * time.Minute
	db.SetConnMaxLifetime(connMaxLifetime)

	// Connection max idle time (default: 1 minute)
	// This closes idle connections after this duration
	connMaxIdleTime := time.Duration(getEnvInt("BFM_DB_CONN_MAX_IDLE_TIME_MINUTES", 1)) * time.Minute
	db.SetConnMaxIdleTime(connMaxIdleTime)
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
