package postgresql

import (
	"context"
	"fmt"
	"strings"

	"bfm/api/internal/backends"
	"bfm/api/internal/registry"
	"bfm/api/internal/state"
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
	var errors []error

	// Validate structured dependencies
	for _, dep := range migration.StructuredDependencies {
		if err := v.validateDependency(ctx, dep, schemaName); err != nil {
			errors = append(errors, fmt.Errorf("dependency validation failed for %s: %w", v.dependencyString(dep), err))
		}
	}

	// Validate simple string dependencies (backward compatibility)
	// For simple dependencies, we only check if the migration exists and is applied
	for _, depName := range migration.Dependencies {
		if err := v.validateSimpleDependency(ctx, depName, schemaName); err != nil {
			errors = append(errors, fmt.Errorf("dependency validation failed for '%s': %w", depName, err))
		}
	}

	return errors
}

// validateDependency validates a single structured dependency
func (v *DependencyValidator) validateDependency(ctx context.Context, dep backends.Dependency, currentSchema string) error {
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
	if dep.RequiresTable != "" {
		schemaToCheck := dep.RequiresSchema
		if schemaToCheck == "" {
			// Use current schema or default to public
			schemaToCheck = currentSchema
			if schemaToCheck == "" {
				schemaToCheck = "public"
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

	// Validate dependency migration is applied
	if err := v.validateMigrationApplied(ctx, dep); err != nil {
		return err
	}

	return nil
}

// validateSimpleDependency validates a simple string dependency
func (v *DependencyValidator) validateSimpleDependency(ctx context.Context, depName string, currentSchema string) error {
	// Find migrations with this name
	targetMigrations := v.registry.GetMigrationByName(depName)
	if len(targetMigrations) == 0 {
		return fmt.Errorf("dependency migration '%s' not found", depName)
	}

	// Check if at least one of the target migrations is applied
	// We check all found migrations and see if any are applied
	for _, targetMigration := range targetMigrations {
		// Generate migration ID for the target
		migrationID := v.getMigrationID(targetMigration, currentSchema)
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

// validateMigrationApplied checks if a dependency migration is applied
func (v *DependencyValidator) validateMigrationApplied(ctx context.Context, dep backends.Dependency) error {
	// Find the target migration
	targetMigrations, err := v.findMigrationByTarget(dep)
	if err != nil {
		return fmt.Errorf("dependency target not found: %w", err)
	}

	// Check if at least one target migration is applied
	for _, targetMigration := range targetMigrations {
		// Determine schema to use for migration ID
		schemaToUse := dep.Schema
		if schemaToUse == "" {
			schemaToUse = targetMigration.Schema
		}

		migrationID := v.getMigrationID(targetMigration, schemaToUse)
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

// getMigrationID generates a migration ID (helper method)
func (v *DependencyValidator) getMigrationID(migration *backends.MigrationScript, schema string) string {
	// Migration ID format: {version}_{name} (base format)
	baseID := fmt.Sprintf("%s_%s", migration.Version, migration.Name)
	if schema != "" {
		// For schema-specific checks, prefix with schema
		return fmt.Sprintf("%s_%s", schema, baseID)
	}
	return baseID
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
