package executor

import (
	"context"
	"testing"

	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"
)

// fakeStateTracker implements the minimal methods we need from state.StateTracker
// for testing expandWithPendingDependencies without hitting a real database.
type fakeStateTracker struct {
	applied map[string]bool
}

func (f *fakeStateTracker) IsMigrationApplied(_ interface{}, migrationID string) (bool, error) {
	return f.applied[migrationID], nil
}

// The remaining methods are not used in these tests; provide empty implementations
// to satisfy the interface.

func (f *fakeStateTracker) Initialize(_ interface{}) error                                { return nil }
func (f *fakeStateTracker) RecordMigration(_ interface{}, _ *state.MigrationRecord) error { return nil }
func (f *fakeStateTracker) GetMigrationHistory(_ interface{}, _ *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	return nil, nil
}
func (f *fakeStateTracker) GetMigrationList(_ interface{}, _ *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	return nil, nil
}
func (f *fakeStateTracker) RegisterScannedMigration(_ interface{}, _ string, _ string, _ string, _ string, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeStateTracker) UpdateMigrationInfo(_ interface{}, _ string, _ string, _ string, _ string, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeStateTracker) GetLastMigrationVersion(_ interface{}, _ string, _ string) (string, error) {
	return "", nil
}
func (f *fakeStateTracker) DeleteMigration(_ interface{}, _ string) error { return nil }
func (f *fakeStateTracker) Close() error                                  { return nil }

// fakeRegistry provides a minimal Registry for the dependency resolver.
type fakeRegistry struct {
	migrations []*backends.MigrationScript
}

func (r *fakeRegistry) GetAll() []*backends.MigrationScript {
	return r.migrations
}

func (r *fakeRegistry) GetMigrationByName(name string) []*backends.MigrationScript {
	var out []*backends.MigrationScript
	for _, m := range r.migrations {
		if m.Name == name {
			out = append(out, m)
		}
	}
	return out
}

// The remaining Registry methods are not needed for this test.
func (r *fakeRegistry) Register(_ *backends.MigrationScript) error { return nil }
func (r *fakeRegistry) FindByTarget(_ *registry.MigrationTarget) ([]*backends.MigrationScript, error) {
	return nil, nil
}
func (r *fakeRegistry) GetByConnection(_ string) []*backends.MigrationScript       { return nil }
func (r *fakeRegistry) GetByBackend(_ string) []*backends.MigrationScript          { return nil }
func (r *fakeRegistry) GetMigrationByVersion(_ string) []*backends.MigrationScript { return nil }
func (r *fakeRegistry) GetMigrationByConnectionAndVersion(_, _ string) []*backends.MigrationScript {
	return nil
}

// TestExpandWithPendingDependenciesCrossConnection verifies that when we execute
// a migration on one connection that depends (via structured dependency) on a
// migration in another connection, the dependency migration is automatically
// included in the execution plan if it is still pending.
func TestExpandWithPendingDependenciesCrossConnection(t *testing.T) {
	// Dependency migration in guard connection (e.g. guard_sso_create_sso_tables).
	guardDep := &backends.MigrationScript{
		Schema:     "guard",
		Version:    "20251222222820",
		Name:       "guard_sso_create_sso_tables",
		Connection: "guard",
		Backend:    "postgresql",
	}

	// Core migration that depends on the guard migration via structured dependency.
	coreMigration := &backends.MigrationScript{
		Schema:     "core",
		Version:    "20251222222821",
		Name:       "core_organizations_add_sso_fields",
		Connection: "core",
		Backend:    "postgresql",
		StructuredDependencies: []backends.Dependency{
			{
				Connection:    "guard",
				Schema:        "guard",
				Target:        "20251222222820",
				TargetType:    "version",
				RequiresTable: "sso_providers",
			},
		},
	}

	reg := &fakeRegistry{
		migrations: []*backends.MigrationScript{guardDep, coreMigration},
	}

	// State tracker reports that neither migration has been applied yet.
	tracker := &fakeStateTracker{
		applied: map[string]bool{},
	}

	exec := &Executor{
		registry:     reg,
		stateTracker: tracker,
	}

	ctx := context.Background()

	expanded, err := exec.expandWithPendingDependencies(ctx, []*backends.MigrationScript{coreMigration})
	if err != nil {
		t.Fatalf("expandWithPendingDependencies returned error: %v", err)
	}

	if len(expanded) != 2 {
		t.Fatalf("expected 2 migrations after expansion, got %d", len(expanded))
	}

	// Ensure both core and guard migrations are present.
	foundGuard := false
	foundCore := false
	for _, m := range expanded {
		if m == guardDep {
			foundGuard = true
		}
		if m == coreMigration {
			foundCore = true
		}
	}

	if !foundGuard || !foundCore {
		t.Fatalf("expanded set missing expected migrations: guard=%v core=%v", foundGuard, foundCore)
	}
}
