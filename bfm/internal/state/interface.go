package state

// MigrationRecord represents a migration execution record in state tracking (moved here to avoid import cycle)
type MigrationRecord struct {
	ID           string
	MigrationID  string // Unique ID: {schema}_{table}_{version}_{name}
	Schema       string
	Table        string
	Version      string
	Connection   string
	Backend      string
	AppliedAt    string
	Status       string // "success", "failed", "pending"
	ErrorMessage string
}

// StateTracker manages migration state tracking
type StateTracker interface {
	// RecordMigration records a migration execution
	RecordMigration(ctx interface{}, migration *MigrationRecord) error

	// GetMigrationHistory retrieves migration history with optional filters
	GetMigrationHistory(ctx interface{}, filters *MigrationFilters) ([]*MigrationRecord, error)

	// IsMigrationApplied checks if a migration has been applied
	IsMigrationApplied(ctx interface{}, migrationID string) (bool, error)

	// GetLastMigrationVersion gets the last applied version for a schema/table
	GetLastMigrationVersion(ctx interface{}, schema, table string) (string, error)

	// Initialize sets up the state tracking table/schema
	Initialize(ctx interface{}) error
}

// MigrationFilters specifies filters for querying migrations
type MigrationFilters struct {
	Schema     string
	Table      string
	Connection string
	Backend    string
	Status     string
	Version    string
}

