package http

import (
	"context"
	_ "embed"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/toolsascode/bfm/api/internal/api/http/dto"
	"github.com/toolsascode/bfm/api/internal/auth"
	"github.com/toolsascode/bfm/api/internal/executor"
	"github.com/toolsascode/bfm/api/internal/state"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Handler handles HTTP API requests
type Handler struct {
	executor *executor.Executor
}

// NewHandler creates a new HTTP handler
func NewHandler(exec *executor.Executor) *Handler {
	return &Handler{
		executor: exec,
	}
}

// RegisterRoutes registers HTTP routes
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	{
		// Handle OPTIONS for all routes
		api.OPTIONS("/*path", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		api.POST("/migrations/up", h.authenticate, h.migrateUp)
		api.POST("/migrations/down", h.authenticate, h.migrateDown)
		api.GET("/migrations", h.authenticate, h.listMigrations)
		api.GET("/migrations/:id", h.authenticate, h.getMigration)
		api.GET("/migrations/:id/status", h.authenticate, h.getMigrationStatus)
		api.GET("/migrations/:id/history", h.authenticate, h.getMigrationHistory)
		api.POST("/migrations/:id/rollback", h.authenticate, h.rollbackMigration)
		api.POST("/migrations/reindex", h.authenticate, h.reindexMigrations)
		api.GET("/health", h.Health)
		api.GET("/openapi.yaml", h.OpenAPISpec)
		api.GET("/openapi.json", h.OpenAPISpecJSON)
	}
}

// authenticate middleware validates API token
func (h *Handler) authenticate(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	token, err := auth.ExtractToken(authHeader)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	if err := auth.ValidateToken(token); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		c.Abort()
		return
	}

	c.Next()
}

// getExecutedBy extracts user identifier from gin context
func (h *Handler) getExecutedBy(c *gin.Context) string {
	// Try to get token from context (set by authenticate middleware)
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		token, err := auth.ExtractToken(authHeader)
		if err == nil && token != "" {
			// Check if request is from frontend (manual execution)
			// For manual executions, use a more descriptive identifier
			if h.isManualExecution(c) {
				return "frontend_user"
			}
			// Use a hash of the token or just "api_user" for now
			// In a real system, you'd extract user from token claims
			return "api_user"
		}
	}
	return "system"
}

// isManualExecution checks if the request is from the FfM frontend (manual execution)
func (h *Handler) isManualExecution(c *gin.Context) bool {
	// Method 1: Check for custom header sent by frontend
	clientType := c.GetHeader("X-Client-Type")
	if clientType == "frontend" || clientType == "FfM" {
		return true
	}

	// Method 2: Check for X-Requested-With header (commonly sent by frontend frameworks)
	requestedWith := c.GetHeader("X-Requested-With")
	if requestedWith == "XMLHttpRequest" || requestedWith == "FfM" {
		return true
	}

	// Method 3: Check Origin header - if it's from a browser origin, likely frontend
	origin := c.GetHeader("Origin")
	if origin != "" {
		// If Origin is present, it's likely a browser request (CORS)
		// This indicates manual execution from frontend
		return true
	}

	// Method 4: Check User-Agent for browser patterns (fallback)
	userAgent := c.GetHeader("User-Agent")
	if userAgent != "" {
		browserPatterns := []string{"Mozilla", "Chrome", "Safari", "Firefox", "Edge", "Opera"}
		for _, pattern := range browserPatterns {
			if strings.Contains(userAgent, pattern) {
				return true
			}
		}
	}

	return false
}

// getExecutionMethod determines execution method from request
func (h *Handler) getExecutionMethod(c *gin.Context) string {
	// Check if request is from FfM frontend (manual execution)
	if h.isManualExecution(c) {
		return "manual"
	}
	return "api"
}

// setExecutionContext sets execution context in the request context
func (h *Handler) setExecutionContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	executedBy := h.getExecutedBy(c)
	executionMethod := h.getExecutionMethod(c)

	executionContext := map[string]interface{}{
		"endpoint":   c.Request.URL.Path,
		"method":     c.Request.Method,
		"request_id": c.GetString("request_id"), // If you add request ID middleware
	}

	return executor.SetExecutionContext(ctx, executedBy, executionMethod, executionContext)
}

