package main

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/config"
	"github.com/toolsascode/bfm/api/internal/executor"
	"github.com/toolsascode/bfm/api/internal/logger"
	"github.com/toolsascode/bfm/api/internal/registry"
)

// autoMigrateDefaultOn is true when BFM_AUTO_MIGRATE is unset. Set BFM_AUTO_MIGRATE=false to disable.
func autoMigrateEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("BFM_AUTO_MIGRATE")))
	if v == "" {
		return true
	}
	return v != "false" && v != "0" && v != "off" && v != "no"
}

// etcdEndpointsExtraNonEmpty returns true if Extra has a non-empty endpoints value (any key casing).
func etcdEndpointsExtraNonEmpty(extra map[string]string) bool {
	if extra == nil {
		return false
	}
	for k, v := range extra {
		if strings.EqualFold(strings.TrimSpace(k), "endpoints") && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// connectionConfigReadyForAutoMigrate reports whether conn has the minimum fields the corresponding
// backend expects, so we do not dial empty hosts or etcd with no endpoints (avoids log spam).
func connectionConfigReadyForAutoMigrate(conn *backends.ConnectionConfig) bool {
	if conn == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(conn.Backend)) {
	case "postgresql":
		return strings.TrimSpace(conn.Host) != ""
	case "greptimedb":
		return strings.TrimSpace(conn.Host) != ""
	case "etcd":
		if etcdEndpointsExtraNonEmpty(conn.Extra) {
			return true
		}
		return strings.TrimSpace(conn.Host) != "" && strings.TrimSpace(conn.Port) != ""
	default:
		return true
	}
}

// autoMigrateRetryInterval returns the pause between full auto-migrate rounds. If the env value is
// zero or negative, only one round is run (legacy single-pass behavior after the startup delay).
func autoMigrateRetryInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("BFM_AUTO_MIGRATE_RETRY_INTERVAL"))
	if v == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 5 * time.Second
	}
	return d
}

// autoMigrateRetryMaxRounds caps how many full passes run over all ready connections.
// When retryInterval is <= 0, this is forced to 1.
func autoMigrateRetryMaxRounds(retryInterval time.Duration) int {
	if retryInterval <= 0 {
		return 1
	}
	v := strings.TrimSpace(os.Getenv("BFM_AUTO_MIGRATE_RETRY_MAX_ROUNDS"))
	if v == "" {
		return 24
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 24
	}
	return n
}

type autoMigrateConn struct {
	name string
	cfg  *backends.ConnectionConfig
}

