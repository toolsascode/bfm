import React, { useState, useEffect, useMemo } from "react";
import { Link } from "react-router-dom";
import { apiClient } from "../services/api";
import { toastService } from "../services/toast";
import type { MigrationListItem, MigrationListFilters } from "../types/api";
import { format } from "date-fns";

export default function MigrationList() {
  const [migrations, setMigrations] = useState<MigrationListItem[]>([]);
  const [allMigrations, setAllMigrations] = useState<MigrationListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState<MigrationListFilters>({});
  const [total, setTotal] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [itemsPerPage, setItemsPerPage] = useState(25);
  const [reindexing, setReindexing] = useState(false);
  const [schemaSearchOpen, setSchemaSearchOpen] = useState(false);
  const [schemaSearchQuery, setSchemaSearchQuery] = useState("");
  const [backendSearchOpen, setBackendSearchOpen] = useState(false);
  const [backendSearchQuery, setBackendSearchQuery] = useState("");
  const [connectionSearchOpen, setConnectionSearchOpen] = useState(false);
  const [connectionSearchQuery, setConnectionSearchQuery] = useState("");
  const [statusSearchOpen, setStatusSearchOpen] = useState(false);
  const [statusSearchQuery, setStatusSearchQuery] = useState("");
  const [selectedMigrations, setSelectedMigrations] = useState<Set<string>>(
    new Set(),
  );
  const [executionSchema, setExecutionSchema] = useState<string>("");
  const [executing, setExecuting] = useState(false);
  const [executionSchemaOpen, setExecutionSchemaOpen] = useState(false);
  const [executionSchemaQuery, setExecutionSchemaQuery] = useState("");
  const [rollbackModalOpen, setRollbackModalOpen] = useState(false);
  const [forceRollback, setForceRollback] = useState(false);
  const [rollingBack, setRollingBack] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");

  useEffect(() => {
    loadMigrations();
  }, [filters]);

  // Load all migrations once to get unique filter values
  useEffect(() => {
    const loadAllMigrations = async () => {
      try {
        const response = await apiClient.listMigrations({});
        setAllMigrations(response.items);
      } catch (err) {
        // Silently fail - filters will just be empty
      }
    };
    loadAllMigrations();
  }, []);

  const loadMigrations = async () => {
    try {
      setLoading(true);
      const response = await apiClient.listMigrations(filters);
      setMigrations(response.items);
      setTotal(response.total);
      setError(null);
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to load migrations";
      setError(errorMsg);
      // Error toast is handled by API interceptor
    } finally {
      setLoading(false);
    }
  };

  // Extract unique values for filter options
  const filterOptions = useMemo(() => {
    const backends = Array.from(
      new Set(allMigrations.map((m) => m.backend).filter(Boolean)),
    ).sort();
    const schemas = Array.from(
      new Set(allMigrations.map((m) => m.schema).filter(Boolean)),
    ).sort();
    const connections = Array.from(
      new Set(allMigrations.map((m) => m.connection).filter(Boolean)),
    ).sort();
    return { backends, schemas, connections };
  }, [allMigrations]);

  // Filtered schemas based on search query
  const filteredSchemas = useMemo(() => {
    if (!schemaSearchQuery.trim()) {
      return filterOptions.schemas.slice(0, 10);
    }
    const query = schemaSearchQuery.toLowerCase();
    return filterOptions.schemas.filter((schema: string) =>
      schema.toLowerCase().includes(query),
    );
  }, [filterOptions.schemas, schemaSearchQuery]);

  // Filtered execution schemas based on search query
  const filteredExecutionSchemas = useMemo(() => {
    if (!executionSchemaQuery.trim()) {
      return filterOptions.schemas.slice(0, 10);
    }
    const query = executionSchemaQuery.toLowerCase();
    return filterOptions.schemas.filter((schema: string) =>
      schema.toLowerCase().includes(query),
    );
  }, [filterOptions.schemas, executionSchemaQuery]);

  // Update execution schema query when execution schema changes externally
  useEffect(() => {
    if (executionSchema && !executionSchemaOpen) {
      setExecutionSchemaQuery(executionSchema);
    }
  }, [executionSchema]);

  // Filtered backends based on search query
  const filteredBackends = useMemo(() => {
    if (!backendSearchQuery.trim()) {
      return filterOptions.backends.slice(0, 10);
    }
    const query = backendSearchQuery.toLowerCase();
    return filterOptions.backends.filter((backend: string) =>
      backend.toLowerCase().includes(query),
    );
  }, [filterOptions.backends, backendSearchQuery]);

  // Filtered connections based on search query
  const filteredConnections = useMemo(() => {
    if (!connectionSearchQuery.trim()) {
      return filterOptions.connections.slice(0, 10);
    }
    const query = connectionSearchQuery.toLowerCase();
    return filterOptions.connections.filter((connection: string) =>
      connection.toLowerCase().includes(query),
    );
  }, [filterOptions.connections, connectionSearchQuery]);

  // Status options (fixed list)
  const statusOptions = ["success", "failed", "pending"];
  const filteredStatuses = useMemo(() => {
    if (!statusSearchQuery.trim()) {
      return statusOptions;
    }
    const query = statusSearchQuery.toLowerCase();
    return statusOptions.filter((status: string) =>
      status.toLowerCase().includes(query),
    );
  }, [statusSearchQuery]);

  // Close dropdowns when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as HTMLElement;
      if (schemaSearchOpen && !target.closest(".schema-search-container")) {
        setSchemaSearchOpen(false);
        setSchemaSearchQuery("");
      }
      if (backendSearchOpen && !target.closest(".backend-search-container")) {
        setBackendSearchOpen(false);
        setBackendSearchQuery("");
      }
      if (
        connectionSearchOpen &&
        !target.closest(".connection-search-container")
      ) {
        setConnectionSearchOpen(false);
        setConnectionSearchQuery("");
      }
      if (statusSearchOpen && !target.closest(".status-search-container")) {
        setStatusSearchOpen(false);
        setStatusSearchQuery("");
      }
      if (
        executionSchemaOpen &&
        !target.closest(".execution-schema-container")
      ) {
        setExecutionSchemaOpen(false);
        setExecutionSchemaQuery("");
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [
    schemaSearchOpen,
    backendSearchOpen,
    connectionSearchOpen,
    statusSearchOpen,
    executionSchemaOpen,
  ]);

  const handleFilterChange = (
    key: keyof MigrationListFilters,
    value: string,
  ) => {
    setFilters((prev) => ({
      ...prev,
      [key]: value || undefined,
    }));
  };

  const clearFilters = () => {
    setFilters({});
    setCurrentPage(1);
  };

  // Helper function to check if a string is a version number (timestamp format: YYYYMMDDHHMMSS)
  const isVersionNumber = (str: string): boolean => {
    // Check if it's a 14-digit number (timestamp format)
    return /^\d{14}$/.test(str);
  };

  // Helper function to extract base ID from schema-specific ID
  const getBaseMigrationID = (migrationID: string): string => {
    const parts = migrationID.split("_");
    // Schema-specific format: {schema}_{version}_{name}_{backend}_{connection}
    // Base format: {version}_{name}_{backend}_{connection}
    // Base migrations start with a version number (timestamp), schema-specific start with schema name
    if (parts.length > 0 && !isVersionNumber(parts[0])) {
      // First part is not a version number, so it's a schema prefix - remove it
      return parts.slice(1).join("_");
    }
    return migrationID;
  };

  // Helper function to check if migration ID is schema-specific
  const isSchemaSpecific = (migrationID: string): boolean => {
    const parts = migrationID.split("_");
    // If first part is not a version number (timestamp), it's schema-specific
    return parts.length > 0 && !isVersionNumber(parts[0]);
  };

  // Filter out schema-specific migrations - only show base migrations identified by BfM
  const baseMigrations = useMemo(() => {
    return migrations.filter((migration) => {
      // Only include base migrations (not schema-specific)
      return !isSchemaSpecific(migration.migration_id);
    });
  }, [migrations]);

  // Calculate schema count for each base migration
  const migrationsWithSchemaCount = useMemo(() => {
    // Create a map to count schema-specific migrations for each base
    const schemaCountMap = new Map<string, number>();

    migrations.forEach((migration) => {
      if (isSchemaSpecific(migration.migration_id)) {
        const baseID = getBaseMigrationID(migration.migration_id);
        schemaCountMap.set(baseID, (schemaCountMap.get(baseID) || 0) + 1);
      }
    });

    // Add schema count to base migrations
    return baseMigrations.map((migration) => ({
      ...migration,
      schemaCount: schemaCountMap.get(migration.migration_id) || 0,
    }));
  }, [baseMigrations, migrations]);

  // Flatten migrations for display (no grouping needed, just base migrations)
  const flattenedMigrations = useMemo(() => {
    return migrationsWithSchemaCount;
  }, [migrationsWithSchemaCount]);

  // Filter migrations based on search query
  const filteredMigrations = useMemo(() => {
    if (!searchQuery.trim()) {
      return flattenedMigrations;
    }
    const query = searchQuery.toLowerCase();
    return flattenedMigrations.filter((migration) => {
      return (
        migration.migration_id.toLowerCase().includes(query) ||
        migration.version.toLowerCase().includes(query) ||
        migration.name.toLowerCase().includes(query) ||
        migration.table.toLowerCase().includes(query) ||
        migration.backend.toLowerCase().includes(query) ||
        (migration.connection &&
          migration.connection.toLowerCase().includes(query)) ||
        (migration.schema && migration.schema.toLowerCase().includes(query))
      );
    });
  }, [flattenedMigrations, searchQuery]);

  // Calculate pagination
  const totalPages = Math.ceil(filteredMigrations.length / itemsPerPage);
  const startIndex = (currentPage - 1) * itemsPerPage;
  const endIndex = startIndex + itemsPerPage;
  const paginatedMigrations = filteredMigrations.slice(startIndex, endIndex);

  // Reset to page 1 when filters or search query change
  useEffect(() => {
    setCurrentPage(1);
  }, [filters, searchQuery]);

  // Clear selection when filters change
  useEffect(() => {
    setSelectedMigrations(new Set());
    setExecutionSchema("");
  }, [filters]);

  // Handle individual migration selection
  const handleMigrationSelect = (migrationId: string) => {
    setSelectedMigrations((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(migrationId)) {
        newSet.delete(migrationId);
      } else {
        newSet.add(migrationId);
      }
      return newSet;
    });
  };

  // Handle select all on current page
  const handleSelectAll = () => {
    if (allPageSelected) {
      // Deselect all on current page
      setSelectedMigrations((prev) => {
        const newSet = new Set(prev);
        paginatedMigrations.forEach((m) => newSet.delete(m.migration_id));
        return newSet;
      });
    } else {
      // Select all on current page
      setSelectedMigrations((prev) => {
        const newSet = new Set(prev);
        paginatedMigrations.forEach((m) => newSet.add(m.migration_id));
        return newSet;
      });
    }
  };

  // Check if all on current page are selected
  const allPageSelected = useMemo(() => {
    if (paginatedMigrations.length === 0) return false;
    return paginatedMigrations.every((m) =>
      selectedMigrations.has(m.migration_id),
    );
  }, [paginatedMigrations, selectedMigrations]);

  // Get selected migration objects
  const selectedMigrationObjects = useMemo(() => {
    return migrations.filter((m) => selectedMigrations.has(m.migration_id));
  }, [migrations, selectedMigrations]);

  // Handle execute selected migrations
  const handleExecuteSelected = async () => {
    if (selectedMigrationObjects.length === 0) {
      toastService.error("Please select at least one migration");
      return;
    }

    if (!executionSchema.trim()) {
      toastService.error("Please enter or select a schema for execution");
      return;
    }

    setExecuting(true);
    try {
      // Group migrations by backend and connection
      const grouped = selectedMigrationObjects.reduce(
        (
          acc: Record<string, MigrationListItem[]>,
          migration: MigrationListItem,
        ) => {
          const key = `${migration.backend}_${migration.connection}`;
          if (!acc[key]) {
            acc[key] = [];
          }
          acc[key].push(migration);
          return acc;
        },
        {} as Record<string, MigrationListItem[]>,
      );

      let totalApplied = 0;
      let totalSkipped = 0;
      const errors: string[] = [];

      // Execute each group
      for (const [, groupMigrations] of Object.entries(grouped)) {
        // Execute each migration in the group
        for (const migration of groupMigrations as MigrationListItem[]) {
          try {
            const response = await apiClient.migrateUp({
              target: {
                backend: migration.backend,
                connection: migration.connection,
                version: migration.version,
              },
              connection: migration.connection,
              schemas: [executionSchema.trim()],
              dry_run: false,
            });

            if (response.success) {
              totalApplied += response.applied.length;
              totalSkipped += response.skipped.length;
              if (response.errors.length > 0) {
                errors.push(...response.errors);
              }
            } else {
              errors.push(
                ...response.errors,
                ...response.applied.map(
                  (id) => `Migration ${id}: Partial success`,
                ),
              );
            }
          } catch (err) {
            const errorMsg =
              err instanceof Error
                ? err.message
                : `Failed to execute migration ${migration.migration_id}`;
            errors.push(errorMsg);
          }
        }
      }

      // Show results
      if (errors.length === 0) {
        toastService.success(
          `Successfully executed ${totalApplied} migration(s). ${totalSkipped > 0 ? `${totalSkipped} skipped.` : ""}`,
        );
      } else {
        toastService.warning(
          `Executed with some errors. Applied: ${totalApplied}, Skipped: ${totalSkipped}, Errors: ${errors.length}`,
        );
      }

      // Clear selection and reload
      setSelectedMigrations(new Set());
      setExecutionSchema("");
      await loadMigrations();
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to execute migrations";
      toastService.error(errorMsg);
    } finally {
      setExecuting(false);
    }
  };

  const handleRollbackSelected = async () => {
    if (selectedMigrationObjects.length === 0) {
      toastService.error("Please select at least one migration");
      return;
    }

    setRollingBack(true);
    try {
      let totalRolledBack = 0;
      const errors: string[] = [];
      const failedMigrations: string[] = [];

      // Rollback each selected migration
      for (const migration of selectedMigrationObjects) {
        try {
          // Use migration schema if available
          const schemas = migration.schema ? [migration.schema] : [];
          const response = await apiClient.rollbackMigration(
            migration.migration_id,
            schemas,
          );

          if (response.success) {
            totalRolledBack++;
          } else {
            errors.push(
              `Migration ${migration.migration_id}: ${response.message || "Rollback failed"}`,
            );
            failedMigrations.push(migration.migration_id);
          }

          if (response.errors && response.errors.length > 0) {
            errors.push(...response.errors);
            failedMigrations.push(migration.migration_id);
          }
        } catch (err) {
          const errorMsg =
            err instanceof Error
              ? err.message
              : `Failed to rollback migration ${migration.migration_id}`;
          errors.push(errorMsg);
          failedMigrations.push(migration.migration_id);
        }
      }

      // Show results
      if (errors.length === 0) {
        toastService.success(
          `Successfully rolled back ${totalRolledBack} migration(s).`,
        );
      } else {
        if (forceRollback) {
          toastService.warning(
            `Rollback completed with some errors. Rolled back: ${totalRolledBack}, Failed: ${failedMigrations.length}. ${failedMigrations.length > 0 ? "Failed migrations should be set to pending status (requires backend support)." : ""}`,
          );
        } else {
          toastService.warning(
            `Rollback completed with some errors. Rolled back: ${totalRolledBack}, Failed: ${failedMigrations.length}`,
          );
        }
      }

      // Clear selection and reload
      setSelectedMigrations(new Set());
      setRollbackModalOpen(false);
      setForceRollback(false);
      await loadMigrations();
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to rollback migrations";
      toastService.error(errorMsg);
    } finally {
      setRollingBack(false);
    }
  };

  const handlePageChange = (page: number) => {
    setCurrentPage(page);
    // Scroll to top of table
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const handleItemsPerPageChange = (value: number) => {
    setItemsPerPage(value);
    setCurrentPage(1);
  };

  const handleReindex = async () => {
    if (reindexing) return;

    setReindexing(true);
    try {
      const result = await apiClient.reindexMigrations();
      const addedCount = result.added.length;
      const removedCount = result.removed.length;

      let message = `Reindexing completed. Total migrations: ${result.total}`;
      if (addedCount > 0 || removedCount > 0) {
        message += ` (Added: ${addedCount}, Removed: ${removedCount})`;
      } else {
        message += " (No changes)";
      }

      toastService.success(message);

      // Reload migrations list to reflect changes
      await loadMigrations();

      // Also reload all migrations for filters
      try {
        const response = await apiClient.listMigrations({});
        setAllMigrations(response.items);
      } catch (err) {
        // Silently fail
      }
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to reindex migrations";
      toastService.error(errorMsg);
    } finally {
      setReindexing(false);
    }
  };

  if (loading && migrations.length === 0) {
    return (
      <div className="space-y-4">
        <div className="h-8 bg-gray-200 rounded animate-pulse w-1/3" />
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-24 bg-gray-200 rounded animate-pulse" />
          ))}
        </div>
        <div className="h-96 bg-gray-200 rounded animate-pulse" />
      </div>
    );
  }

  return (
    <div className="w-full animate-fade-in">
      {/* Execution Bar - shown when migrations are selected */}
      {selectedMigrations.size > 0 && (
        <div className="bg-bfm-blue text-white p-4 rounded-lg mb-4 flex flex-col sm:flex-row items-start sm:items-center gap-4 animate-slide-down">
          <div className="flex-1">
            <div className="text-sm font-medium">
              {selectedMigrations.size} migration
              {selectedMigrations.size !== 1 ? "s" : ""} selected
            </div>
          </div>
          <div className="flex items-center gap-3 w-full sm:w-auto">
            <div className="relative flex-1 sm:flex-initial execution-schema-container">
              <div className="relative">
                <input
                  type="text"
                  placeholder="Enter or select schema..."
                  value={executionSchema}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                    setExecutionSchema(e.target.value);
                    setExecutionSchemaQuery(e.target.value);
                    setExecutionSchemaOpen(true);
                  }}
                  onFocus={() => {
                    setExecutionSchemaOpen(true);
                    setExecutionSchemaQuery(executionSchema);
                  }}
                  onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      setExecutionSchema(executionSchema.trim());
                      setExecutionSchemaOpen(false);
                      setExecutionSchemaQuery("");
                    } else if (e.key === "Escape") {
                      setExecutionSchemaOpen(false);
                    }
                  }}
                  onClick={(e: React.MouseEvent<HTMLInputElement>) =>
                    e.stopPropagation()
                  }
                  className="w-full sm:w-64 px-3 py-2 pr-8 border border-white/30 rounded text-sm bg-white/10 text-white placeholder-white/50 focus:outline-none focus:border-white/50 focus:ring-2 focus:ring-white/20"
                />
                <svg
                  className={`absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4 text-white/70 transition-transform pointer-events-none ${
                    executionSchemaOpen ? "rotate-180" : ""
                  }`}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 9l-7 7-7-7"
                  />
                </svg>
              </div>
              {executionSchemaOpen && (
                <div className="absolute z-50 w-full sm:w-64 mt-1 bg-white border border-gray-300 rounded shadow-lg max-h-60 overflow-hidden">
                  <div className="max-h-60 overflow-y-auto">
                    {filteredExecutionSchemas.length > 0 ? (
                      <>
                        {filteredExecutionSchemas.map((schema: string) => (
                          <button
                            key={schema}
                            type="button"
                            onClick={() => {
                              setExecutionSchema(schema);
                              setExecutionSchemaOpen(false);
                              setExecutionSchemaQuery("");
                            }}
                            className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                              executionSchema === schema
                                ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                                : "text-gray-800"
                            }`}
                          >
                            {schema}
                          </button>
                        ))}
                        {executionSchemaQuery.trim() &&
                          !filteredExecutionSchemas.includes(
                            executionSchemaQuery.trim(),
                          ) && (
                            <div className="border-t border-gray-200">
                              <button
                                type="button"
                                onClick={() => {
                                  setExecutionSchema(
                                    executionSchemaQuery.trim(),
                                  );
                                  setExecutionSchemaOpen(false);
                                  setExecutionSchemaQuery("");
                                }}
                                className="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors text-gray-800 font-medium"
                              >
                                Use "{executionSchemaQuery.trim()}" (new)
                              </button>
                            </div>
                          )}
                      </>
                    ) : (
                      <div>
                        {executionSchemaQuery.trim() ? (
                          <div className="border-t border-gray-200">
                            <button
                              type="button"
                              onClick={() => {
                                setExecutionSchema(executionSchemaQuery.trim());
                                setExecutionSchemaOpen(false);
                                setExecutionSchemaQuery("");
                              }}
                              className="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors text-gray-800 font-medium"
                            >
                              Use "{executionSchemaQuery.trim()}" (new)
                            </button>
                          </div>
                        ) : (
                          <div className="px-3 py-2 text-sm text-gray-500">
                            Type a schema name or select from list
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
            <button
              onClick={handleExecuteSelected}
              disabled={executing || !executionSchema.trim()}
              className="px-4 py-2 bg-white text-bfm-blue rounded text-sm font-medium transition-colors hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {executing ? "Executing..." : "Execute Selected"}
            </button>
            <button
              onClick={() => setRollbackModalOpen(true)}
              disabled={executing || rollingBack}
              className="px-4 py-2 bg-red-600 text-white rounded text-sm font-medium transition-colors hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {rollingBack ? "Rolling Back..." : "Rollback Selected"}
            </button>
            <button
              onClick={() => {
                setSelectedMigrations(new Set());
                setExecutionSchema("");
              }}
              className="px-4 py-2 bg-white/20 text-white rounded text-sm font-medium transition-colors hover:bg-white/30"
            >
              Clear
            </button>
          </div>
        </div>
      )}

      {/* Rollback Confirmation Modal */}
      {rollbackModalOpen && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
          <div className="bg-white rounded-lg shadow-xl max-w-md w-full animate-scale-in">
            <div className="p-6">
              <h2 className="text-xl font-semibold text-gray-800 mb-4">
                Confirm Rollback
              </h2>
              <div className="mb-4">
                <p className="text-gray-700 mb-2">
                  You are about to rollback{" "}
                  <strong>{selectedMigrations.size}</strong> migration
                  {selectedMigrations.size !== 1 ? "s" : ""}. This action cannot
                  be undone.
                </p>
                <p className="text-red-600 text-sm font-medium">
                  ⚠️ Warning: This operation is irreversible!
                </p>
              </div>
              <div className="mb-6">
                <label className="flex items-start gap-3 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={forceRollback}
                    onChange={(e) => setForceRollback(e.target.checked)}
                    className="mt-1 w-4 h-4 border-gray-300 rounded focus:ring-bfm-blue focus:ring-2 cursor-pointer"
                    style={{
                      accentColor: "#3498db",
                      backgroundColor: forceRollback ? "#3498db" : "white",
                    }}
                  />
                  <div className="flex-1">
                    <span className="text-sm font-medium text-gray-800 block">
                      Force rollback (set status to pending on failure)
                    </span>
                    <span className="text-xs text-gray-500 block mt-1">
                      If enabled, even if rollback fails, the migration status
                      will be changed to pending.
                    </span>
                  </div>
                </label>
              </div>
              <div className="flex gap-3 justify-end">
                <button
                  onClick={() => {
                    setRollbackModalOpen(false);
                    setForceRollback(false);
                  }}
                  disabled={rollingBack}
                  className="px-4 py-2 border border-gray-300 rounded text-sm font-medium text-gray-700 hover:bg-gray-50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Cancel
                </button>
                <button
                  onClick={handleRollbackSelected}
                  disabled={rollingBack}
                  className="px-4 py-2 bg-red-600 text-white rounded text-sm font-medium transition-colors hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {rollingBack ? "Rolling Back..." : "Confirm Rollback"}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6 animate-slide-up">
        <h1 className="text-3xl font-semibold text-gray-800">Migrations</h1>
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-4 w-full sm:w-auto">
          <div className="flex items-center gap-2">
            <input
              type="text"
              placeholder="Search migrations..."
              value={searchQuery}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                setSearchQuery(e.target.value)
              }
              onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  setCurrentPage(1);
                } else if (e.key === "Escape") {
                  setSearchQuery("");
                }
              }}
              className="flex-1 sm:w-64 px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
            />
            <button
              onClick={() => setCurrentPage(1)}
              className="px-4 py-2 bg-bfm-blue text-white rounded text-sm transition-colors hover:bg-bfm-blue-dark font-medium"
            >
              Search
            </button>
            {searchQuery && (
              <button
                onClick={() => setSearchQuery("")}
                className="px-3 py-2 bg-gray-500 text-white rounded text-sm transition-colors hover:bg-gray-600 font-medium"
                title="Clear search"
              >
                Clear
              </button>
            )}
          </div>
          <button
            onClick={handleReindex}
            disabled={reindexing}
            className="px-4 py-2 bg-bfm-blue text-white rounded text-sm transition-colors hover:bg-bfm-blue-dark disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            {reindexing ? "Reindexing..." : "Reindex"}
          </button>
          <div className="text-base text-gray-500">
            Showing {startIndex + 1}-
            {Math.min(endIndex, filteredMigrations.length)} of{" "}
            {searchQuery ? filteredMigrations.length : total}
            {searchQuery && ` (filtered from ${total} total)`}
          </div>
          <div className="flex items-center gap-2">
            <label htmlFor="items-per-page" className="text-sm text-gray-600">
              Per page:
            </label>
            <select
              id="items-per-page"
              value={itemsPerPage}
              onChange={(e) => handleItemsPerPageChange(Number(e.target.value))}
              className="px-2 py-1 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
            >
              <option value={10}>10</option>
              <option value={25}>25</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </div>
        </div>
      </div>

      <div className="bg-white p-4 md:p-6 rounded-lg shadow-md mb-6 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5 gap-4 md:gap-6 items-start animate-scale-in transition-all hover:shadow-lg">
        <div className="flex flex-col gap-2 backend-search-container">
          <label className="text-sm font-semibold text-gray-800 block">
            Backend
          </label>
          <div className="relative">
            <button
              type="button"
              onClick={() => {
                setBackendSearchOpen(!backendSearchOpen);
                if (!backendSearchOpen) {
                  setBackendSearchQuery("");
                }
              }}
              className="w-full px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 text-left flex items-center justify-between focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
            >
              <span className={filters.backend ? "" : "text-gray-500"}>
                {filters.backend || "All Backends"}
              </span>
              <svg
                className={`w-4 h-4 text-gray-400 transition-transform ${
                  backendSearchOpen ? "rotate-180" : ""
                }`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
            {backendSearchOpen && (
              <div className="absolute z-50 w-full mt-1 bg-white border border-gray-300 rounded shadow-lg max-h-60 overflow-hidden">
                <div className="p-2 border-b border-gray-200">
                  <input
                    type="text"
                    placeholder="Search backends..."
                    value={backendSearchQuery}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setBackendSearchQuery(e.target.value)
                    }
                    onClick={(e: React.MouseEvent<HTMLInputElement>) =>
                      e.stopPropagation()
                    }
                    className="w-full px-2 py-1.5 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-1 focus:ring-bfm-blue"
                    autoFocus
                  />
                </div>
                <div className="max-h-48 overflow-y-auto">
                  <button
                    type="button"
                    onClick={() => {
                      handleFilterChange("backend", "");
                      setBackendSearchOpen(false);
                      setBackendSearchQuery("");
                    }}
                    className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                      !filters.backend
                        ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                        : "text-gray-800"
                    }`}
                  >
                    All Backends
                  </button>
                  {filteredBackends.length > 0 ? (
                    filteredBackends.map((backend: string) => (
                      <button
                        key={backend}
                        type="button"
                        onClick={() => {
                          handleFilterChange("backend", backend);
                          setBackendSearchOpen(false);
                          setBackendSearchQuery("");
                        }}
                        className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                          filters.backend === backend
                            ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                            : "text-gray-800"
                        }`}
                      >
                        {backend}
                      </button>
                    ))
                  ) : (
                    <div className="px-3 py-2 text-sm text-gray-500">
                      No backends found
                    </div>
                  )}
                  {filterOptions.backends.length > 10 &&
                    !backendSearchQuery.trim() && (
                      <div className="px-3 py-2 text-xs text-gray-500 border-t border-gray-200 bg-gray-50">
                        Showing first 10 of {filterOptions.backends.length}.
                        Search to find more.
                      </div>
                    )}
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="flex flex-col gap-2 schema-search-container">
          <label className="text-sm font-semibold text-gray-800 block">
            Schema
          </label>
          <div className="relative">
            <button
              type="button"
              onClick={() => {
                setSchemaSearchOpen(!schemaSearchOpen);
                if (!schemaSearchOpen) {
                  setSchemaSearchQuery("");
                }
              }}
              className="w-full px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 text-left flex items-center justify-between focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
            >
              <span className={filters.schema ? "" : "text-gray-500"}>
                {filters.schema || "All Schemas"}
              </span>
              <svg
                className={`w-4 h-4 text-gray-400 transition-transform ${
                  schemaSearchOpen ? "rotate-180" : ""
                }`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
            {schemaSearchOpen && (
              <div className="absolute z-50 w-full mt-1 bg-white border border-gray-300 rounded shadow-lg max-h-60 overflow-hidden">
                <div className="p-2 border-b border-gray-200">
                  <input
                    type="text"
                    placeholder="Search schemas..."
                    value={schemaSearchQuery}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setSchemaSearchQuery(e.target.value)
                    }
                    onClick={(e: React.MouseEvent<HTMLInputElement>) =>
                      e.stopPropagation()
                    }
                    className="w-full px-2 py-1.5 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-1 focus:ring-bfm-blue"
                    autoFocus
                  />
                </div>
                <div className="max-h-48 overflow-y-auto">
                  <button
                    type="button"
                    onClick={() => {
                      handleFilterChange("schema", "");
                      setSchemaSearchOpen(false);
                      setSchemaSearchQuery("");
                    }}
                    className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                      !filters.schema
                        ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                        : "text-gray-800"
                    }`}
                  >
                    All Schemas
                  </button>
                  {filteredSchemas.length > 0 ? (
                    filteredSchemas.map((schema: string) => (
                      <button
                        key={schema}
                        type="button"
                        onClick={() => {
                          handleFilterChange("schema", schema);
                          setSchemaSearchOpen(false);
                          setSchemaSearchQuery("");
                        }}
                        className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                          filters.schema === schema
                            ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                            : "text-gray-800"
                        }`}
                      >
                        {schema}
                      </button>
                    ))
                  ) : (
                    <div className="px-3 py-2 text-sm text-gray-500">
                      No schemas found
                    </div>
                  )}
                  {filterOptions.schemas.length > 10 &&
                    !schemaSearchQuery.trim() && (
                      <div className="px-3 py-2 text-xs text-gray-500 border-t border-gray-200 bg-gray-50">
                        Showing first 10 of {filterOptions.schemas.length}.
                        Search to find more.
                      </div>
                    )}
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="flex flex-col gap-2 connection-search-container">
          <label className="text-sm font-semibold text-gray-800 block">
            Connection
          </label>
          <div className="relative">
            <button
              type="button"
              onClick={() => {
                setConnectionSearchOpen(!connectionSearchOpen);
                if (!connectionSearchOpen) {
                  setConnectionSearchQuery("");
                }
              }}
              className="w-full px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 text-left flex items-center justify-between focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
            >
              <span className={filters.connection ? "" : "text-gray-500"}>
                {filters.connection || "All Connections"}
              </span>
              <svg
                className={`w-4 h-4 text-gray-400 transition-transform ${
                  connectionSearchOpen ? "rotate-180" : ""
                }`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
            {connectionSearchOpen && (
              <div className="absolute z-50 w-full mt-1 bg-white border border-gray-300 rounded shadow-lg max-h-60 overflow-hidden">
                <div className="p-2 border-b border-gray-200">
                  <input
                    type="text"
                    placeholder="Search connections..."
                    value={connectionSearchQuery}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setConnectionSearchQuery(e.target.value)
                    }
                    onClick={(e: React.MouseEvent<HTMLInputElement>) =>
                      e.stopPropagation()
                    }
                    className="w-full px-2 py-1.5 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-1 focus:ring-bfm-blue"
                    autoFocus
                  />
                </div>
                <div className="max-h-48 overflow-y-auto">
                  <button
                    type="button"
                    onClick={() => {
                      handleFilterChange("connection", "");
                      setConnectionSearchOpen(false);
                      setConnectionSearchQuery("");
                    }}
                    className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                      !filters.connection
                        ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                        : "text-gray-800"
                    }`}
                  >
                    All Connections
                  </button>
                  {filteredConnections.length > 0 ? (
                    filteredConnections.map((connection: string) => (
                      <button
                        key={connection}
                        type="button"
                        onClick={() => {
                          handleFilterChange("connection", connection);
                          setConnectionSearchOpen(false);
                          setConnectionSearchQuery("");
                        }}
                        className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                          filters.connection === connection
                            ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                            : "text-gray-800"
                        }`}
                      >
                        {connection}
                      </button>
                    ))
                  ) : (
                    <div className="px-3 py-2 text-sm text-gray-500">
                      No connections found
                    </div>
                  )}
                  {filterOptions.connections.length > 10 &&
                    !connectionSearchQuery.trim() && (
                      <div className="px-3 py-2 text-xs text-gray-500 border-t border-gray-200 bg-gray-50">
                        Showing first 10 of {filterOptions.connections.length}.
                        Search to find more.
                      </div>
                    )}
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="flex flex-col gap-2 status-search-container">
          <label className="text-sm font-semibold text-gray-800 block">
            Status
          </label>
          <div className="relative">
            <button
              type="button"
              onClick={() => {
                setStatusSearchOpen(!statusSearchOpen);
                if (!statusSearchOpen) {
                  setStatusSearchQuery("");
                }
              }}
              className="w-full px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 text-left flex items-center justify-between focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
            >
              <span className={filters.status ? "" : "text-gray-500"}>
                {filters.status
                  ? filters.status.charAt(0).toUpperCase() +
                    filters.status.slice(1)
                  : "All"}
              </span>
              <svg
                className={`w-4 h-4 text-gray-400 transition-transform ${
                  statusSearchOpen ? "rotate-180" : ""
                }`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
            {statusSearchOpen && (
              <div className="absolute z-50 w-full mt-1 bg-white border border-gray-300 rounded shadow-lg max-h-60 overflow-hidden">
                <div className="p-2 border-b border-gray-200">
                  <input
                    type="text"
                    placeholder="Search status..."
                    value={statusSearchQuery}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setStatusSearchQuery(e.target.value)
                    }
                    onClick={(e: React.MouseEvent<HTMLInputElement>) =>
                      e.stopPropagation()
                    }
                    className="w-full px-2 py-1.5 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-1 focus:ring-bfm-blue"
                    autoFocus
                  />
                </div>
                <div className="max-h-48 overflow-y-auto">
                  <button
                    type="button"
                    onClick={() => {
                      handleFilterChange("status", "");
                      setStatusSearchOpen(false);
                      setStatusSearchQuery("");
                    }}
                    className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                      !filters.status
                        ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                        : "text-gray-800"
                    }`}
                  >
                    All
                  </button>
                  {filteredStatuses.length > 0 ? (
                    filteredStatuses.map((status: string) => (
                      <button
                        key={status}
                        type="button"
                        onClick={() => {
                          handleFilterChange("status", status);
                          setStatusSearchOpen(false);
                          setStatusSearchQuery("");
                        }}
                        className={`w-full px-3 py-2 text-left text-sm hover:bg-gray-100 transition-colors ${
                          filters.status === status
                            ? "bg-bfm-blue/10 text-bfm-blue font-medium"
                            : "text-gray-800"
                        }`}
                      >
                        {status.charAt(0).toUpperCase() + status.slice(1)}
                      </button>
                    ))
                  ) : (
                    <div className="px-3 py-2 text-sm text-gray-500">
                      No status found
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
        <button
          onClick={clearFilters}
          className="px-4 py-2.5 bg-gray-500 text-white rounded text-sm transition-colors hover:bg-gray-600 sm:col-span-2 lg:col-span-1 lg:self-end lg:mt-6"
        >
          Clear Filters
        </button>
      </div>

      {error && (
        <div className="bg-red-100 text-red-800 p-4 rounded mb-4">
          Error: {error}
        </div>
      )}

      <div className="bg-white rounded-lg shadow-md overflow-x-auto animate-scale-in transition-all hover:shadow-lg">
        <table className="w-full overflow-hidden border-collapse">
          <thead>
            <tr>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0 w-12">
                <input
                  type="checkbox"
                  checked={allPageSelected}
                  onChange={handleSelectAll}
                  className="w-4 h-4 border-gray-300 rounded focus:ring-bfm-blue focus:ring-2 cursor-pointer"
                  style={{
                    accentColor: "#3498db",
                    backgroundColor: allPageSelected ? "#3498db" : "white",
                  }}
                  title={
                    allPageSelected
                      ? "Deselect all on this page"
                      : "Select all on this page"
                  }
                />
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Version
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Name
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Connection
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Backend
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Schema
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Total Schemas
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Status
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Applied
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Applied At
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {paginatedMigrations.length === 0 ? (
              <tr>
                <td colSpan={11} className="text-center text-gray-500 py-8">
                  No migrations found
                </td>
              </tr>
            ) : (
              paginatedMigrations.map((migration, index) => (
                <tr
                  key={migration.migration_id}
                  className="hover:bg-gray-50 transition-colors animate-fade-in"
                  style={{ animationDelay: `${index * 0.05}s` }}
                >
                  <td className="p-4 border-b border-gray-200">
                    <input
                      type="checkbox"
                      checked={selectedMigrations.has(migration.migration_id)}
                      onChange={() =>
                        handleMigrationSelect(migration.migration_id)
                      }
                      className="w-4 h-4 border-gray-300 rounded focus:ring-bfm-blue focus:ring-2 cursor-pointer"
                      style={{
                        accentColor: "#3498db",
                        backgroundColor: selectedMigrations.has(
                          migration.migration_id,
                        )
                          ? "#3498db"
                          : "bg-white",
                      }}
                    />
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.version}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.name || "-"}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.connection || "-"}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.backend}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    <span className="text-gray-500 italic">
                      {migration.schemaCount > 0
                        ? "Multiple"
                        : migration.schema || "-"}
                    </span>
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    <span className="text-gray-800 font-medium">
                      {migration.schemaCount || 0}
                    </span>
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    <span
                      className={`inline-block px-3 py-1 rounded-full text-xs font-medium ${
                        migration.status === "success" ||
                        migration.status === "applied"
                          ? "bg-green-100 text-green-800"
                          : migration.status === "failed"
                            ? "bg-red-100 text-red-800"
                            : migration.status === "rolled_back"
                              ? "bg-orange-100 text-orange-800"
                              : "bg-yellow-100 text-yellow-800"
                      }`}
                    >
                      {migration.status === "rolled_back"
                        ? "Rolled Back"
                        : migration.status || "pending"}
                    </span>
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    <span
                      className={
                        migration.applied
                          ? "text-bfm-green-dark font-medium"
                          : "text-gray-500"
                      }
                    >
                      {migration.applied ? "Yes" : "No"}
                    </span>
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.applied_at
                      ? format(
                          new Date(migration.applied_at),
                          "yyyy-MM-dd HH:mm:ss",
                        )
                      : "-"}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    <Link
                      to={`/migrations/${migration.migration_id}`}
                      className="inline-block px-3 py-1 bg-bfm-blue text-white rounded text-sm no-underline transition-all duration-200 hover:bg-bfm-blue-dark hover:shadow-md"
                    >
                      View
                    </Link>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination Controls */}
      {filteredMigrations.length > 0 && totalPages > 1 && (
        <div className="mt-6 flex flex-col sm:flex-row justify-between items-center gap-4 bg-white p-4 rounded-lg shadow-md">
          <div className="text-sm text-gray-600">
            Page {currentPage} of {totalPages}
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => handlePageChange(1)}
              disabled={currentPage === 1}
              className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              First
            </button>
            <button
              onClick={() => handlePageChange(currentPage - 1)}
              disabled={currentPage === 1}
              className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Previous
            </button>
            <div className="flex items-center gap-1">
              {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
                let pageNum: number;
                if (totalPages <= 5) {
                  pageNum = i + 1;
                } else if (currentPage <= 3) {
                  pageNum = i + 1;
                } else if (currentPage >= totalPages - 2) {
                  pageNum = totalPages - 4 + i;
                } else {
                  pageNum = currentPage - 2 + i;
                }
                return (
                  <button
                    key={pageNum}
                    onClick={() => handlePageChange(pageNum)}
                    className={`px-3 py-2 border rounded text-sm transition-colors ${
                      currentPage === pageNum
                        ? "bg-bfm-blue text-white border-bfm-blue"
                        : "bg-white text-gray-700 border-gray-300 hover:bg-gray-50"
                    }`}
                  >
                    {pageNum}
                  </button>
                );
              })}
            </div>
            <button
              onClick={() => handlePageChange(currentPage + 1)}
              disabled={currentPage === totalPages}
              className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Next
            </button>
            <button
              onClick={() => handlePageChange(totalPages)}
              disabled={currentPage === totalPages}
              className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Last
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
