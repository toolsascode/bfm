import { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { apiClient } from '../services/api';
import type { MigrationDetailResponse, MigrationStatusResponse } from '../types/api';
import { format } from 'date-fns';
import './MigrationDetail.css';

export default function MigrationDetail() {
  const { id } = useParams<{ id: string }>();
  const [migration, setMigration] = useState<MigrationDetailResponse | null>(null);
  const [status, setStatus] = useState<MigrationStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (id) {
      loadMigration();
      loadStatus();
      const interval = setInterval(() => {
        loadStatus();
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
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load migration');
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

  const handleRollback = async () => {
    if (!id) return;
    if (!confirm('Are you sure you want to rollback this migration?')) {
      return;
    }

    try {
      const result = await apiClient.rollbackMigration(id);
      if (result.success) {
        alert('Migration rolled back successfully');
        loadMigration();
        loadStatus();
      } else {
        alert(`Rollback failed: ${result.message}`);
      }
    } catch (err) {
      alert(`Rollback error: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
  };

  if (loading) {
    return <div className="migration-detail-loading">Loading migration details...</div>;
  }

  if (error || !migration) {
    return (
      <div className="migration-detail-error">
        {error || 'Migration not found'}
        <Link to="/migrations" className="back-link">
          ← Back to Migrations
        </Link>
      </div>
    );
  }

  return (
    <div className="migration-detail">
      <div className="migration-detail-header">
        <Link to="/migrations" className="back-link">
          ← Back to Migrations
        </Link>
        <h1>Migration Details</h1>
      </div>

      <div className="detail-cards">
        <div className="detail-card">
          <h2>Basic Information</h2>
          <div className="detail-grid">
            <div className="detail-item">
              <label>Migration ID</label>
              <div className="detail-value">{migration.migration_id}</div>
            </div>
            <div className="detail-item">
              <label>Name</label>
              <div className="detail-value">{migration.name}</div>
            </div>
            <div className="detail-item">
              <label>Schema</label>
              <div className="detail-value">{migration.schema}</div>
            </div>
            <div className="detail-item">
              <label>Table</label>
              <div className="detail-value">{migration.table}</div>
            </div>
            <div className="detail-item">
              <label>Version</label>
              <div className="detail-value">{migration.version}</div>
            </div>
            <div className="detail-item">
              <label>Backend</label>
              <div className="detail-value">{migration.backend}</div>
            </div>
            <div className="detail-item">
              <label>Connection</label>
              <div className="detail-value">{migration.connection}</div>
            </div>
            <div className="detail-item">
              <label>Applied</label>
              <div className={`detail-value ${migration.applied ? 'applied-yes' : 'applied-no'}`}>
                {migration.applied ? 'Yes' : 'No'}
              </div>
            </div>
          </div>
        </div>

        {status && (
          <div className="detail-card">
            <h2>Status Information</h2>
            <div className="detail-grid">
              <div className="detail-item">
                <label>Status</label>
                <div className="detail-value">
                  <span className={`status-badge status-${status.status || 'pending'}`}>
                    {status.status || 'pending'}
                  </span>
                </div>
              </div>
              {status.applied_at && (
                <div className="detail-item">
                  <label>Applied At</label>
                  <div className="detail-value">
                    {format(new Date(status.applied_at), 'yyyy-MM-dd HH:mm:ss')}
                  </div>
                </div>
              )}
              {status.error_message && (
                <div className="detail-item full-width">
                  <label>Error Message</label>
                  <div className="detail-value error-message">{status.error_message}</div>
                </div>
              )}
            </div>
          </div>
        )}

        {migration.applied && (
          <div className="detail-card">
            <h2>Actions</h2>
            <button onClick={handleRollback} className="rollback-button">
              Rollback Migration
            </button>
            <p className="action-warning">
              Warning: Rolling back will execute the down migration script.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

