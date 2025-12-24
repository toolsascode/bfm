package protobuf

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/executor"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"

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
			_ = stream.Send(progress)
			continue
		}

		if applied {
			progress.Status = "skipped"
			progress.Message = "Migration already applied"
			_ = stream.Send(progress)
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
				_ = stream.Send(progress)
				continue
			}

			backend := s.executor.GetBackend(connectionConfig.Backend)
			if backend == nil {
				progress.Status = "failed"
				progress.Message = fmt.Sprintf("Backend %s not found", connectionConfig.Backend)
				_ = stream.Send(progress)
				continue
			}

			if err := backend.Connect(connectionConfig); err != nil {
				progress.Status = "failed"
				progress.Message = fmt.Sprintf("Failed to connect: %v", err)
				_ = stream.Send(progress)
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
			_ = backend.Close()

			if err != nil {
				progress.Status = "failed"
				progress.Message = err.Error()
				_ = stream.Send(progress)
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
			_ = s.executor.GetStateTracker().RecordMigration(stream.Context(), record)
		}

		progress.Status = "success"
		progress.Message = "Migration completed successfully"
		_ = stream.Send(progress)
	}

	return nil
}

// MigrateDown executes down migrations (rollback)
func (s *Server) MigrateDown(ctx context.Context, req *MigrateDownRequest) (*MigrateResponse, error) {
	if req == nil || req.MigrationId == "" {
		return nil, status.Error(codes.InvalidArgument, "request and migration_id are required")
	}

	// Convert schemas slice
	schemas := req.Schemas
	if len(schemas) == 0 {
		schemas = []string{""}
	}

	// Execute down migrations
	result, err := s.executor.ExecuteDown(ctx, req.MigrationId, schemas, req.DryRun)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to execute down migrations: %v", err)
	}

	response := &MigrateResponse{
		Success: result.Success,
		Applied: result.Applied,
		Skipped: result.Skipped,
		Errors:  result.Errors,
	}

	return response, nil
}

// ListMigrations lists all migrations with optional filtering
func (s *Server) ListMigrations(ctx context.Context, req *ListMigrationsRequest) (*ListMigrationsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	// Convert protobuf filters to state filters
	stateFilters := &state.MigrationFilters{
		Schema:     req.Schema,
		Table:      req.Table,
		Connection: req.Connection,
		Backend:    req.Backend,
		Status:     req.Status,
		Version:    req.Version,
	}

	// Get migration list from state tracker
	migrationList, err := s.executor.GetMigrationList(ctx, stateFilters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get migration list: %v", err)
	}

	// Convert to protobuf response
	items := make([]*MigrationListItem, 0, len(migrationList))
	for _, item := range migrationList {
		items = append(items, &MigrationListItem{
			MigrationId:  item.MigrationID,
			Schema:       item.Schema,
			Table:        item.Table,
			Version:      item.Version,
			Name:         item.Name,
			Connection:   item.Connection,
			Backend:      item.Backend,
			Applied:      item.Applied,
			Status:       item.LastStatus,
			AppliedAt:    item.LastAppliedAt,
			ErrorMessage: item.LastErrorMessage,
		})
	}

	response := &ListMigrationsResponse{
		Items: items,
		Total: int32(len(items)),
	}

	return response, nil
}

// GetMigration gets detailed information about a specific migration
func (s *Server) GetMigration(ctx context.Context, req *GetMigrationRequest) (*MigrationDetailResponse, error) {
	if req == nil || req.MigrationId == "" {
		return nil, status.Error(codes.InvalidArgument, "request and migration_id are required")
	}

	// Get migration from registry
	migration := s.executor.GetMigrationByID(req.MigrationId)
	if migration == nil {
		return nil, status.Errorf(codes.NotFound, "migration not found: %s", req.MigrationId)
	}

	// Get status from state tracker
	applied, err := s.executor.IsMigrationApplied(ctx, req.MigrationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check migration status: %v", err)
	}

	// Get schema and table from state tracker
	var schemaValue, tableValue string
	migrationList, err := s.executor.GetMigrationList(ctx, &state.MigrationFilters{})
	if err == nil {
		for _, item := range migrationList {
			if item.MigrationID == req.MigrationId {
				schemaValue = item.Schema
				tableValue = item.Table
				break
			}
		}
	}

	// Fallback to registry values if not found in state tracker
	if tableValue == "" && migration.Table != nil {
		tableValue = *migration.Table
	}
	if schemaValue == "" {
		schemaValue = migration.Schema
	}

	// Convert structured dependencies to response format
	structuredDeps := make([]*DependencyResponse, 0, len(migration.StructuredDependencies))
	for _, dep := range migration.StructuredDependencies {
		structuredDeps = append(structuredDeps, &DependencyResponse{
			Connection:     dep.Connection,
			Schema:         dep.Schema,
			Target:         dep.Target,
			TargetType:     dep.TargetType,
			RequiresTable:  dep.RequiresTable,
			RequiresSchema: dep.RequiresSchema,
		})
	}

	response := &MigrationDetailResponse{
		MigrationId:            req.MigrationId,
		Schema:                 schemaValue,
		Table:                  tableValue,
		Version:                migration.Version,
		Name:                   migration.Name,
		Connection:             migration.Connection,
		Backend:                migration.Backend,
		Applied:                applied,
		UpSql:                  migration.UpSQL,
		DownSql:                migration.DownSQL,
		Dependencies:           migration.Dependencies,
		StructuredDependencies: structuredDeps,
	}

	return response, nil
}

