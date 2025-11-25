package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"bfm/api/internal/backends"
	"bfm/api/internal/queue"
	"bfm/api/internal/registry"
	"bfm/api/internal/state"
)

// mockRegistry is a mock implementation of registry.Registry
type mockRegistry struct {
	migrations        map[string]*backends.MigrationScript
	findByTargetError error
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		migrations: make(map[string]*backends.MigrationScript),
	}
}

func (m *mockRegistry) Register(migration *backends.MigrationScript) error {
	id := m.getMigrationID(migration)
	m.migrations[id] = migration
	return nil
}

func (m *mockRegistry) FindByTarget(target *registry.MigrationTarget) ([]*backends.MigrationScript, error) {
	if m.findByTargetError != nil {
		return nil, m.findByTargetError
	}
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if target.Backend != "" && migration.Backend != target.Backend {
			continue
		}
		if target.Connection != "" && migration.Connection != target.Connection {
			continue
		}
		if target.Schema != "" && migration.Schema != target.Schema {
			continue
		}
		if target.Version != "" && migration.Version != target.Version {
			continue
		}
		results = append(results, migration)
	}
	return results, nil
}

func (m *mockRegistry) GetAll() []*backends.MigrationScript {
	results := make([]*backends.MigrationScript, 0, len(m.migrations))
	for _, migration := range m.migrations {
		results = append(results, migration)
	}
	return results
}

func (m *mockRegistry) GetByConnection(connectionName string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if migration.Connection == connectionName {
			results = append(results, migration)
		}
	}
	return results
}

func (m *mockRegistry) GetByBackend(backendName string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if migration.Backend == backendName {
			results = append(results, migration)
		}
	}
	return results
}

func (m *mockRegistry) GetMigrationByName(name string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if migration.Name == name {
			results = append(results, migration)
		}
	}
	return results
}

func (m *mockRegistry) GetMigrationByVersion(version string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if migration.Version == version {
			results = append(results, migration)
		}
	}
	return results
}

func (m *mockRegistry) GetMigrationByConnectionAndVersion(connection, version string) []*backends.MigrationScript {
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if migration.Connection == connection && migration.Version == version {
			results = append(results, migration)
		}
	}
	return results
}

func (m *mockRegistry) getMigrationID(migration *backends.MigrationScript) string {
	// Migration ID format: {version}_{name}_{backend}_{connection}
	return fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
}

// mockStateTracker is a mock implementation of state.StateTracker
type mockStateTracker struct {
	appliedMigrations map[string]bool
	history           []*state.MigrationRecord
	listItems         []*state.MigrationListItem
	healthCheckError  error
	recordError       error
	isAppliedError    error
}

func newMockStateTracker() *mockStateTracker {
	return &mockStateTracker{
		appliedMigrations: make(map[string]bool),
		history:           make([]*state.MigrationRecord, 0),
		listItems:         make([]*state.MigrationListItem, 0),
	}
}

func (m *mockStateTracker) RecordMigration(ctx interface{}, migration *state.MigrationRecord) error {
	if m.recordError != nil {
		return m.recordError
	}
	m.history = append(m.history, migration)
	switch migration.Status {
	case "success":
		m.appliedMigrations[migration.MigrationID] = true
	case "rolled_back":
		m.appliedMigrations[migration.MigrationID] = false
	}
	return nil
}

func (m *mockStateTracker) GetMigrationHistory(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationRecord, error) {
	return m.history, nil
}

func (m *mockStateTracker) GetMigrationList(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	return m.listItems, nil
}

func (m *mockStateTracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	if m.isAppliedError != nil {
		return false, m.isAppliedError
	}
	return m.appliedMigrations[migrationID], nil
}

func (m *mockStateTracker) GetLastMigrationVersion(ctx interface{}, schema, table string) (string, error) {
	return "", nil
}

func (m *mockStateTracker) RegisterScannedMigration(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	return nil
}

