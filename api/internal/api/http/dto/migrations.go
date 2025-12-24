package dto

import "github.com/toolsascode/bfm/api/internal/registry"

// MigrationListFilters specifies filters for listing migrations
type MigrationListFilters struct {
	Schema     string `form:"schema"`
	Table      string `form:"table"`
	Connection string `form:"connection"`
	Backend    string `form:"backend"`
	Status     string `form:"status"`
	Version    string `form:"version"`
}

// MigrationListResponse represents a list of migrations
type MigrationListResponse struct {
	Items []MigrationListItem `json:"items"`
	Total int                 `json:"total"`
}

// MigrationListItem represents a single migration in the list
type MigrationListItem struct {
	MigrationID  string `json:"migration_id"`
	Schema       string `json:"schema"`
	Table        string `json:"table"`
	Version      string `json:"version"`
	Name         string `json:"name"`
	Connection   string `json:"connection"`
	Backend      string `json:"backend"`
	Applied      bool   `json:"applied"`
	Status       string `json:"status"`
	AppliedAt    string `json:"applied_at,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// DependencyResponse represents a structured dependency
type DependencyResponse struct {
	Connection     string `json:"connection"`
	Schema         string `json:"schema"`
	Target         string `json:"target"`
	TargetType     string `json:"target_type"`
	RequiresTable  string `json:"requires_table,omitempty"`
	RequiresSchema string `json:"requires_schema,omitempty"`
}

// MigrationDetailResponse represents detailed migration information
type MigrationDetailResponse struct {
	MigrationID            string               `json:"migration_id"`
	Schema                 string               `json:"schema"`
	Table                  string               `json:"table"`
	Version                string               `json:"version"`
	Name                   string               `json:"name"`
	Connection             string               `json:"connection"`
	Backend                string               `json:"backend"`
	Applied                bool                 `json:"applied"`
	UpSQL                  string               `json:"up_sql,omitempty"`                  // Contains SQL for SQL backends or JSON for NoSQL backends
	DownSQL                string               `json:"down_sql,omitempty"`                // Contains SQL for SQL backends or JSON for NoSQL backends
	Dependencies           []string             `json:"dependencies,omitempty"`            // List of migration names this migration depends on (backward compatibility)
	StructuredDependencies []DependencyResponse `json:"structured_dependencies,omitempty"` // Structured dependencies with validation requirements
}

// RollbackRequest represents a request to rollback a migration
type RollbackRequest struct {
	Schemas []string `json:"schemas,omitempty"` // Array for dynamic schemas
}

// RollbackResponse represents a rollback operation result
type RollbackResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Errors  []string `json:"errors,omitempty"`
}

// ReindexResponse represents the result of a reindex operation
type ReindexResponse struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Updated []string `json:"updated"`
	Total   int      `json:"total"`
}

// MigrateUpRequest represents a request to execute up migrations
type MigrateUpRequest struct {
	Target     *registry.MigrationTarget `json:"target"`
	Connection string                    `json:"connection" binding:"required"`
	Schemas    []string                  `json:"schemas"` // Array for dynamic schemas
	DryRun     bool                      `json:"dry_run"`
}

// MigrationExecutionResponse represents an execution record from migrations_executions
type MigrationExecutionResponse struct {
	ID          int    `json:"id"`
	MigrationID string `json:"migration_id"`
	Schema      string `json:"schema"`
	Version     string `json:"version"`
	Connection  string `json:"connection"`
	Backend     string `json:"backend"`
	Status      string `json:"status"`
	Applied     bool   `json:"applied"`
	AppliedAt   string `json:"applied_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// MigrateDownRequest represents a request to execute down migrations
type MigrateDownRequest struct {
	MigrationID string   `json:"migration_id" binding:"required"`
	Schemas     []string `json:"schemas"` // Array for dynamic schemas
	DryRun      bool     `json:"dry_run"`
}
