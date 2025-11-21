package migrations

import "bfm/api/internal/backends"

// MigrationScript represents a database migration script with up and down SQL.
// MigrationScript is a public alias for backends.MigrationScript that allows
// migration files outside the bfm module to use this type when registering migrations.
type MigrationScript = backends.MigrationScript