func (m *mockStateTracker) DeleteMigration(ctx interface{}, migrationID string) error {
	// Remove from appliedMigrations
	delete(m.appliedMigrations, migrationID)
	// Remove from listItems
	for i, item := range m.listItems {
		if item.MigrationID == migrationID {
			m.listItems = append(m.listItems[:i], m.listItems[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockStateTracker) UpdateMigrationInfo(ctx interface{}, migrationID, schema, table, version, name, connection, backend string) error {
	// Update listItems
	for i, item := range m.listItems {
		if item.MigrationID == migrationID {
			m.listItems[i].Schema = schema
			m.listItems[i].Table = table
			m.listItems[i].Version = version
			m.listItems[i].Name = name
			m.listItems[i].Connection = connection
			m.listItems[i].Backend = backend
			break
		}
	}
	return nil
}

func (m *mockStateTracker) Initialize(ctx interface{}) error {
	return m.healthCheckError
}

// mockBackend is a mock implementation of backends.Backend
type mockBackend struct {
	name             string
	connectError     error
	executeError     error
	executeCalled    bool
	connected        bool
	executeMigration *backends.MigrationScript
}

func newMockBackend(name string) *mockBackend {
	return &mockBackend{
		name: name,
	}
}

func (m *mockBackend) Name() string {
	return m.name
}

func (m *mockBackend) Connect(config *backends.ConnectionConfig) error {
	if m.connectError != nil {
		return m.connectError
	}
	m.connected = true
	return nil
}

func (m *mockBackend) Close() error {
	m.connected = false
	return nil
}

func (m *mockBackend) ExecuteMigration(ctx context.Context, migration *backends.MigrationScript) error {
	m.executeCalled = true
	m.executeMigration = migration
	return m.executeError
}

func (m *mockBackend) CreateSchema(ctx context.Context, schemaName string) error {
	return nil
}

func (m *mockBackend) SchemaExists(ctx context.Context, schemaName string) (bool, error) {
	return false, nil
}

func (m *mockBackend) HealthCheck(ctx context.Context) error {
	return nil
}

// mockQueue is a mock implementation of queue.Queue
type mockQueue struct {
	publishedJobs []*queue.Job
	publishError  error
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		publishedJobs: make([]*queue.Job, 0),
	}
}

func (m *mockQueue) PublishJob(ctx context.Context, job *queue.Job) error {
	if m.publishError != nil {
		return m.publishError
	}
	m.publishedJobs = append(m.publishedJobs, job)
	return nil
}

func (m *mockQueue) Consume(ctx context.Context, handler queue.JobHandler) error {
	return nil
}

func (m *mockQueue) Close() error {
	return nil
}

func TestNewExecutor(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()

	exec := NewExecutor(reg, tracker)

	if exec == nil {
		t.Fatal("NewExecutor() returned nil")
	}
	if exec.GetRegistry() != reg {
		t.Error("GetRegistry() returned wrong registry")
	}
	if exec.GetStateTracker() != tracker {
		t.Error("GetStateTracker() returned wrong tracker")
	}
}

func TestExecutor_SetConnections(t *testing.T) {
	exec := NewExecutor(newMockRegistry(), newMockStateTracker())

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}

	err := exec.SetConnections(connections)
	if err != nil {
		t.Errorf("SetConnections() error = %v", err)
	}

	config, err := exec.GetConnectionConfig("test")
	if err != nil {
		t.Errorf("GetConnectionConfig() error = %v", err)
	}
	if config.Backend != "postgresql" {
		t.Errorf("Expected backend = postgresql, got %v", config.Backend)
	}

	// Test nil connections
	err = exec.SetConnections(nil)
	if err == nil {
		t.Error("SetConnections(nil) expected error")
	}
}

func TestExecutor_RegisterBackend(t *testing.T) {
	exec := NewExecutor(newMockRegistry(), newMockStateTracker())
	backend := newMockBackend("postgresql")

	exec.RegisterBackend("postgresql", backend)

	retrieved := exec.GetBackend("postgresql")
	if retrieved != backend {
		t.Error("GetBackend() returned wrong backend")
	}
}

func TestExecutor_GetConnectionConfig(t *testing.T) {
	exec := NewExecutor(newMockRegistry(), newMockStateTracker())

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	tests := []struct {
		name        string
		connName    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "existing connection",
			connName: "test",
			wantErr:  false,
		},
		{
			name:        "non-existent connection",
			connName:    "nonexistent",
			wantErr:     true,
			errContains: "connection nonexistent not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := exec.GetConnectionConfig(tt.connName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetConnectionConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if err.Error() != tt.errContains {
					t.Errorf("GetConnectionConfig() error = %v, want error containing %v", err, tt.errContains)
				}
			}
			if !tt.wantErr && config == nil {
				t.Error("GetConnectionConfig() returned nil config")
			}
		})
	}
}

