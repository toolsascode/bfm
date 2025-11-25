package registry

import (
	"testing"

	"bfm/api/internal/backends"
	"bfm/api/internal/state"
)

// mockStateTracker for testing
type mockStateTracker struct {
	appliedMigrations map[string]bool
}

func newMockStateTracker() *mockStateTracker {
	return &mockStateTracker{
		appliedMigrations: make(map[string]bool),
	}
}

func (m *mockStateTracker) RecordMigration(ctx interface{}, migration *state.MigrationRecord) error {
	return nil
}

func (m *mockStateTracker) GetMigrationHistory(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	return nil, nil
}

func (m *mockStateTracker) GetMigrationList(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	return nil, nil
}

func (m *mockStateTracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	return m.appliedMigrations[migrationID], nil
}

func (m *mockStateTracker) GetLastMigrationVersion(ctx interface{}, schema, table string) (string, error) {
	return "", nil
}

func (m *mockStateTracker) RegisterScannedMigration(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	return nil
}

func (m *mockStateTracker) UpdateMigrationInfo(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	return nil
}

func (m *mockStateTracker) DeleteMigration(ctx interface{}, migrationID string) error {
	return nil
}

func (m *mockStateTracker) Initialize(ctx interface{}) error {
	return nil
}

func TestDependencyGraph_AddNode(t *testing.T) {
	graph := NewDependencyGraph()
	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
	}

	graph.AddNode(migration, "test_id")
	if len(graph.nodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(graph.nodes))
	}
	if graph.nodes["test_id"] == nil {
		t.Error("Node not added correctly")
	}
}

func TestDependencyGraph_AddEdge(t *testing.T) {
	graph := NewDependencyGraph()
	m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
	m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}

	graph.AddNode(m1, "m1")
	graph.AddNode(m2, "m2")
	graph.AddEdge("m1", "m2") // m1 depends on m2

	if len(graph.edges["m1"]) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(graph.edges["m1"]))
	}
	if graph.edges["m1"][0] != "m2" {
		t.Errorf("Expected edge to m2, got %s", graph.edges["m1"][0])
	}
}

func TestDependencyGraph_DetectCycles(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *DependencyGraph
		wantCycle bool
	}{
		{
			name: "no cycle",
			setup: func() *DependencyGraph {
				graph := NewDependencyGraph()
				m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
				m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}
				graph.AddNode(m1, "m1")
				graph.AddNode(m2, "m2")
				graph.AddEdge("m1", "m2") // m1 depends on m2
				return graph
			},
			wantCycle: false,
		},
		{
			name: "simple cycle",
			setup: func() *DependencyGraph {
				graph := NewDependencyGraph()
				m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
				m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}
				graph.AddNode(m1, "m1")
				graph.AddNode(m2, "m2")
				graph.AddEdge("m1", "m2") // m1 depends on m2
				graph.AddEdge("m2", "m1") // m2 depends on m1 (cycle!)
				return graph
			},
			wantCycle: true,
		},
		{
			name: "three node cycle",
			setup: func() *DependencyGraph {
				graph := NewDependencyGraph()
				m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
				m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}
				m3 := &backends.MigrationScript{Version: "20240101120002", Name: "m3"}
				graph.AddNode(m1, "m1")
				graph.AddNode(m2, "m2")
				graph.AddNode(m3, "m3")
				graph.AddEdge("m1", "m2")
				graph.AddEdge("m2", "m3")
				graph.AddEdge("m3", "m1") // cycle!
				return graph
			},
			wantCycle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := tt.setup()
			cyclePath, err := graph.DetectCycles()
			hasCycle := err != nil || len(cyclePath) > 0
			if hasCycle != tt.wantCycle {
				t.Errorf("DetectCycles() cycle = %v, want %v (error: %v, path: %v)", hasCycle, tt.wantCycle, err, cyclePath)
			}
		})
	}
}

func TestDependencyGraph_TopologicalSort(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *DependencyGraph
		wantErr bool
		wantLen int
	}{
		{
			name: "simple linear dependencies",
			setup: func() *DependencyGraph {
				graph := NewDependencyGraph()
				m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
				m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}
				m3 := &backends.MigrationScript{Version: "20240101120002", Name: "m3"}
				graph.AddNode(m1, "m1")
				graph.AddNode(m2, "m2")
				graph.AddNode(m3, "m3")
				graph.AddEdge("m2", "m1") // m2 depends on m1
				graph.AddEdge("m3", "m2") // m3 depends on m2
				return graph
			},
			wantErr: false,
			wantLen: 3,
		},
		{
			name: "no dependencies",
			setup: func() *DependencyGraph {
				graph := NewDependencyGraph()
				m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
				m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}
				graph.AddNode(m1, "m1")
				graph.AddNode(m2, "m2")
				return graph
			},
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "circular dependency",
			setup: func() *DependencyGraph {
				graph := NewDependencyGraph()
				m1 := &backends.MigrationScript{Version: "20240101120000", Name: "m1"}
				m2 := &backends.MigrationScript{Version: "20240101120001", Name: "m2"}
				graph.AddNode(m1, "m1")
				graph.AddNode(m2, "m2")
				graph.AddEdge("m1", "m2")
				graph.AddEdge("m2", "m1") // cycle!
				return graph
			},
			wantErr: true,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := tt.setup()
			sorted, err := graph.TopologicalSort()
			if (err != nil) != tt.wantErr {
				t.Errorf("TopologicalSort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(sorted) != tt.wantLen {
				t.Errorf("TopologicalSort() len = %v, want %v", len(sorted), tt.wantLen)
			}
			if !tt.wantErr && len(sorted) > 0 {
				// Verify order: dependencies come before dependents
				// For linear case: m1 should come before m2, m2 before m3
				if tt.name == "simple linear dependencies" {
					m1Index := -1
					m2Index := -1
					m3Index := -1
					for i, m := range sorted {
						if m.Name == "m1" {
							m1Index = i
						}
						if m.Name == "m2" {
							m2Index = i
						}
						if m.Name == "m3" {
							m3Index = i
						}
					}
					if m1Index >= m2Index || m2Index >= m3Index {
						t.Errorf("Incorrect order: m1 at %d, m2 at %d, m3 at %d", m1Index, m2Index, m3Index)
					}
				}
			}
		})
	}
}

