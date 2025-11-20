package worker

import (
	"context"

	"mops/bfm/internal/executor"
	"mops/bfm/internal/logger"
	"mops/bfm/internal/queue"
	"mops/bfm/internal/registry"
)

// Worker processes migration jobs from the queue
type Worker struct {
	executor *executor.Executor
	queue    queue.Queue
}

// NewWorker creates a new migration worker
func NewWorker(exec *executor.Executor, q queue.Queue) *Worker {
	return &Worker{
		executor: exec,
		queue:    q,
	}
}

// Start starts the worker to consume and process jobs
func (w *Worker) Start(ctx context.Context) error {
	logger.Info("Starting migration worker...")

	// Create job handler
	handler := func(ctx context.Context, job *queue.Job) (*queue.JobResult, error) {
		return w.processJob(ctx, job)
	}

	// Start consuming from queue
	return w.queue.Consume(ctx, handler)
}

// processJob processes a single migration job
func (w *Worker) processJob(ctx context.Context, job *queue.Job) (*queue.JobResult, error) {
	logger.Infof("Processing migration job %s", job.ID)

	// Convert queue.MigrationTarget to registry.MigrationTarget
	target := convertQueueTarget(job.Target)

	// Execute migration
	result, err := w.executor.ExecuteSync(ctx, target, job.Connection, job.Schema, job.DryRun)
	if err != nil {
		return &queue.JobResult{
			JobID:   job.ID,
			Success: false,
			Errors:  []string{err.Error()},
		}, err
	}

	// Convert ExecuteResult to JobResult
	return &queue.JobResult{
		JobID:   job.ID,
		Success: result.Success,
		Applied: result.Applied,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	}, nil
}

// convertQueueTarget converts queue.MigrationTarget to registry.MigrationTarget
func convertQueueTarget(target *queue.MigrationTarget) *registry.MigrationTarget {
	if target == nil {
		return nil
	}
	return &registry.MigrationTarget{
		Backend:    target.Backend,
		Schema:     target.Schema,
		Tables:     target.Tables,
		Version:    target.Version,
		Connection: target.Connection,
	}
}

// Stop stops the worker
func (w *Worker) Stop() error {
	logger.Info("Stopping migration worker...")
	return w.queue.Close()
}