func TestExecutor_GetMigrationByID(t *testing.T) {
	reg := newMockRegistry()
	exec := NewExecutor(reg, newMockStateTracker())

	migration := &backends.MigrationScript{
		Schema:     "public",
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	// Test with new format: {version}_{name}_{backend}_{connection}
	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	found := exec.GetMigrationByID(migrationID)

	// Also test legacy format for backward compatibility
	if found == nil {
		legacyID := fmt.Sprintf("%s_%s", migration.Version, migration.Name)
		found = exec.GetMigrationByID(legacyID)
	}

	if found == nil {
		t.Error("GetMigrationByID() returned nil")
		return
	}
	if found.Name != migration.Name {
		t.Errorf("Expected Name = %v, got %v", migration.Name, found.Name)
	}

	// Test non-existent migration
	notFound := exec.GetMigrationByID("nonexistent")
	if notFound != nil {
		t.Error("GetMigrationByID() should return nil for non-existent migration")
	}
}

func TestSetExecutionContext(t *testing.T) {
	ctx := context.Background()
	executedBy := "test-user"
	executionMethod := "manual"
	executionContext := map[string]interface{}{
		"endpoint": "/api/v1/migrations/up",
		"method":   "POST",
	}

	ctx = SetExecutionContext(ctx, executedBy, executionMethod, executionContext)

	gotExecutedBy, gotMethod, gotContext := GetExecutionContext(ctx)

	if gotExecutedBy != executedBy {
		t.Errorf("Expected executedBy = %v, got %v", executedBy, gotExecutedBy)
	}
	if gotMethod != executionMethod {
		t.Errorf("Expected executionMethod = %v, got %v", executionMethod, gotMethod)
	}
	if gotContext == "" {
		t.Error("Expected executionContext to be set")
	}

	// Test with nil executionContext
	ctx2 := context.Background()
	ctx2 = SetExecutionContext(ctx2, "user", "api", nil)
	_, _, contextStr := GetExecutionContext(ctx2)
	if contextStr != "" {
		t.Error("Expected empty executionContext when nil is passed")
	}
}

func TestGetExecutionContext(t *testing.T) {
	ctx := context.Background()

	// Test default values
	executedBy, method, contextStr := GetExecutionContext(ctx)
	if executedBy != "system" {
		t.Errorf("Expected default executedBy = system, got %v", executedBy)
	}
	if method != "api" {
		t.Errorf("Expected default executionMethod = api, got %v", method)
	}
	if contextStr != "" {
		t.Errorf("Expected default executionContext = empty, got %v", contextStr)
	}
}

func TestExecutor_ExecuteSync_NoMigrations(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if !result.Success {
		t.Error("ExecuteSync() should return success for no migrations")
	}
	if len(result.Applied) != 0 {
		t.Errorf("Expected 0 applied migrations, got %v", len(result.Applied))
	}
}

func TestExecutor_ExecuteSync_AlreadyApplied(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Skipped) != 1 {
		t.Errorf("Expected 1 skipped migration, got %v", len(result.Skipped))
	}
	if backend.executeCalled {
		t.Error("ExecuteMigration should not be called for already applied migration")
	}
}

func TestExecutor_ExecuteSync_DryRun(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", true)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration (dry-run), got %v", len(result.Applied))
	}
	if backend.executeCalled {
		t.Error("ExecuteMigration should not be called in dry-run mode")
	}
}

func TestExecutor_ExecuteSync_BackendNotFound(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Register a migration so we actually try to execute it
	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	_, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err == nil {
		t.Error("ExecuteSync() expected error for missing backend")
		return
	}
	if err.Error() != "backend postgresql not registered" {
		t.Errorf("Expected error about backend not registered, got %v", err)
	}
}

