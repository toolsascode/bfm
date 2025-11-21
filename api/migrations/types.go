package migrations

import "bfm/api/internal/backends"

// MigrationScript is a public alias for backends.MigrationScript
// This allows migration files outside the bfm module to use this type
type MigrationScript = backends.MigrationScript
