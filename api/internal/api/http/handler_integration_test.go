//go:build integration

package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/toolsascode/bfm/api/internal/api/http/dto"
	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/executor"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"

	"github.com/gin-gonic/gin"
)

// TestIntegration_MigrateUp_Down_Rollback tests the full flow of migration execution
func TestIntegration_MigrateUp_Down_Rollback(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

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

	// Register backend
	mockBackend := &mockBackend{name: "postgresql"}
	exec.RegisterBackend("postgresql", mockBackend)

	// Set connection
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

	// Register migration
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
	handler := NewHandler(exec)
	handler.RegisterRoutes(router)

	// Step 1: Execute migration up
	upRequest := dto.MigrateUpRequest{
		Target: &registry.MigrationTarget{
			Backend:    "postgresql",
			Connection: "test",
		},
		Connection: "test",
		Schemas:    []string{},
		DryRun:     false,
	}

	body, _ := json.Marshal(upRequest)
	req, _ := http.NewRequest("POST", "/api/v1/migrations/up", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusPartialContent {
		t.Fatalf("Expected status %d or %d, got %d. Body: %s", http.StatusOK, http.StatusPartialContent, w.Code, w.Body.String())
	}

	var upResponse dto.MigrateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &upResponse); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Step 2: Check migration status
	migrationID := reg.getMigrationID(migration)
	req, _ = http.NewRequest("GET", "/api/v1/migrations/"+migrationID+"/status", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	// Step 3: Rollback migration
	req, _ = http.NewRequest("POST", "/api/v1/migrations/"+migrationID+"/rollback", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var rollbackResponse dto.RollbackResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rollbackResponse); err != nil {
		t.Fatalf("Failed to unmarshal rollback response: %v", err)
	}

	if !rollbackResponse.Success {
		t.Errorf("Expected rollback to succeed, but got errors: %v", rollbackResponse.Errors)
	}
}

// TestIntegration_ListMigrations_WithFilters tests listing migrations with various filters
func TestIntegration_ListMigrations_WithFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

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

	// Add some migrations to the list
	tracker.listItems = []*state.MigrationListItem{
		{
			MigrationID: "20250101000000_test1_postgresql_test",
			Backend:     "postgresql",
			Connection:  "test",
			Schema:      "public",
			LastStatus:  "success",
			Applied:     true,
		},
		{
			MigrationID: "20250102000000_test2_postgresql_test",
			Backend:     "postgresql",
			Connection:  "test",
			Schema:      "public",
			LastStatus:  "pending",
			Applied:     false,
		},
		{
			MigrationID: "20250103000000_test3_mysql_prod",
			Backend:     "mysql",
			Connection:  "prod",
			Schema:      "app",
			LastStatus:  "success",
			Applied:     true,
		},
	}

	router, _ := setupTestRouter(reg, tracker)

	tests := []struct {
		name           string
		query          string
		expectedCount  int
		expectedStatus int
	}{
		{
			name:           "no filters",
			query:          "",
			expectedCount:  3,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "filter by backend",
			query:          "?backend=postgresql",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "filter by connection",
			query:          "?connection=test",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "filter by status",
			query:          "?status=success",
			expectedCount:  2,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "multiple filters",
			query:          "?backend=postgresql&connection=test&status=success",
			expectedCount:  1,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/v1/migrations"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
				return
			}

			if w.Code == http.StatusOK {
				var response dto.MigrationListResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				if len(response.Items) != tt.expectedCount {
					t.Errorf("Expected %d items, got %d", tt.expectedCount, len(response.Items))
				}
			}
		})
	}
}

// TestIntegration_GetMigration_History tests getting migration details and history
func TestIntegration_GetMigration_History(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

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

	// Register migration
	migration := &backends.MigrationScript{
		Backend:    "postgresql",
		Connection: "test",
		Version:    "20250101000000",
		Name:       "test_migration",
		UpSQL:      "CREATE TABLE test (id INT);",
		DownSQL:    "DROP TABLE test;",
	}
	_ = reg.Register(migration)

	// Add history
	migrationID := reg.getMigrationID(migration)
	tracker.history = []*state.MigrationRecord{
		{
			MigrationID: migrationID,
			Status:      "success",
			AppliedAt:   "2025-01-01T12:00:00Z",
		},
	}

	router, _ := setupTestRouter(reg, tracker)

	// Test get migration
	req, _ := http.NewRequest("GET", "/api/v1/migrations/"+migrationID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var detailResponse dto.MigrationDetailResponse
	if err := json.Unmarshal(w.Body.Bytes(), &detailResponse); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if detailResponse.MigrationID != migrationID {
		t.Errorf("Expected migration ID %s, got %s", migrationID, detailResponse.MigrationID)
	}

	// Test get migration history
	req, _ = http.NewRequest("GET", "/api/v1/migrations/"+migrationID+"/history", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var historyResponse map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &historyResponse); err != nil {
		t.Fatalf("Failed to unmarshal history response: %v", err)
	}

	history, ok := historyResponse["history"].([]interface{})
	if !ok {
		t.Fatal("Expected history field in response")
	}

	if len(history) == 0 {
		t.Error("Expected at least one history record")
	}
}
