# Development Guide

This guide explains how to set up hot-reload for both the backend (bfm) and frontend (ffm) projects.

**⚠️ IMPORTANT: Hot-reload is for DEVELOPMENT ONLY. Production builds do NOT use these tools.**

## Hot-Reload Setup

### Backend (bfm) - Using Air

Air is a live reload utility for Go applications. It automatically rebuilds and restarts your Go application when you make changes.

#### Installation

Install Air using one of these methods:

**Using Go:**
```bash
go install github.com/cosmtrek/air@latest
```

**Using Homebrew (macOS):**
```bash
brew install air
```

**Using curl (Linux/macOS):**
```bash
curl -sSfL https://raw.githubusercontent.com/cosmtrek/air/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

#### Usage

Navigate to the bfm directory and run:
```bash
cd bfm
air
```

Air will:
- Watch for changes in `.go` files
- Automatically rebuild the application
- Restart the server when changes are detected
- Show colored output for build status

The configuration is already set up in `bfm/.air.toml`. It will:
- Build from `./cmd/server`
- Output to `./tmp/main`
- Exclude test files, vendor, and deploy directories
- Watch for changes in `.go`, `.tpl`, `.tmpl`, and `.html` files

### Frontend (ffm) - Using Vite HMR

Vite has built-in Hot Module Replacement (HMR) that works out of the box.

#### Usage

Navigate to the ffm directory and run:
```bash
cd ffm
npm run dev
```

Vite will:
- Start the development server on `http://localhost:4040`
- Enable HMR for React components
- Automatically reload CSS changes
- Preserve component state during updates
- Show build errors in the browser

#### Configuration

The Vite configuration (`ffm/vite.config.ts`) is optimized for:
- WebSocket-based HMR
- File system polling (useful for Docker/WSL)
- API proxying to the backend
- Source maps for debugging

## Running Both Services

### Option 1: Docker with Hot-Reload (Recommended for Docker)

Start both services in Docker with hot-reload:
```bash
# Build and start all services with hot-reload
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

### Option 2: Local Development (No Docker)

Start both services locally with hot-reload:
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

**Terminal 1 - Backend (manual):**
```bash
cd bfm
air
```

**Terminal 2 - Frontend (manual):**
```bash
cd ffm
npm run dev
```

### Option 3: Docker Compose (Production-like, No Hot-Reload)

For production-like environment without hot-reload:
```bash
make dev
```

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

### Docker Development

1. **Start all services with hot-reload:**
   ```bash
   make dev-docker
   ```

2. **Make changes:**
   - Edit Go files in `bfm/` - Air will rebuild automatically in container
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

### Local Development

1. **Start the backend:**
   ```bash
   cd bfm && air
   ```

2. **Start the frontend:**
   ```bash
   cd ffm && npm run dev
   ```

3. **Make changes:**
   - Edit Go files in `bfm/` - Air will rebuild automatically
   - Edit React/TypeScript files in `ffm/src/` - Vite will hot-reload
   - Edit CSS files - Changes appear instantly

4. **View logs:**
   - Backend logs appear in the Air terminal
   - Frontend logs appear in the Vite terminal and browser console

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

