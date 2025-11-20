// API Types matching BFM server DTOs

export interface MigrationTarget {
  backend?: string;
  schema?: string;
  tables?: string[];
  version?: string;
  connection?: string;
}

export interface MigrateRequest {
  target?: MigrationTarget;
  connection: string;
  schema?: string;
  environment?: string;
  dry_run?: boolean;
}

export interface MigrateResponse {
  success: boolean;
  applied: string[];
  skipped: string[];
  errors: string[];
  queued?: boolean;
  job_id?: string;
}

export interface MigrationListItem {
  migration_id: string;
  schema: string;
  table: string;
  version: string;
  name: string;
  connection: string;
  backend: string;
  applied: boolean;
  status: string;
  applied_at?: string;
  error_message?: string;
}

export interface MigrationListResponse {
  items: MigrationListItem[];
  total: number;
}

export interface MigrationDetailResponse {
  migration_id: string;
  schema: string;
  table: string;
  version: string;
  name: string;
  connection: string;
  backend: string;
  applied: boolean;
  up_sql?: string;
  down_sql?: string;
}

export interface MigrationStatusResponse {
  migration_id: string;
  applied: boolean;
  status?: string;
  applied_at?: string;
  error_message?: string;
}

export interface RollbackResponse {
  success: boolean;
  message: string;
  errors?: string[];
}

export interface HealthResponse {
  status: string;
  checks: Record<string, string>;
}

export interface MigrationListFilters {
  schema?: string;
  table?: string;
  connection?: string;
  backend?: string;
  status?: string;
  version?: string;
}

