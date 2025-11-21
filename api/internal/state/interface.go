package state

// MigrationRecord represents a migration execution record in state tracking (moved here to avoid import cycle)
type MigrationRecord struct {
	ID              string
	MigrationID     string // Unique ID: {schema}_{connection}_{version}_{name}
	Schema          string
	Table           string
	Version         string
	Connection      string
	Backend         string
	AppliedAt       string
	Status          string // "success", "failed", "pending", "rolled_back"
	ErrorMessage    string
	ExecutedBy      string // User identifier (from auth context)
	ExecutionMethod string // "manual", "api", "cli", "worker"
	ExecutionContext string // JSON with additional context (job_id, request_id, etc.)
}

// MigrationListItem represents a migration in the list with its last execution status
type MigrationListItem struct {
	MigrationID      string
	Schema           string
	Table            string
	Version          string
	Name             string
	Connection       string
	Backend          string
	LastStatus       string // "success", "failed", "pending", "rolled_back"
	LastAppliedAt    string
	LastErrorMessage string
	Applied          bool
}

// StateTracker manages migration state tracking
type StateTracker interface {
	// RecordMigration records a migration execution
	RecordMigration(ctx interface{}, migration *MigrationRecord) error

	// GetMigrationHistory retrieves migration history with optional filters
	GetMigrationHistory(ctx interface{}, filters *MigrationFilters) ([]*MigrationRecord, error)

	// GetMigrationList retrieves the list of migrations with their last status
	GetMigrationList(ctx interface{}, filters *MigrationFilters) ([]*MigrationListItem, error)

	// IsMigrationApplied checks if a migration has been applied
	IsMigrationApplied(ctx interface{}, migrationID string) (bool, error)

	// GetLastMigrationVersion gets the last applied version for a schema/table
	GetLastMigrationVersion(ctx interface{}, schema, table string) (string, error)

	// RegisterScannedMigration registers a scanned migration in migrations_list (status: pending)
	RegisterScannedMigration(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error

	// Initialize sets up the state tracking tables
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

