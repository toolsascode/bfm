import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import { apiClient } from "../services/api";
import { toastService } from "../services/toast";
import type { MigrationListItem, MigrationExecution } from "../types/api";
import { format } from "date-fns";
import {
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";

export default function Dashboard() {
  const [migrations, setMigrations] = useState<MigrationListItem[]>([]);
  const [recentExecutions, setRecentExecutions] = useState<
    MigrationExecution[]
  >([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [healthStatus, setHealthStatus] = useState<string>("unknown");
  const [reindexing, setReindexing] = useState(false);

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000); // Refresh every 30 seconds
    return () => clearInterval(interval);
  }, []);

  const loadData = async () => {
    try {
      setLoading(true);
      const [migrationsData, health, executionsData] = await Promise.all([
        apiClient.listMigrations(),
        apiClient
          .healthCheck()
          .catch(() => ({ status: "unknown", checks: {} })),
        apiClient.getRecentExecutions(10).catch(() => ({ executions: [] })),
      ]);
      setMigrations(migrationsData.items);
      setHealthStatus(health.status);
      setRecentExecutions(executionsData.executions);
      setError(null);
    } catch (err) {
      const errorMsg =
        err instanceof Error ? err.message : "Failed to load data";
      setError(errorMsg);
      // Error toast is handled by API interceptor, but we can add a specific message here if needed
    } finally {
      setLoading(false);
    }
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
      await loadData();
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
      <div className="space-y-6">
        <div className="h-10 bg-gray-200 rounded animate-pulse w-1/2" />
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {[1, 2, 3, 4].map((i) => (
            <div
              key={i}
              className="h-32 bg-gray-200 rounded-lg animate-pulse"
            />
          ))}
        </div>
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className="h-80 bg-gray-200 rounded-lg animate-pulse"
            />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-8 text-xl text-red-600">
        Error: {error}
      </div>
    );
  }

  // Calculate statistics based on status field
  const total = migrations.length;
  const applied = migrations.filter(
    (m) => m.status === "success" || m.status === "applied",
  ).length;
  const pending = migrations.filter((m) => m.status === "pending").length;
  const failed = migrations.filter((m) => m.status === "failed").length;
  const rolledBack = migrations.filter(
    (m) => m.status === "rolled_back",
  ).length;

  // Calculate Overall Health Score
  // Formula: (applied / total) * 100, with penalties for failed migrations
  // Perfect score (100%) = all migrations applied successfully
  // Penalties: -10% per failed migration, -5% per rolled back migration
  const calculateHealthScore = (): number => {
    if (total === 0) return 100; // No migrations = perfect health

    const baseScore = (applied / total) * 100;
    const failedPenalty = (failed / total) * 10; // -10% per failed migration
    const rolledBackPenalty = (rolledBack / total) * 5; // -5% per rolled back migration

    const score = Math.max(
      0,
      Math.min(100, baseScore - failedPenalty - rolledBackPenalty),
    );
    return Math.round(score);
  };

  const healthScore = calculateHealthScore();
  const healthScoreColor =
    healthScore >= 80
      ? "text-bfm-green-dark"
      : healthScore >= 60
        ? "text-yellow-600"
        : "text-red-600";

  // Prepare gauge chart data for Health Score
  // Gauge chart uses a half-circle (180 degrees)
  const gaugeData = [
    {
      name: "Score",
      value: healthScore,
      fill:
        healthScore >= 80
          ? "#27ae60"
          : healthScore >= 60
            ? "#f39c12"
            : "#e74c3c",
    },
    { name: "Remaining", value: 100 - healthScore, fill: "#e5e7eb" },
  ];

  // Calculate needle angle for gauge (0-180 degrees)
  // Health score 0% = 180° (left), 100% = 0° (right)
  // For SVG: 0° points right, 90° points up, 180° points left
  const needleAngle = 180 - (healthScore / 100) * 180;

  // Group by backend
  const byBackend = migrations.reduce(
    (acc, m) => {
      acc[m.backend] = (acc[m.backend] || 0) + 1;
      return acc;
    },
    {} as Record<string, number>,
  );

  // Group by connection
  const byConnection = migrations.reduce(
    (acc, m) => {
      acc[m.connection] = (acc[m.connection] || 0) + 1;
      return acc;
    },
    {} as Record<string, number>,
  );

  // Status distribution (filter out zero values)
  const statusData = [
    { name: "Applied", value: applied, color: "#27ae60" },
    { name: "Pending", value: pending, color: "#f39c12" },
    { name: "Failed", value: failed, color: "#e74c3c" },
    { name: "Rolled Back", value: rolledBack, color: "#e67e22" },
  ].filter((item) => item.value > 0);

  // Backend distribution
  const backendData = Object.entries(byBackend).map(([name, value]) => ({
    name,
    value,
  }));

  // Connection distribution
  const connectionData = Object.entries(byConnection).map(([name, value]) => ({
    name,
    value,
  }));

  // Recently added files - show migrations sorted by version (newest first)
  // The version field contains a timestamp (YYYYMMDDHHmmss), so higher version = more recent
  const recentlyAddedFiles = [...migrations]
    .sort((a, b) => {
      // Sort by version (descending) to get newest first
      return b.version.localeCompare(a.version);
    })
    .slice(0, 5);

  return (
    <div className="w-full overflow-y-hidden px-4 md:px-6 lg:px-8 animate-fade-in">
      <div className="flex justify-between items-center mb-8 animate-slide-up">
        <h1 className="text-3xl font-semibold text-gray-800">
          Migration Dashboard
        </h1>
        <div className="flex items-center gap-4">
          <button
            onClick={handleReindex}
            disabled={reindexing}
            className="px-4 py-2 bg-bfm-blue text-white rounded text-sm transition-colors hover:bg-bfm-blue-dark disabled:opacity-50 disabled:cursor-not-allowed font-medium"
          >
            {reindexing ? "Reindexing..." : "Reindex"}
          </button>
          <div
            className={`px-4 py-2 rounded font-medium transition-all ${
              healthStatus === "healthy"
                ? "bg-green-100 text-green-800"
                : "bg-red-100 text-red-800"
            }`}
          >
            Status: {healthStatus === "healthy" ? "✓ Healthy" : "✗ Unhealthy"}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4 md:gap-6 mb-8">
        <div
          className={`bg-white p-6 rounded-lg shadow-md text-center border-l-4 animate-scale-in transition-all hover:shadow-lg hover:-translate-y-1`}
          style={{ animationDelay: "0.05s" }}
        >
          <div className="text-gray-500 text-sm uppercase tracking-wide">
            Health Score
          </div>
          <div
            className="relative w-full"
            style={{
              height: "100px",
              minHeight: "100px",
              width: "100%",
              position: "relative",
            }}
          >
            <ResponsiveContainer
              width="100%"
              height={100}
              minWidth={0}
              minHeight={100}
            >
              <PieChart>
                <Pie
                  data={gaugeData}
                  cx="50%"
                  cy="90%"
                  startAngle={180}
                  endAngle={0}
                  innerRadius={35}
                  outerRadius={55}
                  paddingAngle={0}
                  dataKey="value"
                  isAnimationActive={true}
                >
                  {gaugeData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.fill} />
                  ))}
                </Pie>
              </PieChart>
            </ResponsiveContainer>
            {/* Needle overlay - positioned to match Pie chart center */}
            <svg
              className="absolute top-0 left-0 pointer-events-none"
              width="100%"
              height="100%"
              viewBox="0 0 200 100"
              preserveAspectRatio="xMidYMid meet"
              style={{ overflow: "visible" }}
            >
              {/* Needle pivot point at (100, 90) - center horizontally, 90% down vertically */}
              <g transform="translate(100, 90)">
                {/* Needle line */}
                <line
                  x1="0"
                  y1="0"
                  x2={Math.cos((needleAngle * Math.PI) / 180) * 45}
                  y2={-Math.sin((needleAngle * Math.PI) / 180) * 45}
                  stroke="#1f2937"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                />
                {/* Needle center circle */}
                <circle cx="0" cy="0" r="3.5" fill="#1f2937" />
              </g>
            </svg>
            {/* Health score text overlay */}
            <div className="absolute bottom-2  top-0 left-0 right-0 flex justify-center pointer-events-none">
              <div className={`text-2xl font-bold ${healthScoreColor}`}>
                {healthScore}%
              </div>
            </div>
          </div>
        </div>

        <div className="bg-white p-6 rounded-lg shadow-md text-center animate-scale-in transition-all hover:shadow-lg hover:-translate-y-1 flex flex-col justify-center items-center">
          <div className="text-4xl font-bold text-gray-800 mb-2">{total}</div>
          <div className="text-gray-500 text-sm uppercase tracking-wide">
            Total Migrations
          </div>
        </div>
        <div
          className="bg-white p-6 rounded-lg shadow-md text-center border-l-4 border-bfm-green animate-scale-in transition-all hover:shadow-lg hover:-translate-y-1 flex flex-col justify-center items-center"
          style={{ animationDelay: "0.1s" }}
        >
          <div className="text-4xl font-bold text-bfm-green-dark mb-2">
            {applied}
          </div>
          <div className="text-gray-500 text-sm uppercase tracking-wide">
            Applied
          </div>
        </div>
        <div
          className="bg-white p-6 rounded-lg shadow-md text-center border-l-4 border-yellow-500 animate-scale-in transition-all hover:shadow-lg hover:-translate-y-1 flex flex-col justify-center items-center"
          style={{ animationDelay: "0.2s" }}
        >
          <div className="text-4xl font-bold text-yellow-600 mb-2">
            {pending}
          </div>
          <div className="text-gray-500 text-sm uppercase tracking-wide">
            Pending
          </div>
        </div>
        <div
          className="bg-white p-6 rounded-lg shadow-md text-center border-l-4 border-red-500 animate-scale-in transition-all hover:shadow-lg hover:-translate-y-1 flex flex-col justify-center items-center"
          style={{ animationDelay: "0.3s" }}
        >
          <div className="text-4xl font-bold text-red-600 mb-2">{failed}</div>
          <div className="text-gray-500 text-sm uppercase tracking-wide">
            Failed
          </div>
        </div>
        <div
          className="bg-white p-6 rounded-lg shadow-md text-center border-l-4 border-orange-500 animate-scale-in transition-all hover:shadow-lg hover:-translate-y-1 flex flex-col justify-center items-center"
          style={{ animationDelay: "0.4s" }}
        >
          <div className="text-4xl font-bold text-orange-600 mb-2">
            {rolledBack}
          </div>
          <div className="text-gray-500 text-sm uppercase tracking-wide">
            Rolled Back
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 md:gap-6 mb-8">
        <div className="bg-white p-3 rounded-lg shadow-md">
          <h3 className="mb-4 text-gray-800 text-lg font-semibold">
            Status Distribution
          </h3>
          <ResponsiveContainer
            width="100%"
            height={300}
            minWidth={0}
            minHeight={300}
          >
            <PieChart>
              <Pie
                data={statusData}
                cx="50%"
                cy="50%"
                labelLine={true}
                label={({ name, value, percent }) => {
                  // Only show label if slice is large enough (>= 5%)
                  if (percent !== undefined && percent >= 0.05) {
                    return `${name}\n${value} (${(percent * 100).toFixed(0)}%)`;
                  }
                  return "";
                }}
                outerRadius={90}
                innerRadius={30}
                fill="#8884d8"
                dataKey="value"
                paddingAngle={2}
              >
                {statusData.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip
                formatter={(
                  value: number | undefined,
                  name: string | undefined,
                ) => {
                  if (value === undefined) return [name || "", ""];
                  return [
                    `${value} (${((value / total) * 100).toFixed(1)}%)`,
                    name || "",
                  ];
                }}
              />
              <Legend
                verticalAlign="bottom"
                height={36}
                formatter={(value: string) => {
                  const entry = statusData.find((d) => d.name === value);
                  return entry ? `${value} (${entry.value})` : value;
                }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>

        <div className="bg-white p-6 rounded-lg shadow-md">
          <h3 className="mb-4 text-gray-800 text-lg font-semibold">
            Migrations by Backend
          </h3>
          <ResponsiveContainer
            width="100%"
            height={300}
            minWidth={0}
            minHeight={300}
          >
            <BarChart data={backendData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#3498db" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="bg-white p-6 rounded-lg shadow-md">
          <h3 className="mb-4 text-gray-800 text-lg font-semibold">
            Migrations by Connection
          </h3>
          <ResponsiveContainer
            width="100%"
            height={300}
            minWidth={0}
            minHeight={300}
          >
            <BarChart data={connectionData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#9b59b6" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="bg-white p-6 rounded-lg shadow-md mb-8">
        <h2 className="mb-4 text-gray-800 text-xl font-semibold">
          Recently Added Files
        </h2>
        <div className="overflow-x-auto max-w-full">
          <table className="w-full border-collapse min-w-full">
            <thead>
              <tr>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Migration ID
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Backend
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Connection
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Status
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Version
                </th>
              </tr>
            </thead>
            <tbody>
              {recentlyAddedFiles.length === 0 ? (
                <tr>
                  <td colSpan={5} className="text-center text-gray-500 py-8">
                    No migrations found
                  </td>
                </tr>
              ) : (
                recentlyAddedFiles.map((migration) => (
                  <tr
                    key={migration.migration_id}
                    className="hover:bg-gray-50 transition-colors"
                  >
                    <td className="p-3 border-b border-gray-200 max-w-[300px]">
                      <Link
                        to={`/migrations/${migration.migration_id}`}
                        className="text-bfm-blue no-underline hover:text-bfm-blue-dark hover:underline truncate block"
                        title={migration.migration_id}
                      >
                        {migration.migration_id}
                      </Link>
                    </td>
                    <td className="p-3 border-b border-gray-200">
                      {migration.backend}
                    </td>
                    <td className="p-3 border-b border-gray-200">
                      {migration.connection || "-"}
                    </td>
                    <td className="p-3 border-b border-gray-200">
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
                    <td className="p-3 border-b border-gray-200 font-mono text-sm">
                      {migration.version}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="bg-white p-6 rounded-lg shadow-md">
        <h2 className="mb-4 text-gray-800 text-xl font-semibold">
          Recent Migrations
        </h2>
        <div className="overflow-x-auto max-w-full">
          <table className="w-full border-collapse min-w-full">
            <thead>
              <tr>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Migration ID
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Schema
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Connection
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Backend
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Status
                </th>
                <th className="bg-gray-50 p-3 text-left font-semibold text-gray-800 border-b-2 border-gray-200">
                  Created At
                </th>
              </tr>
            </thead>
            <tbody>
              {recentExecutions.length === 0 ? (
                <tr>
                  <td colSpan={6} className="text-center text-gray-500 py-8">
                    No executions found
                  </td>
                </tr>
              ) : (
                recentExecutions.map((execution) => (
                  <tr key={execution.id} className="hover:bg-gray-50">
                    <td className="p-3 border-b border-gray-200 max-w-[300px]">
                      <Link
                        to={`/migrations/${execution.migration_id}`}
                        className="text-bfm-blue no-underline hover:text-bfm-blue-dark hover:underline truncate block"
                        title={execution.migration_id}
                      >
                        {execution.migration_id}
                      </Link>
                    </td>
                    <td className="p-3 border-b border-gray-200">
                      {execution.schema || "-"}
                    </td>
                    <td className="p-3 border-b border-gray-200">
                      {execution.connection}
                    </td>
                    <td className="p-3 border-b border-gray-200">
                      {execution.backend}
                    </td>
                    <td className="p-3 border-b border-gray-200">
                      <span
                        className={`inline-block px-3 py-1 rounded-full text-xs font-medium ${
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
                      {execution.created_at
                        ? format(
                            new Date(execution.created_at),
                            "yyyy-MM-dd HH:mm:ss",
                          )
                        : "-"}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