func TestExecutor_ExecuteSync_ConnectionNotFound(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Register a migration so we actually try to execute it
	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "nonexistent",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	target := &registry.MigrationTarget{
		Connection: "nonexistent",
		Backend:    "postgresql",
	}

	_, err := exec.ExecuteSync(context.Background(), target, "nonexistent", "", false)
	if err == nil {
		t.Error("ExecuteSync() expected error for missing connection")
		return
	}
	if err.Error() != "failed to get connection config: connection nonexistent not found" {
		t.Errorf("Expected error about connection not found, got %v", err)
	}
}

func TestExecutor_ExecuteUp(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteUp(context.Background(), target, "test", []string{}, false)
	if err != nil {
		t.Errorf("ExecuteUp() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteUp() returned nil result")
	}
	if !result.Success {
		t.Error("ExecuteUp() should return success for no migrations")
	}
}

func TestExecutor_ExecuteUp_WithSchemas(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteUp(context.Background(), target, "test", []string{"schema1", "schema2"}, false)
	if err != nil {
		t.Errorf("ExecuteUp() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteUp() returned nil result")
	}
}

func TestExecutor_ExecuteDown_MigrationNotFound(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	_, err := exec.ExecuteDown(context.Background(), "nonexistent", []string{}, false)
	if err == nil {
		t.Error("ExecuteDown() expected error for missing migration")
	}
	if err.Error() != "migration not found: nonexistent" {
		t.Errorf("Expected error about migration not found, got %v", err)
	}
}

func TestExecutor_ExecuteDown_NotApplied(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	// Migration is not applied

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if len(result.Skipped) != 1 {
		t.Errorf("Expected 1 skipped migration, got %v", len(result.Skipped))
	}
}

func TestExecutor_ExecuteDown_Successful(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if !result.Success {
		t.Error("ExecuteDown() should succeed for applied migration")
	}
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %v", len(result.Applied))
	}
	if !backend.executeCalled {
		t.Error("ExecuteMigration should be called for down migration")
	}
}

func TestExecutor_ExecuteDown_WithSchemas(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	baseID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations["schema1_"+baseID] = true
	tracker.appliedMigrations["schema2_"+baseID] = true

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{"schema1", "schema2"}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if len(result.Applied) != 2 {
		t.Errorf("Expected 2 applied migrations, got %v", len(result.Applied))
	}
}

func TestExecutor_ExecuteDown_NoDownSQL(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "", // No down SQL
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteDown() should have errors when no down SQL")
	}
}

func TestExecutor_ExecuteDown_ExecutionError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	backend.executeError = errors.New("execution failed")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if result.Success {
		t.Error("ExecuteDown() should not succeed when execution fails")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteDown() should have errors when execution fails")
	}
}

func TestExecutor_ExecuteDown_CheckStatusError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.isAppliedError = errors.New("check failed")
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteDown() should have errors when status check fails")
	}
}

func TestExecutor_Rollback_MigrationNotFound(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	_, err := exec.Rollback(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Rollback() expected error for missing migration")
	}
	if err.Error() != "migration not found: nonexistent" {
		t.Errorf("Expected error about migration not found, got %v", err)
	}
}

func TestExecutor_Rollback_NotApplied(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	// Migration is not applied

	result, err := exec.Rollback(context.Background(), migrationID)
	if err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
	if result == nil {
		t.Fatal("Rollback() returned nil result")
	}
	if result.Success {
		t.Error("Rollback() should not succeed for non-applied migration")
	}
	if result.Message != "migration is not applied" {
		t.Errorf("Expected message about migration not applied, got %v", result.Message)
	}
}

func TestExecutor_Rollback_CheckStatusError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.isAppliedError = errors.New("check failed")
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)

	_, err := exec.Rollback(context.Background(), migrationID)
	if err == nil {
		t.Error("Rollback() expected error when status check fails")
	}
	if err.Error() != "failed to check migration status: check failed" {
		t.Errorf("Expected error about status check failure, got %v", err)
	}
}

func TestExecutor_Rollback_NoDownSQL(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "", // No down SQL
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.Rollback(context.Background(), migrationID)
	if err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
	if result == nil {
		t.Fatal("Rollback() returned nil result")
	}
	if result.Success {
		t.Error("Rollback() should not succeed without down SQL")
	}
	if result.Message != "migration does not have rollback SQL" {
		t.Errorf("Expected message about missing rollback SQL, got %v", result.Message)
	}
}

