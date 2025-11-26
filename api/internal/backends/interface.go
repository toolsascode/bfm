package backends

import (
	"context"
)

// Dependency represents a structured dependency on another migration
type Dependency struct {
	Connection     string // Connection name (e.g., "core", "guard")
	Schema         string // Schema name (optional, for cross-schema dependencies)
	Target         string // Migration version or name to depend on
	TargetType     string // "version" or "name" (default: "name" for backward compatibility)
	RequiresTable  string // Optional table that must exist before execution
	RequiresSchema string // Optional schema that must exist before execution
}

// MigrationScript represents a migration script (moved here to avoid import cycle)
type MigrationScript struct {
	Schema                 string
	Table                  *string // Optional: can be nil for backends that don't use tables
	Version                string  // Required: version timestamp
	Name                   string
	Connection             string
	Backend                string
	UpSQL                  string
	DownSQL                string
	Dependencies           []string     // Optional: list of migration names this migration depends on (backward compatibility)
	StructuredDependencies []Dependency // Optional: structured dependencies with validation requirements
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
	Backend  string // "postgresql", "greptimedb", "etcd"
	Host     string
	Port     string
	Username string
	Password string
	Database string
	Schema   string            // Can be fixed or dynamic
	Extra    map[string]string // Additional backend-specific config
}

// MigrationResult represents the result of a migration execution
type MigrationResult struct {
	Success      bool
	Error        error
	Duration     string
	RowsAffected int64
}
