import axios, { AxiosInstance } from 'axios';
import type {
  MigrateRequest,
  MigrateResponse,
  MigrationListResponse,
  MigrationDetailResponse,
  MigrationStatusResponse,
  RollbackResponse,
  HealthResponse,
  MigrationListFilters,
} from '../types/api';

class BFMApiClient {
  private client: AxiosInstance;
  private apiToken: string | null = null;

  constructor(baseURL?: string) {
    // Get runtime config from window (injected at runtime) or fallback to build-time env
    // Wait a bit for the runtime config script to load if needed
    const getRuntimeConfig = () => {
      if (typeof window !== 'undefined') {
        return (window as any).__RUNTIME_CONFIG__ || {};
      }
      return {};
    };
    
    const runtimeConfig = getRuntimeConfig();
    const apiURL = baseURL || runtimeConfig.VITE_BFM_API_URL || import.meta.env.VITE_BFM_API_URL || '/api';
    
    this.client = axios.create({
      baseURL: apiURL,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Load API token from runtime config, environment, or localStorage
    const runtimeToken = runtimeConfig.VITE_BFM_API_TOKEN;
    const envToken = import.meta.env.VITE_BFM_API_TOKEN;
    const storedToken = localStorage.getItem('bfm_api_token');
    const token = runtimeToken || envToken || storedToken;
    
    if (token) {
      this.setToken(token);
    } else {
      console.warn('BFM API token not found. Please set VITE_BFM_API_TOKEN environment variable.');
    }

    // Add response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      (error) => {
        if (error.response?.status === 401) {
          // Clear token on unauthorized
          this.clearToken();
        }
        return Promise.reject(error);
      }
    );
  }

  setToken(token: string) {
    this.apiToken = token;
    this.client.defaults.headers.common['Authorization'] = `Bearer ${token}`;
    localStorage.setItem('bfm_api_token', token);
  }

  clearToken() {
    this.apiToken = null;
    delete this.client.defaults.headers.common['Authorization'];
    localStorage.removeItem('bfm_api_token');
  }

  getToken(): string | null {
    return this.apiToken;
  }

  async listMigrations(filters?: MigrationListFilters): Promise<MigrationListResponse> {
    const response = await this.client.get<MigrationListResponse>('/v1/migrations', {
      params: filters,
    });
    return response.data;
  }

  async getMigration(migrationId: string): Promise<MigrationDetailResponse> {
    const response = await this.client.get<MigrationDetailResponse>(
      `/v1/migrations/${migrationId}`
    );
    return response.data;
  }

  async getMigrationStatus(migrationId: string): Promise<MigrationStatusResponse> {
    const response = await this.client.get<MigrationStatusResponse>(
      `/v1/migrations/${migrationId}/status`
    );
    return response.data;
  }

  async migrate(request: MigrateRequest): Promise<MigrateResponse> {
    const response = await this.client.post<MigrateResponse>('/v1/migrate', request);
    return response.data;
  }

  async rollbackMigration(migrationId: string): Promise<RollbackResponse> {
    const response = await this.client.post<RollbackResponse>(
      `/v1/migrations/${migrationId}/rollback`
    );
    return response.data;
  }

  async healthCheck(): Promise<HealthResponse> {
    const response = await this.client.get<HealthResponse>('/v1/health');
    return response.data;
  }
}

export const apiClient = new BFMApiClient();