// migrateUp handles up migration requests
func (h *Handler) migrateUp(c *gin.Context) {
	var req dto.MigrateUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set execution context
	ctx := h.setExecutionContext(c)

	// Execute migrations
	result, err := h.executor.ExecuteUp(
		ctx,
		req.Target,
		req.Connection,
		req.Schemas,
		req.DryRun,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response
	response := dto.MigrateResponse{
		Success: result.Success,
		Applied: result.Applied,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	}

	statusCode := http.StatusOK
	if !result.Success {
		statusCode = http.StatusPartialContent
	}

	c.JSON(statusCode, response)
}

// migrateDown handles down migration requests
func (h *Handler) migrateDown(c *gin.Context) {
	var req dto.MigrateDownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set execution context
	ctx := h.setExecutionContext(c)

	// Execute down migrations
	result, err := h.executor.ExecuteDown(
		ctx,
		req.MigrationID,
		req.Schemas,
		req.DryRun,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response
	response := dto.MigrateResponse{
		Success: result.Success,
		Applied: result.Applied,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	}

	statusCode := http.StatusOK
	if !result.Success {
		statusCode = http.StatusPartialContent
	}

	c.JSON(statusCode, response)
}

// listMigrations lists all migrations with their status
func (h *Handler) listMigrations(c *gin.Context) {
	var filters dto.MigrationListFilters
	if err := c.ShouldBindQuery(&filters); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert DTO filters to state filters
	stateFilters := &state.MigrationFilters{
		Schema:     filters.Schema,
		Table:      filters.Table,
		Connection: filters.Connection,
		Backend:    filters.Backend,
		Status:     filters.Status,
		Version:    filters.Version,
	}

	// Get migration list from state tracker (only migrations registered in database)
	migrationList, err := h.executor.GetMigrationList(c.Request.Context(), stateFilters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Convert to DTO response (only migrations from database)
	items := make([]dto.MigrationListItem, 0, len(migrationList))
	for _, item := range migrationList {
		items = append(items, dto.MigrationListItem{
			MigrationID:  item.MigrationID,
			Schema:       item.Schema,
			Table:        item.Table,
			Version:      item.Version,
			Name:         item.Name,
			Connection:   item.Connection,
			Backend:      item.Backend,
			Applied:      item.Applied,
			Status:       item.LastStatus,
			AppliedAt:    item.LastAppliedAt,
			ErrorMessage: item.LastErrorMessage,
		})
	}

	response := dto.MigrationListResponse{
		Items: items,
		Total: len(items),
	}

	c.JSON(http.StatusOK, response)
}

// getMigration gets a specific migration by ID
func (h *Handler) getMigration(c *gin.Context) {
	migrationID := c.Param("id")

	// Get migration from registry
	migration := h.executor.GetMigrationByID(migrationID)
	if migration == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "migration not found"})
		return
	}

	// Get status from state tracker
	applied, err := h.executor.IsMigrationApplied(c.Request.Context(), migrationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get schema and table from state tracker (migrations_list table)
	// These are populated when the migration is executed or registered
	var schemaValue, tableValue string
	migrationList, err := h.executor.GetMigrationList(c.Request.Context(), &state.MigrationFilters{})
	if err == nil {
		for _, item := range migrationList {
			if item.MigrationID == migrationID {
				schemaValue = item.Schema
				tableValue = item.Table
				break
			}
		}
	}

	// Fallback to registry values if not found in state tracker
	if tableValue == "" && migration.Table != nil {
		tableValue = *migration.Table
	}
	if schemaValue == "" {
		schemaValue = migration.Schema
	}

	// Convert structured dependencies to response format
	structuredDeps := make([]dto.DependencyResponse, 0, len(migration.StructuredDependencies))
	for _, dep := range migration.StructuredDependencies {
		structuredDeps = append(structuredDeps, dto.DependencyResponse{
			Connection:     dep.Connection,
			Schema:         dep.Schema,
			Target:         dep.Target,
			TargetType:     dep.TargetType,
			RequiresTable:  dep.RequiresTable,
			RequiresSchema: dep.RequiresSchema,
		})
	}

	response := dto.MigrationDetailResponse{
		MigrationID:            migrationID,
		Schema:                 schemaValue,
		Table:                  tableValue,
		Version:                migration.Version,
		Name:                   migration.Name,
		Connection:             migration.Connection,
		Backend:                migration.Backend,
		Applied:                applied,
		UpSQL:                  migration.UpSQL,
		DownSQL:                migration.DownSQL,
		Dependencies:           migration.Dependencies,
		StructuredDependencies: structuredDeps,
	}

	c.JSON(http.StatusOK, response)
}

// getMigrationStatus gets the status of a specific migration
func (h *Handler) getMigrationStatus(c *gin.Context) {
	migrationID := c.Param("id")

	// Get all migration history to find the latest status
	allHistory, err := h.executor.GetMigrationHistory(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Find all related records (base migration and rollbacks)
	var relatedRecords []*state.MigrationRecord
	for _, record := range allHistory {
		// Match exact migration_id or migration_id_rollback or any variation starting with migration_id_
		if record.MigrationID == migrationID ||
			record.MigrationID == migrationID+"_rollback" ||
			(len(record.MigrationID) > len(migrationID) && record.MigrationID[:len(migrationID)] == migrationID && record.MigrationID[len(migrationID)] == '_') {
			relatedRecords = append(relatedRecords, record)
		}
	}

	// Determine applied status and get latest applied_at
	applied := false
	status := "pending"
	var appliedAt string
	var errorMessage string

	if len(relatedRecords) > 0 {
		// Get the latest record (first in the list since history is sorted DESC)
		latestRecord := relatedRecords[0]

		// Find the latest successful, non-rollback record
		var latestSuccessRecord *state.MigrationRecord
		for _, record := range relatedRecords {
			if !strings.Contains(record.MigrationID, "_rollback") && record.Status == "success" {
				latestSuccessRecord = record
				break // Records are sorted DESC, so first match is most recent
			}
		}

		// Find the latest rollback record
		var latestRollbackRecord *state.MigrationRecord
		for _, record := range relatedRecords {
			if strings.Contains(record.MigrationID, "_rollback") {
				latestRollbackRecord = record
				break // Records are sorted DESC, so first match is most recent
			}
		}

		// Determine status based on which is more recent
		if latestSuccessRecord != nil && latestRollbackRecord != nil {
			// Compare timestamps to see which is more recent
			successTime, _ := time.Parse(time.RFC3339, latestSuccessRecord.AppliedAt)
			rollbackTime, _ := time.Parse(time.RFC3339, latestRollbackRecord.AppliedAt)

			if successTime.After(rollbackTime) {
				// Success is more recent - migration is applied
				applied = true
				status = latestSuccessRecord.Status
				appliedAt = latestSuccessRecord.AppliedAt
				errorMessage = latestSuccessRecord.ErrorMessage
			} else {
				// Rollback is more recent - migration is rolled back
				applied = false
				status = "rolled_back"
				appliedAt = latestSuccessRecord.AppliedAt // Still show last successful applied_at
				errorMessage = latestRollbackRecord.ErrorMessage
			}
		} else if latestSuccessRecord != nil {
			// Only success record exists
			applied = true
			status = latestSuccessRecord.Status
			appliedAt = latestSuccessRecord.AppliedAt
			errorMessage = latestSuccessRecord.ErrorMessage
		} else if latestRollbackRecord != nil {
			// Only rollback record exists
			applied = false
			status = "rolled_back"
			errorMessage = latestRollbackRecord.ErrorMessage
		} else {
			// Use latest record (could be failed, pending, etc.)
			applied = !strings.Contains(latestRecord.MigrationID, "_rollback")
			status = latestRecord.Status
			appliedAt = latestRecord.AppliedAt
			errorMessage = latestRecord.ErrorMessage
		}
	}

	response := gin.H{
		"migration_id": migrationID,
		"status":       status,
		"applied":      applied,
	}

	if appliedAt != "" {
		response["applied_at"] = appliedAt
	}

	if errorMessage != "" {
		response["error_message"] = errorMessage
	}

	c.JSON(http.StatusOK, response)
}

// getMigrationHistory gets the execution history for a specific migration (including rollbacks)
func (h *Handler) getMigrationHistory(c *gin.Context) {
	migrationID := c.Param("id")

	// Get migration from registry to verify it exists
	migration := h.executor.GetMigrationByID(migrationID)
	if migration == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "migration not found"})
		return
	}

	// Get all migration history
	allHistory, err := h.executor.GetMigrationHistory(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Filter history to include:
	// 1. Records with exact migration_id match
	// 2. Records with migration_id_rollback (rollback records)
	// 3. Records that start with migration_id_ (to catch any variations)
	var relatedHistory []*state.MigrationRecord
	for _, record := range allHistory {
		if record.MigrationID == migrationID ||
			record.MigrationID == migrationID+"_rollback" ||
			(len(record.MigrationID) > len(migrationID) && record.MigrationID[:len(migrationID)] == migrationID && record.MigrationID[len(migrationID)] == '_') {
			relatedHistory = append(relatedHistory, record)
		}
	}

	// Convert to response format
	historyItems := make([]gin.H, 0, len(relatedHistory))
	for _, record := range relatedHistory {
		historyItems = append(historyItems, gin.H{
			"migration_id":      record.MigrationID,
			"schema":            record.Schema,
			"table":             record.Table,
			"version":           record.Version,
			"connection":        record.Connection,
			"backend":           record.Backend,
			"applied_at":        record.AppliedAt,
			"status":            record.Status,
			"error_message":     record.ErrorMessage,
			"executed_by":       record.ExecutedBy,
			"execution_method":  record.ExecutionMethod,
			"execution_context": record.ExecutionContext,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"migration_id": migrationID,
		"history":      historyItems,
	})
}

// rollbackMigration rolls back a specific migration
func (h *Handler) rollbackMigration(c *gin.Context) {
	migrationID := c.Param("id")

	// Get migration from registry
	migration := h.executor.GetMigrationByID(migrationID)
	if migration == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "migration not found"})
		return
	}

	// Check if migration is applied
	applied, err := h.executor.IsMigrationApplied(c.Request.Context(), migrationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !applied {
		c.JSON(http.StatusBadRequest, gin.H{"error": "migration is not applied"})
		return
	}

	// Set execution context
	ctx := h.setExecutionContext(c)

	// Execute rollback
	result, err := h.executor.Rollback(ctx, migrationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"message": result.Message,
		"errors":  result.Errors,
	})
}

