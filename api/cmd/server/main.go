package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httpapi "github.com/toolsascode/bfm/api/internal/api/http"
	pbapi "github.com/toolsascode/bfm/api/internal/api/protobuf"
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

	_ "github.com/toolsascode/bfm/api/docs"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// @title           Backend For Migrations (BfM) API
// @version         0.3.0
// @description     BfM is a comprehensive database migration system that supports multiple backends
// @description     (PostgreSQL, GreptimeDB, Etcd) with HTTP and Protobuf APIs.
// @description
// @description     This API allows you to:
// @description     - Execute up migrations
// @description     - Execute down migrations (rollback)
// @description     - List migrations with filtering
// @description     - Get migration details and status
// @description     - View migration history
// @description     - Check system health
//
// @contact.name   BfM Support
// @contact.url    https://github.com/toolsascode/bfm
//
// @license.name  Apache 2.0
// @license.url   https://www.apache.org/licenses/LICENSE-2.0.html
//
// @host      localhost:7070
// @BasePath  /api/v1
//
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description API token authentication. Include the token in the Authorization header: Authorization: Bearer {BFM_API_TOKEN}

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
	defer func() { _ = stateTracker.Close() }()

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
		defer func() { _ = q.Close() }()

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

	// Validate SFM path exists
	if _, err := os.Stat(sfmPath); os.IsNotExist(err) {
		logger.Fatalf("SFM directory does not exist: %s (set BFM_SFM_PATH environment variable)", sfmPath)
	}

	logger.Infof("Loading migrations from SFM directory: %s", sfmPath)

	loader := executor.NewLoader(sfmPath)
	loader.SetExecutor(exec) // Set executor so loader can register scanned migrations
	if err := loader.LoadAll(registry.GlobalRegistry); err != nil {
		logger.Fatalf("Failed to load migrations from %s: %v", sfmPath, err)
	}

	migrationCount := len(registry.GlobalRegistry.GetAll())
	if migrationCount == 0 {
		logger.Warnf("No migrations loaded from %s - ensure migration files exist in the expected directory structure", sfmPath)
	} else {
		logger.Infof("Successfully loaded %d migration(s) from %s", migrationCount, sfmPath)

		// Log migration breakdown by backend/connection for better visibility
		allMigrations := registry.GlobalRegistry.GetAll()
		backendCounts := make(map[string]map[string]int)
		for _, mig := range allMigrations {
			if backendCounts[mig.Backend] == nil {
				backendCounts[mig.Backend] = make(map[string]int)
			}
			backendCounts[mig.Backend][mig.Connection]++
		}

		for backend, connections := range backendCounts {
			for connection, count := range connections {
				logger.Infof("  - %s/%s: %d migration(s)", backend, connection, count)
			}
		}
	}

	// Start watching for new migration files
	loader.StartWatching()
	defer loader.StopWatching()

	// Start background reindexer
	reindexInterval := 5 * time.Minute
	if intervalStr := os.Getenv("BFM_REINDEX_INTERVAL_MINUTES"); intervalStr != "" {
		if intervalMinutes, err := time.ParseDuration(intervalStr + "m"); err == nil {
			reindexInterval = intervalMinutes
		}
	}
	reindexer := state.NewReindexer(stateTracker, registry.GlobalRegistry, reindexInterval)
	reindexer.Start()
	defer reindexer.Stop()
	logger.Infof("Background reindexer started with interval: %v", reindexInterval)

	// Set Gin mode - use BFM_APP_MODE env var if set, otherwise default to release mode
	if ginMode := os.Getenv("BFM_APP_MODE"); ginMode != "" {
		gin.SetMode(ginMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize HTTP server
	router := gin.New()

	// Determine log format from environment variable (default to JSON)
	logFormat := strings.ToLower(os.Getenv("BFM_LOG_FORMAT"))
	useJSON := logFormat != "plaintext" && logFormat != "plain" && logFormat != "text"

	// Custom logger middleware that skips health check endpoints and supports JSON/plaintext
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Skip logging for health check endpoints
		if param.Path == "/health" || param.Path == "/api/v1/health" {
			return ""
		}

		if useJSON {
			// JSON format
			logEntry := map[string]interface{}{
				"timestamp": param.TimeStamp.Format("2006-01-02T15:04:05.000Z07:00"),
				"status":    param.StatusCode,
				"latency":   param.Latency.String(),
				"client_ip": param.ClientIP,
				"method":    param.Method,
				"path":      param.Path,
				"error":     param.ErrorMessage,
			}
			if jsonBytes, err := json.Marshal(logEntry); err == nil {
				return string(jsonBytes) + "\n"
			}
		}

		// Plaintext format (fallback or when explicitly set)
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

	// Serve static files from frontend directory if it exists
	frontendPath := os.Getenv("BFM_FRONTEND_PATH")
	if frontendPath == "" {
		frontendPath = "/app/frontend"
	}

	// Check if frontend directory exists
	if _, err := os.Stat(frontendPath); err == nil {
		// Serve static files for non-API routes
		// Use NoRoute to catch all routes that don't match API endpoints
		router.NoRoute(func(c *gin.Context) {
			// Don't serve frontend for API routes
			if strings.HasPrefix(c.Request.URL.Path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}

			// Try to serve the requested file if it exists
			filePath := frontendPath + c.Request.URL.Path
			if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
				c.File(filePath)
				return
			}

			// For SPA routing, serve index.html for any route that doesn't match a file
			c.File(frontendPath + "/index.html")
		})

		logger.Infof("Frontend static files will be served from %s", frontendPath)
	} else {
		logger.Warnf("Frontend directory not found at %s, skipping static file serving", frontendPath)
	}

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
