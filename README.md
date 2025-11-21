<p align="center">
<img src="./assets/BfM.svg" alt="drawing" width="150" />
</p>
<p align="center">
<span style="font-size: 25px; font-weight: bold; text-align: center; font-family: 'Montserrat', system-ui, sans-serif;" >
Backend For Migrations (BfM)
</span>
</p>

BfM is a comprehensive database migration system that supports multiple backends (PostgreSQL, GreptimeDB, Etcd) with HTTP and Protobuf APIs.

## Features

- Multi-backend support: PostgreSQL, GreptimeDB, Etcd
- HTTP REST API with authentication
- Protobuf/gRPC API (requires code generation)
- Migration state tracking in PostgreSQL/MySQL
- Support for fixed and dynamic schemas
- Embedded SQL scripts in Go files
- Dry-run mode for testing
- Idempotent migrations

## Configuration

### Environment Variables

#### Server Configuration

- `BFM_HTTP_PORT` - HTTP server port (default: 7070)
- `BFM_GRPC_PORT` - gRPC server port (default: 9090)
- `BFM_API_TOKEN` - API token for authentication (required)

#### State Database Configuration

- `BFM_STATE_BACKEND` - State database type: "postgresql" or "mysql" (default: "postgresql")
- `BFM_STATE_DB_HOST` - State database host (default: "localhost")
- `BFM_STATE_DB_PORT` - State database port (default: "5432")
- `BFM_STATE_DB_USERNAME` - State database username (default: "postgres")
- `BFM_STATE_DB_PASSWORD` - State database password (required)
- `BFM_STATE_DB_NAME` - State database name (default: "migration_state")
- `BFM_STATE_SCHEMA` - State database schema (default: "public")

#### Connection Configuration

For each connection (e.g., "core", "guard", "logs"), set:

- `{CONNECTION}_BACKEND` - Backend type: "postgresql", "greptimedb", or "etcd"
- `{CONNECTION}_DB_HOST` - Database host
- `{CONNECTION}_DB_PORT` - Database port
- `{CONNECTION}_DB_USERNAME` - Database username
- `{CONNECTION}_DB_PASSWORD` - Database password
- `{CONNECTION}_DB_NAME` - Database name
- `{CONNECTION}_SCHEMA` - Schema name (optional, for fixed schemas)

Example:

```bash
CORE_BACKEND=postgresql
CORE_DB_HOST=localhost
CORE_DB_PORT=5432
CORE_DB_USERNAME=dashcloud
CORE_DB_PASSWORD=password
CORE_DB_NAME=dashcloud
CORE_SCHEMA=core
```

## Development

### Hot-Reload Setup (Development Only)

Both `bfm` (backend) and `ffm` (frontend) support hot-reload for faster development.

**Note:** Hot-reload tools are only used during local development. Production builds use standard build processes and do not include hot-reload functionality.

#### Backend (bfm) - Using Air

Install Air (if not already installed):
```bash
go install github.com/cosmtrek/air@latest
```

Or using Homebrew (macOS):
```bash
brew install air
```

Start the backend with hot-reload:
```bash
cd bfm
air
```

The `.air.toml` configuration file is already set up in the `bfm` directory. Air will automatically rebuild and restart the server when you make changes to Go files.

#### Frontend (ffm) - Using Vite

Vite has built-in Hot Module Replacement (HMR). Start the development server:

```bash
cd ffm
npm run dev
```

### Docker Development with Hot-Reload

For development in Docker with hot-reload support:

```bash
# Start all services with hot-reload in Docker
make dev-docker

# View logs
make dev-docker-logs

# Stop services
make dev-docker-down
```

This uses `docker-compose.dev.yml` which:
- Mounts source code as volumes for live updates
- Runs Air in the BFM container for Go hot-reload
- Runs Vite dev server in the FFM container for frontend hot-reload
- Automatically rebuilds and restarts on code changes

See `docs/DEVELOPMENT.md` for detailed development setup instructions.

The frontend will automatically reload when you make changes to React components, CSS, or TypeScript files.

## Usage

### Starting the Server (without hot-reload)

```bash
cd bfm/cmd/server
go run main.go
```

### Starting the Server (with hot-reload - recommended for development)

```bash
cd bfm
air
```

### HTTP API

#### Migrate Endpoint

```bash
POST /api/v1/migrate
Authorization: Bearer {BFM_API_TOKEN}
Content-Type: application/json

{
  "target": {
    "backend": "postgresql",
    "schema": "core",
    "tables": [],
    "version": "",
    "connection": "core"
  },
  "connection": "core",
  "schema": "core",
  "environment": "",
  "dry_run": false
}
```

Response:

```json
{
  "success": true,
  "applied": ["core_users_20250101120000_create_users"],
  "skipped": [],
  "errors": []
}
```

### Health Check

```bash
GET /health
```

## Migration Scripts

Migration scripts are located in `/bfm/sfm` and follow the naming convention:
`{schema}_{table}_{version}_{name}.go`

Each migration file:

1. Embeds SQL files using `//go:embed`
2. Registers itself in the global registry via `init()`
3. Includes both up and down migrations

Example structure:

```
sfm/postgresql/core/core_users_20250101120000_create_users.go
sfm/postgresql/core/core_users_20250101120000_create_users.sql
sfm/postgresql/core/core_users_20250101120000_create_users_down.sql
```

## Migration from Existing System

To migrate from the existing GORM AutoMigrate system:

1. Extract table definitions from GORM models
2. Create SQL migration scripts following the naming convention
3. Place scripts in appropriate `sfm/{backend}/{connection}/` directory
4. Register migrations via `init()` functions
5. Run migrations via HTTP API or Protobuf API

See `MIGRATION_GUIDE.md` for detailed instructions.