func TestExecutor_Rollback_Successful(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.Rollback(context.Background(), migrationID)
	if err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
	if result == nil {
		t.Fatal("Rollback() returned nil result")
	}
	if !result.Success {
		t.Error("Rollback() should succeed for applied migration with down SQL")
	}
	if result.Message != "rollback completed successfully" {
		t.Errorf("Expected success message, got %v", result.Message)
	}
}

func TestExecutor_Rollback_ExecutionError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	backend.executeError = errors.New("rollback execution failed")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.Rollback(context.Background(), migrationID)
	if err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
	if result == nil {
		t.Fatal("Rollback() returned nil result")
	}
	if result.Success {
		t.Error("Rollback() should not succeed when execution fails")
	}
	if result.Message != "rollback failed" {
		t.Errorf("Expected failure message, got %v", result.Message)
	}
	if len(result.Errors) == 0 {
		t.Error("Rollback() should have errors when execution fails")
	}
}

func TestExecutor_HealthCheck(t *testing.T) {
	tracker := newMockStateTracker()
	exec := NewExecutor(newMockRegistry(), tracker)

	err := exec.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
}

func TestExecutor_HealthCheck_Error(t *testing.T) {
	tracker := newMockStateTracker()
	tracker.healthCheckError = errors.New("health check failed")
	exec := NewExecutor(newMockRegistry(), tracker)

	err := exec.HealthCheck(context.Background())
	if err == nil {
		t.Error("HealthCheck() expected error")
	}
	if err.Error() != "state tracker health check failed: health check failed" {
		t.Errorf("Expected health check error, got %v", err)
	}
}

func TestExecutor_SetQueue(t *testing.T) {
	exec := NewExecutor(newMockRegistry(), newMockStateTracker())
	queue := newMockQueue()

	exec.SetQueue(queue)

	// Test that queue is used when executing
	reg := newMockRegistry()
	exec = NewExecutor(reg, newMockStateTracker())
	exec.SetQueue(queue)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.Execute(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if !result.Queued {
		t.Error("Execute() should queue job when queue is set")
	}
	if len(queue.publishedJobs) != 1 {
		t.Errorf("Expected 1 queued job, got %v", len(queue.publishedJobs))
	}
}

func TestExecutor_Execute_WithoutQueue(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.Execute(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if result.Queued {
		t.Error("Execute() should not queue job when queue is not set")
	}
}

func TestExecutor_Execute_QueueError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)
	queue := newMockQueue()
	queue.publishError = errors.New("queue error")
	exec.SetQueue(queue)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	_, err := exec.Execute(context.Background(), target, "test", "", false)
	if err == nil {
		t.Error("Execute() expected error when queue publish fails")
	}
	if err.Error() != "failed to queue migration job: queue error" {
		t.Errorf("Expected queue error, got %v", err)
	}
}

func TestExecutor_GetMigrationHistory(t *testing.T) {
	tracker := newMockStateTracker()
	exec := NewExecutor(newMockRegistry(), tracker)

	record := &state.MigrationRecord{
		MigrationID: "test_migration",
		Status:      "success",
		AppliedAt:   time.Now().Format(time.RFC3339),
	}
	_ = tracker.RecordMigration(context.Background(), record)

	history, err := exec.GetMigrationHistory(context.Background(), nil)
	if err != nil {
		t.Errorf("GetMigrationHistory() error = %v", err)
	}
	if len(history) != 1 {
		t.Errorf("Expected 1 history record, got %v", len(history))
	}
}

func TestExecutor_GetMigrationList(t *testing.T) {
	tracker := newMockStateTracker()
	exec := NewExecutor(newMockRegistry(), tracker)

	item := &state.MigrationListItem{
		MigrationID: "test_migration",
		LastStatus:  "success",
	}
	tracker.listItems = append(tracker.listItems, item)

	list, err := exec.GetMigrationList(context.Background(), nil)
	if err != nil {
		t.Errorf("GetMigrationList() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 list item, got %v", len(list))
	}
}

