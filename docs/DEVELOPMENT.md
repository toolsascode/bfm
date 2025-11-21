# Development Guide

This guide explains how to set up and run BfM in a development environment, including hot-reload for both the backend (bfm) and frontend (ffm) projects.

**⚠️ IMPORTANT: Hot-reload tools are for DEVELOPMENT ONLY. Production builds use standard build processes and do NOT include hot-reload functionality.**

## Prerequisites

- **Go 1.24+** - For backend development
- **Node.js 20+** - For frontend development
- **Docker & Docker Compose** - For containerized development (optional)
- **Air** - For Go hot-reload (install instructions below)
- **Make** - For using Makefile commands (optional but recommended)

## Local Development Setup

### Starting the Server (Development Mode)

#### Option 1: Using Air (Recommended - with hot-reload)

Air is a live reload utility for Go applications. It automatically rebuilds and restarts your Go application when you make changes.

**Installation:**

Install Air using one of these methods:

```bash
# Using Go
go install github.com/cosmtrek/air@latest

# Using Homebrew (macOS)
brew install air

# Using curl (Linux/macOS)
curl -sSfL https://raw.githubusercontent.com/cosmtrek/air/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

**Usage:**

Navigate to the api directory and run:
```bash
cd api
air
```

Air will:
- Watch for changes in `.go` files
- Automatically rebuild the application
- Restart the server when changes are detected
- Show colored output for build status

The configuration is already set up in `api/.air.toml`. It will:
- Build from `./cmd/server`
- Output to `./tmp/main`
- Exclude test files, vendor, and deploy directories
- Watch for changes in `.go`, `.tpl`, `.tmpl`, and `.html` files

#### Option 2: Using go run (without hot-reload)

For a simple development server without hot-reload:

```bash
cd api/cmd/server
go run main.go
```

### Frontend Development (ffm) - Using Vite HMR

Vite has built-in Hot Module Replacement (HMR) that works out of the box.

**Usage:**

Navigate to the ffm directory and run:
```bash
cd ffm
npm install  # First time only
npm run dev
```

Vite will:
- Start the development server on `http://localhost:4040`
- Enable HMR for React components
- Automatically reload CSS changes
- Preserve component state during updates
- Show build errors in the browser

**Configuration:**

The Vite configuration (`ffm/vite.config.ts`) is optimized for:
- WebSocket-based HMR
- File system polling (useful for Docker/WSL)
- API proxying to the backend
- Source maps for debugging

## Running Both Services

### Option 1: Local Development (No Docker)

Start both services locally with hot-reload:

**Using Makefile (recommended):**
```bash
# Terminal 1 - Backend
make dev-bfm

# Terminal 2 - Frontend
make dev-ffm
```

Or use the combined command:
```bash
make dev-local
```

**Manual setup:**

**Terminal 1 - Backend:**
```bash
cd api
air
```

**Terminal 2 - Frontend:**
```bash
cd ffm
npm run dev
```

The frontend will automatically reload when you make changes to React components, CSS, or TypeScript files.

### Option 2: Docker with Hot-Reload (Recommended for Docker)

Start both services in Docker with hot-reload:
```bash
# Build and start all services with hot-reload
make dev-docker

# View logs
make dev-docker-logs

# Stop services
make dev-docker-down
```

This uses `deploy/docker-compose.dev.yml` which:
- Mounts source code as volumes for live updates
- Runs Air in the BFM container for Go hot-reload
- Runs Vite dev server in the FFM container for frontend hot-reload
- Automatically rebuilds and restarts on code changes

### Option 3: Docker Compose (Production-like, No Hot-Reload)

For production-like environment without hot-reload:
```bash
make dev
```

This uses the standard `deploy/docker-compose.yml` without hot-reload.

## Troubleshooting

### Backend (Air) Issues