func TestDependencyResolver_FindDependencyTarget(t *testing.T) {
	reg := NewInMemoryRegistry()
	tracker := newMockStateTracker()
	resolver := NewDependencyResolver(reg, tracker)

	// Register test migrations
	m1 := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "bootstrap",
		Connection: "core",
		Schema:     "core",
		Backend:    "postgresql",
	}
	_ = reg.Register(m1)

	m2 := &backends.MigrationScript{
		Version:    "20240101120001",
		Name:       "bootstrap",
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
				Target:     "bootstrap",
				TargetType: "name",
			},
			wantLen: 2, // Should find both migrations with name "bootstrap"
			wantErr: false,
		},
		{
			name: "find by name and connection",
			dep: backends.Dependency{
				Connection: "core",
				Target:     "bootstrap",
				TargetType: "name",
			},
			wantLen: 1, // Should find only core connection
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
			name: "find by version and connection",
			dep: backends.Dependency{
				Connection: "core",
				Target:     "20240101120000",
				TargetType: "version",
			},
			wantLen: 1,
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
			targets, err := resolver.findDependencyTarget(tt.dep)
			if (err != nil) != tt.wantErr {
				t.Errorf("findDependencyTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(targets) != tt.wantLen {
				t.Errorf("findDependencyTarget() len = %v, want %v", len(targets), tt.wantLen)
			}
		})
	}
}

func TestDependencyResolver_ResolveDependencies(t *testing.T) {
	reg := NewInMemoryRegistry()
	tracker := newMockStateTracker()
	resolver := NewDependencyResolver(reg, tracker)

	// Create migrations
	m1 := &backends.MigrationScript{
		Version:      "20240101120000",
		Name:         "base",
		Connection:   "core",
		Backend:      "postgresql",
		Dependencies: []string{},
	}
	_ = reg.Register(m1)

	m2 := &backends.MigrationScript{
		Version:      "20240101120001",
		Name:         "dependent",
		Connection:   "core",
		Backend:      "postgresql",
		Dependencies: []string{"base"},
	}
	_ = reg.Register(m2)

	getMigrationID := func(m *backends.MigrationScript) string {
		return m.Version + "_" + m.Name
	}

	t.Run("resolve simple dependencies", func(t *testing.T) {
		migrations := []*backends.MigrationScript{m1, m2}
		sorted, err := resolver.ResolveDependencies(migrations, getMigrationID)
		if err != nil {
			t.Fatalf("ResolveDependencies() error = %v", err)
		}
		if len(sorted) != 2 {
			t.Fatalf("ResolveDependencies() len = %v, want 2", len(sorted))
		}
		// base should come before dependent
		if sorted[0].Name != "base" {
			t.Errorf("Expected base first, got %s", sorted[0].Name)
		}
		if sorted[1].Name != "dependent" {
			t.Errorf("Expected dependent second, got %s", sorted[1].Name)
		}
	})

	t.Run("resolve structured dependencies", func(t *testing.T) {
		m3 := &backends.MigrationScript{
			Version:    "20240101120002",
			Name:       "structured_dep",
			Connection: "core",
			Backend:    "postgresql",
			StructuredDependencies: []backends.Dependency{
				{
					Connection: "core",
					Target:     "base",
					TargetType: "name",
				},
			},
		}
		_ = reg.Register(m3)

		migrations := []*backends.MigrationScript{m1, m3}
		sorted, err := resolver.ResolveDependencies(migrations, getMigrationID)
		if err != nil {
			t.Fatalf("ResolveDependencies() error = %v", err)
		}
		if len(sorted) != 2 {
			t.Fatalf("ResolveDependencies() len = %v, want 2", len(sorted))
		}
		if sorted[0].Name != "base" {
			t.Errorf("Expected base first, got %s", sorted[0].Name)
		}
	})

	t.Run("missing dependency", func(t *testing.T) {
		m4 := &backends.MigrationScript{
			Version:      "20240101120003",
			Name:         "missing_dep",
			Connection:   "core",
			Backend:      "postgresql",
			Dependencies: []string{"nonexistent"},
		}
		migrations := []*backends.MigrationScript{m4}
		_, err := resolver.ResolveDependencies(migrations, getMigrationID)
		if err == nil {
			t.Error("Expected error for missing dependency")
		}
	})
}
