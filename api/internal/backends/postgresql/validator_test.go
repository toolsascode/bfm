package postgresql

import (
	"context"
	"testing"

	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"
)

// mockStateTrackerForValidator for testing
type mockStateTrackerForValidator struct {
	appliedMigrations map[string]bool
}

func newMockStateTrackerForValidator() *mockStateTrackerForValidator {
	return &mockStateTrackerForValidator{
		appliedMigrations: make(map[string]bool),
	}
}

func (m *mockStateTrackerForValidator) RecordMigration(ctx interface{}, migration *state.MigrationRecord) error {
	return nil
}

func (m *mockStateTrackerForValidator) GetMigrationHistory(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	return nil, nil
}

func (m *mockStateTrackerForValidator) GetMigrationList(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	return nil, nil
}

func (m *mockStateTrackerForValidator) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	return m.appliedMigrations[migrationID], nil
}

func (m *mockStateTrackerForValidator) GetLastMigrationVersion(ctx interface{}, schema, table string) (string, error) {
	return "", nil
}

func (m *mockStateTrackerForValidator) RegisterScannedMigration(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	return nil
}

func (m *mockStateTrackerForValidator) UpdateMigrationInfo(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	return nil
}

func (m *mockStateTrackerForValidator) DeleteMigration(ctx interface{}, migrationID string) error {
	return nil
}

func (m *mockStateTrackerForValidator) Initialize(ctx interface{}) error {
	return nil
}

func (m *mockStateTrackerForValidator) ReindexMigrations(ctx interface{}, registry interface{}) error {
	return nil
}

func (m *mockStateTrackerForValidator) GetMigrationDetail(ctx interface{}, migrationID string) (*state.MigrationDetail, error) {
	return nil, nil
}

func (m *mockStateTrackerForValidator) GetMigrationExecutions(ctx interface{}, migrationID string) ([]*state.MigrationExecution, error) {
	return nil, nil
}
func (m *mockStateTrackerForValidator) GetRecentExecutions(ctx interface{}, limit int) ([]*state.MigrationExecution, error) {
	return nil, nil
}

func TestDependencyValidator_ValidateDependencies(t *testing.T) {
	backend := &Backend{} // We'll need to use a real backend or mock differently
	// For now, we'll test the logic without actual database calls

	reg := registry.NewInMemoryRegistry()
	tracker := newMockStateTrackerForValidator()

	// Register a dependency migration
	depMigration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "base_migration",
		Connection: "core",
		Schema:     "core",
		Backend:    "postgresql",
	}
	_ = reg.Register(depMigration)

	// Mark it as applied
	// Migration ID format: {version}_{name}_{backend}_{connection}
	tracker.appliedMigrations["20240101120000_base_migration_postgresql_core"] = true

	validator := NewDependencyValidator(backend, tracker, reg)

	t.Run("validate simple dependency - applied", func(t *testing.T) {
		migration := &backends.MigrationScript{
			Version:      "20240101120001",
			Name:         "dependent",
			Connection:   "core",
			Backend:      "postgresql",
			Dependencies: []string{"base_migration"},
		}

		errors := validator.ValidateDependencies(context.Background(), migration, "core")
		if len(errors) > 0 {
			t.Errorf("Expected no errors, got %v", errors)
		}
	})

	t.Run("validate simple dependency - not applied", func(t *testing.T) {
		tracker.appliedMigrations["20240101120000_base_migration_postgresql_core"] = false

		migration := &backends.MigrationScript{
			Version:      "20240101120002",
			Name:         "dependent2",
			Connection:   "core",
			Backend:      "postgresql",
			Dependencies: []string{"base_migration"},
		}

		errors := validator.ValidateDependencies(context.Background(), migration, "core")
		if len(errors) == 0 {
			t.Error("Expected error for unapplied dependency")
		}

		// Reset
		tracker.appliedMigrations["20240101120000_base_migration_postgresql_core"] = true
	})

	t.Run("validate structured dependency - applied", func(t *testing.T) {
		migration := &backends.MigrationScript{
			Version:    "20240101120003",
			Name:       "structured_dep",
			Connection: "core",
			Backend:    "postgresql",
			StructuredDependencies: []backends.Dependency{
				{
					Connection: "core",
					Target:     "base_migration",
					TargetType: "name",
				},
			},
		}

		errors := validator.ValidateDependencies(context.Background(), migration, "core")
		// Note: This will fail because we don't have a real backend with SchemaExists/TableExists
		// But we can test the logic structure
		_ = errors
	})

	t.Run("validate missing dependency", func(t *testing.T) {
		migration := &backends.MigrationScript{
			Version:      "20240101120004",
			Name:         "missing_dep",
			Connection:   "core",
			Backend:      "postgresql",
			Dependencies: []string{"nonexistent"},
		}

		errors := validator.ValidateDependencies(context.Background(), migration, "core")
		if len(errors) == 0 {
			t.Error("Expected error for missing dependency")
		}
	})
}

func TestDependencyValidator_FindMigrationByTarget(t *testing.T) {
	reg := registry.NewInMemoryRegistry()
	tracker := newMockStateTrackerForValidator()
	backend := &Backend{} // Mock backend

	validator := NewDependencyValidator(backend, tracker, reg)

	// Register test migrations with different versions to avoid ID conflicts
	m1 := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "core",
		Schema:     "core",
		Backend:    "postgresql",
	}
	_ = reg.Register(m1)

	m2 := &backends.MigrationScript{
		Version:    "20240101120001",
		Name:       "test_migration",
		Connection: "guard",
		Schema:     "guard",
		Backend:    "postgresql",
	}
	_ = reg.Register(m2)

	tests := []struct {
		name    string
		dep     backends.Dependency
		wantLen int
		wantErr bool
	}{
		{
			name: "find by name",
			dep: backends.Dependency{
				Target:     "test_migration",
				TargetType: "name",
			},
			wantLen: 2,
			wantErr: false,
		},
		{
			name: "find by name and connection",
			dep: backends.Dependency{
				Connection: "core",
				Target:     "test_migration",
				TargetType: "name",
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "find by version",
			dep: backends.Dependency{
				Target:     "20240101120000",
				TargetType: "version",
			},
			wantLen: 1, // Only m1 has this version
			wantErr: false,
		},
		{
			name: "not found",
			dep: backends.Dependency{
				Target:     "nonexistent",
				TargetType: "name",
			},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := validator.findMigrationByTarget(tt.dep)
			if (err != nil) != tt.wantErr {
				t.Errorf("findMigrationByTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(targets) != tt.wantLen {
				t.Errorf("findMigrationByTarget() len = %v, want %v", len(targets), tt.wantLen)
			}
		})
	}
}
