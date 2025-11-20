package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"mops/bfm/internal/state"
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

// Initialize creates the migration state table
func (t *Tracker) Initialize(ctx interface{}) error {
	ctxVal := ctx.(context.Context)

	// Ensure schema exists
	if t.schema != "" && t.schema != "public" {
		schemaQuery := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(t.schema))
		if _, err := t.db.ExecContext(ctxVal, schemaQuery); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	// Create migrations table
	tableName := "bfm_migrations"
	if t.schema != "" && t.schema != "public" {
		tableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("bfm_migrations"))
	}

	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			migration_id VARCHAR(255) NOT NULL UNIQUE,
			schema VARCHAR(255) NOT NULL,
			table_name VARCHAR(255) NOT NULL,
			version VARCHAR(50) NOT NULL,
			connection VARCHAR(255) NOT NULL,
			backend VARCHAR(50) NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status VARCHAR(20) NOT NULL DEFAULT 'success',
			error_message TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, tableName)

	if _, err := t.db.ExecContext(ctxVal, createTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Create index on migration_id for faster lookups
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_bfm_migrations_migration_id ON %s (migration_id)
	`, tableName)
	t.db.ExecContext(ctxVal, indexSQL)

	// Create index on schema and table for filtering
	indexSQL2 := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_bfm_migrations_schema_table ON %s (schema, table_name)
	`, tableName)
	t.db.ExecContext(ctxVal, indexSQL2)

	return nil
}

// RecordMigration records a migration execution
func (t *Tracker) RecordMigration(ctx interface{}, migration *state.MigrationRecord) error {
	ctxVal := ctx.(context.Context)

	tableName := "bfm_migrations"
	if t.schema != "" && t.schema != "public" {
		tableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("bfm_migrations"))
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (migration_id, schema, table_name, version, connection, backend, applied_at, status, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (migration_id) 
		DO UPDATE SET 
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message,
			applied_at = EXCLUDED.applied_at
	`, tableName)

	appliedAt := time.Now()
	if migration.AppliedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, migration.AppliedAt); err == nil {
			appliedAt = parsed
		}
	}

	_, err := t.db.ExecContext(ctxVal, query,
		migration.MigrationID,
		migration.Schema,
		migration.Table,
		migration.Version,
		migration.Connection,
		migration.Backend,
		appliedAt,
		migration.Status,
		migration.ErrorMessage,
	)

	return err
}

// GetMigrationHistory retrieves migration history with optional filters
func (t *Tracker) GetMigrationHistory(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	ctxVal := ctx.(context.Context)

	tableName := "bfm_migrations"
	if t.schema != "" && t.schema != "public" {
		tableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("bfm_migrations"))
	}

	query := fmt.Sprintf("SELECT migration_id, schema, table_name, version, connection, backend, applied_at, status, error_message FROM %s WHERE 1=1", tableName)
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
			argIndex++
		}
	}

	query += " ORDER BY applied_at DESC"

	rows, err := t.db.QueryContext(ctxVal, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	var records []*state.MigrationRecord
	for rows.Next() {
		var record state.MigrationRecord
		var appliedAt time.Time

		err := rows.Scan(
			&record.MigrationID,
			&record.Schema,
			&record.Table,
			&record.Version,
			&record.Connection,
			&record.Backend,
			&appliedAt,
			&record.Status,
			&record.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}

		record.AppliedAt = appliedAt.Format(time.RFC3339)
		records = append(records, &record)
	}

	return records, rows.Err()
}

// IsMigrationApplied checks if a migration has been applied
func (t *Tracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	ctxVal := ctx.(context.Context)

	tableName := "bfm_migrations"
	if t.schema != "" && t.schema != "public" {
		tableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("bfm_migrations"))
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE migration_id = $1 AND status = 'success')", tableName)
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

	tableName := "bfm_migrations"
	if t.schema != "" && t.schema != "public" {
		tableName = fmt.Sprintf("%s.%s", quoteIdentifier(t.schema), quoteIdentifier("bfm_migrations"))
	}

	query := fmt.Sprintf(`
		SELECT version 
		FROM %s 
		WHERE schema = $1 AND table_name = $2 AND status = 'success'
		ORDER BY version DESC 
		LIMIT 1
	`, tableName)

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

// Close closes the database connection
func (t *Tracker) Close() error {
	if t.db != nil {
		return t.db.Close()
	}
	return nil
}

// quoteIdentifier quotes a PostgreSQL identifier
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