func sumPendingAutoMigratable(ctx context.Context, exec *executor.Executor, conns []autoMigrateConn) (int, error) {
	total := 0
	for _, c := range conns {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		n, err := exec.CountPendingAutoMigratable(ctx, c.name, c.cfg.Backend)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// startAutoMigrateBackground runs pending migrations per configured connection after startup,
// retrying in bounded rounds until fixed-schema work is cleared, a stall is detected, or limits hit.
// It uses the same ExecuteUp path as the HTTP API (synchronous execution, not the job queue).
//
// Limitations (documented for operators):
//   - Migrations with dynamic schema (empty migration.Schema) require an explicit schema in the request;
//     auto-migrate passes an empty schema, so those migrations are skipped with an info log until run manually with schemas.
//   - Optional BFM_AUTO_MIGRATE_CONNECTIONS (comma-separated) restricts which connection names are processed;
//     if unset, all connections from config are attempted.
//   - Connections with incomplete config for their backend (e.g. etcd without endpoints or host+port) are skipped.
func startAutoMigrateBackground(ctx context.Context, exec *executor.Executor, cfg *config.Config) {
	if !autoMigrateEnabled() {
		logger.Info("BFM_AUTO_MIGRATE is disabled; skipping startup auto-migrate")
		return
	}

	filterRaw := strings.TrimSpace(os.Getenv("BFM_AUTO_MIGRATE_CONNECTIONS"))
	var allow map[string]bool
	if filterRaw != "" {
		allow = make(map[string]bool)
		for _, p := range strings.Split(filterRaw, ",") {
			k := strings.TrimSpace(strings.ToLower(p))
			if k != "" {
				allow[k] = true
			}
		}
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		retryInterval := autoMigrateRetryInterval()
		maxRounds := autoMigrateRetryMaxRounds(retryInterval)
		if retryInterval > 0 {
			logger.Infof("Auto-migrate: retry enabled (interval=%s, max_rounds=%d)", retryInterval, maxRounds)
		} else {
			logger.Info("Auto-migrate: single round only (BFM_AUTO_MIGRATE_RETRY_INTERVAL is 0 or invalid)")
		}

		connNames := make([]string, 0, len(cfg.Connections))
		for name := range cfg.Connections {
			connNames = append(connNames, name)
		}
		sort.Strings(connNames)

		var toRun []autoMigrateConn
		for _, connName := range connNames {
			if allow != nil && !allow[strings.ToLower(connName)] {
				continue
			}
			connCfg := cfg.Connections[connName]
			if connCfg == nil {
				continue
			}
			if !connectionConfigReadyForAutoMigrate(connCfg) {
				logger.Infof("Auto-migrate: skipping connection %q (backend=%s): incomplete connection config for auto-migrate", connName, connCfg.Backend)
				continue
			}
			toRun = append(toRun, autoMigrateConn{name: connName, cfg: connCfg})
		}

		for round := 1; round <= maxRounds; round++ {
			select {
			case <-ctx.Done():
				logger.Info("Auto-migrate cancelled during shutdown")
				return
			default:
			}

			pendingBefore, err := sumPendingAutoMigratable(ctx, exec, toRun)
			if err != nil {
				logger.Errorf("Auto-migrate: failed to count pending migrations: %v", err)
				break
			}
			if pendingBefore == 0 {
				logger.Info("Auto-migrate: no pending fixed-schema migrations for ready connections")
				logger.Info("Auto-migrate: startup pass completed")
				return
			}

			logger.Infof("Auto-migrate: round %d/%d (%d pending fixed-schema migration(s) across ready connections)", round, maxRounds, pendingBefore)

			anyApplied := false
			anyErr := false
			for _, cr := range toRun {
				select {
				case <-ctx.Done():
					logger.Info("Auto-migrate cancelled during shutdown")
					return
				default:
				}

				target := &registry.MigrationTarget{
					Backend:    cr.cfg.Backend,
					Connection: cr.name,
				}
				runCtx := executor.WithAutoMigrateContext(executor.SetExecutionContext(context.Background(), "bfm-server", "auto_migrate", map[string]interface{}{
					"connection": cr.name,
					"source":     "BFM_AUTO_MIGRATE",
					"round":      round,
				}))

				logger.Infof("Auto-migrate: running pending migrations for connection %q (backend=%s)", cr.name, cr.cfg.Backend)
				result, err := exec.ExecuteUp(runCtx, target, cr.name, []string{""}, false, false)
				if err != nil {
					anyErr = true
					logger.Errorf("Auto-migrate: ExecuteUp failed for connection %q: %v", cr.name, err)
					continue
				}
				if len(result.Applied) > 0 {
					anyApplied = true
					logger.Infof("Auto-migrate: applied for %q: %v", cr.name, result.Applied)
				}
				if len(result.Skipped) > 0 {
					logger.Debug("Auto-migrate: skipped for %q (already applied): %v", cr.name, result.Skipped)
				}
				if len(result.Errors) > 0 {
					anyErr = true
					for _, e := range result.Errors {
						logger.Warnf("Auto-migrate: error for %q: %s", cr.name, e)
					}
				}
			}

			pendingAfter, err := sumPendingAutoMigratable(ctx, exec, toRun)
			if err != nil {
				logger.Errorf("Auto-migrate: failed to count pending migrations after round: %v", err)
				break
			}
			if pendingAfter == 0 {
				logger.Info("Auto-migrate: all auto-migratable migrations applied")
				logger.Info("Auto-migrate: startup pass completed")
				return
			}
			if !anyApplied && !anyErr && pendingAfter == pendingBefore {
				logger.Warnf("Auto-migrate: no progress after round %d (%d pending fixed-schema migration(s) unchanged); check backend/connection alignment and logs. Stopping retries.", round, pendingAfter)
				logger.Info("Auto-migrate: startup pass completed")
				return
			}
			if round == maxRounds {
				logger.Warnf("Auto-migrate: reached max rounds (%d); %d pending auto-migratable migration(s) remain", maxRounds, pendingAfter)
				logger.Info("Auto-migrate: startup pass completed")
				return
			}
			if retryInterval <= 0 {
				logger.Info("Auto-migrate: startup pass completed")
				return
			}
			select {
			case <-ctx.Done():
				logger.Info("Auto-migrate cancelled during shutdown")
				return
			case <-time.After(retryInterval):
			}
		}

		logger.Info("Auto-migrate: startup pass completed")
	}()
}
