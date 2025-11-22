import { useState, useEffect, useMemo } from "react";
import { Link } from "react-router-dom";
import { apiClient } from "../services/api";
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

  // Calculate pagination
  const totalPages = Math.ceil(migrations.length / itemsPerPage);
  const startIndex = (currentPage - 1) * itemsPerPage;
  const endIndex = startIndex + itemsPerPage;
  const paginatedMigrations = migrations.slice(startIndex, endIndex);

  // Reset to page 1 when filters change
  useEffect(() => {
    setCurrentPage(1);
  }, [filters]);

  const handlePageChange = (page: number) => {
    setCurrentPage(page);
    // Scroll to top of table
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const handleItemsPerPageChange = (value: number) => {
    setItemsPerPage(value);
    setCurrentPage(1);
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
      <div className="flex justify-between items-center mb-6 animate-slide-up">
        <h1 className="text-3xl font-semibold text-gray-800">Migrations</h1>
        <div className="flex items-center gap-4">
          <div className="text-base text-gray-500">
            Showing {startIndex + 1}-{Math.min(endIndex, migrations.length)} of{" "}
            {total}
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
        <div className="flex flex-col gap-2">
          <label className="text-sm font-semibold text-gray-800 block">
            Backend
          </label>
          <select
            value={filters.backend || ""}
            onChange={(e) => handleFilterChange("backend", e.target.value)}
            className="px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 w-full focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
          >
            <option value="">All Backends</option>
            {filterOptions.backends.map((backend) => (
              <option key={backend} value={backend}>
                {backend}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-2">
          <label className="text-sm font-semibold text-gray-800 block">
            Schema
          </label>
          <select
            value={filters.schema || ""}
            onChange={(e) => handleFilterChange("schema", e.target.value)}
            className="px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 w-full focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
          >
            <option value="">All Schemas</option>
            {filterOptions.schemas.map((schema) => (
              <option key={schema} value={schema}>
                {schema}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-2">
          <label className="text-sm font-semibold text-gray-800 block">
            Connection
          </label>
          <select
            value={filters.connection || ""}
            onChange={(e) => handleFilterChange("connection", e.target.value)}
            className="px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 w-full focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
          >
            <option value="">All Connections</option>
            {filterOptions.connections.map((connection) => (
              <option key={connection} value={connection}>
                {connection}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-2">
          <label className="text-sm font-semibold text-gray-800 block">
            Status
          </label>
          <select
            value={filters.status || ""}
            onChange={(e) => handleFilterChange("status", e.target.value)}
            className="px-2.5 py-2.5 border border-gray-300 rounded text-sm bg-white text-gray-800 w-full focus:outline-none focus:border-bfm-blue focus:ring-2 focus:ring-bfm-blue/20"
          >
            <option value="">All</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
            <option value="pending">Pending</option>
          </select>
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
        <table className="w-full border-collapse">
          <thead>
            <tr>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Migration ID
              </th>
              {/* <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Schema
              </th> */}
              {/* <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Name
              </th> */}
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Version
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Backend
              </th>
              <th className="bg-gray-50 p-4 text-left font-semibold text-gray-800 border-b-2 border-gray-200 sticky top-0">
                Connection
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
                <td colSpan={9} className="text-center text-gray-500 py-8">
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
                    <Link
                      to={`/migrations/${migration.migration_id}`}
                      className="text-bfm-blue no-underline hover:underline"
                    >
                      {migration.migration_id}
                    </Link>
                  </td>
                  {/* <td className="p-4 border-b border-gray-200">{migration.schema || '-'}</td>
                    <td className="p-4 border-b border-gray-200">{migration.name || '-'}</td> */}
                  <td className="p-4 border-b border-gray-200">
                    {migration.version}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.backend}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    {migration.connection || "-"}
                  </td>
                  <td className="p-4 border-b border-gray-200">
                    <span
                      className={`inline-block px-3 py-1 rounded-full text-xs font-medium ${
                        migration.status === "success"
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
                    <div className="flex items-center gap-2">
                      <Link
                        to={`/migrations/${migration.migration_id}`}
                        className="inline-block px-3 py-1 bg-bfm-blue text-white rounded text-sm no-underline transition-all duration-200 hover:bg-bfm-blue-dark hover:shadow-md"
                      >
                        View
                      </Link>
                      <div className="relative group">
                        <svg
                          className="w-4 h-4 text-gray-400 cursor-help"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                          xmlns="http://www.w3.org/2000/svg"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                          />
                        </svg>
                        <div className="absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 p-2 bg-gray-900 text-white text-xs rounded-lg shadow-lg opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all duration-200 pointer-events-none z-10 whitespace-normal">
                          Execute from the Migration Details page
                          <div className="absolute left-1/2 -translate-x-1/2 top-full w-0 h-0 border-l-4 border-r-4 border-t-4 border-transparent border-t-gray-900"></div>
                        </div>
                      </div>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination Controls */}
      {migrations.length > 0 && totalPages > 1 && (
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
