import { useState, useEffect, useMemo } from "react";
import { useParams, Link } from "react-router-dom";
import { apiClient } from "../services/api";
import type {
  MigrationDetailResponse,
  MigrationStatusResponse,
  MigrationHistoryItem,
  MigrateUpRequest,
  MigrateResponse,
  MigrationListItem,
  MigrationExecution,
} from "../types/api";
import { format } from "date-fns";
import { toastService } from "../services/toast";

// Confirmation Modal Component
function ConfirmModal({
  isOpen,
  onClose,
  onConfirm,
  title,
  message,
  confirmText = "Confirm",
  cancelText = "Cancel",
  confirmButtonClass = "bg-bfm-green-dark text-white hover:bg-bfm-green",
}: {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  confirmButtonClass?: string;
}) {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl max-w-md w-full mx-4 animate-scale-in">
        <div className="p-6">
          <h2 className="text-xl font-semibold text-gray-800 mb-4">{title}</h2>
          <p className="text-gray-600 text-sm mb-6">{message}</p>
          <div className="flex gap-3 justify-end">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-gray-200 text-gray-800 rounded hover:bg-gray-300 transition-colors"
            >
              {cancelText}
            </button>
            <button
              type="button"
              onClick={onConfirm}
              className={`px-4 py-2 rounded transition-colors ${confirmButtonClass}`}
            >
              {confirmText}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// Schema/Prefix Selection Modal Component
function SchemaModal({
  isOpen,
  onClose,
  onConfirm,
  connection,
  backend,
  isNoSQL = false,
}: {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (value: string) => void;
  connection: string;
  backend: string;
  isNoSQL?: boolean;
}) {
  const [value, setValue] = useState("");

  if (!isOpen) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (value.trim()) {
      onConfirm(value.trim());
      setValue("");
    }
  };

  const label = isNoSQL ? "Prefix" : "Schema Name";
  const placeholder = isNoSQL
    ? "e.g., /bfm/metadata/"
    : "e.g., public, core, logs";
  const envVarType = isNoSQL ? "PREFIX" : "SCHEMA";
  const description = isNoSQL
    ? `The prefix is required for NoSQL connections (${backend}). Please specify which prefix will be used for this migration. The prefix should match the {CONNECTION}_PREFIX environment variable if configured.`
    : `The schema name is required for SQL connections (${backend}). Please specify which schema will be used for this migration. The schema should match the {CONNECTION}_SCHEMA environment variable if configured.`;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl max-w-md w-full mx-4 animate-scale-in">
        <div className="p-6">
          <h2 className="text-xl font-semibold text-gray-800 mb-4">
            Specify {label}
          </h2>
          <p className="text-gray-600 text-sm mb-4">
            {description}
            {connection && (
              <span className="block mt-2 text-xs text-gray-500">
                Connection: <strong>{connection}</strong>
                <br />
                Expected env var:{" "}
                <strong>
                  {connection.toUpperCase()}_{envVarType}
                </strong>
              </span>
            )}
          </p>
          <form onSubmit={handleSubmit}>
            <div className="mb-4">
              <label
                htmlFor="value-input"
                className="block mb-2 text-gray-800 font-medium"
              >
                {label} *
              </label>
              <input
                id="value-input"
                type="text"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder={placeholder}
                required
                autoFocus
                className="w-full px-3 py-3 border border-gray-300 rounded text-base focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
              />
            </div>
            <div className="flex gap-3 justify-end">
              <button
                type="button"
                onClick={() => {
                  onClose();
                  setValue("");
                }}
                className="px-4 py-2 bg-gray-200 text-gray-800 rounded hover:bg-gray-300 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={!value.trim()}
                className="px-4 py-2 bg-bfm-green-dark text-white rounded hover:bg-bfm-green transition-colors disabled:opacity-60 disabled:cursor-not-allowed"
              >
                Confirm
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}