// GetMigrationStatus gets the current status of a specific migration
func (s *Server) GetMigrationStatus(ctx context.Context, req *GetMigrationStatusRequest) (*MigrationStatusResponse, error) {
	if req == nil || req.MigrationId == "" {
		return nil, status.Error(codes.InvalidArgument, "request and migration_id are required")
	}

	// Get all migration history to find the latest status
	allHistory, err := s.executor.GetMigrationHistory(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get migration history: %v", err)
	}

	// Find all related records (base migration and rollbacks)
	var relatedRecords []*state.MigrationRecord
	for _, record := range allHistory {
		if record.MigrationID == req.MigrationId ||
			record.MigrationID == req.MigrationId+"_rollback" ||
			(len(record.MigrationID) > len(req.MigrationId) && record.MigrationID[:len(req.MigrationId)] == req.MigrationId && record.MigrationID[len(req.MigrationId)] == '_') {
			relatedRecords = append(relatedRecords, record)
		}
	}

	// Determine applied status and get latest applied_at
	applied := false
	statusVal := "pending"
	var appliedAt string
	var errorMessage string

	if len(relatedRecords) > 0 {
		// Get the latest record (first in the list since history is sorted DESC)
		latestRecord := relatedRecords[0]

		// Find the latest successful, non-rollback record
		var latestSuccessRecord *state.MigrationRecord
		for _, record := range relatedRecords {
			if !strings.Contains(record.MigrationID, "_rollback") && record.Status == "success" {
				latestSuccessRecord = record
				break
			}
		}

		// Find the latest rollback record
		var latestRollbackRecord *state.MigrationRecord
		for _, record := range relatedRecords {
			if strings.Contains(record.MigrationID, "_rollback") {
				latestRollbackRecord = record
				break
			}
		}

		// Determine status based on which is more recent
		if latestSuccessRecord != nil && latestRollbackRecord != nil {
			successTime, _ := time.Parse(time.RFC3339, latestSuccessRecord.AppliedAt)
			rollbackTime, _ := time.Parse(time.RFC3339, latestRollbackRecord.AppliedAt)

			if successTime.After(rollbackTime) {
				applied = true
				statusVal = latestSuccessRecord.Status
				appliedAt = latestSuccessRecord.AppliedAt
				errorMessage = latestSuccessRecord.ErrorMessage
			} else {
				applied = false
				statusVal = "rolled_back"
				appliedAt = latestSuccessRecord.AppliedAt
				errorMessage = latestRollbackRecord.ErrorMessage
			}
		} else if latestSuccessRecord != nil {
			applied = true
			statusVal = latestSuccessRecord.Status
			appliedAt = latestSuccessRecord.AppliedAt
			errorMessage = latestSuccessRecord.ErrorMessage
		} else if latestRollbackRecord != nil {
			applied = false
			statusVal = "rolled_back"
			errorMessage = latestRollbackRecord.ErrorMessage
		} else {
			applied = !strings.Contains(latestRecord.MigrationID, "_rollback")
			statusVal = latestRecord.Status
			appliedAt = latestRecord.AppliedAt
			errorMessage = latestRecord.ErrorMessage
		}
	}

	response := &MigrationStatusResponse{
		MigrationId: req.MigrationId,
		Status:      statusVal,
		Applied:     applied,
	}

	if appliedAt != "" {
		response.AppliedAt = appliedAt
	}

	if errorMessage != "" {
		response.ErrorMessage = errorMessage
	}

	return response, nil
}

