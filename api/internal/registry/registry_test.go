package registry

import (
	"testing"

	"bfm/api/internal/backends"
)

func TestNewInMemoryRegistry(t *testing.T) {
	reg := NewInMemoryRegistry()
	if reg == nil {
		t.Fatal("NewInMemoryRegistry() returned nil")
	}
}

func TestInMemoryRegistry_Register(t *testing.T) {
	reg := NewInMemoryRegistry()

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}

	err := reg.Register(migration)
	if err != nil {
		t.Errorf("Register() error = %v", err)
	}

	all := reg.GetAll()
	if len(all) != 1 {
		t.Errorf("Expected 1 migration, got %v", len(all))
	}
}

func TestInMemoryRegistry_FindByTarget(t *testing.T) {
	reg := NewInMemoryRegistry()

	migration1 := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "migration1",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test1;",
	}
	_ = reg.Register(migration1)

	migration2 := &backends.MigrationScript{
		Version:    "20240101120001",
		Name:       "migration2",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test2;",
	}
	_ = reg.Register(migration2)

	migration3 := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "migration3",
		Connection: "other",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test3;",
	}
	_ = reg.Register(migration3)

	tests := []struct {
		name     string
		target   *MigrationTarget
		wantLen  int
		wantName string
	}{
		{
			name: "filter by connection",
			target: &MigrationTarget{
				Connection: "test",
			},
			wantLen: 2,
		},
		{
			name: "filter by backend",
			target: &MigrationTarget{
				Backend: "postgresql",
			},
			wantLen: 3,
		},
		{
			name: "filter by connection and backend",
			target: &MigrationTarget{
				Connection: "test",
				Backend:    "postgresql",
			},
			wantLen: 2,
		},
		{
			name: "filter by version",
			target: &MigrationTarget{
				Version: "20240101120000",
			},
			wantLen: 2,
		},
		{
			name: "filter by schema",
			target: &MigrationTarget{
				Schema: "public",
			},
			wantLen: 0,
		},
		{
			name: "filter by tables",
			target: &MigrationTarget{
				Tables: []string{"users"},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := reg.FindByTarget(tt.target)
			if err != nil {
				t.Errorf("FindByTarget() error = %v", err)
			}
			if len(results) != tt.wantLen {
				t.Errorf("Expected %d results, got %d", tt.wantLen, len(results))
			}
		})
	}
}

func TestInMemoryRegistry_GetAll(t *testing.T) {
	reg := NewInMemoryRegistry()

	if len(reg.GetAll()) != 0 {
		t.Error("Expected empty registry initially")
	}

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	all := reg.GetAll()
	if len(all) != 1 {
		t.Errorf("Expected 1 migration, got %v", len(all))
	}
}

func TestInMemoryRegistry_GetByConnection(t *testing.T) {
	reg := NewInMemoryRegistry()

	migration1 := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "migration1",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test1;",
	}
	_ = reg.Register(migration1)

	migration2 := &backends.MigrationScript{
		Version:    "20240101120001",
		Name:       "migration2",
		Connection: "other",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test2;",
	}
	_ = reg.Register(migration2)

	results := reg.GetByConnection("test")
	if len(results) != 1 {
		t.Errorf("Expected 1 migration for connection 'test', got %v", len(results))
	}
	if results[0].Name != "migration1" {
		t.Errorf("Expected migration1, got %v", results[0].Name)
	}

	results = reg.GetByConnection("nonexistent")
	if len(results) != 0 {
		t.Errorf("Expected 0 migrations for nonexistent connection, got %v", len(results))
	}
}

func TestInMemoryRegistry_GetByBackend(t *testing.T) {
	reg := NewInMemoryRegistry()

	migration1 := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "migration1",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test1;",
	}
	_ = reg.Register(migration1)

	migration2 := &backends.MigrationScript{
		Version:    "20240101120001",
		Name:       "migration2",
		Connection: "test",
		Backend:    "mysql",
		UpSQL:      "CREATE TABLE test2;",
	}
	_ = reg.Register(migration2)

	results := reg.GetByBackend("postgresql")
	if len(results) != 1 {
		t.Errorf("Expected 1 migration for backend 'postgresql', got %v", len(results))
	}
	if results[0].Name != "migration1" {
		t.Errorf("Expected migration1, got %v", results[0].Name)
	}

	results = reg.GetByBackend("nonexistent")
	if len(results) != 0 {
		t.Errorf("Expected 0 migrations for nonexistent backend, got %v", len(results))
	}
}

func TestInMemoryRegistry_FindByTarget_WithSchema(t *testing.T) {
	reg := NewInMemoryRegistry()

	migration1 := &backends.MigrationScript{
		Schema:     "public",
		Version:    "20240101120000",
		Name:       "migration1",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test1;",
	}
	_ = reg.Register(migration1)

	migration2 := &backends.MigrationScript{
		Schema:     "private",
		Version:    "20240101120000",
		Name:       "migration2",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test2;",
	}
	_ = reg.Register(migration2)

	target := &MigrationTarget{
		Schema:     "public",
		Connection: "test",
	}

	results, err := reg.FindByTarget(target)
	if err != nil {
		t.Errorf("FindByTarget() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %v", len(results))
	}
	if results[0].Name != "migration1" {
		t.Errorf("Expected migration1, got %v", results[0].Name)
	}
}

func TestInMemoryRegistry_FindByTarget_WithTables(t *testing.T) {
	reg := NewInMemoryRegistry()

	tableName := "users"
	migration1 := &backends.MigrationScript{
		Schema:     "public",
		Table:      &tableName,
		Version:    "20240101120000",
		Name:       "migration1",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE users;",
	}
	_ = reg.Register(migration1)

	tableName2 := "posts"
	migration2 := &backends.MigrationScript{
		Schema:     "public",
		Table:      &tableName2,
		Version:    "20240101120000",
		Name:       "migration2",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE posts;",
	}
	_ = reg.Register(migration2)

	target := &MigrationTarget{
		Tables: []string{"users"},
	}

	results, err := reg.FindByTarget(target)
	if err != nil {
		t.Errorf("FindByTarget() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %v", len(results))
	}
	if results[0].Name != "migration1" {
		t.Errorf("Expected migration1, got %v", results[0].Name)
	}
}