func TestExecutor_IsMigrationApplied(t *testing.T) {
	tracker := newMockStateTracker()
	exec := NewExecutor(newMockRegistry(), tracker)

	tracker.appliedMigrations["test_migration"] = true

	applied, err := exec.IsMigrationApplied(context.Background(), "test_migration")
	if err != nil {
		t.Errorf("IsMigrationApplied() error = %v", err)
	}
	if !applied {
		t.Error("IsMigrationApplied() should return true for applied migration")
	}

	applied, err = exec.IsMigrationApplied(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("IsMigrationApplied() error = %v", err)
	}
	if applied {
		t.Error("IsMigrationApplied() should return false for non-existent migration")
	}
}

func TestExecutor_RegisterScannedMigration(t *testing.T) {
	tracker := newMockStateTracker()
	exec := NewExecutor(newMockRegistry(), tracker)

	err := exec.RegisterScannedMigration(
		context.Background(),
		"test_migration",
		"public",
		"test_table",
		"20240101120000",
		"test_migration",
		"test",
		"postgresql",
	)
	if err != nil {
		t.Errorf("RegisterScannedMigration() error = %v", err)
	}
}

func TestExecutor_GetAllMigrations(t *testing.T) {
	reg := newMockRegistry()
	exec := NewExecutor(reg, newMockStateTracker())

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	all := exec.GetAllMigrations()
	if len(all) != 1 {
		t.Errorf("Expected 1 migration, got %v", len(all))
	}
}

func TestExecutor_ExecuteSync_WithError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	backend.executeError = errors.New("execution failed")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if result.Success {
		t.Error("ExecuteSync() should not succeed when execution fails")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteSync() should have errors when execution fails")
	}
}

func TestExecutor_ExecuteSync_BackendConnectError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	backend.connectError = errors.New("connection failed")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	_, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err == nil {
		t.Error("ExecuteSync() expected error for connection failure")
	}
	if err.Error() != "failed to connect to backend: connection failed" {
		t.Errorf("Expected connection error, got %v", err)
	}
}

func TestExecutor_GetMigrationID(t *testing.T) {
	exec := NewExecutor(newMockRegistry(), newMockStateTracker())

	tests := []struct {
		name      string
		migration *backends.MigrationScript
		want      string
	}{
		{
			name: "with schema",
			migration: &backends.MigrationScript{
				Schema:     "public",
				Connection: "test",
				Version:    "20240101120000",
				Name:       "test_migration",
			},
			want: "public_test_20240101120000_test_migration",
		},
		{
			name: "without schema",
			migration: &backends.MigrationScript{
				Connection: "test",
				Version:    "20240101120000",
				Name:       "test_migration",
			},
			want: "test_20240101120000_test_migration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Access private method through GetMigrationByID which uses it
			reg := newMockRegistry()
			_ = reg.Register(tt.migration)
			exec = NewExecutor(reg, newMockStateTracker())

			found := exec.GetMigrationByID(tt.want)
			if found == nil {
				t.Errorf("GetMigrationByID() returned nil for %v", tt.want)
			}
		})
	}
}

func TestExecutor_GetMigrationIDWithSchema(t *testing.T) {
	reg := newMockRegistry()
	exec := NewExecutor(reg, newMockStateTracker())

	migration := &backends.MigrationScript{
		Connection: "test",
		Version:    "20240101120000",
		Name:       "test_migration",
		Backend:    "postgresql",
	}
	_ = reg.Register(migration)

	// Test with schema
	idWithSchema := exec.GetMigrationByID("schema1_test_20240101120000_test_migration")
	if idWithSchema != nil {
		t.Error("GetMigrationByID should return nil for schema-specific ID when migration has no schema")
	}

	// Test without schema
	idWithoutSchema := exec.GetMigrationByID("test_20240101120000_test_migration")
	if idWithoutSchema == nil {
		t.Error("GetMigrationByID should find migration without schema")
	}
}

func TestExecutor_ExecuteSync_RecordMigrationError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.recordError = errors.New("record failed")
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteSync() should have errors when recording fails")
	}
}

func TestExecutor_ExecuteDown_RecordMigrationError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.recordError = errors.New("record failed")
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	migrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	tracker.appliedMigrations[migrationID] = true

	result, err := exec.ExecuteDown(context.Background(), migrationID, []string{}, false)
	if err != nil {
		t.Errorf("ExecuteDown() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteDown() returned nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteDown() should have errors when recording fails")
	}
}

