package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"bfm/api/internal/state"

	_ "github.com/lib/pq"
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
			id SERIAL PRIMARY KEY,
			migration_id VARCHAR(255) NOT NULL UNIQUE,
			schema VARCHAR(255) NOT NULL,
			table_name VARCHAR(255) NOT NULL,
			version VARCHAR(50) NOT NULL,
			name VARCHAR(255) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			backend VARCHAR(50) NOT NULL,
			last_status VARCHAR(20) NOT NULL DEFAULT 'pending',
			last_applied_at TIMESTAMP,
			last_error_message TEXT,
			last_history_id INTEGER,
			first_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, listTableName)

	if _, err := t.db.ExecContext(ctxVal, createListTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations_list table: %w", err)
	}

	// Create indexes for migrations_list
	indexSQL1 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_migration_id ON %s (migration_id)", listTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL1)

	indexSQL2 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_connection_backend ON %s (connection, backend)", listTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL2)

	indexSQL3 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_list_schema_table ON %s (schema, table_name)", listTableName)
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
			table_name VARCHAR(255) NOT NULL,
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
	indexSQL4 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_migration_id ON %s (migration_id)", historyTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL4)

	indexSQL5 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_applied_at ON %s (applied_at DESC)", historyTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL5)

	indexSQL6 := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_migrations_history_status ON %s (status)", historyTableName)
	_, _ = t.db.ExecContext(ctxVal, indexSQL6)

	// Migrate existing data from bfm_migrations if it exists
	if err := t.migrateExistingData(ctxVal, listTableName, historyTableName); err != nil {
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
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
		historyTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_history"))
	}

	appliedAt := time.Now()
	if migration.AppliedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, migration.AppliedAt); err == nil {
			appliedAt = parsed
		}
	}

	// Extract base migration_id (remove _rollback suffix if present)
	baseMigrationID := migration.MigrationID
	isRollback := strings.Contains(migration.MigrationID, "_rollback")
	if isRollback {
		baseMigrationID = strings.TrimSuffix(migration.MigrationID, "_rollback")
	}

	// Insert into migrations_history
	insertHistorySQL := fmt.Sprintf(`
		INSERT INTO %s (migration_id, schema, table_name, version, connection, backend,
		                status, error_message, executed_by, execution_method, execution_context, applied_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`, historyTableName)

	var historyID int
	executedBy := migration.ExecutedBy
	if executedBy == "" {
		executedBy = "system"
	}
	executionMethod := migration.ExecutionMethod
	if executionMethod == "" {
		executionMethod = "api"
	}

	err := t.db.QueryRowContext(ctxVal, insertHistorySQL,
		baseMigrationID, migration.Schema, migration.Table, migration.Version,
		migration.Connection, migration.Backend, migration.Status, migration.ErrorMessage,
		executedBy, executionMethod, migration.ExecutionContext, appliedAt, appliedAt).Scan(&historyID)
	if err != nil {
		return fmt.Errorf("failed to insert into migrations_history: %w", err)
	}

	// Update or insert into migrations_list (only for base migration_id, not rollbacks)
	if !isRollback {
		// Extract name from migration_id
		name := baseMigrationID
		parts := strings.Split(baseMigrationID, "_")
		if len(parts) >= 4 {
			name = strings.Join(parts[3:], "_")
		}

		upsertListSQL := fmt.Sprintf(`
			INSERT INTO %s (migration_id, schema, table_name, version, name, connection, backend,
			                last_status, last_applied_at, last_error_message, last_history_id, first_seen_at, last_updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (migration_id) DO UPDATE SET
				last_status = EXCLUDED.last_status,
				last_applied_at = EXCLUDED.last_applied_at,
				last_error_message = EXCLUDED.last_error_message,
				last_history_id = EXCLUDED.last_history_id,
				last_updated_at = CURRENT_TIMESTAMP
		`, listTableName)

		_, err = t.db.ExecContext(ctxVal, upsertListSQL,
			baseMigrationID, migration.Schema, migration.Table, migration.Version, name,
			migration.Connection, migration.Backend, migration.Status, appliedAt,
			migration.ErrorMessage, historyID, appliedAt, time.Now())
		if err != nil {
			return fmt.Errorf("failed to upsert into migrations_list: %w", err)
		}
	} else {
		// For rollbacks, update the status in migrations_list to "rolled_back"
		// but keep the last_applied_at from the most recent successful execution
		updateRollbackSQL := fmt.Sprintf(`
			UPDATE %s 
			SET last_status = 'rolled_back',
			    last_history_id = $1,
			    last_updated_at = CURRENT_TIMESTAMP
			WHERE migration_id = $2
		`, listTableName)

		_, err = t.db.ExecContext(ctxVal, updateRollbackSQL, historyID, baseMigrationID)
		if err != nil {
			// If update fails, it might be because the migration doesn't exist in list yet
			// This can happen if a rollback is executed before the migration is in the list
			// Log but don't fail
			fmt.Printf("Warning: Failed to update migrations_list for rollback: %v\n", err)
		}
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
		SELECT id, migration_id, schema, table_name, version, connection, backend, 
		       applied_at, status, error_message, executed_by, execution_method, execution_context
		FROM %s WHERE 1=1
	`, historyTableName)

	args := []interface{}{}
	argIndex := 1

	if filters != nil {
		if filters.Schema != "" {
			query += fmt.Sprintf(" AND schema = $%d", argIndex)
			args = append(args, filters.Schema)
			argIndex++
		}
		if filters.Table != "" {
			query += fmt.Sprintf(" AND table_name = $%d", argIndex)
			args = append(args, filters.Table)
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
			&record.Table,
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
		SELECT migration_id, schema, table_name, version, name, connection, backend,
		       last_status, last_applied_at, last_error_message
		FROM %s WHERE 1=1
	`, listTableName)

	args := []interface{}{}
	argIndex := 1

	if filters != nil {
		if filters.Schema != "" {
			query += fmt.Sprintf(" AND schema = $%d", argIndex)
			args = append(args, filters.Schema)
			argIndex++
		}
		if filters.Table != "" {
			query += fmt.Sprintf(" AND table_name = $%d", argIndex)
			args = append(args, filters.Table)
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
			query += fmt.Sprintf(" AND last_status = $%d", argIndex)
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
		var lastAppliedAt sql.NullTime
		var lastErrorMessage sql.NullString

		err := rows.Scan(
			&item.MigrationID,
			&item.Schema,
			&item.Table,
			&item.Version,
			&item.Name,
			&item.Connection,
			&item.Backend,
			&item.LastStatus,
			&lastAppliedAt,
			&lastErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration list item: %w", err)
		}

		if lastAppliedAt.Valid {
			item.LastAppliedAt = lastAppliedAt.Time.Format(time.RFC3339)
		}
		if lastErrorMessage.Valid {
			item.LastErrorMessage = lastErrorMessage.String
		}
		item.Applied = item.LastStatus == "success"

		items = append(items, &item)
	}

	return items, rows.Err()
}

