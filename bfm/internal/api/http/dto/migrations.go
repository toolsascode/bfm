package dto

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

// MigrationDetailResponse represents detailed migration information
type MigrationDetailResponse struct {
	MigrationID string `json:"migration_id"`
	Schema      string `json:"schema"`
	Table       string `json:"table"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Connection  string `json:"connection"`
	Backend     string `json:"backend"`
	Applied     bool   `json:"applied"`
}

// RollbackResponse represents a rollback operation result
type RollbackResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Errors  []string `json:"errors,omitempty"`
}

