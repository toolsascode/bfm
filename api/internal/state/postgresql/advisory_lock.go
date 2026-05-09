package postgresql

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/toolsascode/bfm/api/internal/state"
)

// migrationAdvisoryLockKeys derives two int4 keys for pg_try_advisory_lock from execution identity.
func migrationAdvisoryLockKeys(migrationID, schema, connection string) (int32, int32) {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s", migrationID, schema, connection)
	v := h.Sum64()
	return int32(v >> 32), int32(v & 0xffffffff)
}

// WithMigrationExecutionLock runs fn while holding a session-level advisory lock on the state DB.
// The lock is per (migration_id, execution schema, connection) so the same migration can run for different schemas concurrently.
func (t *Tracker) WithMigrationExecutionLock(ctx interface{}, migrationID, schema, connection string, fn func() error) error {
	ctxVal := ctx.(context.Context)

	conn, err := t.pool.Acquire(ctxVal)
	if err != nil {
		return fmt.Errorf("acquire connection for migration lock: %w", err)
	}

	k1, k2 := migrationAdvisoryLockKeys(migrationID, schema, connection)
	var ok bool
	if err := conn.QueryRow(ctxVal, `SELECT pg_try_advisory_lock($1::integer, $2::integer)`, k1, k2).Scan(&ok); err != nil {
		conn.Release()
		return fmt.Errorf("pg_try_advisory_lock: %w", err)
	}
	if !ok {
		conn.Release()
		return state.ErrMigrationAlreadyInProgress
	}

	defer func() {
		ctxUnlock := context.Background()
		_, _ = conn.Exec(ctxUnlock, `SELECT pg_advisory_unlock($1::integer, $2::integer)`, k1, k2)
		conn.Release()
	}()

	return fn()
}
