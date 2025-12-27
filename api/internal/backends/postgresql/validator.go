package postgresql

import (
	"context"
	"fmt"
	"strings"

	"github.com/toolsascode/bfm/api/internal/backends"
	"github.com/toolsascode/bfm/api/internal/registry"
	"github.com/toolsascode/bfm/api/internal/state"
)

// DependencyValidator validates migration dependencies
type DependencyValidator struct {
	backend      *Backend
	stateTracker state.StateTracker
	registry     registry.Registry
}

// NewDependencyValidator creates a new dependency validator
func NewDependencyValidator(backend *Backend, tracker state.StateTracker, reg registry.Registry) *DependencyValidator {
	return &DependencyValidator{
		backend:      backend,
		stateTracker: tracker,
		registry:     reg,
	}
}

// ValidateDependencies validates all dependencies for a migration
func (v *DependencyValidator) ValidateDependencies(ctx context.Context, migration *backends.MigrationScript, schemaName string) []error {
	return v.ValidateDependenciesWithExecutionSet(ctx, migration, schemaName, nil)
}

// ValidateDependenciesWithExecutionSet validates all dependencies for a migration,
// considering migrations in the execution set as satisfied dependencies
func (v *DependencyValidator) ValidateDependenciesWithExecutionSet(ctx context.Context, migration *backends.MigrationScript, schemaName string, executionSet []*backends.MigrationScript) []error {
	var errors []error

	// Build a map of migration IDs in the execution set for quick lookup
	executionSetMap := make(map[string]bool)
	for _, m := range executionSet {
		// Generate migration ID using the same format as executor
		migrationID := fmt.Sprintf("%s_%s_%s_%s", m.Version, m.Name, m.Backend, m.Connection)
		executionSetMap[migrationID] = true
	}

	// Validate structured dependencies
	for _, dep := range migration.StructuredDependencies {
		if err := v.validateDependencyWithExecutionSet(ctx, dep, schemaName, executionSetMap); err != nil {
			errors = append(errors, fmt.Errorf("dependency validation failed for %s: %w", v.dependencyString(dep), err))
		}
	}

	// Validate simple string dependencies (backward compatibility)
	// For simple dependencies, we only check if the migration exists and is applied
	for _, depName := range migration.Dependencies {
		if err := v.validateSimpleDependencyWithExecutionSet(ctx, depName, schemaName, executionSetMap); err != nil {
			errors = append(errors, fmt.Errorf("dependency validation failed for '%s': %w", depName, err))
		}
	}

	return errors
}

// validateDependencyWithExecutionSet validates a single structured dependency,
// considering migrations in the execution set as satisfied dependencies
func (v *DependencyValidator) validateDependencyWithExecutionSet(ctx context.Context, dep backends.Dependency, currentSchema string, executionSetMap map[string]bool) error {
	// Validate required schema exists
	if dep.RequiresSchema != "" {
		exists, err := v.backend.SchemaExists(ctx, dep.RequiresSchema)
		if err != nil {
			return fmt.Errorf("failed to check schema existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("required schema '%s' does not exist", dep.RequiresSchema)
		}
	}

	// Validate required table exists
	// Note: If the table is created by a dependency migration in the execution set,
	// it won't exist yet, so we skip this check if the dependency is in the execution set
	if dep.RequiresTable != "" {
		// First check if the dependency migration is in the execution set
		targetMigrations, err := v.findMigrationByTarget(dep)
		if err == nil {
			// Check if any target migration is in the execution set
			dependencyInExecutionSet := false
			for _, targetMigration := range targetMigrations {
				migrationID := fmt.Sprintf("%s_%s_%s_%s", targetMigration.Version, targetMigration.Name, targetMigration.Backend, targetMigration.Connection)
				if executionSetMap != nil && executionSetMap[migrationID] {
					dependencyInExecutionSet = true
					break
				}
			}

			// If dependency is in execution set, skip table existence check (it will be created)
			if !dependencyInExecutionSet {
				schemaToCheck := dep.RequiresSchema
				if schemaToCheck == "" {
					// If RequiresSchema is not specified, use the dependency's Schema if available
					// This handles cross-schema dependencies where the table is in the dependency's schema
					if dep.Schema != "" {
						schemaToCheck = dep.Schema
					} else {
						// Fall back to current schema or default to public
						schemaToCheck = currentSchema
						if schemaToCheck == "" {
							schemaToCheck = "public"
						}
					}
				}
				exists, err := v.backend.TableExists(ctx, schemaToCheck, dep.RequiresTable)
				if err != nil {
					return fmt.Errorf("failed to check table existence: %w", err)
				}
				if !exists {
					return fmt.Errorf("required table '%s.%s' does not exist", schemaToCheck, dep.RequiresTable)
				}
			}
		}
	}

	// Validate dependency migration is applied or in execution set
	if err := v.validateMigrationAppliedWithExecutionSet(ctx, dep, executionSetMap); err != nil {
		return err
	}

	return nil
}

// validateSimpleDependencyWithExecutionSet validates a simple string dependency,
// considering migrations in the execution set as satisfied dependencies
func (v *DependencyValidator) validateSimpleDependencyWithExecutionSet(ctx context.Context, depName string, currentSchema string, executionSetMap map[string]bool) error {
	// Find migrations with this name
	targetMigrations := v.registry.GetMigrationByName(depName)
	if len(targetMigrations) == 0 {
		return fmt.Errorf("dependency migration '%s' not found", depName)
	}

	// Check if at least one of the target migrations is applied or in execution set
	// We check all found migrations and see if any are applied or in execution set
	for _, targetMigration := range targetMigrations {
		// Generate migration ID using executor format: {version}_{name}_{backend}_{connection}
		migrationID := fmt.Sprintf("%s_%s_%s_%s", targetMigration.Version, targetMigration.Name, targetMigration.Backend, targetMigration.Connection)

		// Check if in execution set
		if executionSetMap != nil && executionSetMap[migrationID] {
			return nil // Dependency is in execution set, will be executed
		}

		// Check if already applied
		applied, err := v.stateTracker.IsMigrationApplied(ctx, migrationID)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}
		if applied {
			return nil // At least one is applied, dependency satisfied
		}
	}

	return fmt.Errorf("dependency migration '%s' is not applied", depName)
}

