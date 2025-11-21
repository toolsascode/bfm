package migrations

import "bfm/api/internal/registry"

// GlobalRegistry provides public access to the global migration registry.
// GlobalRegistry allows migration files outside the bfm module to register
// migrations by accessing this exported variable.
var GlobalRegistry = registry.GlobalRegistry
