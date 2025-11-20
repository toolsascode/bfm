import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { apiClient } from '../services/api';
import type { MigrationListItem, MigrationListFilters } from '../types/api';
import { format } from 'date-fns';
import './MigrationList.css';

export default function MigrationList() {
  const [migrations, setMigrations] = useState<MigrationListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState<MigrationListFilters>({});
  const [total, setTotal] = useState(0);

  useEffect(() => {
    loadMigrations();
  }, [filters]);

  const loadMigrations = async () => {
    try {
      setLoading(true);
      const response = await apiClient.listMigrations(filters);
      setMigrations(response.items);
      setTotal(response.total);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load migrations');
    } finally {
      setLoading(false);
    }
  };

  const handleFilterChange = (key: keyof MigrationListFilters, value: string) => {
    setFilters((prev) => ({
      ...prev,
      [key]: value || undefined,
    }));
  };

  const clearFilters = () => {
    setFilters({});
  };

  if (loading && migrations.length === 0) {
    return <div className="migration-list-loading">Loading migrations...</div>;
  }

  return (
    <div className="migration-list">
      <div className="migration-list-header">
        <h1>Migrations</h1>
        <div className="migration-count">Total: {total}</div>
      </div>

      <div className="filters">
        <div className="filter-group">
          <label>Backend</label>
          <input
            type="text"
            value={filters.backend || ''}
            onChange={(e) => handleFilterChange('backend', e.target.value)}
            placeholder="Filter by backend"
          />
        </div>
        <div className="filter-group">
          <label>Schema</label>
          <input
            type="text"
            value={filters.schema || ''}
            onChange={(e) => handleFilterChange('schema', e.target.value)}
            placeholder="Filter by schema"
          />
        </div>
        <div className="filter-group">
          <label>Connection</label>
          <input
            type="text"
            value={filters.connection || ''}
            onChange={(e) => handleFilterChange('connection', e.target.value)}
            placeholder="Filter by connection"
          />
        </div>
        <div className="filter-group">
          <label>Status</label>
          <select
            value={filters.status || ''}
            onChange={(e) => handleFilterChange('status', e.target.value)}
          >
            <option value="">All</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
            <option value="pending">Pending</option>
          </select>
        </div>
        <button onClick={clearFilters} className="clear-filters">
          Clear Filters
        </button>
      </div>

      {error && <div className="error-message">Error: {error}</div>}

      <div className="migrations-table-container">
        <table className="migrations-table">
          <thead>
            <tr>
              <th>Migration ID</th>
              <th>Schema</th>
              <th>Table</th>
              <th>Version</th>
              <th>Backend</th>
              <th>Connection</th>
              <th>Status</th>
              <th>Applied</th>
              <th>Applied At</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {migrations.length === 0 ? (
              <tr>
                <td colSpan={10} className="no-data">
                  No migrations found
                </td>
              </tr>
            ) : (
              migrations.map((migration) => (
                <tr key={migration.migration_id}>
                  <td>
                    <Link to={`/migrations/${migration.migration_id}`}>
                      {migration.migration_id}
                    </Link>
                  </td>
                  <td>{migration.schema}</td>
                  <td>{migration.table}</td>
                  <td>{migration.version}</td>
                  <td>{migration.backend}</td>
                  <td>{migration.connection}</td>
                  <td>
                    <span className={`status-badge status-${migration.status}`}>
                      {migration.status || 'pending'}
                    </span>
                  </td>
                  <td>
                    <span className={migration.applied ? 'applied-yes' : 'applied-no'}>
                      {migration.applied ? 'Yes' : 'No'}
                    </span>
                  </td>
                  <td>
                    {migration.applied_at
                      ? format(new Date(migration.applied_at), 'yyyy-MM-dd HH:mm:ss')
                      : '-'}
                  </td>
                  <td>
                    <Link
                      to={`/migrations/${migration.migration_id}`}
                      className="view-button"
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
    </div>
  );
}

