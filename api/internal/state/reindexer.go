package state

import (
	"context"
	"time"
)

// Reindexer handles background reindexing of migrations
type Reindexer struct {
	tracker  StateTracker
	registry interface{} // registry.Registry
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
}

// NewReindexer creates a new reindexer
func NewReindexer(tracker StateTracker, registry interface{}, interval time.Duration) *Reindexer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Reindexer{
		tracker:  tracker,
		registry: registry,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
		running:  false,
	}
}

// Start starts the background reindexing process
func (r *Reindexer) Start() {
	if r.running {
		return
	}
	r.running = true

	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		// Run immediately on start
		r.reindex()

		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				r.reindex()
			}
		}
	}()
}

// Stop stops the background reindexing process
func (r *Reindexer) Stop() {
	if !r.running {
		return
	}
	r.cancel()
	r.running = false
}

// reindex performs the reindexing operation
func (r *Reindexer) reindex() {
	if err := r.tracker.ReindexMigrations(r.ctx, r.registry); err != nil {
		// Log error (you may want to use a logger here)
		_ = err
	}
}
