package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/toolsascode/bfm/api/internal/backends/etcd"
	"github.com/toolsascode/bfm/api/internal/backends/greptimedb"
	"github.com/toolsascode/bfm/api/internal/backends/postgresql"
	"github.com/toolsascode/bfm/api/internal/config"
	"github.com/toolsascode/bfm/api/internal/executor"
	"github.com/toolsascode/bfm/api/internal/logger"
	"github.com/toolsascode/bfm/api/internal/queuefactory"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"
	statepg "github.com/toolsascode/bfm/api/internal/state/postgresql"
	"github.com/toolsascode/bfm/api/internal/worker"
)

func main() {
	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Check if queue is enabled
	if !cfg.Queue.Enabled {
		logger.Fatalf("Queue is not enabled. Set BFM_QUEUE_ENABLED=true to use the worker")
	}

	// Initialize state tracker
	var stateTracker state.StateTracker
	switch cfg.StateDB.Type {
	case "postgresql":
		stateConnStr := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			cfg.StateDB.Host,
			cfg.StateDB.Port,
			cfg.StateDB.Username,
			cfg.StateDB.Password,
			cfg.StateDB.Database,
		)
		stateTracker, err = statepg.NewTracker(stateConnStr, cfg.StateDB.Schema)
		if err != nil {
			logger.Fatalf("Failed to create state tracker: %v", err)
		}
		// Note: Close is handled by the concrete Tracker type, not the interface
		// We'll close it explicitly if needed, but NewTracker already initializes
	default:
		logger.Fatalf("Unsupported state backend: %s", cfg.StateDB.Type)
	}

	// Initialize state tracker
	if err := stateTracker.Initialize(context.Background()); err != nil {
		logger.Fatalf("Failed to initialize state tracker: %v", err)
	}

	// Create executor (using global registry)
	exec := executor.NewExecutor(registry.GlobalRegistry, stateTracker)
	if err := exec.SetConnections(cfg.Connections); err != nil {
		logger.Fatalf("Failed to set connections: %v", err)
	}

	// Register backends
	pgBackend := postgresql.NewBackend()
	exec.RegisterBackend("postgresql", pgBackend)

	gtBackend := greptimedb.NewBackend()
	exec.RegisterBackend("greptimedb", gtBackend)

	etcdBackend := etcd.NewBackend()
	exec.RegisterBackend("etcd", etcdBackend)

	// Dynamically load migration scripts from SFM directory
	sfmPath := os.Getenv("BFM_SFM_PATH")
	if sfmPath == "" {
		// Default to ../sfm relative to bfm directory
		sfmPath = "../sfm"
	}

	loader := executor.NewLoader(sfmPath)
	loader.SetExecutor(exec) // Set executor so loader can register scanned migrations
	if err := loader.LoadAll(registry.GlobalRegistry); err != nil {
		logger.Fatalf("Failed to load migrations: %v", err)
	}

	migrationCount := len(registry.GlobalRegistry.GetAll())
	logger.Infof("Loaded %d migration(s) from %s", migrationCount, sfmPath)

	// Create queue
	queueConfig := &queuefactory.QueueConfig{
		Type:               cfg.Queue.Type,
		KafkaBrokers:       cfg.Queue.KafkaBrokers,
		KafkaTopic:         cfg.Queue.KafkaTopic,
		KafkaGroupID:       cfg.Queue.KafkaGroupID,
		PulsarURL:          cfg.Queue.PulsarURL,
		PulsarTopic:        cfg.Queue.PulsarTopic,
		PulsarSubscription: cfg.Queue.PulsarSubscription,
	}

	q, err := queuefactory.NewQueue(queueConfig)
	if err != nil {
		logger.Fatalf("Failed to create queue: %v", err)
	}
	defer func() { _ = q.Close() }()

	// Create worker
	w := worker.NewWorker(exec, q)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start worker in goroutine
	go func() {
		if err := w.Start(ctx); err != nil {
			logger.Errorf("Worker error: %v", err)
			cancel()
		}
	}()

	logger.Info("Migration worker started. Press Ctrl+C to stop.")

	// Wait for signal
	<-sigChan
	logger.Info("Shutting down worker...")

	// Stop worker
	if err := w.Stop(); err != nil {
		logger.Errorf("Error stopping worker: %v", err)
	}

	logger.Info("Worker stopped")
}
