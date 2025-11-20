package dto

import "mops/bfm/internal/registry"

// MigrateRequest represents a migration request
type MigrateRequest struct {
	Target      *registry.MigrationTarget `json:"target" binding:"required"`
	Connection  string                     `json:"connection" binding:"required"`
	Schema      string                     `json:"schema"`       // Optional
	Environment string                     `json:"environment"`  // For dynamic schemas
	DryRun      bool                       `json:"dry_run"`      // Optional, default false
}

// MigrateResponse represents a migration response
type MigrateResponse struct {
	Success bool     `json:"success"`
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
	Errors  []string `json:"errors"`
}

