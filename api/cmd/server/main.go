package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "bfm/api/internal/api/http"
	pbapi "bfm/api/internal/api/protobuf"
	"bfm/api/internal/backends/etcd"
	"bfm/api/internal/backends/greptimedb"
	"bfm/api/internal/backends/postgresql"
	"bfm/api/internal/config"
	"bfm/api/internal/executor"
	"bfm/api/internal/logger"
	"bfm/api/internal/queuefactory"
	"bfm/api/internal/registry"
	statepg "bfm/api/internal/state/postgresql"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

func main() {
	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize state tracker
	stateConnStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.StateDB.Host,
		cfg.StateDB.Port,
		cfg.StateDB.Username,
		cfg.StateDB.Password,
		cfg.StateDB.Database,
	)

	stateTracker, err := statepg.NewTracker(stateConnStr, cfg.StateDB.Schema)
	if err != nil {
		logger.Fatalf("Failed to initialize state tracker: %v", err)
	}
	defer stateTracker.Close()

	logger.Info("Initializing BFM server...")

	// Initialize executor (using global registry)
	exec := executor.NewExecutor(registry.GlobalRegistry, stateTracker)
	if err := exec.SetConnections(cfg.Connections); err != nil {
		logger.Fatalf("Failed to set connections: %v", err)
	}

	// Initialize queue if enabled
	if cfg.Queue.Enabled {
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
		defer q.Close()

		exec.SetQueue(q)
		logger.Info("Queue enabled - migrations will be queued for async execution")
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

	// Start watching for new migration files
	loader.StartWatching()
	defer loader.StopWatching()

	// Initialize HTTP server
	router := gin.New()

	// Custom logger middleware that skips health check endpoints
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Skip logging for health check endpoints
		if param.Path == "/health" || param.Path == "/api/v1/health" {
			return ""
		}
		return fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %-7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			param.Path,
		)
	}))
	router.Use(gin.Recovery())

	// Add CORS middleware - must be before routes
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Client-Type")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	httpHandler := httpapi.NewHandler(exec)
	httpHandler.RegisterRoutes(router)

	// Add /health endpoint to prevent 404s (uses same handler as /api/v1/health)
	router.GET("/health", httpHandler.Health)

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    ":" + cfg.Server.HTTPPort,
		Handler: router,
	}

	go func() {
		logger.Infof("Starting HTTP server on port %s", cfg.Server.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Start gRPC server
	grpcServer := grpc.NewServer()
	pbServer := pbapi.NewServer(exec)
	pbapi.RegisterMigrationServiceServer(grpcServer, pbServer)

	grpcListener, err := net.Listen("tcp", ":"+cfg.Server.GRPCPort)
	if err != nil {
		logger.Fatalf("Failed to listen on gRPC port %s: %v", cfg.Server.GRPCPort, err)
	}

	go func() {
		logger.Infof("Starting gRPC server on port %s", cfg.Server.GRPCPort)
		if err := grpcServer.Serve(grpcListener); err != nil {
			logger.Fatalf("Failed to start gRPC server: %v", err)
		}
	}()

	logger.Info("BFM server started successfully")
	logger.Infof("HTTP API available at http://localhost:%s", cfg.Server.HTTPPort)
	logger.Infof("gRPC API available at localhost:%s", cfg.Server.GRPCPort)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down servers...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Warnf("HTTP server forced to shutdown: %v", err)
	}

	// Shutdown gRPC server
	grpcServer.GracefulStop()

	logger.Info("Servers exited")
}