// IsMigrationApplied checks if a migration has been applied
func (t *Tracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	ctxVal := ctx.(context.Context)

	listTableName := "migrations_list"
	if t.schema != "" && t.schema != "public" {
		listTableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("migrations_list"))
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE migration_id = $1 AND last_status = 'success')", listTableName)
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
		WHERE schema = $1 AND table_name = $2 AND last_status = 'success'
		ORDER BY version DESC 
		LIMIT 1
	`, listTableName)

	var version string
	err := t.db.QueryRowContext(ctxVal, query, schema, table).Scan(&version)
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

	insertListSQL := `INSERT INTO ` + listTableName + ` (migration_id, schema, table_name, version, name, connection, backend, 
		                last_status, first_seen_at, last_updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (migration_id) DO NOTHING`

	now := time.Now()
	_, err := t.db.ExecContext(ctxVal, insertListSQL,
		migrationID, schema, table, version, name, connection, backend,
		"pending", now, now)
	if err != nil {
		return fmt.Errorf("failed to register scanned migration: %w", err)
	}

	return nil
}

// Close closes the database connection
func (t *Tracker) Close() error {
	if t.db != nil {
		return t.db.Close()
	}
	return nil
}

// migrateExistingData migrates data from old bfm_migrations table to new tables
func (t *Tracker) migrateExistingData(ctx context.Context, listTableName, historyTableName string) error {
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
		lastErrorMessage := latestRecord.errorMsg

		// If latest is a rollback, check if there's a more recent success
		if strings.Contains(latestRecord.migrationID, "_rollback") {
			lastStatus = "rolled_back"
			if latestSuccessRecord != nil {
				lastAppliedAt = latestSuccessRecord.appliedAt
				lastErrorMessage = latestSuccessRecord.errorMsg
			}
		}

		// Extract name from migration_id (format: {schema}_{connection}_{version}_{name})
		name := baseMigrationID
		parts := strings.Split(baseMigrationID, "_")
		if len(parts) >= 4 {
			name = strings.Join(parts[3:], "_")
		}

		// Use metadata from the first record (all records for same baseMigrationID should have same metadata)
		schema := latestRecord.schema
		tableName := latestRecord.tableName
		version := latestRecord.version
		connection := latestRecord.connection
		backend := latestRecord.backend

		insertListSQL := fmt.Sprintf(`
			INSERT INTO %s (migration_id, schema, table_name, version, name, connection, backend, 
			                last_status, last_applied_at, last_error_message, first_seen_at, last_updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (migration_id) DO UPDATE SET
				last_status = EXCLUDED.last_status,
				last_applied_at = EXCLUDED.last_applied_at,
				last_error_message = EXCLUDED.last_error_message,
				last_updated_at = CURRENT_TIMESTAMP
		`, listTableName)

		_, err := t.db.ExecContext(ctx, insertListSQL,
			baseMigrationID, schema, tableName, version, name, connection, backend,
			lastStatus, lastAppliedAt, lastErrorMessage, lastAppliedAt, time.Now())
		if err != nil {
			return fmt.Errorf("failed to insert into migrations_list: %w", err)
		}
	}

	// PHASE 2: Now insert all history records (foreign key constraint is satisfied)
	for baseMigrationID, records := range migrationRecords {
		for _, record := range records {
			// Extract base migration_id (remove _rollback suffix if present)
			isRollback := strings.Contains(record.migrationID, "_rollback")

			// Insert into migrations_history (all records, including rollbacks)
			insertHistorySQL := fmt.Sprintf(`
				INSERT INTO %s (migration_id, schema, table_name, version, connection, backend,
				                status, error_message, executed_by, execution_method, applied_at, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			`, historyTableName)

			executionMethod := "api" // Default for migrated data
			if isRollback {
				executionMethod = "api" // Rollbacks are typically via API
			}

			_, err := t.db.ExecContext(ctx, insertHistorySQL,
				baseMigrationID, record.schema, record.tableName, record.version, record.connection, record.backend,
				record.status, record.errorMsg, "system", executionMethod, record.appliedAt, record.appliedAt)
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
