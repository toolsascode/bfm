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
  schema_name?: string;
  dry_run?: boolean;
}

export interface MigrateUpRequest {
  target?: MigrationTarget;
  connection: string;
  schemas?: string[]; // Array for dynamic schemas
  dry_run?: boolean;
}

export interface MigrateDownRequest {
  migration_id: string;
  schemas?: string[]; // Array for dynamic schemas
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

export interface Dependency {
  connection: string;
  schema: string;
  target: string;
  target_type: string;
  requires_table?: string;
  requires_schema?: string;
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
  dependencies?: string[];
  structured_dependencies?: Dependency[];
}

export interface MigrationStatusResponse {
  migration_id: string;
  applied: boolean;
  status?: string;
  applied_at?: string;
  error_message?: string;
}

export interface MigrationHistoryItem {
  migration_id: string;
  schema: string;
  table: string;
  version: string;
  connection: string;
  backend: string;
  applied_at: string;
  status: string;
  error_message?: string;
  executed_by?: string;
  execution_method?: string;
  execution_context?: string;
}

export interface MigrationHistoryResponse {
  migration_id: string;
  history: MigrationHistoryItem[];
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

export interface ReindexResponse {
  added: string[];
  removed: string[];
  total: number;
}

export interface MigrationListFilters {
  schema?: string;
  table?: string;
  connection?: string;
  backend?: string;
  status?: string;
  version?: string;
}
