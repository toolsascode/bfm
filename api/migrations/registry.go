package migrations

import "bfm/api/internal/registry"

// GlobalRegistry is a public accessor to the global migration registry
// This allows migration files outside the bfm module to register migrations
var GlobalRegistry = registry.GlobalRegistry