// Health handles health check requests
func (h *Handler) Health(c *gin.Context) {
	// Check state tracker health
	healthStatus := gin.H{
		"status": "healthy",
		"checks": gin.H{},
	}

	// Add backend health checks if executor supports it
	if err := h.executor.HealthCheck(c.Request.Context()); err != nil {
		healthStatus["status"] = "unhealthy"
		healthStatus["checks"].(gin.H)["executor"] = err.Error()
	} else {
		healthStatus["checks"].(gin.H)["executor"] = "ok"
	}

	statusCode := http.StatusOK
	if healthStatus["status"] == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, healthStatus)
}

// reindexMigrations reindexes all migration files and synchronizes with database
func (h *Handler) reindexMigrations(c *gin.Context) {
	// Get SFM path from environment variable
	sfmPath := os.Getenv("BFM_SFM_PATH")
	if sfmPath == "" {
		// Default to ../sfm relative to bfm directory
		sfmPath = "../sfm"
	}

	result, err := h.executor.ReindexMigrations(c.Request.Context(), sfmPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := dto.ReindexResponse{
		Added:   result.Added,
		Removed: result.Removed,
		Updated: result.Updated,
		Total:   result.Total,
	}

	c.JSON(http.StatusOK, response)
}

//go:embed openapi.yaml
var openAPISpecYAML []byte

// OpenAPISpec serves the OpenAPI specification in YAML format
func (h *Handler) OpenAPISpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/x-yaml", openAPISpecYAML)
}

// OpenAPISpecJSON serves the OpenAPI specification in JSON format
func (h *Handler) OpenAPISpecJSON(c *gin.Context) {
	var spec map[string]interface{}
	if err := yaml.Unmarshal(openAPISpecYAML, &spec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse OpenAPI spec"})
		return
	}
	c.JSON(http.StatusOK, spec)
}

// getMigrationID generates a migration ID
// This function is kept for potential future use
// func (h *Handler) getMigrationID(migration *backends.MigrationScript) string {
// 	// If schema is provided, include it in the ID for uniqueness
// 	// Format: {schema}_{connection}_{version}_{name} or {connection}_{version}_{name}
// 	if migration.Schema != "" {
// 		return fmt.Sprintf("%s_%s_%s_%s", migration.Schema, migration.Connection, migration.Version, migration.Name)
// 	}
// 	return fmt.Sprintf("%s_%s_%s", migration.Connection, migration.Version, migration.Name)
// }
