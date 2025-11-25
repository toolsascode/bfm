package registry

import (
	"fmt"

	"bfm/api/internal/backends"
)

// MigrationTarget specifies which migrations to execute (moved here to avoid import cycle)
type MigrationTarget struct {
	Backend    string   // Backend type filter
	Schema     string   // Schema filter (optional)
	Tables     []string // Table filters (optional, empty = all)
	Version    string   // Version filter (optional, empty = latest)
	Connection string   // Connection name filter
}

// Registry manages migration script registration and lookup
type Registry interface {
	// Register registers a migration script
	Register(migration *backends.MigrationScript) error

	// FindByTarget finds migrations matching a target specification
	FindByTarget(target *MigrationTarget) ([]*backends.MigrationScript, error)

	// GetAll returns all registered migrations
	GetAll() []*backends.MigrationScript

	// GetByConnection returns migrations for a specific connection
	GetByConnection(connectionName string) []*backends.MigrationScript

	// GetByBackend returns migrations for a specific backend
	GetByBackend(backendName string) []*backends.MigrationScript

	// GetMigrationByName finds migrations by name across all connections/backends
	GetMigrationByName(name string) []*backends.MigrationScript

	// GetMigrationByVersion finds migrations by version across all connections/backends
	GetMigrationByVersion(version string) []*backends.MigrationScript

	// GetMigrationByConnectionAndVersion finds migrations by connection and version
	GetMigrationByConnectionAndVersion(connection, version string) []*backends.MigrationScript
}

// GlobalRegistry is the global migration registry instance
var GlobalRegistry Registry = NewInMemoryRegistry()

// NewInMemoryRegistry creates a new in-memory registry
func NewInMemoryRegistry() Registry {
	return &inMemoryRegistry{
		migrations: make(map[string]*backends.MigrationScript),
	}
}

type inMemoryRegistry struct {
	migrations map[string]*backends.MigrationScript
}

func (r *inMemoryRegistry) Register(migration *backends.MigrationScript) error {
	migrationID := r.getMigrationID(migration)
	r.migrations[migrationID] = migration
	return nil
}

func (r *inMemoryRegistry) FindByTarget(target *MigrationTarget) ([]*backends.MigrationScript, error) {
	var results []*backends.MigrationScript

	for _, migration := range r.migrations {
		if target.Backend != "" && migration.Backend != target.Backend {
			continue
		}
		if target.Connection != "" && migration.Connection != target.Connection {
			continue
		}
		if target.Schema != "" && migration.Schema != target.Schema {
			continue
		}
		if len(target.Tables) > 0 {
			found := false
			migrationTable := ""
			if migration.Table != nil {
				migrationTable = *migration.Table
			}
			for _, table := range target.Tables {
				if migrationTable == table {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if target.Version != "" && migration.Version != target.Version {
			continue
		}
		results = append(results, migration)
	}

	return results, nil
}

func (r *inMemoryRegistry) GetAll() []*backends.MigrationScript {
	results := make([]*backends.MigrationScript, 0, len(r.migrations))
	for _, migration := range r.migrations {
		results = append(results, migration)
	}
	return results
}

func (r *inMemoryRegistry) GetByConnection(connectionName string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range r.migrations {
		if migration.Connection == connectionName {
			results = append(results, migration)
		}
	}
	return results
}

func (r *inMemoryRegistry) GetByBackend(backendName string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range r.migrations {
		if migration.Backend == backendName {
			results = append(results, migration)
		}
	}
	return results
}

func (r *inMemoryRegistry) GetMigrationByName(name string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range r.migrations {
		if migration.Name == name {
			results = append(results, migration)
		}
	}
	return results
}

func (r *inMemoryRegistry) GetMigrationByVersion(version string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range r.migrations {
		if migration.Version == version {
			results = append(results, migration)
		}
	}
	return results
}

func (r *inMemoryRegistry) GetMigrationByConnectionAndVersion(connection, version string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range r.migrations {
		if migration.Connection == connection && migration.Version == version {
			results = append(results, migration)
		}
	}
	return results
}

func (r *inMemoryRegistry) getMigrationID(migration *backends.MigrationScript) string {
	// Migration ID format: {version}_{name}_{backend}_{connection}
	return fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
}