export default function MigrationDetail() {
  const { id } = useParams<{ id: string }>();
  const [migration, setMigration] = useState<MigrationDetailResponse | null>(
    null,
  );
  const [status, setStatus] = useState<MigrationStatusResponse | null>(null);
  const [history, setHistory] = useState<MigrationHistoryItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isExecuting, setIsExecuting] = useState(false);
  const [executionResult, setExecutionResult] =
    useState<MigrateResponse | null>(null);
  const [executionError, setExecutionError] = useState<string | null>(null);
  const [showSchemaModal, setShowSchemaModal] = useState(false);
  const [showConfirmModal, setShowConfirmModal] = useState(false);
  const [confirmModalConfig, setConfirmModalConfig] = useState<{
    title: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
    confirmButtonClass?: string;
    onConfirm: () => void;
  } | null>(null);
  const [expandedFiles, setExpandedFiles] = useState<{
    up: boolean;
    down: boolean;
  }>({ up: false, down: false });
  const [schemaExecutions, setSchemaExecutions] = useState<MigrationListItem[]>(
    [],
  );
  const [schemaExecutionsExpanded, setSchemaExecutionsExpanded] =
    useState(true);
  const [schemaExecutionsPage, setSchemaExecutionsPage] = useState(1);
  const [schemaExecutionsPerPage, setSchemaExecutionsPerPage] = useState(10);
  const [executions, setExecutions] = useState<MigrationExecution[]>([]);
  const [executionsLoading, setExecutionsLoading] = useState(false);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyPerPage, setHistoryPerPage] = useState(10);

  useEffect(() => {
    if (id) {
      loadMigration();
      loadStatus();
      loadHistory();
      loadExecutions();
      const interval = setInterval(() => {
        loadStatus();
        loadHistory();
        loadExecutions();
      }, 5000); // Refresh status every 5 seconds
      return () => clearInterval(interval);
    }
  }, [id]);

  const loadMigration = async () => {
    if (!id) return;
    try {
      setLoading(true);
      const data = await apiClient.getMigration(id);
      setMigration(data);
      setError(null);
      // Load schema executions after migration is loaded
      if (data) {
        loadSchemaExecutions();
      }
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to load migration";
      setError(errorMsg);
      // Error toast is handled by API interceptor
    } finally {
      setLoading(false);
    }
  };

  const loadStatus = async () => {
    if (!id) return;
    try {
      const data = await apiClient.getMigrationStatus(id);
      setStatus(data);
    } catch (err) {
      // Silently fail status updates
    }
  };

  const loadHistory = async () => {
    if (!id) return;
    try {
      const data = await apiClient.getMigrationHistory(id);
      // Sort by applied_at descending (most recent first)
      const sortedHistory = [...data.history].sort((a, b) => {
        const dateA = new Date(a.applied_at).getTime();
        const dateB = new Date(b.applied_at).getTime();
        return dateB - dateA;
      });
      setHistory(sortedHistory);
    } catch (err) {
      // Silently fail history updates
    }
  };

  const loadExecutions = async () => {
    if (!id) return;
    try {
      setExecutionsLoading(true);
      const data = await apiClient.getMigrationExecutions(id);
      setExecutions(data.executions);
    } catch (err) {
      // Silently fail executions updates
    } finally {
      setExecutionsLoading(false);
    }
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

  const loadSchemaExecutions = async () => {
    if (!id || !migration) return;

    try {
      // Fetch all migrations and filter for schema-specific executions
      const allMigrations = await apiClient.listMigrations({});

      // Determine the base migration ID
      // If this is a schema-specific migration, extract the base ID
      // If this is a base migration, use the ID directly
      const baseID = getBaseMigrationID(id);

      // Find all schema-specific migrations that match this base
      const executions = allMigrations.items.filter((item) => {
        // Only include schema-specific migrations
        if (!isSchemaSpecific(item.migration_id)) {
          return false;
        }
        // Extract base ID by removing schema prefix
        const itemBaseID = getBaseMigrationID(item.migration_id);
        return itemBaseID === baseID;
      });

      setSchemaExecutions(executions);
    } catch (err) {
      // Silently fail
    }
  };

  // Compute actual applied status from history
  // A migration is applied if the latest record is not a rollback
  const isActuallyApplied = useMemo(() => {
    if (history.length === 0) {
      // If no history, fall back to migration.applied
      return migration?.applied ?? false;
    }
    // Get the latest record (history is sorted by most recent first)
    const latestRecord = history[0];
    // Migration is applied if the latest record is not a rollback
    return (
      !latestRecord.migration_id.includes("_rollback") &&
      latestRecord.status === "success"
    );
  }, [history, migration?.applied]);

  // Get the latest applied_at from history (for successful, non-rollback records)
  const latestAppliedAt = useMemo(() => {
    if (history.length === 0) {
      return status?.applied_at || null;
    }
    // Find the latest successful, non-rollback record
    const latestSuccessRecord = history.find(
      (record) =>
        !record.migration_id.includes("_rollback") &&
        record.status === "success",
    );
    return latestSuccessRecord?.applied_at || null;
  }, [history, status?.applied_at]);

  // Compute the actual status from history (not from API response which might be stale)
  const actualStatus = useMemo(() => {
    if (history.length === 0) {
      return status?.status || "pending";
    }
    // Get the latest record (history is sorted by most recent first)
    const latestRecord = history[0];

    // If the latest record is a rollback, check if there's a more recent successful application
    if (latestRecord.migration_id.includes("_rollback")) {
      // Find the most recent successful, non-rollback record
      const latestSuccessRecord = history.find(
        (record) =>
          !record.migration_id.includes("_rollback") &&
          record.status === "success",
      );
      if (latestSuccessRecord) {
        // Compare timestamps - if success record is more recent, use it
        const rollbackTime = new Date(latestRecord.applied_at).getTime();
        const successTime = new Date(latestSuccessRecord.applied_at).getTime();
        if (successTime > rollbackTime) {
          return latestSuccessRecord.status;
        }
      }
      return "rolled_back";
    }

    // Latest record is not a rollback, use its status
    return latestRecord.status || "pending";
  }, [history, status?.status]);

  // Helper function to check if backend is SQL-based
  const isSQLBackend = (backend: string): boolean => {
    return backend === "postgresql" || backend === "greptimedb";
  };

  // Helper function to check if backend is NoSQL-based
  const isNoSQLBackend = (backend: string): boolean => {
    return backend === "etcd";
  };

  // Check if schema/prefix is required and missing
  const needsSchemaOrPrefix = (migration: MigrationDetailResponse): boolean => {
    if (!migration.schema || migration.schema.trim() === "") {
      return (
        isSQLBackend(migration.backend) || isNoSQLBackend(migration.backend)
      );
    }
    return false;
  };

  const executeMigration = async (userSchema?: string) => {
    if (!migration || !id) return;

    setIsExecuting(true);
    setExecutionError(null);
    setExecutionResult(null);

    try {
      // Determine schema to use
      // For SQL: use user-provided schema or migration.schema
      // For NoSQL: schema might represent prefix, but it's handled server-side via env vars
      // The frontend still needs to provide it in the schemas array for consistency
      const schemaToUse = userSchema || migration.schema || "";

      const migrateRequest: MigrateUpRequest = {
        connection: migration.connection,
        target: {
          backend: migration.backend,
          connection: migration.connection,
          version: migration.version,
        },
        schemas: schemaToUse ? [schemaToUse] : [],
      };

      const response = await apiClient.migrateUp(migrateRequest);
      setExecutionResult(response);

      if (response.success) {
        if (response.queued) {
          toastService.info(`Migration queued with job ID: ${response.job_id}`);
        } else {
          toastService.success(
            `Migration executed successfully! ${response.applied.length} migration(s) applied.`,
          );
        }
      } else {
        const errorCount = response.errors.length;
        toastService.warning(
          `Migration completed with ${errorCount} error(s). ${response.applied.length} migration(s) applied.`,
        );
      }

      loadMigration();
      loadStatus();
      loadHistory(); // Reload history to get the latest status and applied_at
      loadExecutions();
      loadSchemaExecutions();
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to execute migration";
      setExecutionError(errorMsg);
      // Error toast handled by API interceptor
    } finally {
      setIsExecuting(false);
    }
  };

  const showExecutionConfirm = () => {
    setConfirmModalConfig({
      title: "Execute Migration",
      message: "Are you sure you want to execute this migration now?",
      confirmText: "Execute",
      cancelText: "Cancel",
      confirmButtonClass: "bg-bfm-green-dark text-white hover:bg-bfm-green",
      onConfirm: () => {
        setShowConfirmModal(false);
        executeMigration();
      },
    });
    setShowConfirmModal(true);
  };

  const handleExecute = async () => {
    if (!migration || !id) return;

    // Check if schema/prefix is required
    const needsValue = needsSchemaOrPrefix(migration);

    if (needsValue) {
      // Show modal to get schema/prefix from user
      setShowSchemaModal(true);
      return;
    }

    // If schema is not needed or already exists, show confirmation modal
    showExecutionConfirm();
  };

  const handleSchemaConfirm = (value: string) => {
    setShowSchemaModal(false);

    // After schema is confirmed, show execution confirmation modal
    setConfirmModalConfig({
      title: "Execute Migration",
      message: "Are you sure you want to execute this migration now?",
      confirmText: "Execute",
      cancelText: "Cancel",
      confirmButtonClass: "bg-bfm-green-dark text-white hover:bg-bfm-green",
      onConfirm: () => {
        setShowConfirmModal(false);
        executeMigration(value);
      },
    });
    setShowConfirmModal(true);
  };

  const handleSchemaModalClose = () => {
    setShowSchemaModal(false);
  };

  const handleRollback = async () => {
    if (!id) return;

    setConfirmModalConfig({
      title: "Rollback Migration",
      message:
        "Are you sure you want to rollback this migration? This will execute the down migration script.",
      confirmText: "Rollback",
      cancelText: "Cancel",
      confirmButtonClass: "bg-red-600 text-white hover:bg-red-700",
      onConfirm: async () => {
        setShowConfirmModal(false);
        try {
          // Use migration schema if available
          const schemas = migration?.schema ? [migration.schema] : [];
          const result = await apiClient.rollbackMigration(id, schemas);
          if (result.success) {
            toastService.success(
              result.message || "Migration rolled back successfully",
            );
            // Reload all data to reflect the rollback
            loadMigration();
            loadStatus();
            loadHistory();
            loadExecutions();
          } else {
            toastService.error(`Rollback failed: ${result.message}`);
          }
        } catch (err) {
          const errorMsg = err instanceof Error ? err.message : "Unknown error";
          toastService.error(`Rollback error: ${errorMsg}`);
          // Error toast is also handled by API interceptor
        }
      },
    });
    setShowConfirmModal(true);
  };

  if (loading) {
    return (
      <div className="text-center py-8 text-xl text-gray-500">
        Loading migration details...
      </div>
    );
  }

  if (error || !migration) {
    return (
      <div className="text-center py-8 text-xl text-red-600">
        {error || "Migration not found"}
        <Link
          to="/migrations"
          className="block mt-4 text-bfm-blue no-underline text-sm hover:underline"
        >
          ← Back to Migrations
        </Link>
      </div>
    );
  }

  return (
    <div className="max-w-5xl mx-auto animate-fade-in">
      <div className="mb-6 md:mb-8 animate-slide-up">
        <Link
          to="/migrations"
          className="inline-block text-bfm-blue no-underline mb-2 text-sm hover:underline"
        >
          ← Back to Migrations
        </Link>
        <h1 className="text-2xl md:text-3xl font-semibold text-gray-800 mt-2">
          Migration Details
        </h1>
      </div>

      <div className="grid gap-6">
        <div className="bg-white p-6 rounded-lg shadow-md animate-scale-in transition-all hover:shadow-lg">
          <h2 className="text-gray-800 mb-4 text-xl font-semibold">
            Basic Information
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Migration ID
              </label>
              <div className="text-gray-800 text-base">
                {migration.migration_id}
              </div>
            </div>
            <div className="flex flex-col col-span-full">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Name
              </label>
              <div className="text-gray-800 text-base font-medium">
                {migration.name}
              </div>
            </div>
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                {isNoSQLBackend(migration.backend) ? "Prefix" : "Schema"}
              </label>
              <div className="text-gray-800 text-base font-medium">
                {migration.schema || (
                  <span className="text-gray-400 italic">Not specified</span>
                )}
              </div>
            </div>
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Table
              </label>
              <div className="text-gray-800 text-base font-medium">
                {migration.table}
              </div>
            </div>
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Version
              </label>
              <div className="text-gray-800 text-base font-medium">
                {migration.version}
              </div>
            </div>
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Backend
              </label>
              <div className="text-gray-800 text-base font-medium">
                {migration.backend}
              </div>
            </div>
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Connection
              </label>
              <div className="text-gray-800 text-base font-medium">
                {migration.connection}
              </div>
            </div>
            <div className="flex flex-col">
              <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                Applied
              </label>
              <div
                className={`text-base font-medium ${
                  migration.applied ? "text-bfm-green-dark" : "text-gray-500"
                }`}
              >
                {migration.applied ? "Yes" : "No"}
              </div>
            </div>
            {((migration.dependencies && migration.dependencies.length > 0) ||
              (migration.structured_dependencies &&
                migration.structured_dependencies.length > 0)) && (
              <div className="flex flex-col col-span-full">
                <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                  Dependencies
                </label>
                {/* Simple string dependencies (backward compatibility) */}
                {migration.dependencies &&
                  migration.dependencies.length > 0 && (
                    <div className="flex flex-wrap gap-2 mb-3">
                      {migration.dependencies.map((dep, index) => (
                        <span
                          key={`simple-${index}`}
                          className="inline-block px-3 py-1 bg-blue-100 text-blue-800 rounded-full text-sm font-medium"
                        >
                          {dep}
                        </span>
                      ))}
                    </div>
                  )}
                {/* Structured dependencies */}
                {migration.structured_dependencies &&
                  migration.structured_dependencies.length > 0 && (
                    <div className="space-y-3">
                      {migration.structured_dependencies.map((dep, index) => (
                        <div
                          key={`structured-${index}`}
                          className="border border-gray-200 rounded-lg p-4 bg-gradient-to-br from-gray-50 to-white shadow-sm hover:shadow-md transition-shadow"
                        >
                          <div className="flex flex-wrap gap-2 mb-3">
                            <div className="flex items-center gap-2">
                              <span className="text-xs font-semibold text-gray-600 uppercase tracking-wide">
                                Target:
                              </span>
                              <span className="inline-block px-2 py-1 bg-blue-100 text-blue-800 rounded text-xs font-medium">
                                {dep.target}
                              </span>
                            </div>
                            {dep.connection && (
                              <span className="inline-block px-2 py-1 bg-green-100 text-green-800 rounded text-xs font-medium">
                                <span className="font-semibold">
                                  Connection:
                                </span>{" "}
                                {dep.connection}
                              </span>
                            )}
                            {dep.schema && (
                              <span className="inline-block px-2 py-1 bg-purple-100 text-purple-800 rounded text-xs font-medium">
                                <span className="font-semibold">Schema:</span>{" "}
                                {dep.schema}
                              </span>
                            )}
                            {dep.target_type && dep.target_type !== "name" && (
                              <span className="inline-block px-2 py-1 bg-yellow-100 text-yellow-800 rounded text-xs font-medium">
                                <span className="font-semibold">Type:</span>{" "}
                                {dep.target_type}
                              </span>
                            )}
                          </div>
                          {(dep.requires_table || dep.requires_schema) && (
                            <div className="mt-3 pt-3 border-t border-gray-300">
                              <p className="text-xs text-gray-700 font-semibold mb-2 flex items-center gap-1">
                                <svg
                                  className="w-4 h-4 text-orange-600"
                                  fill="none"
                                  stroke="currentColor"
                                  viewBox="0 0 24 24"
                                >
                                  <path
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                    strokeWidth={2}
                                    d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                                  />
                                </svg>
                                Validation Requirements
                              </p>
                              <div className="flex flex-wrap gap-2">
                                {dep.requires_schema && (
                                  <span className="inline-flex items-center gap-1 px-2 py-1 bg-orange-100 text-orange-800 rounded text-xs font-medium">
                                    <svg
                                      className="w-3 h-3"
                                      fill="none"
                                      stroke="currentColor"
                                      viewBox="0 0 24 24"
                                    >
                                      <path
                                        strokeLinecap="round"
                                        strokeLinejoin="round"
                                        strokeWidth={2}
                                        d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4"
                                      />
                                    </svg>
                                    <span className="font-semibold">
                                      Schema:
                                    </span>{" "}
                                    {dep.requires_schema}
                                  </span>
                                )}
                                {dep.requires_table && (
                                  <span className="inline-flex items-center gap-1 px-2 py-1 bg-red-100 text-red-800 rounded text-xs font-medium">
                                    <svg
                                      className="w-3 h-3"
                                      fill="none"
                                      stroke="currentColor"
                                      viewBox="0 0 24 24"
                                    >
                                      <path
                                        strokeLinecap="round"
                                        strokeLinejoin="round"
                                        strokeWidth={2}
                                        d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"
                                      />
                                    </svg>
                                    <span className="font-semibold">
                                      Table:
                                    </span>{" "}
                                    {dep.requires_table}
                                  </span>
                                )}
                              </div>
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                <p className="text-gray-500 text-xs mt-2 italic">
                  This migration depends on the above migrations and will
                  execute after them. Structured dependencies include validation
                  requirements that are checked before execution.
                </p>
              </div>
            )}
          </div>
        </div>

        {(migration.up_sql || migration.down_sql) && (
          <div className="bg-white p-6 rounded-lg shadow-md">
            <h2 className="text-gray-800 mb-4 text-xl font-semibold">
              Migration Files
            </h2>
            <div className="space-y-4">
              {migration.up_sql && (
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <button
                      onClick={() =>
                        setExpandedFiles((prev) => ({ ...prev, up: !prev.up }))
                      }
                      className="flex items-center gap-2 text-gray-700 text-base font-medium hover:text-gray-900 transition-colors"
                    >
                      <svg
                        className={`w-5 h-5 transition-transform ${
                          expandedFiles.up ? "rotate-90" : ""
                        }`}
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9 5l7 7-7 7"
                        />
                      </svg>
                      {isNoSQLBackend(migration.backend) ? (
                        <>Up Migration (.up.json)</>
                      ) : (
                        <>Up Migration (.up.sql)</>
                      )}
                    </button>
                    {expandedFiles.up && (
                      <button
                        onClick={() => {
                          navigator.clipboard.writeText(migration.up_sql || "");
                          toastService.success("Copied to clipboard");
                        }}
                        className="px-3 py-1 text-xs bg-gray-100 text-gray-700 rounded hover:bg-gray-200 transition-colors"
                        title="Copy to clipboard"
                      >
                        Copy
                      </button>
                    )}
                  </div>
                  {expandedFiles.up && (
                    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 overflow-x-auto max-h-96 overflow-y-auto">
                      <pre className="text-sm font-mono text-gray-800 whitespace-pre-wrap break-words m-0">
                        <code>
                          {(() => {
                            const content = migration.up_sql || "";
                            // Try to format JSON if it looks like JSON
                            if (
                              isNoSQLBackend(migration.backend) &&
                              content.trim().startsWith("{")
                            ) {
                              try {
                                const parsed = JSON.parse(content);
                                return JSON.stringify(parsed, null, 2);
                              } catch {
                                return content;
                              }
                            }
                            return content;
                          })()}
                        </code>
                      </pre>
                    </div>
                  )}
                </div>
              )}
              {migration.down_sql && (
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <button
                      onClick={() =>
                        setExpandedFiles((prev) => ({
                          ...prev,
                          down: !prev.down,
                        }))
                      }
                      className="flex items-center gap-2 text-gray-700 text-base font-medium hover:text-gray-900 transition-colors"
                    >
                      <svg
                        className={`w-5 h-5 transition-transform ${
                          expandedFiles.down ? "rotate-90" : ""
                        }`}
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M9 5l7 7-7 7"
                        />
                      </svg>
                      {isNoSQLBackend(migration.backend) ? (
                        <>Down Migration (.down.json)</>
                      ) : (
                        <>Down Migration (.down.sql)</>
                      )}
                    </button>
                    {expandedFiles.down && (
                      <button
                        onClick={() => {
                          navigator.clipboard.writeText(
                            migration.down_sql || "",
                          );
                          toastService.success("Copied to clipboard");
                        }}
                        className="px-3 py-1 text-xs bg-gray-100 text-gray-700 rounded hover:bg-gray-200 transition-colors"
                        title="Copy to clipboard"
                      >
                        Copy
                      </button>
                    )}
                  </div>
                  {expandedFiles.down && (
                    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 overflow-x-auto max-h-96 overflow-y-auto">
                      <pre className="text-sm font-mono text-gray-800 whitespace-pre-wrap break-words m-0">
                        <code>
                          {(() => {
                            const content = migration.down_sql || "";
                            // Try to format JSON if it looks like JSON
                            if (
                              isNoSQLBackend(migration.backend) &&
                              content.trim().startsWith("{")
                            ) {
                              try {
                                const parsed = JSON.parse(content);
                                return JSON.stringify(parsed, null, 2);
                              } catch {
                                return content;
                              }
                            }
                            return content;
                          })()}
                        </code>
                      </pre>
                    </div>
                  )}
                </div>
              )}
              {!migration.up_sql && !migration.down_sql && (
                <p className="text-gray-500 text-sm italic">
                  No migration files available
                </p>
              )}
            </div>
          </div>
        )}

        {status && (
          <div className="bg-white p-6 rounded-lg shadow-md">
            <h2 className="text-gray-800 mb-4 text-xl font-semibold">
              Status Information
            </h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              <div className="flex flex-col">
                <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                  Status
                </label>
                <div className="text-base font-medium">
                  <span
                    className={`inline-block px-3 py-1 rounded-full text-xs font-medium ${
                      actualStatus === "success" || actualStatus === "applied"
                        ? "bg-green-100 text-green-800"
                        : actualStatus === "failed"
                          ? "bg-red-100 text-red-800"
                          : actualStatus === "rolled_back"
                            ? "bg-orange-100 text-orange-800"
                            : "bg-yellow-100 text-yellow-800"
                    }`}
                  >
                    {actualStatus === "rolled_back"
                      ? "Rolled Back"
                      : actualStatus || "pending"}
                  </span>
                </div>
              </div>
              {latestAppliedAt && (
                <div className="flex flex-col">
                  <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                    Applied At
                  </label>
                  <div className="text-gray-800 text-base font-medium">
                    {format(new Date(latestAppliedAt), "yyyy-MM-dd HH:mm:ss")}
                  </div>
                </div>
              )}
              {status.error_message && (
                <div className="flex flex-col col-span-full">
                  <label className="text-gray-500 text-xs mb-1 uppercase tracking-wide">
                    Error Message
                  </label>
                  <div className="text-red-600 bg-red-50 p-3 rounded font-mono text-sm whitespace-pre-wrap">
                    {status.error_message}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        <div className="bg-white p-6 rounded-lg shadow-md">
          <h2 className="text-gray-800 mb-3 text-xl font-semibold">
            Manual Execution
          </h2>
          <p className="text-gray-600 text-sm mb-4">
            All migrations must be executed manually from this page. Review the
            details above before running.
            {needsSchemaOrPrefix(migration) && (
              <span className="block mt-2 text-xs text-yellow-700 bg-yellow-50 p-2 rounded">
                ⚠️ {isNoSQLBackend(migration.backend) ? "Prefix" : "Schema"} is
                required for {migration.backend} connections. You will be
                prompted to specify it before execution.
              </span>
            )}
          </p>
          <button
            onClick={handleExecute}
            disabled={isActuallyApplied || isExecuting}
            className="px-6 py-3 bg-bfm-green-dark text-white border-none rounded text-base font-medium cursor-pointer transition-colors duration-200 disabled:opacity-60 disabled:cursor-not-allowed disabled:hover:shadow-none hover:bg-bfm-green hover:shadow-md"
          >
            {isActuallyApplied
              ? "Migration Already Applied"
              : isExecuting
                ? "Executing..."
                : "Execute Migration"}
          </button>
          {isActuallyApplied && (
            <p className="mt-2 text-sm text-gray-600">
              Rollback this migration if you need to execute it again.
            </p>
          )}
          {executionError && (
            <div className="mt-4 bg-red-100 text-red-800 p-3 rounded">
              {executionError}
            </div>
          )}
          {executionResult && (
            <div className="mt-4 border border-gray-200 rounded-lg p-4 bg-gray-50">
              <div
                className={`px-4 py-3 rounded font-medium mb-4 ${
                  executionResult.success
                    ? "bg-green-100 text-green-800"
                    : "bg-red-100 text-red-800"
                }`}
              >
                {executionResult.success ? "✓ Success" : "✗ Failed"}
              </div>
              {executionResult.queued && (
                <div className="bg-blue-100 text-blue-800 p-3 rounded mb-4">
                  <strong>Job ID:</strong> {executionResult.job_id}
                  <br />
                  <em>Migration queued for async execution.</em>
                </div>
              )}
              {executionResult.applied.length > 0 && (
                <div className="mb-4">
                  <h3 className="text-gray-800 mb-2 text-base font-semibold">
                    Applied ({executionResult.applied.length})
                  </h3>
                  <ul className="bg-white rounded border border-gray-200 divide-y divide-gray-200 font-mono text-sm">
                    {executionResult.applied.map((item) => (
                      <li key={item} className="px-3 py-2">
                        {item}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {executionResult.skipped.length > 0 && (
                <div className="mb-4">
                  <h3 className="text-gray-800 mb-2 text-base font-semibold">
                    Skipped ({executionResult.skipped.length})
                  </h3>
                  <ul className="bg-white rounded border border-gray-200 divide-y divide-gray-200 font-mono text-sm">
                    {executionResult.skipped.map((item) => (
                      <li key={item} className="px-3 py-2">
                        {item}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {executionResult.errors.length > 0 && (
                <div>
                  <h3 className="text-gray-800 mb-2 text-base font-semibold">
                    Errors ({executionResult.errors.length})
                  </h3>
                  <ul className="bg-red-50 rounded border border-red-200 divide-y divide-red-200 font-mono text-sm text-red-800">
                    {executionResult.errors.map((item, idx) => (
                      <li key={idx} className="px-3 py-2">
                        {item}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Executions Table */}
        <div className="bg-white p-6 rounded-lg shadow-md">
          <h2 className="text-gray-800 mb-4 text-xl font-semibold">
            Executions ({executions.length})
          </h2>
          {executionsLoading ? (
            <div className="text-center py-8 text-gray-500">
              Loading executions...
            </div>
          ) : executions.length === 0 ? (
            <div className="text-center py-8 text-gray-500">
              No executions found for this migration.
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead>
                  <tr>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      ID
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Schema
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Version
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Connection
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Backend
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Status
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Applied
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Created At
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {executions.map((execution) => (
                    <tr
                      key={execution.id}
                      className="hover:bg-gray-50 transition-colors"
                    >
                      <td className="p-3 border-b border-gray-200 text-sm font-mono">
                        {execution.id}
                      </td>
                      <td className="p-3 border-b border-gray-200 text-sm">
                        {execution.schema || "-"}
                      </td>
                      <td className="p-3 border-b border-gray-200 text-sm font-mono">
                        {execution.version}
                      </td>
                      <td className="p-3 border-b border-gray-200 text-sm">
                        {execution.connection}
                      </td>
                      <td className="p-3 border-b border-gray-200 text-sm">
                        {execution.backend}
                      </td>
                      <td className="p-3 border-b border-gray-200">
                        <span
                          className={`inline-block px-2 py-1 rounded text-xs font-medium ${
                            execution.status === "applied"
                              ? "bg-green-100 text-green-800"
                              : execution.status === "failed"
                                ? "bg-red-100 text-red-800"
                                : "bg-yellow-100 text-yellow-800"
                          }`}
                        >
                          {execution.status}
                        </span>
                      </td>
                      <td className="p-3 border-b border-gray-200 text-sm">
                        {execution.applied ? (
                          <span className="text-green-600 font-medium">
                            Yes
                          </span>
                        ) : (
                          <span className="text-gray-400">No</span>
                        )}
                      </td>
                      <td className="p-3 border-b border-gray-200 text-sm">
                        {execution.created_at
                          ? format(
                              new Date(execution.created_at),
                              "yyyy-MM-dd HH:mm:ss",
                            )
                          : "-"}
                      </td>
                      <td className="p-3 border-b border-gray-200">
                        <div className="flex gap-2">
                          {execution.applied && (
                            <button
                              onClick={() => {
                                setConfirmModalConfig({
                                  title: "Rollback Execution",
                                  message: `Are you sure you want to rollback execution #${execution.id}? This will rollback the migration for schema: ${execution.schema || "default"}, version: ${execution.version}, connection: ${execution.connection}`,
                                  confirmText: "Rollback",
                                  cancelText: "Cancel",
                                  confirmButtonClass:
                                    "bg-red-600 text-white hover:bg-red-700",
                                  onConfirm: async () => {
                                    setShowConfirmModal(false);
                                    try {
                                      const schemas = execution.schema
                                        ? [execution.schema]
                                        : [];
                                      const result =
                                        await apiClient.rollbackMigration(
                                          execution.migration_id,
                                          schemas,
                                        );
                                      if (result.success) {
                                        toastService.success(
                                          result.message ||
                                            "Rollback completed successfully",
                                        );
                                      } else {
                                        toastService.warning(
                                          result.message ||
                                            "Rollback completed with errors",
                                        );
                                      }
                                      loadExecutions();
                                      loadStatus();
                                      loadHistory();
                                    } catch (err) {
                                      toastService.error(
                                        err instanceof Error
                                          ? err.message
                                          : "Failed to rollback",
                                      );
                                    }
                                  },
                                });
                                setShowConfirmModal(true);
                              }}
                              className="px-3 py-1 text-xs bg-red-100 text-red-800 rounded hover:bg-red-200 transition-colors"
                            >
                              Rollback
                            </button>
                          )}
                          {!execution.applied && (
                            <button
                              onClick={() => {
                                setConfirmModalConfig({
                                  title: "Re-execute Migration",
                                  message: `Are you sure you want to re-execute execution #${execution.id}? This will execute the migration for schema: ${execution.schema || "default"}, version: ${execution.version}, connection: ${execution.connection}`,
                                  confirmText: "Execute",
                                  cancelText: "Cancel",
                                  confirmButtonClass:
                                    "bg-bfm-green-dark text-white hover:bg-bfm-green",
                                  onConfirm: async () => {
                                    setShowConfirmModal(false);
                                    if (!migration) return;
                                    const schemaToUse =
                                      execution.schema ||
                                      migration.schema ||
                                      "";
                                    try {
                                      const migrateRequest: MigrateUpRequest = {
                                        connection: execution.connection,
                                        target: {
                                          backend: execution.backend,
                                          connection: execution.connection,
                                          version: execution.version,
                                        },
                                        schemas: schemaToUse
                                          ? [schemaToUse]
                                          : [],
                                      };
                                      const response =
                                        await apiClient.migrateUp(
                                          migrateRequest,
                                        );
                                      if (response.success) {
                                        toastService.success(
                                          "Migration executed successfully",
                                        );
                                      } else {
                                        toastService.warning(
                                          "Migration completed with errors",
                                        );
                                      }
                                      loadExecutions();
                                      loadStatus();
                                      loadHistory();
                                    } catch (err) {
                                      toastService.error(
                                        err instanceof Error
                                          ? err.message
                                          : "Failed to execute",
                                      );
                                    }
                                  },
                                });
                                setShowConfirmModal(true);
                              }}
                              className="px-3 py-1 text-xs bg-bfm-green-dark text-white rounded hover:bg-bfm-green transition-colors"
                            >
                              Re-execute
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {schemaExecutions.length > 0 && (
          <div className="bg-white p-6 rounded-lg shadow-md">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-gray-800 text-xl font-semibold">
                Schema Executions ({schemaExecutions.length})
              </h2>
              <button
                onClick={() =>
                  setSchemaExecutionsExpanded(!schemaExecutionsExpanded)
                }
                className="flex items-center gap-2 text-gray-700 hover:text-gray-900 transition-colors"
              >
                <svg
                  className={`w-5 h-5 transition-transform ${
                    schemaExecutionsExpanded ? "rotate-90" : ""
                  }`}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 5l7 7-7 7"
                  />
                </svg>
                {schemaExecutionsExpanded ? "Collapse" : "Expand"}
              </button>
            </div>
            {schemaExecutionsExpanded && (
              <>
                <p className="text-gray-600 text-sm mb-4">
                  This migration has been executed on the following schemas:
                </p>
                <div className="overflow-x-auto">
                  <table className="w-full border-collapse">
                    <thead>
                      <tr>
                        <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                          Migration ID
                        </th>
                        <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                          Schema
                        </th>
                        <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                          Status
                        </th>
                        <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                          Applied
                        </th>
                        <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                          Applied At
                        </th>
                        <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                          Actions
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {(() => {
                        const startIndex =
                          (schemaExecutionsPage - 1) * schemaExecutionsPerPage;
                        const endIndex = startIndex + schemaExecutionsPerPage;
                        const paginatedExecutions = schemaExecutions.slice(
                          startIndex,
                          endIndex,
                        );

                        return (
                          <>
                            {paginatedExecutions.length === 0 ? (
                              <tr>
                                <td
                                  colSpan={6}
                                  className="text-center text-gray-500 py-8"
                                >
                                  No schema executions found
                                </td>
                              </tr>
                            ) : (
                              paginatedExecutions.map((execution) => (
                                <tr
                                  key={execution.migration_id}
                                  className="hover:bg-gray-50 transition-colors"
                                >
                                  <td className="p-3 border-b border-gray-200">
                                    <Link
                                      to={`/migrations/${execution.migration_id}`}
                                      className="text-bfm-blue no-underline hover:underline text-sm font-mono"
                                    >
                                      {execution.migration_id}
                                    </Link>
                                  </td>
                                  <td className="p-3 border-b border-gray-200">
                                    <span className="text-gray-800 text-sm font-medium">
                                      {execution.schema || "-"}
                                    </span>
                                  </td>
                                  <td className="p-3 border-b border-gray-200">
                                    <span
                                      className={`inline-block px-3 py-1 rounded-full text-xs font-medium ${
                                        execution.status === "success" ||
                                        execution.status === "applied"
                                          ? "bg-green-100 text-green-800"
                                          : execution.status === "failed"
                                            ? "bg-red-100 text-red-800"
                                            : execution.status === "rolled_back"
                                              ? "bg-orange-100 text-orange-800"
                                              : "bg-yellow-100 text-yellow-800"
                                      }`}
                                    >
                                      {execution.status === "rolled_back"
                                        ? "Rolled Back"
                                        : execution.status || "pending"}
                                    </span>
                                  </td>
                                  <td className="p-3 border-b border-gray-200">
                                    <span
                                      className={
                                        execution.applied
                                          ? "text-bfm-green-dark font-medium"
                                          : "text-gray-500"
                                      }
                                    >
                                      {execution.applied ? "Yes" : "No"}
                                    </span>
                                  </td>
                                  <td className="p-3 border-b border-gray-200 text-sm text-gray-800">
                                    {execution.applied_at
                                      ? format(
                                          new Date(execution.applied_at),
                                          "yyyy-MM-dd HH:mm:ss",
                                        )
                                      : "-"}
                                  </td>
                                  <td className="p-3 border-b border-gray-200">
                                    <Link
                                      to={`/migrations/${execution.migration_id}`}
                                      className="inline-block px-3 py-1 bg-bfm-blue text-white rounded text-sm no-underline transition-all duration-200 hover:bg-bfm-blue-dark hover:shadow-md"
                                    >
                                      View
                                    </Link>
                                  </td>
                                </tr>
                              ))
                            )}
                          </>
                        );
                      })()}
                    </tbody>
                  </table>
                </div>
                {schemaExecutions.length > schemaExecutionsPerPage && (
                  <div className="mt-4 flex flex-col sm:flex-row justify-between items-center gap-4">
                    <div className="text-sm text-gray-600">
                      Showing{" "}
                      {Math.min(
                        (schemaExecutionsPage - 1) * schemaExecutionsPerPage +
                          1,
                        schemaExecutions.length,
                      )}
                      -
                      {Math.min(
                        schemaExecutionsPage * schemaExecutionsPerPage,
                        schemaExecutions.length,
                      )}{" "}
                      of {schemaExecutions.length}
                    </div>
                    <div className="flex items-center gap-2">
                      <label
                        htmlFor="schema-executions-per-page"
                        className="text-sm text-gray-600"
                      >
                        Per page:
                      </label>
                      <select
                        id="schema-executions-per-page"
                        value={schemaExecutionsPerPage}
                        onChange={(e) => {
                          setSchemaExecutionsPerPage(Number(e.target.value));
                          setSchemaExecutionsPage(1);
                        }}
                        className="px-2 py-1 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
                      >
                        <option value={10}>10</option>
                        <option value={25}>25</option>
                        <option value={50}>50</option>
                        <option value={100}>100</option>
                      </select>
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => setSchemaExecutionsPage(1)}
                        disabled={schemaExecutionsPage === 1}
                        className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                      >
                        First
                      </button>
                      <button
                        onClick={() =>
                          setSchemaExecutionsPage(schemaExecutionsPage - 1)
                        }
                        disabled={schemaExecutionsPage === 1}
                        className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                      >
                        Previous
                      </button>
                      <div className="text-sm text-gray-600">
                        Page {schemaExecutionsPage} of{" "}
                        {Math.ceil(
                          schemaExecutions.length / schemaExecutionsPerPage,
                        )}
                      </div>
                      <button
                        onClick={() =>
                          setSchemaExecutionsPage(schemaExecutionsPage + 1)
                        }
                        disabled={
                          schemaExecutionsPage >=
                          Math.ceil(
                            schemaExecutions.length / schemaExecutionsPerPage,
                          )
                        }
                        className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                      >
                        Next
                      </button>
                      <button
                        onClick={() =>
                          setSchemaExecutionsPage(
                            Math.ceil(
                              schemaExecutions.length / schemaExecutionsPerPage,
                            ),
                          )
                        }
                        disabled={
                          schemaExecutionsPage >=
                          Math.ceil(
                            schemaExecutions.length / schemaExecutionsPerPage,
                          )
                        }
                        className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                      >
                        Last
                      </button>
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        )}

        {history.length > 0 && (
          <div className="bg-white p-6 rounded-lg shadow-md">
            <h2 className="text-gray-800 mb-4 text-xl font-semibold">
              Execution History
            </h2>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead>
                  <tr>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Execution ID
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Status
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Applied At
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Executed By
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Method
                    </th>
                    <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200 text-sm">
                      Error Message
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {(() => {
                    const startIndex = (historyPage - 1) * historyPerPage;
                    const endIndex = startIndex + historyPerPage;
                    const paginatedHistory = history.slice(
                      startIndex,
                      endIndex,
                    );

                    return (
                      <>
                        {paginatedHistory.map((record, index) => (
                          <tr
                            key={`${record.migration_id}-${record.applied_at}-${index}`}
                            className="hover:bg-gray-50 transition-colors"
                          >
                            <td className="p-3 border-b border-gray-200">
                              <div className="flex flex-col">
                                <span className="text-gray-800 text-sm font-mono">
                                  {record.migration_id}
                                </span>
                                {record.migration_id.includes("_rollback") && (
                                  <span className="text-xs text-orange-600 italic mt-1">
                                    Rollback
                                  </span>
                                )}
                              </div>
                            </td>
                            <td className="p-3 border-b border-gray-200">
                              <span
                                className={`inline-block px-3 py-1 rounded-full text-xs font-medium ${
                                  record.status === "success" ||
                                  record.status === "applied"
                                    ? "bg-green-100 text-green-800"
                                    : record.status === "failed"
                                      ? "bg-red-100 text-red-800"
                                      : record.status === "rolled_back"
                                        ? "bg-orange-100 text-orange-800"
                                        : "bg-yellow-100 text-yellow-800"
                                }`}
                              >
                                {record.status === "rolled_back"
                                  ? "Rolled Back"
                                  : record.status}
                              </span>
                            </td>
                            <td className="p-3 border-b border-gray-200 text-sm text-gray-800">
                              {format(
                                new Date(record.applied_at),
                                "yyyy-MM-dd HH:mm:ss",
                              )}
                            </td>
                            <td className="p-3 border-b border-gray-200 text-sm text-gray-700">
                              {record.executed_by || (
                                <span className="text-gray-400">-</span>
                              )}
                            </td>
                            <td className="p-3 border-b border-gray-200">
                              {record.execution_method ? (
                                <span
                                  className={`inline-block px-2 py-1 rounded text-xs font-medium ${
                                    record.execution_method === "manual"
                                      ? "bg-blue-100 text-blue-800"
                                      : record.execution_method === "api"
                                        ? "bg-purple-100 text-purple-800"
                                        : record.execution_method === "cli"
                                          ? "bg-gray-100 text-gray-800"
                                          : "bg-yellow-100 text-yellow-800"
                                  }`}
                                >
                                  {record.execution_method}
                                </span>
                              ) : (
                                <span className="text-gray-400">-</span>
                              )}
                            </td>
                            <td className="p-3 border-b border-gray-200">
                              {record.error_message ? (
                                <div className="text-red-600 bg-red-50 p-2 rounded font-mono text-xs whitespace-pre-wrap max-w-md">
                                  {record.error_message}
                                </div>
                              ) : (
                                <span className="text-gray-400">-</span>
                              )}
                            </td>
                          </tr>
                        ))}
                      </>
                    );
                  })()}
                </tbody>
              </table>
            </div>
            {history.length > historyPerPage && (
              <div className="mt-4 flex flex-col sm:flex-row justify-between items-center gap-4">
                <div className="text-sm text-gray-600">
                  Showing{" "}
                  {Math.min(
                    (historyPage - 1) * historyPerPage + 1,
                    history.length,
                  )}
                  -{Math.min(historyPage * historyPerPage, history.length)} of{" "}
                  {history.length}
                </div>
                <div className="flex items-center gap-2">
                  <label
                    htmlFor="history-per-page"
                    className="text-sm text-gray-600"
                  >
                    Per page:
                  </label>
                  <select
                    id="history-per-page"
                    value={historyPerPage}
                    onChange={(e) => {
                      setHistoryPerPage(Number(e.target.value));
                      setHistoryPage(1);
                    }}
                    className="px-2 py-1 border border-gray-300 rounded text-sm bg-white text-gray-800 focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
                  >
                    <option value={10}>10</option>
                    <option value={25}>25</option>
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                  </select>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setHistoryPage(1)}
                    disabled={historyPage === 1}
                    className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    First
                  </button>
                  <button
                    onClick={() => setHistoryPage(historyPage - 1)}
                    disabled={historyPage === 1}
                    className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    Previous
                  </button>
                  <div className="text-sm text-gray-600">
                    Page {historyPage} of{" "}
                    {Math.ceil(history.length / historyPerPage)}
                  </div>
                  <button
                    onClick={() => setHistoryPage(historyPage + 1)}
                    disabled={
                      historyPage >= Math.ceil(history.length / historyPerPage)
                    }
                    className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    Next
                  </button>
                  <button
                    onClick={() =>
                      setHistoryPage(Math.ceil(history.length / historyPerPage))
                    }
                    disabled={
                      historyPage >= Math.ceil(history.length / historyPerPage)
                    }
                    className="px-3 py-2 border border-gray-300 rounded text-sm bg-white text-gray-700 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  >
                    Last
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {isActuallyApplied && (
          <div className="bg-white p-6 rounded-lg shadow-md">
            <h2 className="text-gray-800 mb-4 text-xl font-semibold">
              Actions
            </h2>
            <button
              onClick={handleRollback}
              className="px-6 py-3 bg-red-600 text-white border-none rounded text-base font-medium cursor-pointer transition-colors hover:bg-red-700"
            >
              Rollback Migration
            </button>
            <p className="mt-4 text-yellow-800 bg-yellow-100 p-3 rounded text-sm">
              Warning: Rolling back will execute the down migration script.
            </p>
          </div>
        )}
      </div>

      {/* Schema/Prefix Selection Modal */}
      {migration && (
        <SchemaModal
          isOpen={showSchemaModal}
          onClose={handleSchemaModalClose}
          onConfirm={handleSchemaConfirm}
          connection={migration.connection}
          backend={migration.backend}
          isNoSQL={isNoSQLBackend(migration.backend)}
        />
      )}

      {/* Confirmation Modal */}
      {confirmModalConfig && (
        <ConfirmModal
          isOpen={showConfirmModal}
          onClose={() => {
            setShowConfirmModal(false);
            setConfirmModalConfig(null);
          }}
          onConfirm={confirmModalConfig.onConfirm}
          title={confirmModalConfig.title}
          message={confirmModalConfig.message}
          confirmText={confirmModalConfig.confirmText}
          cancelText={confirmModalConfig.cancelText}
          confirmButtonClass={confirmModalConfig.confirmButtonClass}
        />
      )}
    </div>
  );
}