**Air not found:**
```bash
# Make sure Go is installed and GOPATH/bin is in your PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

**Build errors:**
- Check `bfm/build-errors.log` for detailed error messages
- Ensure all dependencies are installed: `go mod download`
- Verify Go version: `go version` (requires Go 1.24+)

**Port already in use:**
- Change the port in your environment variables or config
- Kill the existing process: `lsof -ti:7070 | xargs kill`

### Frontend (Vite) Issues

**Port already in use:**
- Vite will automatically try the next available port
- Or specify a different port: `npm run dev -- --port 3001`

**HMR not working:**
- Check browser console for WebSocket errors
- Ensure firewall allows WebSocket connections
- Try disabling polling in `vite.config.ts` if not using Docker/WSL

**API proxy not working:**
- Verify `VITE_BFM_API_URL` is set correctly
- Check that the backend is running on the expected port
- Review browser network tab for proxy errors

## Development Workflow

### Local Development

1. **Start the backend:**
   ```bash
   cd api && air
   ```
   The server will start on `http://localhost:7070` (or the port specified in your environment variables).

2. **Start the frontend:**
   ```bash
   cd ffm && npm run dev
   ```
   The frontend will start on `http://localhost:4040`.

3. **Make changes:**
   - Edit Go files in `api/` - Air will rebuild automatically
   - Edit React/TypeScript files in `ffm/src/` - Vite will hot-reload
   - Edit CSS files - Changes appear instantly

4. **View logs:**
   - Backend logs appear in the Air terminal
   - Frontend logs appear in the Vite terminal and browser console

### Docker Development

1. **Start all services with hot-reload:**
   ```bash
   make dev-docker
   ```

2. **Make changes:**
   - Edit Go files in `api/` - Air will rebuild automatically in container
   - Edit React/TypeScript files in `ffm/src/` - Vite will hot-reload in container
   - Edit CSS files - Changes appear instantly
   - All changes are synced via volume mounts

3. **View logs:**
   ```bash
   make dev-docker-logs        # All services
   make dev-docker-logs-bfm    # Backend only
   make dev-docker-logs-ffm    # Frontend only
   ```

4. **Stop services:**
   ```bash
   make dev-docker-down
   ```

## Building the CLI for Development

To build and use the BfM CLI tool during development:

```bash
# Build the CLI
make build-cli

# Or manually
cd api && go build -o ../bfm-cli ./cmd/cli

# Use the CLI
./bfm-cli version
./bfm-cli build examples/sfm --verbose
```

## Environment Configuration

For local development, create a `.env` file in the project root or set environment variables:

```bash
# Server Configuration
BFM_HTTP_PORT=7070
BFM_GRPC_PORT=9090
BFM_API_TOKEN=dev-token-change-in-production

# State Database Configuration
BFM_STATE_BACKEND=postgresql
BFM_STATE_DB_HOST=localhost
BFM_STATE_DB_PORT=5432
BFM_STATE_DB_USERNAME=postgres
BFM_STATE_DB_PASSWORD=postgres
BFM_STATE_DB_NAME=migration_state

# Backend Connections (configure as needed)
CORE_BACKEND=postgresql
CORE_DB_HOST=localhost
CORE_DB_PORT=5432
CORE_DB_USERNAME=postgres
CORE_DB_PASSWORD=postgres
CORE_DB_NAME=migration_state
CORE_SCHEMA=core
```

## Tips

- **Backend:** Air watches all `.go` files by default. Test files (`_test.go`) are excluded.
- **Frontend:** Vite's HMR preserves React component state, so you can test interactions without losing state.
- **Both:** Use separate terminals for each service to see logs clearly.
- **Docker Development:** 
  - Use `make dev-docker` for Docker-based development with hot-reload
  - Source code is mounted as volumes, so changes are immediately reflected
  - No need to rebuild images after code changes
  - Air and Vite run inside containers with full hot-reload support
- **Local Development:** 
  - Use `make dev-bfm` and `make dev-ffm` for local development
  - Requires Air and Node.js installed locally
  - Faster startup time, no Docker overhead

