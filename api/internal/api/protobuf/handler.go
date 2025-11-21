package protobuf

import (
	"context"
	"fmt"
	"sort"
	"time"

	"bfm/api/internal/backends"
	"bfm/api/internal/executor"
	"bfm/api/internal/registry"
	"bfm/api/internal/state"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the MigrationServiceServer interface
type Server struct {
	UnimplementedMigrationServiceServer
	executor *executor.Executor
}

// NewServer creates a new gRPC server
func NewServer(exec *executor.Executor) *Server {
	return &Server{
		executor: exec,
	}
}

// Migrate executes database migrations
func (s *Server) Migrate(ctx context.Context, req *MigrateRequest) (*MigrateResponse, error) {
	if req == nil || req.Target == nil {
		return nil, status.Error(codes.InvalidArgument, "request and target are required")
	}

	// Convert protobuf target to registry target
	target := &registry.MigrationTarget{
		Backend:    req.Target.Backend,
		Schema:     req.Target.Schema,
		Tables:     req.Target.Tables,
		Version:    req.Target.Version,
		Connection: req.Target.Connection,
	}

	// Resolve schema (use schema_name if provided for dynamic schemas)
	schema := req.Schema
	if schema == "" && req.SchemaName != "" {
		// For dynamic schemas, use schema_name value directly as schema name
		schema = req.SchemaName
	}

	// Execute migrations
	result, err := s.executor.Execute(ctx, target, req.Connection, schema, req.DryRun)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to execute migrations: %v", err)
	}

	// Convert result to protobuf response
	response := &MigrateResponse{
		Success: result.Success,
		Applied: result.Applied,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	}

	return response, nil
}

// StreamMigrate executes migrations with streaming progress updates
func (s *Server) StreamMigrate(req *MigrateRequest, stream MigrationService_StreamMigrateServer) error {
	if req == nil || req.Target == nil {
		return status.Error(codes.InvalidArgument, "request and target are required")
	}

	// Convert protobuf target to registry target
	target := &registry.MigrationTarget{
		Backend:    req.Target.Backend,
		Schema:     req.Target.Schema,
		Tables:     req.Target.Tables,
		Version:    req.Target.Version,
		Connection: req.Target.Connection,
	}

	// Resolve schema (use schema_name if provided for dynamic schemas)
	schema := req.Schema
	if schema == "" && req.SchemaName != "" {
		// For dynamic schemas, use schema_name value directly as schema name
		schema = req.SchemaName
	}

	// Get migrations matching target
	migrations, err := s.executor.GetRegistry().FindByTarget(target)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to find migrations: %v", err)
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	total := len(migrations)
	for i, migration := range migrations {
		tablePart := ""
		if migration.Table != nil {
			tablePart = *migration.Table + "_"
		}
		migrationID := fmt.Sprintf("%s_%s%s_%s", migration.Schema, tablePart, migration.Version, migration.Name)

		// Send progress update
		progress := &MigrateProgress{
			MigrationId: migrationID,
			Status:      "running",
			Message:     fmt.Sprintf("Executing migration %d of %d", i+1, total),
			Progress:    int32((i + 1) * 100 / total),
		}

		if err := stream.Send(progress); err != nil {
			return status.Errorf(codes.Internal, "failed to send progress: %v", err)
		}

		// Check if already applied
		applied, err := s.executor.IsMigrationApplied(stream.Context(), migrationID)
		if err != nil {
			progress.Status = "failed"
			progress.Message = fmt.Sprintf("Error checking status: %v", err)
			stream.Send(progress)
			continue
		}

		if applied {
			progress.Status = "skipped"
			progress.Message = "Migration already applied"
			stream.Send(progress)
			continue
		}

		// Execute migration (simplified - in production, you'd want more error handling)
		if !req.DryRun {
			// Execute migration using executor (simplified)
			// In production, you'd want to use the executor's Execute method
			// but for streaming, we need to execute one at a time
			connectionConfig, err := s.executor.GetConnectionConfig(migration.Connection)
			if err != nil {
				progress.Status = "failed"
				progress.Message = fmt.Sprintf("Failed to get connection config: %v", err)
				stream.Send(progress)
				continue
			}

			backend := s.executor.GetBackend(connectionConfig.Backend)
			if backend == nil {
				progress.Status = "failed"
				progress.Message = fmt.Sprintf("Backend %s not found", connectionConfig.Backend)
				stream.Send(progress)
				continue
			}

			if err := backend.Connect(connectionConfig); err != nil {
				progress.Status = "failed"
				progress.Message = fmt.Sprintf("Failed to connect: %v", err)
				stream.Send(progress)
				continue
			}

			backendMigration := &backends.MigrationScript{
				Schema:     migration.Schema,
				Table:      migration.Table, // Already *string, can be nil
				Version:    migration.Version,
				Name:       migration.Name,
				Connection: migration.Connection,
				Backend:    migration.Backend,
				UpSQL:      migration.UpSQL,
				DownSQL:    migration.DownSQL,
			}

			err = backend.ExecuteMigration(stream.Context(), backendMigration)
			backend.Close()

			if err != nil {
				progress.Status = "failed"
				progress.Message = err.Error()
				stream.Send(progress)
				continue
			}

			// Record migration in state tracker
			tableValue := ""
			if migration.Table != nil {
				tableValue = *migration.Table
			}
			record := &state.MigrationRecord{
				MigrationID:  migrationID,
				Schema:       migration.Schema,
				Table:        tableValue,
				Version:      migration.Version,
				Connection:   migration.Connection,
				Backend:      migration.Backend,
				Status:       "success",
				AppliedAt:    time.Now().Format(time.RFC3339),
				ErrorMessage: "",
			}
			s.executor.GetStateTracker().RecordMigration(stream.Context(), record)
		}

		progress.Status = "success"
		progress.Message = "Migration completed successfully"
		stream.Send(progress)
	}

	return nil
}

// Helper methods (these would need to be added to executor)
// For now, we'll add them as needed
