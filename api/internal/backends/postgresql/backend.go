package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/toolsascode/bfm/api/internal/backends"

	_ "github.com/lib/pq"
)

// Backend implements the Backend interface for PostgreSQL
type Backend struct {
	db     *sql.DB
	config *backends.ConnectionConfig
}

// NewBackend creates a new PostgreSQL backend
func NewBackend() *Backend {
	return &Backend{}
}

// Name returns the backend name
func (b *Backend) Name() string {
	return "postgresql"
}

// Connect establishes a connection to PostgreSQL
func (b *Backend) Connect(config *backends.ConnectionConfig) error {
	b.config = config

	// Build connection string
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config.Host,
		config.Port,
		config.Username,
		config.Password,
		config.Database,
	)

	var err error
	b.db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Configure connection pool settings
	configureConnectionPool(b.db)

	// Test connection
	if err := b.db.Ping(); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	return nil
}

// Close closes the PostgreSQL connection
func (b *Backend) Close() error {
	if b.db != nil {
		return b.db.Close()
	}
	return nil
}

// CreateSchema creates a schema if it doesn't exist
func (b *Backend) CreateSchema(ctx context.Context, schemaName string) error {
	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(schemaName))
	_, err := b.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create schema %s: %w", schemaName, err)
	}
	return nil
}

// SchemaExists checks if a schema exists
func (b *Backend) SchemaExists(ctx context.Context, schemaName string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM information_schema.schemata
			WHERE schema_name = $1
		)
	`
	var exists bool
	err := b.db.QueryRowContext(ctx, query, schemaName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check schema existence: %w", err)
	}
	return exists, nil
}

// TableExists checks if a table exists in a schema
func (b *Backend) TableExists(ctx context.Context, schemaName, tableName string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2
		)
	`
	var exists bool
	err := b.db.QueryRowContext(ctx, query, schemaName, tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}
	return exists, nil
}

// ExecuteMigration executes a migration script
func (b *Backend) ExecuteMigration(ctx context.Context, migration *backends.MigrationScript) error {
	// Ensure schema exists if specified
	if migration.Schema != "" {
		exists, err := b.SchemaExists(ctx, migration.Schema)
		if err != nil {
			return fmt.Errorf("failed to check schema existence: %w", err)
		}
		if !exists {
			if err := b.CreateSchema(ctx, migration.Schema); err != nil {
				return fmt.Errorf("failed to create schema: %w", err)
			}
		}
	}

	// Begin transaction
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Execute migration SQL
	// If schema is specified, set search_path or use schema-qualified names
	sql := migration.UpSQL
	if migration.Schema != "" {
		// Set search_path for the transaction
		setPathSQL := fmt.Sprintf("SET search_path TO %s, public", quoteIdentifier(migration.Schema))
		if _, err := tx.ExecContext(ctx, setPathSQL); err != nil {
			return fmt.Errorf("failed to set search_path: %w", err)
		}
	}

	// Execute the migration SQL
	if _, err := tx.ExecContext(ctx, sql); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// HealthCheck verifies the backend is accessible
func (b *Backend) HealthCheck(ctx context.Context) error {
	if b.db == nil {
		return fmt.Errorf("database connection not initialized")
	}
	return b.db.PingContext(ctx)
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
