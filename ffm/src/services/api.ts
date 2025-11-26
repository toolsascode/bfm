import axios, { AxiosInstance, AxiosError } from "axios";
import type {
  MigrateRequest,
  MigrateUpRequest,
  MigrateDownRequest,
  MigrateResponse,
  MigrationListResponse,
  MigrationDetailResponse,
  MigrationStatusResponse,
  MigrationHistoryResponse,
  RollbackResponse,
  HealthResponse,
  MigrationListFilters,
  ReindexResponse,
} from "../types/api";
import { toastService } from "./toast";

class BFMApiClient {
  private client: AxiosInstance;
  private apiToken: string | null = null;

  constructor(baseURL?: string) {
    // Get runtime config from window (injected at runtime) or fallback to build-time env
    // Wait a bit for the runtime config script to load if needed
    const getRuntimeConfig = () => {
      if (typeof window !== "undefined") {
        return (window as any).__RUNTIME_CONFIG__ || {};
      }
      return {};
    };

    const runtimeConfig = getRuntimeConfig();
    // Support both BFM_* (production via runtime config) and VITE_* (dev via Vite)
    const apiURL =
      baseURL ||
      runtimeConfig.BFM_API_URL ||
      import.meta.env.VITE_BFM_API_URL ||
      "/api";

    this.client = axios.create({
      baseURL: apiURL,
      headers: {
        "Content-Type": "application/json",
        "X-Client-Type": "frontend", // Identify requests from FfM frontend
        "X-Requested-With": "FfM", // Additional identifier for manual executions
      },
    });

    // Load API token from runtime config, Vite env (dev), or localStorage
    const runtimeToken = runtimeConfig.BFM_API_TOKEN;
    const envToken = import.meta.env.VITE_BFM_API_TOKEN;
    const storedToken = localStorage.getItem("bfm_api_token");
    const token = runtimeToken || envToken || storedToken;

    if (token) {
      this.setToken(token);
    } else {
      console.warn(
        "BFM API token not found. Please set BFM_API_TOKEN (production) or VITE_BFM_API_TOKEN (dev) environment variable.",
      );
    }

    // Add response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      (error: AxiosError) => {
        // First, try to extract error message from response data
        let errorMessage: string | null = null;

        if (error.response?.data) {
          const data = error.response.data as any;

          // Log for debugging - show full error details
          if (process.env.NODE_ENV === "development") {
            console.group("🔴 API Error Response");
            console.error("Status:", error.response.status);
            console.error("Data Type:", typeof data);
            console.error("Data:", data);
            if (typeof data === "object" && data !== null) {
              console.error("Data Keys:", Object.keys(data));
              console.error("Stringified Data:", JSON.stringify(data, null, 2));
            }
            console.groupEnd();
          }

          if (typeof data === "string") {
            errorMessage = data;
          } else if (data?.error) {
            errorMessage =
              typeof data.error === "string"
                ? data.error
                : data.error.message || JSON.stringify(data.error);
          } else if (data?.message) {
            errorMessage = data.message;
          } else if (data?.error?.message) {
            errorMessage = data.error.message;
          } else if (typeof data === "object") {
            // Try to find any string value that might be an error message
            const values = Object.values(data);
            const stringValue = values.find(
              (v) => typeof v === "string" && v.length > 0,
            ) as string | undefined;
            if (stringValue) {
              errorMessage = stringValue;
            } else {
              // Last resort: stringify the object
              errorMessage = JSON.stringify(data);
            }
          }

          // Log extracted error message for debugging
          if (process.env.NODE_ENV === "development") {
            console.log("Extracted Error Message:", errorMessage);
          }
        }

        // Handle specific status codes
        if (error.response?.status === 401) {
          // Clear token on unauthorized
          this.clearToken();
          toastService.error(
            errorMessage || "Authentication failed. Please login again.",
          );
        } else if (error.response?.status === 403) {
          toastService.error(
            errorMessage ||
              "You do not have permission to perform this action.",
          );
        } else if (error.response?.status === 404) {
          toastService.warning(errorMessage || "Resource not found.");
        } else if (error.response?.status >= 500) {
          // For 500 errors, show the actual error message if available
          const messageToShow =
            errorMessage || "Server error. Please try again later.";
          if (process.env.NODE_ENV === "development") {
            console.log("Showing error toast:", messageToShow);
          }
          toastService.error(messageToShow);
        } else if (
          error.code === "ERR_NETWORK" ||
          error.message === "Network Error"
        ) {
          toastService.error("Network error. Please check your connection.");
        } else if (errorMessage) {
          // If we have an error message but no specific status handling, show it
          toastService.error(errorMessage);
        } else {
          // Fallback
          toastService.error("An error occurred. Please try again.");
        }

        return Promise.reject(error);
      },
    );
  }

  setToken(token: string) {
    this.apiToken = token;
    this.client.defaults.headers.common["Authorization"] = `Bearer ${token}`;
    localStorage.setItem("bfm_api_token", token);
  }

  clearToken() {
    this.apiToken = null;
    delete this.client.defaults.headers.common["Authorization"];
    localStorage.removeItem("bfm_api_token");
  }

  getToken(): string | null {
    return this.apiToken;
  }

  async listMigrations(
    filters?: MigrationListFilters,
  ): Promise<MigrationListResponse> {
    const response = await this.client.get<MigrationListResponse>(
      "/v1/migrations",
      {
        params: filters,
      },
    );
    return response.data;
  }

  async getMigration(migrationId: string): Promise<MigrationDetailResponse> {
    const response = await this.client.get<MigrationDetailResponse>(
      `/v1/migrations/${migrationId}`,
    );
    return response.data;
  }

  async getMigrationStatus(
    migrationId: string,
  ): Promise<MigrationStatusResponse> {
    const response = await this.client.get<MigrationStatusResponse>(
      `/v1/migrations/${migrationId}/status`,
    );
    return response.data;
  }

  async getMigrationHistory(
    migrationId: string,
  ): Promise<MigrationHistoryResponse> {
    const response = await this.client.get<MigrationHistoryResponse>(
      `/v1/migrations/${migrationId}/history`,
    );
    return response.data;
  }

  async migrate(request: MigrateRequest): Promise<MigrateResponse> {
    const response = await this.client.post<MigrateResponse>(
      "/v1/migrate",
      request,
    );
    return response.data;
  }

  async migrateUp(request: MigrateUpRequest): Promise<MigrateResponse> {
    const response = await this.client.post<MigrateResponse>(
      "/v1/migrations/up",
      request,
    );
    return response.data;
  }

  async migrateDown(request: MigrateDownRequest): Promise<MigrateResponse> {
    const response = await this.client.post<MigrateResponse>(
      "/v1/migrations/down",
      request,
    );
    return response.data;
  }

  async rollbackMigration(migrationId: string): Promise<RollbackResponse> {
    const response = await this.client.post<RollbackResponse>(
      `/v1/migrations/${migrationId}/rollback`,
    );
    return response.data;
  }

  async healthCheck(): Promise<HealthResponse> {
    const response = await this.client.get<HealthResponse>("/v1/health");
    return response.data;
  }

  async reindexMigrations(): Promise<ReindexResponse> {
    const response = await this.client.post<ReindexResponse>(
      "/v1/migrations/reindex",
    );
    return response.data;
  }
}

export const apiClient = new BFMApiClient();
