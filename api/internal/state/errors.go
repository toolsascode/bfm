package state

import "errors"

// ErrMigrationAlreadyInProgress is returned when another process holds the execution
// lock for the same migration key (migration_id + schema + connection).
var ErrMigrationAlreadyInProgress = errors.New("migration is already being executed")