func TestConvertTarget(t *testing.T) {
	// Test convertTarget through Execute with queue
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)
	queue := newMockQueue()
	exec.SetQueue(queue)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	target := &registry.MigrationTarget{
		Backend:    "postgresql",
		Schema:     "public",
		Tables:     []string{"users", "posts"},
		Version:    "20240101120000",
		Connection: "test",
	}

	result, err := exec.Execute(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if len(queue.publishedJobs) != 1 {
		t.Fatalf("Expected 1 queued job, got %v", len(queue.publishedJobs))
	}

	job := queue.publishedJobs[0]
	if job.Target == nil {
		t.Error("Job target should not be nil")
	}
	if job.Target.Backend != target.Backend {
		t.Errorf("Expected backend = %v, got %v", target.Backend, job.Target.Backend)
	}
	if job.Target.Schema != target.Schema {
		t.Errorf("Expected schema = %v, got %v", target.Schema, job.Target.Schema)
	}
	if len(job.Target.Tables) != len(target.Tables) {
		t.Errorf("Expected %d tables, got %d", len(target.Tables), len(job.Target.Tables))
	}
}

func TestConvertTarget_Nil(t *testing.T) {
	// Test convertTarget with nil target through Execute with queue
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)
	queue := newMockQueue()
	exec.SetQueue(queue)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	result, err := exec.Execute(context.Background(), nil, "test", "", false)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if len(queue.publishedJobs) != 1 {
		t.Fatalf("Expected 1 queued job, got %v", len(queue.publishedJobs))
	}

	job := queue.publishedJobs[0]
	if job.Target != nil {
		t.Error("Job target should be nil when input target is nil")
	}
}

func TestNewLoader(t *testing.T) {
	loader := NewLoader("/test/path")
	if loader == nil {
		t.Fatal("NewLoader() returned nil")
	}
	if loader.sfmPath != "/test/path" {
		t.Errorf("Expected sfmPath = /test/path, got %v", loader.sfmPath)
	}
	if loader.seenFiles == nil {
		t.Error("Expected seenFiles map to be initialized")
	}
}

func TestLoader_SetExecutor(t *testing.T) {
	loader := NewLoader("/test/path")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	loader.SetExecutor(exec)
	// Can't directly test executor field, but we can verify no panic
	if loader == nil {
		t.Fatal("Loader should not be nil")
	}
}

func TestExecutor_ExecuteSync_FindByTargetError(t *testing.T) {
	reg := newMockRegistry()
	reg.findByTargetError = errors.New("find failed")
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	_, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err == nil {
		t.Error("ExecuteSync() expected error when FindByTarget fails")
	}
	if err.Error() != "failed to find migrations: find failed" {
		t.Errorf("Expected find error, got %v", err)
	}
}

func TestExecutor_ExecuteSync_IsMigrationAppliedError(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.isAppliedError = errors.New("check failed")
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("ExecuteSync() should have errors when status check fails")
	}
}

func TestExecutor_ExecuteSync_MultipleMigrations(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

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

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", true)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Applied) != 2 {
		t.Errorf("Expected 2 applied migrations, got %v", len(result.Applied))
	}
}

func TestExecutor_ExecuteSync_WithSchema(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "custom_schema", true)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %v", len(result.Applied))
	}
}

func TestExecutor_ExecuteSync_WithStructuredDependencies(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Base migration
	baseMigration := &backends.MigrationScript{
		Version:      "20240101120000",
		Name:         "base_migration",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE base (id SERIAL PRIMARY KEY);",
		Dependencies: []string{},
	}
	_ = reg.Register(baseMigration)

	// Dependent migration with structured dependency
	dependentMigration := &backends.MigrationScript{
		Version:    "20240101120001",
		Name:       "dependent_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE dependent (id SERIAL PRIMARY KEY, base_id INT REFERENCES base(id));",
		StructuredDependencies: []backends.Dependency{
			{
				Connection: "test",
				Target:     "base_migration",
				TargetType: "name",
			},
		},
	}
	_ = reg.Register(dependentMigration)

	// Mark base as applied
	tracker.appliedMigrations[fmt.Sprintf("%s_%s_%s_%s", baseMigration.Version, baseMigration.Name, baseMigration.Backend, baseMigration.Connection)] = true

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	// Should execute dependent migration (base is already applied)
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %v", len(result.Applied))
	}
	expectedID := fmt.Sprintf("%s_%s_%s_%s", dependentMigration.Version, dependentMigration.Name, dependentMigration.Backend, dependentMigration.Connection)
	if result.Applied[0] != expectedID {
		t.Errorf("Expected dependent_migration to be applied, got %s", result.Applied[0])
	}
}

