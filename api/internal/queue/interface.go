package queue

import (
	"context"
)

// Job represents a migration job to be queued
type Job struct {
	ID         string                 `json:"id"`
	Target     *MigrationTarget       `json:"target"`
	Connection string                 `json:"connection"`
	Schema     string                 `json:"schema,omitempty"`
	SchemaName string                 `json:"schema_name,omitempty"`
	DryRun     bool                   `json:"dry_run,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MigrationTarget specifies which migrations to execute
type MigrationTarget struct {
	Backend    string   `json:"backend,omitempty"`
	Schema     string   `json:"schema,omitempty"`
	Tables     []string `json:"tables,omitempty"`
	Version    string   `json:"version,omitempty"`
	Connection string   `json:"connection,omitempty"`
}

// JobResult represents the result of a migration job
type JobResult struct {
	JobID   string   `json:"job_id"`
	Success bool     `json:"success"`
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
	Errors  []string `json:"errors"`
}

// Producer publishes migration jobs to the queue
type Producer interface {
	// PublishJob publishes a migration job to the queue
	PublishJob(ctx context.Context, job *Job) error

	// Close closes the producer connection
	Close() error
}

// Consumer consumes migration jobs from the queue
type Consumer interface {
	// Consume starts consuming jobs from the queue
	// The handler function is called for each job
	Consume(ctx context.Context, handler JobHandler) error

	// Close closes the consumer connection
	Close() error
}

// JobHandler processes a migration job
type JobHandler func(ctx context.Context, job *Job) (*JobResult, error)

// Queue provides both producer and consumer capabilities
type Queue interface {
	Producer
	Consumer
}
