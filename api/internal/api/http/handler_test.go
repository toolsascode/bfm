package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/toolsascode/bfm/api/internal/api/http/dto"
	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/executor"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"

	"github.com/gin-gonic/gin"
)

// mockBackend is a mock implementation of backends.Backend
type mockBackend struct {
	name             string
	connectError     error
	executeError     error
	executeCalled    bool
	connected        bool
	executeMigration *backends.MigrationScript
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

// mockRegistry is a mock implementation of registry.Registry
type mockRegistry struct {
	migrations map[string]*backends.MigrationScript
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
	var results []*backends.MigrationScript
	for _, migration := range m.migrations {
		if target.Backend != "" && migration.Backend != target.Backend {
			continue
		}
		if target.Connection != "" && migration.Connection != target.Connection {
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
	// Match executor's getMigrationID format: {version}_{name}_{backend}_{connection}
	return fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
}

// mockStateTracker is a mock implementation of state.StateTracker
type mockStateTracker struct {
	appliedMigrations        map[string]bool
	history                  []*state.MigrationRecord
	listItems                []*state.MigrationListItem
	healthCheckError         error
	getMigrationListError    error
	getMigrationHistoryError error
	isMigrationAppliedError  error
}

func newMockStateTracker() *mockStateTracker {
	return &mockStateTracker{
		appliedMigrations: make(map[string]bool),
		history:           make([]*state.MigrationRecord, 0),
		listItems:         make([]*state.MigrationListItem, 0),
	}
}

func (m *mockStateTracker) RecordMigration(ctx interface{}, migration *state.MigrationRecord) error {
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
	if m.getMigrationHistoryError != nil {
		return nil, m.getMigrationHistoryError
	}
	return m.history, nil
}

func (m *mockStateTracker) GetMigrationList(ctx interface{}, filters *state.MigrationFilters) ([]*state.MigrationListItem, error) {
	if m.getMigrationListError != nil {
		return nil, m.getMigrationListError
	}

	// Apply filters if provided
	if filters == nil {
		return m.listItems, nil
	}

	var filtered []*state.MigrationListItem
	for _, item := range m.listItems {
		// Apply filters
		if filters.Schema != "" && item.Schema != filters.Schema {
			continue
		}
		if filters.Table != "" && item.Table != filters.Table {
			continue
		}
		if filters.Connection != "" && item.Connection != filters.Connection {
			continue
		}
		if filters.Backend != "" && item.Backend != filters.Backend {
			continue
		}
		if filters.Status != "" && item.LastStatus != filters.Status {
			continue
		}
		if filters.Version != "" && item.Version != filters.Version {
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered, nil
}

func (m *mockStateTracker) IsMigrationApplied(ctx interface{}, migrationID string) (bool, error) {
	if m.isMigrationAppliedError != nil {
		return false, m.isMigrationAppliedError
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

func (m *mockStateTracker) ReindexMigrations(ctx interface{}, registry interface{}) error {
	return nil
}

func (m *mockStateTracker) GetMigrationDetail(ctx interface{}, migrationID string) (*state.MigrationDetail, error) {
	// Find migration in listItems
	for _, item := range m.listItems {
		if item.MigrationID == migrationID {
			return &state.MigrationDetail{
				MigrationID:            item.MigrationID,
				Schema:                 item.Schema,
				Version:                item.Version,
				Name:                   item.Name,
				Connection:             item.Connection,
				Backend:                item.Backend,
				UpSQL:                  "",
				DownSQL:                "",
				Dependencies:           []string{},
				StructuredDependencies: []backends.Dependency{},
				Status:                 item.LastStatus,
			}, nil
		}
	}
	return nil, nil
}

func (m *mockStateTracker) GetMigrationExecutions(ctx interface{}, migrationID string) ([]*state.MigrationExecution, error) {
	// Check if this migration is applied
	applied := m.appliedMigrations[migrationID]
	if !applied {
		return []*state.MigrationExecution{}, nil
	}

	// Parse migration ID to extract details: {version}_{name}_{backend}_{connection}
	parts := strings.Split(migrationID, "_")
	if len(parts) < 4 {
		return []*state.MigrationExecution{}, nil
	}

	// Extract version, backend, and connection
	version := parts[0]
	backend := parts[len(parts)-2]
	connection := parts[len(parts)-1]

	// Return execution records for both empty schema and "public" schema
	// This allows the executor to find the execution regardless of which schema it's looking for
	return []*state.MigrationExecution{
		{
			ID:          1,
			MigrationID: migrationID,
			Schema:      "", // Empty schema
			Version:     version,
			Connection:  connection,
			Backend:     backend,
			Status:      "applied",
			Applied:     true,
			AppliedAt:   time.Now().Format(time.RFC3339),
			CreatedAt:   time.Now().Format(time.RFC3339),
			UpdatedAt:   time.Now().Format(time.RFC3339),
		},
		{
			ID:          2,
			MigrationID: migrationID,
			Schema:      "public", // Public schema
			Version:     version,
			Connection:  connection,
			Backend:     backend,
			Status:      "applied",
			Applied:     true,
			AppliedAt:   time.Now().Format(time.RFC3339),
			CreatedAt:   time.Now().Format(time.RFC3339),
			UpdatedAt:   time.Now().Format(time.RFC3339),
		},
	}, nil
}
func (m *mockStateTracker) GetRecentExecutions(ctx interface{}, limit int) ([]*state.MigrationExecution, error) {
	return []*state.MigrationExecution{}, nil
}

func setupTestRouter(reg *mockRegistry, tracker *mockStateTracker) (*gin.Engine, *executor.Executor) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)
	handler.RegisterRoutes(router)
	return router, exec
}

func TestNewHandler(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)

	if handler == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if handler.executor != exec { //nolint:SA5011 // t.Fatal exits the test, so handler is not nil after this point
		t.Error("NewHandler() executor mismatch")
	}
}

func TestHandler_Health(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status = healthy, got %v", response["status"])
	}
}

func TestHandler_Health_Unhealthy(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.healthCheckError = errors.New("health check failed")
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "unhealthy" {
		t.Errorf("Expected status = unhealthy, got %v", response["status"])
	}
}

func TestHandler_authenticate(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "valid token",
			authHeader:     "Bearer test-token",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid format",
			authHeader:     "Invalid test-token",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/v1/migrations", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandler_migrateUp(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name: "valid request",
			requestBody: dto.MigrateUpRequest{
				Target: &registry.MigrationTarget{
					Backend:    "postgresql",
					Connection: "test",
				},
				Connection: "test",
				Schemas:    []string{},
				DryRun:     false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid request body",
			requestBody: map[string]interface{}{
				"invalid": "data",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing connection",
			requestBody:    dto.MigrateUpRequest{},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/api/v1/migrations/up", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_migrateUp_PartialContent(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, exec := setupTestRouter(reg, tracker)

	// Register a migration that will fail
	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
	}
	_ = reg.Register(migration)

	// Set up backend that will fail
	backend := &mockBackend{name: "postgresql", executeError: errors.New("execution failed")}
	exec.RegisterBackend("postgresql", backend)

	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	reqBody := dto.MigrateUpRequest{
		Target: &registry.MigrationTarget{
			Backend:    "postgresql",
			Connection: "test",
		},
		Connection: "test",
		Schemas:    []string{},
		DryRun:     false,
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/v1/migrations/up", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Errorf("Expected status %d, got %d", http.StatusPartialContent, w.Code)
	}
}

func TestHandler_migrateDown(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, exec := setupTestRouter(reg, tracker)

	// Register a migration for the valid request test
	migration := &backends.MigrationScript{
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
		UpSQL:      "CREATE TABLE test;",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)
	migrationID := "test_20240101120000_test_migration"
	tracker.appliedMigrations[migrationID] = true

	// Set up backend and connection for down migration
	backend := &mockBackend{name: "postgresql"}
	exec.RegisterBackend("postgresql", backend)
	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name: "valid request",
			requestBody: dto.MigrateDownRequest{
				MigrationID: migrationID,
				Schemas:     []string{},
				DryRun:      false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid request body",
			requestBody: map[string]interface{}{
				"invalid": "data",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing migration_id",
			requestBody:    dto.MigrateDownRequest{},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req, _ := http.NewRequest("POST", "/api/v1/migrations/down", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestHandler_listMigrations(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.listItems = []*state.MigrationListItem{
		{
			MigrationID: "migration1",
			Schema:      "public",
			Version:     "20240101120000",
			Name:        "test_migration",
			Connection:  "test",
			Backend:     "postgresql",
			Applied:     true,
			LastStatus:  "success",
		},
	}
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response dto.MigrationListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Total != 1 {
		t.Errorf("Expected total = 1, got %d", response.Total)
	}
	if len(response.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(response.Items))
	}
}

func TestHandler_listMigrations_WithFilters(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations?schema=public&connection=test", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHandler_getMigration(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
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
	migrationID := "public_test_20240101120000_test_migration"
	tracker.appliedMigrations[migrationID] = true
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations/"+migrationID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response dto.MigrationDetailResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.MigrationID != migrationID {
		t.Errorf("Expected MigrationID = %v, got %v", migrationID, response.MigrationID)
	}
	if !response.Applied {
		t.Error("Expected Applied = true")
	}
}

func TestHandler_getMigration_NotFound(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_getMigrationStatus(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	migrationID := "test_20240101120000_test_migration"
	record := &state.MigrationRecord{
		MigrationID: migrationID,
		Status:      "success",
		AppliedAt:   time.Now().Format(time.RFC3339),
	}
	tracker.history = []*state.MigrationRecord{record}
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations/"+migrationID+"/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["migration_id"] != migrationID {
		t.Errorf("Expected migration_id = %v, got %v", migrationID, response["migration_id"])
	}
}

func TestHandler_getMigrationHistory(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	migration := &backends.MigrationScript{
		Schema:     "public",
		Version:    "20240101120000",
		Name:       "test_migration",
		Connection: "test",
		Backend:    "postgresql",
	}
	_ = reg.Register(migration)
	migrationID := "public_test_20240101120000_test_migration"
	record := &state.MigrationRecord{
		MigrationID:     migrationID,
		Status:          "success",
		AppliedAt:       time.Now().Format(time.RFC3339),
		ExecutedBy:      "test-user",
		ExecutionMethod: "manual",
	}
	tracker.history = []*state.MigrationRecord{record}
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations/"+migrationID+"/history", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["migration_id"] != migrationID {
		t.Errorf("Expected migration_id = %v, got %v", migrationID, response["migration_id"])
	}
}

func TestHandler_getMigrationHistory_NotFound(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations/nonexistent/history", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_rollbackMigration(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
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
	migrationID := "public_test_20240101120000_test_migration"
	// Use the base migration ID format that executor expects: {version}_{name}_{backend}_{connection}
	baseMigrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	// Set applied status using base ID (executor uses base ID when checking via GetMigrationExecutions)
	tracker.appliedMigrations[baseMigrationID] = true
	router, exec := setupTestRouter(reg, tracker)

	// Set up backend and connection for rollback
	backend := &mockBackend{name: "postgresql"}
	exec.RegisterBackend("postgresql", backend)
	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	req, _ := http.NewRequest("POST", "/api/v1/migrations/"+migrationID+"/rollback", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Errorf("Expected success = true, got %v", response["success"])
	}
}

func TestHandler_rollbackMigration_NotFound(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("POST", "/api/v1/migrations/nonexistent/rollback", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandler_rollbackMigration_NotApplied(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
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
	// Use the base migration ID format that executor expects: {version}_{name}_{backend}_{connection}
	baseMigrationID := fmt.Sprintf("%s_%s_%s_%s", migration.Version, migration.Name, migration.Backend, migration.Connection)
	migrationID := "public_test_20240101120000_test_migration"
	// Set applied status to false using base ID (executor uses base ID when checking)
	tracker.appliedMigrations[baseMigrationID] = false
	router, exec := setupTestRouter(reg, tracker)

	// Set up backend and connection for rollback
	backend := &mockBackend{name: "postgresql"}
	exec.RegisterBackend("postgresql", backend)
	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend: "postgresql",
			Host:    "localhost",
		},
	}
	_ = exec.SetConnections(connections)

	req, _ := http.NewRequest("POST", "/api/v1/migrations/"+migrationID+"/rollback", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandler_isManualExecution(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)

	tests := []struct {
		name   string
		header string
		value  string
		want   bool
	}{
		{
			name:   "X-Client-Type frontend",
			header: "X-Client-Type",
			value:  "frontend",
			want:   true,
		},
		{
			name:   "X-Client-Type FfM",
			header: "X-Client-Type",
			value:  "FfM",
			want:   true,
		},
		{
			name:   "X-Requested-With XMLHttpRequest",
			header: "X-Requested-With",
			value:  "XMLHttpRequest",
			want:   true,
		},
		{
			name:   "Origin header present",
			header: "Origin",
			value:  "http://localhost:3000",
			want:   true,
		},
		{
			name:   "User-Agent browser",
			header: "User-Agent",
			value:  "Mozilla/5.0",
			want:   true,
		},
		{
			name:   "API request",
			header: "User-Agent",
			value:  "curl/7.0",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/v1/migrations", nil)
			req.Header.Set(tt.header, tt.value)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			got := handler.isManualExecution(c)
			if got != tt.want {
				t.Errorf("isManualExecution() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandler_getExecutedBy(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)

	tests := []struct {
		name       string
		authHeader string
		headers    map[string]string
		want       string
	}{
		{
			name:       "frontend user",
			authHeader: "Bearer test-token",
			headers: map[string]string{
				"X-Client-Type": "frontend",
			},
			want: "frontend_user",
		},
		{
			name:       "API user",
			authHeader: "Bearer test-token",
			headers:    map[string]string{},
			want:       "api_user",
		},
		{
			name:       "no auth header",
			authHeader: "",
			headers:    map[string]string{},
			want:       "system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/v1/migrations", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			got := handler.getExecutedBy(c)
			if got != tt.want {
				t.Errorf("getExecutedBy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)

	router := gin.New()
	handler.RegisterRoutes(router)

	// Test that routes are registered
	routes := router.Routes()
	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	expectedRoutes := []string{
		"/api/v1/migrations/up",
		"/api/v1/migrations/down",
		"/api/v1/migrations",
		"/api/v1/health",
	}

	for _, expected := range expectedRoutes {
		if !routePaths[expected] {
			t.Errorf("Expected route %s to be registered", expected)
		}
	}
}

func TestHandler_Options(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("OPTIONS", "/api/v1/migrations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status %d, got %d", http.StatusNoContent, w.Code)
	}
}

func TestHandler_OpenAPISpec(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/openapi.yaml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Header().Get("Content-Type") != "application/x-yaml" {
		t.Errorf("Expected Content-Type application/x-yaml, got %s", w.Header().Get("Content-Type"))
	}

	if len(w.Body.Bytes()) == 0 {
		t.Error("Expected non-empty OpenAPI spec")
	}
}

func TestHandler_OpenAPISpecJSON(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/openapi.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal JSON response: %v", err)
	}

	// Verify it's a valid OpenAPI/Swagger spec structure
	// Swag generates Swagger 2.0 format (uses "swagger" field)
	// OpenAPI 3.x format uses "openapi" field
	if _, ok := response["openapi"]; !ok {
		if _, ok := response["swagger"]; !ok {
			t.Error("Expected 'openapi' or 'swagger' field in response")
		}
	}
}

func TestHandler_reindexMigrations(t *testing.T) {
	// Save original token and SFM path
	originalToken := os.Getenv("BFM_API_TOKEN")
	originalSfmPath := os.Getenv("BFM_SFM_PATH")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
		if originalSfmPath != "" {
			_ = os.Setenv("BFM_SFM_PATH", originalSfmPath)
		} else {
			_ = os.Unsetenv("BFM_SFM_PATH")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Set SFM path
	_ = os.Setenv("BFM_SFM_PATH", tmpDir)

	req, _ := http.NewRequest("POST", "/api/v1/migrations/reindex", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Reindex should succeed even with empty directory
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d or %d, got %d. Body: %s", http.StatusOK, http.StatusInternalServerError, w.Code, w.Body.String())
	}

	if w.Code == http.StatusOK {
		var response dto.ReindexResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Response should have Total field
		if response.Total < 0 {
			t.Errorf("Expected Total >= 0, got %d", response.Total)
		}
	}
}

func TestHandler_reindexMigrations_Unauthorized(t *testing.T) {
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("POST", "/api/v1/migrations/reindex", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestHandler_migrateUp_ExecutorError(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)

	// Create a backend that will fail
	mockBackend := &mockBackend{
		name:         "postgresql",
		connectError: errors.New("connection failed"),
	}
	exec.RegisterBackend("postgresql", mockBackend)

	// Set connection config
	connections := map[string]*backends.ConnectionConfig{
		"test": {
			Backend:  "postgresql",
			Host:     "localhost",
			Port:     "5432",
			Database: "test",
			Username: "test",
			Password: "test",
			Extra:    map[string]string{},
		},
	}
	_ = exec.SetConnections(connections)

	// Register a migration
	migration := &backends.MigrationScript{
		Backend:    "postgresql",
		Connection: "test",
		Version:    "20250101000000",
		Name:       "test_migration",
		UpSQL:      "CREATE TABLE test (id INT);",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	router := gin.New()
	handler.RegisterRoutes(router)

	requestBody := dto.MigrateUpRequest{
		Target: &registry.MigrationTarget{
			Backend:    "postgresql",
			Connection: "test",
		},
		Connection: "test",
		Schemas:    []string{},
		DryRun:     false,
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/v1/migrations/up", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 500 or 206 (partial content) depending on error handling
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusPartialContent {
		t.Errorf("Expected status %d or %d, got %d. Body: %s", http.StatusInternalServerError, http.StatusPartialContent, w.Code, w.Body.String())
	}
}

func TestHandler_migrateDown_ExecutorError(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	exec := executor.NewExecutor(reg, tracker)
	handler := NewHandler(exec)

	router := gin.New()
	handler.RegisterRoutes(router)

	requestBody := dto.MigrateDownRequest{
		MigrationID: "nonexistent_migration",
		Schemas:     []string{},
		DryRun:      false,
	}

	body, _ := json.Marshal(requestBody)
	req, _ := http.NewRequest("POST", "/api/v1/migrations/down", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 500 or 206 depending on error handling
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusPartialContent {
		t.Errorf("Expected status %d or %d, got %d. Body: %s", http.StatusInternalServerError, http.StatusPartialContent, w.Code, w.Body.String())
	}
}

func TestHandler_listMigrations_Error(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.getMigrationListError = errors.New("database error")
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
}

func TestHandler_getMigration_StateTrackerError(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()

	// Register a migration
	migration := &backends.MigrationScript{
		Backend:    "postgresql",
		Connection: "test",
		Version:    "20250101000000",
		Name:       "test_migration",
		UpSQL:      "CREATE TABLE test (id INT);",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	tracker.isMigrationAppliedError = errors.New("database error")
	router, _ := setupTestRouter(reg, tracker)

	migrationID := reg.getMigrationID(migration)
	req, _ := http.NewRequest("GET", "/api/v1/migrations/"+migrationID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
}

func TestHandler_getMigrationStatus_Error(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()
	tracker.getMigrationHistoryError = errors.New("database error")
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("GET", "/api/v1/migrations/test_migration/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
}

func TestHandler_rollbackMigration_ExecutorError(t *testing.T) {
	// Save original token
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			_ = os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	_ = os.Setenv("BFM_API_TOKEN", "test-token")
	reg := newMockRegistry()
	tracker := newMockStateTracker()

	// Register a migration
	migration := &backends.MigrationScript{
		Backend:    "postgresql",
		Connection: "test",
		Version:    "20250101000000",
		Name:       "test_migration",
		UpSQL:      "CREATE TABLE test (id INT);",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	// Mark as applied
	migrationID := reg.getMigrationID(migration)
	tracker.appliedMigrations[migrationID] = true

	// Make rollback fail
	tracker.isMigrationAppliedError = errors.New("database error")
	router, _ := setupTestRouter(reg, tracker)

	req, _ := http.NewRequest("POST", "/api/v1/migrations/"+migrationID+"/rollback", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
}
