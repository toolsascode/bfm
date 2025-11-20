package http

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"mops/bfm/internal/api/http/dto"
	"mops/bfm/internal/auth"
	"mops/bfm/internal/backends"
	"mops/bfm/internal/executor"
	"mops/bfm/internal/state"
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
		
		api.POST("/migrate", h.authenticate, h.migrate)
		api.GET("/migrations", h.authenticate, h.listMigrations)
		api.GET("/migrations/:id", h.authenticate, h.getMigration)
		api.GET("/migrations/:id/status", h.authenticate, h.getMigrationStatus)
		api.POST("/migrations/:id/rollback", h.authenticate, h.rollbackMigration)
		api.GET("/health", h.Health)
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

// migrate handles migration requests
func (h *Handler) migrate(c *gin.Context) {
	var req dto.MigrateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Execute migrations
	result, err := h.executor.Execute(
		c.Request.Context(),
		req.Target,
		req.Connection,
		req.Schema,
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

	// Get all registered migrations (no filters on registry)
	allMigrations := h.executor.GetAllMigrations()

	// Get all migration history (without filters) to ensure accurate status for all migrations
	// We'll apply filters to the final response items instead
	allHistory, err := h.executor.GetMigrationHistory(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response with accurate status for all migrations
	response := h.buildMigrationListResponse(allMigrations, allHistory)

	// Apply filters to the response items
	response = h.applyFiltersToResponse(response, filters)

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

	response := dto.MigrationDetailResponse{
		MigrationID: migrationID,
		Schema:      migration.Schema,
		Table:       migration.Table,
		Version:     migration.Version,
		Name:        migration.Name,
		Connection:  migration.Connection,
		Backend:     migration.Backend,
		Applied:     applied,
	}

	c.JSON(http.StatusOK, response)
}

// getMigrationStatus gets the status of a specific migration
func (h *Handler) getMigrationStatus(c *gin.Context) {
	migrationID := c.Param("id")

	applied, err := h.executor.IsMigrationApplied(c.Request.Context(), migrationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := "pending"
	if applied {
		status = "applied"
	}

	c.JSON(http.StatusOK, gin.H{
		"migration_id": migrationID,
		"status":       status,
		"applied":      applied,
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

	// Execute rollback
	result, err := h.executor.Rollback(c.Request.Context(), migrationID)
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

// buildMigrationListResponse builds the response for list migrations
func (h *Handler) buildMigrationListResponse(migrations []*backends.MigrationScript, history []*state.MigrationRecord) dto.MigrationListResponse {
	// Create a map of applied migrations
	appliedMap := make(map[string]*state.MigrationRecord)
	for _, record := range history {
		appliedMap[record.MigrationID] = record
	}

	items := make([]dto.MigrationListItem, 0, len(migrations))
	for _, migration := range migrations {
		migrationID := h.getMigrationID(migration)
		record, applied := appliedMap[migrationID]

		item := dto.MigrationListItem{
			MigrationID: migrationID,
			Schema:      migration.Schema,
			Table:       migration.Table,
			Version:     migration.Version,
			Name:        migration.Name,
			Connection:  migration.Connection,
			Backend:     migration.Backend,
			Applied:     applied,
		}

		if record != nil {
			item.Status = record.Status
			item.AppliedAt = record.AppliedAt
			item.ErrorMessage = record.ErrorMessage
		} else {
			item.Status = "pending"
		}

		items = append(items, item)
	}

	return dto.MigrationListResponse{
		Items: items,
		Total: len(items),
	}
}

// getMigrationID generates a migration ID
func (h *Handler) getMigrationID(migration *backends.MigrationScript) string {
	return fmt.Sprintf("%s_%s_%s_%s", migration.Schema, migration.Table, migration.Version, migration.Name)
}

// applyFiltersToResponse applies filters to the migration list response
func (h *Handler) applyFiltersToResponse(response dto.MigrationListResponse, filters dto.MigrationListFilters) dto.MigrationListResponse {
	if filters.Schema == "" && filters.Table == "" && filters.Connection == "" && filters.Backend == "" && filters.Status == "" && filters.Version == "" {
		// No filters applied, return as-is
		return response
	}

	filteredItems := make([]dto.MigrationListItem, 0)
	for _, item := range response.Items {
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
		if filters.Status != "" && item.Status != filters.Status {
			continue
		}
		if filters.Version != "" && item.Version != filters.Version {
			continue
		}
		filteredItems = append(filteredItems, item)
	}

	return dto.MigrationListResponse{
		Items: filteredItems,
		Total: len(filteredItems),
	}
}