// validateMigrationAppliedWithExecutionSet checks if a dependency migration is applied or in the execution set
func (v *DependencyValidator) validateMigrationAppliedWithExecutionSet(ctx context.Context, dep backends.Dependency, executionSetMap map[string]bool) error {
	// Find the target migration
	targetMigrations, err := v.findMigrationByTarget(dep)
	if err != nil {
		return fmt.Errorf("dependency target not found: %w", err)
	}

	// Check if at least one target migration is applied or in execution set
	for _, targetMigration := range targetMigrations {
		// Generate migration ID using the same format as executor: {version}_{name}_{backend}_{connection}
		migrationID := fmt.Sprintf("%s_%s_%s_%s", targetMigration.Version, targetMigration.Name, targetMigration.Backend, targetMigration.Connection)

		// Check if in execution set
		if executionSetMap != nil && executionSetMap[migrationID] {
			return nil // Dependency is in execution set, will be executed
		}

		// Check if already applied
		// Use the same ID format as executor for state tracker
		applied, err := v.stateTracker.IsMigrationApplied(ctx, migrationID)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}
		if applied {
			return nil // At least one is applied, dependency satisfied
		}
	}

	return fmt.Errorf("dependency migration is not applied: %s", v.dependencyString(dep))
}

// findMigrationByTarget finds migration(s) matching a dependency target
func (v *DependencyValidator) findMigrationByTarget(dep backends.Dependency) ([]*backends.MigrationScript, error) {
	var candidates []*backends.MigrationScript

	// Get all migrations
	allMigrations := v.registry.GetAll()

	// Filter by target type
	for _, migration := range allMigrations {
		// Match connection if specified
		if dep.Connection != "" && migration.Connection != dep.Connection {
			continue
		}

		// Match schema if specified
		if dep.Schema != "" && migration.Schema != dep.Schema {
			continue
		}

		// Match target based on type
		if dep.TargetType == "version" {
			if migration.Version == dep.Target {
				candidates = append(candidates, migration)
			}
		} else {
			// Default to "name"
			if migration.Name == dep.Target {
				candidates = append(candidates, migration)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("connection=%s, schema=%s, target=%s, type=%s",
			dep.Connection, dep.Schema, dep.Target, dep.TargetType)
	}

	return candidates, nil
}

// dependencyString returns a string representation of a dependency for error messages
func (v *DependencyValidator) dependencyString(dep backends.Dependency) string {
	parts := []string{}
	if dep.Connection != "" {
		parts = append(parts, fmt.Sprintf("connection=%s", dep.Connection))
	}
	if dep.Schema != "" {
		parts = append(parts, fmt.Sprintf("schema=%s", dep.Schema))
	}
	parts = append(parts, fmt.Sprintf("target=%s", dep.Target))
	if dep.TargetType != "" && dep.TargetType != "name" {
		parts = append(parts, fmt.Sprintf("type=%s", dep.TargetType))
	}
	return strings.Join(parts, ", ")
}