// GetMigrationHistory gets the execution history for a specific migration
func (s *Server) GetMigrationHistory(ctx context.Context, req *GetMigrationHistoryRequest) (*MigrationHistoryResponse, error) {
	if req == nil || req.MigrationId == "" {
		return nil, status.Error(codes.InvalidArgument, "request and migration_id are required")
	}

	// Get migration from registry to verify it exists
	migration := s.executor.GetMigrationByID(req.MigrationId)
	if migration == nil {
		return nil, status.Errorf(codes.NotFound, "migration not found: %s", req.MigrationId)
	}

	// Get all migration history
	allHistory, err := s.executor.GetMigrationHistory(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get migration history: %v", err)
	}

	// Filter history to include related records
	var relatedHistory []*state.MigrationRecord
	for _, record := range allHistory {
		if record.MigrationID == req.MigrationId ||
			record.MigrationID == req.MigrationId+"_rollback" ||
			(len(record.MigrationID) > len(req.MigrationId) && record.MigrationID[:len(req.MigrationId)] == req.MigrationId && record.MigrationID[len(req.MigrationId)] == '_') {
			relatedHistory = append(relatedHistory, record)
		}
	}

	// Convert to response format
	historyItems := make([]*MigrationHistoryItem, 0, len(relatedHistory))
	for _, record := range relatedHistory {
		historyItems = append(historyItems, &MigrationHistoryItem{
			MigrationId:      record.MigrationID,
			Schema:           record.Schema,
			Table:            record.Table,
			Version:          record.Version,
			Connection:       record.Connection,
			Backend:          record.Backend,
			AppliedAt:        record.AppliedAt,
			Status:           record.Status,
			ErrorMessage:     record.ErrorMessage,
			ExecutedBy:       record.ExecutedBy,
			ExecutionMethod:  record.ExecutionMethod,
			ExecutionContext: record.ExecutionContext,
		})
	}

	response := &MigrationHistoryResponse{
		MigrationId: req.MigrationId,
		History:     historyItems,
	}

	return response, nil
}

// RollbackMigration rolls back a specific migration
func (s *Server) RollbackMigration(ctx context.Context, req *RollbackMigrationRequest) (*RollbackResponse, error) {
	if req == nil || req.MigrationId == "" {
		return nil, status.Error(codes.InvalidArgument, "request and migration_id are required")
	}

	// Get migration from registry
	migration := s.executor.GetMigrationByID(req.MigrationId)
	if migration == nil {
		return nil, status.Errorf(codes.NotFound, "migration not found: %s", req.MigrationId)
	}

	// Check if migration is applied
	applied, err := s.executor.IsMigrationApplied(ctx, req.MigrationId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check migration status: %v", err)
	}

	if !applied {
		return nil, status.Errorf(codes.FailedPrecondition, "migration is not applied: %s", req.MigrationId)
	}

	// Execute rollback with schemas
	result, err := s.executor.Rollback(ctx, req.MigrationId, req.Schemas)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to rollback migration: %v", err)
	}

	response := &RollbackResponse{
		Success: result.Success,
		Message: result.Message,
		Errors:  result.Errors,
	}

	return response, nil
}

// ReindexMigrations reindexes all migration files and synchronizes with database
func (s *Server) ReindexMigrations(ctx context.Context, req *ReindexMigrationsRequest) (*ReindexResponse, error) {
	// Get SFM path from request or environment variable
	sfmPath := req.SfmPath
	if sfmPath == "" {
		sfmPath = os.Getenv("BFM_SFM_PATH")
		if sfmPath == "" {
			// Default to ../sfm relative to bfm directory
			sfmPath = "../sfm"
		}
	}

	result, err := s.executor.ReindexMigrations(ctx, sfmPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to reindex migrations: %v", err)
	}

	response := &ReindexResponse{
		Added:   result.Added,
		Removed: result.Removed,
		Updated: result.Updated,
		Total:   int32(result.Total),
	}

	return response, nil
}

// Health checks the health status of the service
func (s *Server) Health(ctx context.Context, req *HealthRequest) (*HealthResponse, error) {
	healthStatus := "healthy"
	checks := make(map[string]string)

	// Add backend health checks if executor supports it
	if err := s.executor.HealthCheck(ctx); err != nil {
		healthStatus = "unhealthy"
		checks["executor"] = err.Error()
	} else {
		checks["executor"] = "ok"
	}

	response := &HealthResponse{
		Status: healthStatus,
		Checks: checks,
	}

	return response, nil
}
