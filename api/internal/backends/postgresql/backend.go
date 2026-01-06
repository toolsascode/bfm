package postgresql

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toolsascode/bfm/api/internal/backends"
)

// Backend implements the Backend interface for PostgreSQL
type Backend struct {
	pool   *pgxpool.Pool
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
	// Close existing connection if any to prevent connection leaks
	if b.pool != nil {
		b.pool.Close()
		b.pool = nil
	}

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

	// Parse connection config
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return fmt.Errorf("failed to parse PostgreSQL connection string: %w", err)
	}

	// Configure connection pool settings
	configureConnectionPool(poolConfig)

	// Create connection pool
	b.pool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL connection pool: %w", err)
	}

	// Test connection
	if err := b.pool.Ping(context.Background()); err != nil {
		b.pool.Close() // Clean up on ping failure
		b.pool = nil
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	return nil
}

// Close closes the PostgreSQL connection
func (b *Backend) Close() error {
	if b.pool != nil {
		b.pool.Close()
		b.pool = nil
	}
	return nil
}

// CreateSchema creates a schema if it doesn't exist
func (b *Backend) CreateSchema(ctx context.Context, schemaName string) error {
	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdentifier(schemaName))
	_, err := b.pool.Exec(ctx, query)
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
	err := b.pool.QueryRow(ctx, query, schemaName).Scan(&exists)
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
	err := b.pool.QueryRow(ctx, query, schemaName, tableName).Scan(&exists)
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
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Execute migration SQL
	// If schema is specified, set search_path or use schema-qualified names
	sql := migration.UpSQL
	if migration.Schema != "" {
		// Set search_path for the transaction
		setPathSQL := fmt.Sprintf("SET search_path TO %s, public", quoteIdentifier(migration.Schema))
		if _, err := tx.Exec(ctx, setPathSQL); err != nil {
			return fmt.Errorf("failed to set search_path: %w", err)
		}
	}

	// Execute the migration SQL
	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// HealthCheck verifies the backend is accessible
func (b *Backend) HealthCheck(ctx context.Context) error {
	if b.pool == nil {
		return fmt.Errorf("database connection not initialized")
	}
	return b.pool.Ping(ctx)
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
