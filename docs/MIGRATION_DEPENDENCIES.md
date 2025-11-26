# Migration Dependency System

## Overview

The BFM migration dependency system ensures that migrations execute in the correct order, validates that required schemas and tables exist before execution, and prevents errors like foreign key violations and missing table references.

## Features

- **Simple Name-Based Dependencies**: Backward compatible with existing `[]string` dependencies
- **Structured Dependencies**: Advanced dependency system with validation requirements
- **Cross-Connection Dependencies**: Dependencies can reference migrations across different connections and backends
- **Version and Name-Based Targeting**: Dependencies can reference migrations by version or name
- **Validation Requirements**: Specify required schemas and tables that must exist before execution
- **Automatic Ordering**: Topological sort ensures correct execution order
- **Cycle Detection**: Detects and reports circular dependencies

## Simple Dependencies (Backward Compatible)

The simplest way to declare dependencies is using a list of migration names:

```go
migration := &migrations.MigrationScript{
    Schema:       "core",
    Version:      "20250115120000",
    Name:         "create_users",
    Connection:   "core",
    Backend:      "postgresql",
    UpSQL:        upSQL,
    DownSQL:      downSQL,
    Dependencies: []string{"bootstrap_solution"}, // Simple name-based dependency
}
```

This migration will execute after any migration named `bootstrap_solution` is applied.

## Structured Dependencies

For more advanced scenarios, use structured dependencies:

```go
migration := &migrations.MigrationScript{
    Schema:       "core",
    Version:      "20250115120027",
    Name:         "alter_core_tables",
    Connection:   "core",
    Backend:      "postgresql",
    UpSQL:        upSQL,
    DownSQL:      downSQL,
    Dependencies: []string{}, // Can combine with simple dependencies
    StructuredDependencies: []migrations.Dependency{
        {
            Connection: "core",
            Schema:     "core",
            Target:     "20250115120000", // Migration version
            TargetType: "version",         // or "name" (default)
            RequiresTable:  "organizations", // Optional: table must exist
            RequiresSchema: "core",          // Optional: schema must exist
        },
    },
}
```

### Dependency Fields

- **Connection** (string): Connection name of the dependency (e.g., "core", "guard")
- **Schema** (string): Schema name of the dependency (optional, for cross-schema dependencies)
- **Target** (string): Migration version or name to depend on (required)
- **TargetType** (string): "version" or "name" (default: "name")
- **RequiresTable** (string): Optional table that must exist before execution
- **RequiresSchema** (string): Optional schema that must exist before execution

## Dependency Resolution

The system automatically resolves dependencies using topological sorting (Kahn's algorithm):

1. **Build Dependency Graph**: Creates a graph of all migrations and their dependencies
2. **Detect Cycles**: Identifies circular dependencies and reports errors
3. **Topological Sort**: Orders migrations so dependencies execute first
4. **Validate Dependencies**: Checks that:
   - Required schemas exist
   - Required tables exist
   - Dependency migrations are applied

## Validation

Before executing a migration, the system validates:

1. **Schema Existence**: If `RequiresSchema` is specified, the schema must exist
2. **Table Existence**: If `RequiresTable` is specified, the table must exist in the specified schema
3. **Migration Applied**: Dependency migrations must be successfully applied

Validation errors are reported before any SQL execution, providing clear error messages.

## Cross-Connection Dependencies

Dependencies can reference migrations in different connections:

```go
StructuredDependencies: []migrations.Dependency{
    {
        Connection: "core",        // Different connection
        Schema:     "core",
        Target:     "bootstrap_solution",
        TargetType: "name",
    },
}
```

This allows migrations in one backend/connection to depend on migrations in another.

## Examples

### Example 1: Base Migration (No Dependencies)

```go
migration := &migrations.MigrationScript{
    Schema:       "core",
    Version:      "20250115120000",
    Name:         "bootstrap_solution",
    Connection:   "core",
    Backend:      "postgresql",
    UpSQL:        upSQL,
    DownSQL:      downSQL,
    Dependencies: []string{}, // No dependencies
}
```

### Example 2: Migration with Table Requirement

```go
migration := &migrations.MigrationScript{
    Schema:       "guard",
    Version:      "20250101120000",
    Name:         "create_api_keys",
    Connection:   "guard",
    Backend:      "postgresql",
    UpSQL:        upSQL,
    DownSQL:      downSQL,
    StructuredDependencies: []migrations.Dependency{
        {
            Connection:     "core",
            Schema:         "core",
            Target:         "20250115120000",
            TargetType:     "version",
            RequiresTable:  "organizations", // Must exist before execution
            RequiresSchema: "core",
        },
    },
}
```

### Example 3: Cross-Connection Dependency

```go
migration := &migrations.MigrationScript{
    Schema:       "/metadata/operations",
    Version:      "20250115000000",
    Name:         "seed_feature_flags",
    Connection:   "metadata",
    Backend:      "etcd",
    UpSQL:        upSQL,
    DownSQL:      downSQL,
    Dependencies: []string{"bootstrap_solution"}, // Simple dependency
    StructuredDependencies: []migrations.Dependency{
        {
            Connection: "core",
            Schema:     "core",
            Target:     "bootstrap_solution",
            TargetType: "name",
        },
    },
}
```

## Error Handling

### Circular Dependencies

If a circular dependency is detected, execution stops with an error:

```
circular dependency detected: migration_a -> migration_b -> migration_a
```

### Missing Dependencies

If a dependency target is not found:

```
dependency validation failed: migration create_users depends on bootstrap_solution (not found)
```

### Validation Failures

If validation requirements are not met:

```
dependency validation failed: required table 'organizations' does not exist in schema 'core'
```

## Best Practices

1. **Use Simple Dependencies for Basic Cases**: If you only need name-based dependencies, use `Dependencies []string`
2. **Use Structured Dependencies for Validation**: When you need to ensure tables/schemas exist, use `StructuredDependencies`
3. **Be Specific with Targets**: Use version-based targeting when you need a specific migration version
4. **Document Dependencies**: Add comments explaining why dependencies are needed
5. **Test Dependency Order**: Verify that migrations execute in the correct order

## Troubleshooting

### Migration Not Executing

- Check that all dependencies are applied
- Verify dependency names/versions are correct
- Check for circular dependencies

### Validation Errors

- Ensure required schemas exist before running migrations
- Ensure required tables exist (created by dependency migrations)
- Check that dependency migrations are successfully applied

### Circular Dependency Errors

- Review dependency graph to identify the cycle
- Restructure migrations to break the cycle
- Consider splitting migrations if dependencies are too complex

## Implementation Details

- **Dependency Resolver**: `api/internal/registry/dependency_resolver.go`
- **Dependency Validator**: `api/internal/backends/postgresql/validator.go`
- **Topological Sort**: Implemented in `DependencyGraph.TopologicalSort()`
- **Cycle Detection**: Implemented in `DependencyGraph.DetectCycles()`
