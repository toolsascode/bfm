import { useState, FormEvent } from 'react';
import { apiClient } from '../services/api';
import type { MigrateRequest, MigrateResponse } from '../types/api';
import './MigrationExecute.css';

export default function MigrationExecute() {
  const [request, setRequest] = useState<MigrateRequest>({
    connection: '',
    schema: '',
    target: {
      backend: '',
      schema: '',
      connection: '',
    },
  });
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<MigrateResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [progress, setProgress] = useState<{
    status: 'idle' | 'running' | 'completed';
    message: string;
  }>({ status: 'idle', message: '' });

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setResult(null);
    setProgress({ status: 'running', message: 'Executing migration...' });

    try {
      const response = await apiClient.migrate(request);
      setResult(response);
      setProgress({
        status: 'completed',
        message: response.success
          ? 'Migration completed successfully'
          : 'Migration completed with errors',
      });

      if (response.queued) {
        setProgress({
          status: 'running',
          message: `Migration queued with job ID: ${response.job_id}. Check status in migrations list.`,
        });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to execute migration');
      setProgress({ status: 'idle', message: '' });
    } finally {
      setLoading(false);
    }
  };

  const updateField = (field: keyof MigrateRequest, value: any) => {
    setRequest((prev) => ({ ...prev, [field]: value }));
  };

  const updateTarget = (field: keyof MigrateRequest['target'], value: any) => {
    setRequest((prev) => ({
      ...prev,
      target: { ...prev.target, [field]: value },
    }));
  };

  return (
    <div className="migration-execute">
      <h1>Execute Migration</h1>

      <form onSubmit={handleSubmit} className="execute-form">
        <div className="form-section">
          <h2>Connection Configuration</h2>
          <div className="form-group">
            <label htmlFor="connection">Connection *</label>
            <input
              id="connection"
              type="text"
              value={request.connection}
              onChange={(e) => updateField('connection', e.target.value)}
              required
              placeholder="e.g., core, guard, organization"
            />
          </div>
          <div className="form-group">
            <label htmlFor="schema">Schema</label>
            <input
              id="schema"
              type="text"
              value={request.schema || ''}
              onChange={(e) => updateField('schema', e.target.value)}
              placeholder="e.g., core, guard, cli_environment_id"
            />
          </div>
          <div className="form-group">
            <label htmlFor="environment">Environment</label>
            <input
              id="environment"
              type="text"
              value={request.environment || ''}
              onChange={(e) => updateField('environment', e.target.value)}
              placeholder="Environment ID for dynamic schemas"
            />
          </div>
        </div>

        <div className="form-section">
          <h2>Migration Target</h2>
          <div className="form-group">
            <label htmlFor="target-backend">Backend</label>
            <select
              id="target-backend"
              value={request.target?.backend || ''}
              onChange={(e) => updateTarget('backend', e.target.value)}
            >
              <option value="">All backends</option>
              <option value="postgresql">PostgreSQL</option>
              <option value="greptimedb">GreptimeDB</option>
              <option value="etcd">Etcd</option>
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="target-schema">Target Schema</label>
            <input
              id="target-schema"
              type="text"
              value={request.target?.schema || ''}
              onChange={(e) => updateTarget('schema', e.target.value)}
              placeholder="Filter by schema"
            />
          </div>
          <div className="form-group">
            <label htmlFor="target-connection">Target Connection</label>
            <input
              id="target-connection"
              type="text"
              value={request.target?.connection || ''}
              onChange={(e) => updateTarget('connection', e.target.value)}
              placeholder="Filter by connection"
            />
          </div>
          <div className="form-group">
            <label htmlFor="target-version">Version</label>
            <input
              id="target-version"
              type="text"
              value={request.target?.version || ''}
              onChange={(e) => updateTarget('version', e.target.value)}
              placeholder="Specific version (optional)"
            />
          </div>
        </div>

        <div className="form-section">
          <div className="form-group checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={request.dry_run || false}
                onChange={(e) => updateField('dry_run', e.target.checked)}
              />
              Dry Run (test without applying changes)
            </label>
          </div>
        </div>

        {error && <div className="error-message">Error: {error}</div>}

        {progress.status !== 'idle' && (
          <div className={`progress-message progress-${progress.status}`}>
            {progress.message}
          </div>
        )}

        <button type="submit" disabled={loading} className="execute-button">
          {loading ? 'Executing...' : 'Execute Migration'}
        </button>
      </form>

      {result && (
        <div className="result-card">
          <h2>Execution Result</h2>
          <div className={`result-status ${result.success ? 'success' : 'error'}`}>
            {result.success ? '✓ Success' : '✗ Failed'}
          </div>

          {result.queued && (
            <div className="result-info">
              <strong>Job ID:</strong> {result.job_id}
              <br />
              <em>Migration has been queued for async execution.</em>
            </div>
          )}

          {result.applied.length > 0 && (
            <div className="result-section">
              <h3>Applied Migrations ({result.applied.length})</h3>
              <ul>
                {result.applied.map((id) => (
                  <li key={id}>{id}</li>
                ))}
              </ul>
            </div>
          )}

          {result.skipped.length > 0 && (
            <div className="result-section">
              <h3>Skipped Migrations ({result.skipped.length})</h3>
              <ul>
                {result.skipped.map((id) => (
                  <li key={id}>{id}</li>
                ))}
              </ul>
            </div>
          )}

          {result.errors.length > 0 && (
            <div className="result-section error">
              <h3>Errors ({result.errors.length})</h3>
              <ul>
                {result.errors.map((err, idx) => (
                  <li key={idx}>{err}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

