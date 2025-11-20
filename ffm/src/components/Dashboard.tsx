import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { apiClient } from '../services/api';
import type { MigrationListItem } from '../types/api';
import { format } from 'date-fns';
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
} from 'recharts';
import './Dashboard.css';

export default function Dashboard() {
  const [migrations, setMigrations] = useState<MigrationListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [healthStatus, setHealthStatus] = useState<string>('unknown');

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 30000); // Refresh every 30 seconds
    return () => clearInterval(interval);
  }, []);

  const loadData = async () => {
    try {
      setLoading(true);
      const [migrationsData, health] = await Promise.all([
        apiClient.listMigrations(),
        apiClient.healthCheck().catch(() => ({ status: 'unknown', checks: {} })),
      ]);
      setMigrations(migrationsData.items);
      setHealthStatus(health.status);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  if (loading && migrations.length === 0) {
    return <div className="dashboard-loading">Loading dashboard...</div>;
  }

  if (error) {
    return <div className="dashboard-error">Error: {error}</div>;
  }

  // Calculate statistics
  const total = migrations.length;
  const applied = migrations.filter((m) => m.applied).length;
  const pending = total - applied;
  const failed = migrations.filter((m) => m.status === 'failed').length;
  const success = migrations.filter((m) => m.status === 'success').length;

  // Group by backend
  const byBackend = migrations.reduce((acc, m) => {
    acc[m.backend] = (acc[m.backend] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);

  // Group by connection
  const byConnection = migrations.reduce((acc, m) => {
    acc[m.connection] = (acc[m.connection] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);

  // Status distribution
  const statusData = [
    { name: 'Applied', value: applied, color: '#27ae60' },
    { name: 'Pending', value: pending, color: '#f39c12' },
    { name: 'Failed', value: failed, color: '#e74c3c' },
  ];

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

  // Recent migrations
  const recentMigrations = migrations
    .filter((m) => m.applied_at)
    .sort((a, b) => {
      const dateA = new Date(a.applied_at || 0).getTime();
      const dateB = new Date(b.applied_at || 0).getTime();
      return dateB - dateA;
    })
    .slice(0, 5);

  const COLORS = ['#3498db', '#9b59b6', '#e74c3c', '#f39c12', '#1abc9c'];

  return (
    <div className="dashboard">
      <div className="dashboard-header">
        <h1>Migration Dashboard</h1>
        <div className={`health-status ${healthStatus}`}>
          Status: {healthStatus === 'healthy' ? '✓ Healthy' : '✗ Unhealthy'}
        </div>
      </div>

      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-value">{total}</div>
          <div className="stat-label">Total Migrations</div>
        </div>
        <div className="stat-card success">
          <div className="stat-value">{applied}</div>
          <div className="stat-label">Applied</div>
        </div>
        <div className="stat-card warning">
          <div className="stat-value">{pending}</div>
          <div className="stat-label">Pending</div>
        </div>
        <div className="stat-card error">
          <div className="stat-value">{failed}</div>
          <div className="stat-label">Failed</div>
        </div>
      </div>

      <div className="charts-grid">
        <div className="chart-card">
          <h3>Status Distribution</h3>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie
                data={statusData}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
                outerRadius={80}
                fill="#8884d8"
                dataKey="value"
              >
                {statusData.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-card">
          <h3>Migrations by Backend</h3>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={backendData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis />
              <Tooltip />
              <Bar dataKey="value" fill="#3498db" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="chart-card">
          <h3>Migrations by Connection</h3>
          <ResponsiveContainer width="100%" height={300}>
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

      <div className="recent-migrations">
        <h2>Recent Migrations</h2>
        <table className="migrations-table">
          <thead>
            <tr>
              <th>Migration ID</th>
              <th>Schema</th>
              <th>Table</th>
              <th>Backend</th>
              <th>Status</th>
              <th>Applied At</th>
            </tr>
          </thead>
          <tbody>
            {recentMigrations.length === 0 ? (
              <tr>
                <td colSpan={6} className="no-data">
                  No migrations applied yet
                </td>
              </tr>
            ) : (
              recentMigrations.map((migration) => (
                <tr key={migration.migration_id}>
                  <td>
                    <Link to={`/migrations/${migration.migration_id}`}>
                      {migration.migration_id}
                    </Link>
                  </td>
                  <td>{migration.schema}</td>
                  <td>{migration.table}</td>
                  <td>{migration.backend}</td>
                  <td>
                    <span className={`status-badge status-${migration.status}`}>
                      {migration.status}
                    </span>
                  </td>
                  <td>
                    {migration.applied_at
                      ? format(new Date(migration.applied_at), 'yyyy-MM-dd HH:mm:ss')
                      : '-'}
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

