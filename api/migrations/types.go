package migrations

import "github.com/toolsascode/bfm/api/internal/backends"

// MigrationScript represents a database migration script with up and down SQL.
// MigrationScript is a public alias for backends.MigrationScript that allows
// migration files outside the bfm module to use this type when registering migrations.
type MigrationScript = backends.MigrationScript

// Dependency represents a structured dependency on another migration.
// Dependency is a public alias for backends.Dependency that allows
// migration files outside the bfm module to use this type when declaring dependencies.
type Dependency = backends.Dependency