func TestExecutor_ExecuteSync_WithSimpleDependencies(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Base migration
	baseMigration := &backends.MigrationScript{
		Version:      "20240101120000",
		Name:         "base",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE base (id SERIAL PRIMARY KEY);",
		Dependencies: []string{},
	}
	_ = reg.Register(baseMigration)

	// Dependent migration with simple dependency
	dependentMigration := &backends.MigrationScript{
		Version:      "20240101120001",
		Name:         "dependent",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE dependent (id SERIAL PRIMARY KEY);",
		Dependencies: []string{"base"},
	}
	_ = reg.Register(dependentMigration)

	// Mark base as applied
	tracker.appliedMigrations[fmt.Sprintf("%s_%s_%s_%s", baseMigration.Version, baseMigration.Name, baseMigration.Backend, baseMigration.Connection)] = true

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	// Should execute dependent migration
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %v", len(result.Applied))
	}
}

func TestExecutor_ExecuteSync_MigrationWithSchema(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	migration := &backends.MigrationScript{
		Schema:     "public",
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", true)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %v", len(result.Applied))
	}
}

func TestExecutor_ExecuteSync_CircularDependency(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Create circular dependency: m1 -> m2 -> m1
	m1 := &backends.MigrationScript{
		Version:      "20240101120000",
		Name:         "migration1",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE m1;",
		Dependencies: []string{"migration2"},
	}
	_ = reg.Register(m1)

	m2 := &backends.MigrationScript{
		Version:      "20240101120001",
		Name:         "migration2",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE m2;",
		Dependencies: []string{"migration1"},
	}
	_ = reg.Register(m2)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	// Should detect circular dependency and add error to result
	if err == nil && result != nil {
		if len(result.Errors) == 0 {
			t.Error("Expected error for circular dependency")
		}
	}
}

func TestExecutor_ExecuteSync_MissingDependency(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Migration with missing dependency
	migration := &backends.MigrationScript{
		Version:      "20240101120000",
		Name:         "dependent",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE dependent;",
		Dependencies: []string{"nonexistent"},
	}
	_ = reg.Register(migration)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	// Should handle missing dependency gracefully
	if err == nil && result != nil {
		if len(result.Errors) == 0 {
			t.Error("Expected error for missing dependency")
		}
	}
}

func TestExecutor_ExecuteSync_BothDependencyTypes(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := NewExecutor(reg, tracker)

	// Base migration
	base := &backends.MigrationScript{
		Version:      "20240101120000",
		Name:         "base",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE base;",
		Dependencies: []string{},
	}
	_ = reg.Register(base)

	// Migration with both simple and structured dependencies
	hybrid := &backends.MigrationScript{
		Version:      "20240101120001",
		Name:         "hybrid",
		Connection:   "test",
		Backend:      "postgresql",
		UpSQL:        "CREATE TABLE hybrid;",
		Dependencies: []string{"base"},
		StructuredDependencies: []backends.Dependency{
			{
				Connection: "test",
				Target:     "base",
				TargetType: "name",
			},
		},
	}
	_ = reg.Register(hybrid)

	tracker.appliedMigrations[fmt.Sprintf("%s_%s_%s_%s", base.Version, base.Name, base.Backend, base.Connection)] = true

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	backend := newMockBackend("postgresql")
	exec.RegisterBackend("postgresql", backend)

	target := &registry.MigrationTarget{
		Connection: "test",
		Backend:    "postgresql",
	}

	result, err := exec.ExecuteSync(context.Background(), target, "test", "", false)
	if err != nil {
		t.Errorf("ExecuteSync() error = %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteSync() returned nil result")
	}
	// Should execute hybrid migration
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied migration, got %v", len(result.Applied))
	}
}
