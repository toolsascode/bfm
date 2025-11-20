package backends

import (
	"context"
)

// MigrationScript represents a migration script (moved here to avoid import cycle)
type MigrationScript struct {
	Schema     string
	Table      string
	Version    string
	Name       string
	Connection string
	Backend    string
	UpSQL      string
	DownSQL    string
}

// Backend represents a database backend that can execute migrations
type Backend interface {
	// Name returns the name of the backend (e.g., "postgresql", "greptimedb", "etcd")
	Name() string

	// Connect establishes a connection to the backend
	Connect(config *ConnectionConfig) error

	// Close closes the connection to the backend
	Close() error

	// ExecuteMigration executes a migration script
	ExecuteMigration(ctx context.Context, migration *MigrationScript) error

	// CreateSchema creates a schema/database if it doesn't exist
	CreateSchema(ctx context.Context, schemaName string) error

	// SchemaExists checks if a schema/database exists
	SchemaExists(ctx context.Context, schemaName string) (bool, error)

	// HealthCheck verifies the backend is accessible
	HealthCheck(ctx context.Context) error
}

// ConnectionConfig holds configuration for a backend connection
type ConnectionConfig struct {
	Backend   string // "postgresql", "greptimedb", "etcd"
	Host      string
	Port      string
	Username  string
	Password  string
	Database  string
	Schema    string // Can be fixed or dynamic
	Extra     map[string]string // Additional backend-specific config
}

// MigrationResult represents the result of a migration execution
type MigrationResult struct {
	Success   bool
	Error     error
	Duration  string
	RowsAffected int64
}

