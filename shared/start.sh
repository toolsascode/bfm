#!/bin/bash
# Startup script for BfM production container
# Manages: bfm-server, bfm-worker (conditional)
# Frontend static files are served directly by bfm-server

# Don't exit on error in cleanup - we want to clean up everything
set -e

# JSON logging helper function
log_json() {
    local level=$1
    shift
    local message="$*"
    echo "{\"level\":\"$level\",\"msg\":\"$message\",\"time\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
}

# PID files
PID_DIR="/tmp"
SERVER_PID_FILE="${PID_DIR}/bfm-server.pid"
WORKER_PID_FILE="${PID_DIR}/bfm-worker.pid"

# Cleanup function
cleanup() {
    log_json "info" "Shutting down services..."

    # Stop worker if running
    if [ -f "$WORKER_PID_FILE" ]; then
        WORKER_PID=$(cat "$WORKER_PID_FILE")
        if kill -0 "$WORKER_PID" 2>/dev/null; then
            log_json "info" "Stopping BfM Worker (PID: $WORKER_PID)"
            kill -TERM "$WORKER_PID" 2>/dev/null || true
            wait "$WORKER_PID" 2>/dev/null || true
        fi
        rm -f "$WORKER_PID_FILE"
    fi

    # Stop server
    if [ -f "$SERVER_PID_FILE" ]; then
        SERVER_PID=$(cat "$SERVER_PID_FILE")
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            log_json "info" "Stopping BfM Server (PID: $SERVER_PID)"
            kill -TERM "$SERVER_PID" 2>/dev/null || true
            wait "$SERVER_PID" 2>/dev/null || true
        fi
        rm -f "$SERVER_PID_FILE"
    fi

    log_json "info" "All services stopped"
    exit 0
}

# Set up signal handlers
trap cleanup SIGTERM SIGINT

# Create PID directory
mkdir -p "$PID_DIR"

# Create SFM directory if it doesn't exist and BFM_SFM_PATH is set
if [ -n "$BFM_SFM_PATH" ]; then
    log_json "info" "Creating SFM directory at $BFM_SFM_PATH if it doesn't exist"
    mkdir -p "$BFM_SFM_PATH" || true
fi

# Start BfM Server
log_json "info" "Starting BfM Server"
/app/bin/bfm-server &
SERVER_PID=$!
echo "$SERVER_PID" > "$SERVER_PID_FILE"
log_json "info" "BfM Server started (PID: $SERVER_PID)"

# Wait for server to be ready
log_json "info" "Waiting for BfM Server to be ready"
SERVER_STARTED=false
for i in {1..60}; do
    if curl -s http://localhost:7070/health > /dev/null 2>&1; then
        log_json "info" "BfM Server is ready"
        SERVER_STARTED=true
        break
    fi
    # Check if server process is still running
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        log_json "error" "BfM Server process (PID: $SERVER_PID) has exited"
        # Don't exit immediately - let the wait loop continue to show logs
        SERVER_STARTED=false
    fi
    if [ $i -eq 60 ]; then
        if [ "$SERVER_STARTED" != "true" ]; then
            log_json "error" "BfM Server failed to start within 60 seconds"
            log_json "info" "Server process status: $(ps aux | grep bfm-server || echo 'Server process not found')"
            cleanup
            exit 1
        fi
    fi
    sleep 1
done

# Start BfM Worker conditionally
if [ "${BFM_QUEUE_ENABLED:-false}" = "true" ]; then
    log_json "info" "Starting BfM Worker (queue enabled)"
    /app/bin/bfm-worker &
    WORKER_PID=$!
    echo "$WORKER_PID" > "$WORKER_PID_FILE"
    log_json "info" "BfM Worker started (PID: $WORKER_PID)"
else
    log_json "info" "BfM Worker not started (BFM_QUEUE_ENABLED is not 'true')"
fi

# Generate runtime config for frontend from BFM_* environment variables
log_json "info" "Generating runtime config for frontend"
cat > /app/frontend/runtime-config.js << EOF
window.__RUNTIME_CONFIG__ = {
  BFM_API_URL: '${BFM_FRONTEND_API_URL:-/api}',
  BFM_API_TOKEN: '${BFM_API_TOKEN:-}',
  BFM_AUTH_ENABLED: '${BFM_FRONTEND_AUTH_ENABLED:-true}',
  BFM_AUTH_USERNAME: '${BFM_FRONTEND_AUTH_USERNAME:-admin}',
  BFM_AUTH_PASSWORD: '${BFM_FRONTEND_AUTH_PASSWORD:-admin123}'
};
EOF

# Inject the script tag into index.html if it doesn't exist
if [ -f /app/frontend/index.html ]; then
    if ! grep -q "runtime-config.js" /app/frontend/index.html; then
        sed -i 's|</head>|<script src="/runtime-config.js"></script></head>|' /app/frontend/index.html
    fi
    log_json "info" "Frontend runtime config generated"
else
    log_json "warn" "Frontend directory not found, skipping runtime config injection"
fi

log_json "info" "All services started successfully"
log_json "info" "Frontend & API: http://localhost:7070"
log_json "info" "API (gRPC): localhost:9090"

# Wait for server (foreground process)
wait "$SERVER_PID" 2>/dev/null || true

# If server exits, trigger cleanup
cleanup
