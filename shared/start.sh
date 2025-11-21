#!/bin/bash
# Startup script for BfM production container
# Manages: bfm-server, bfm-worker (conditional)
# Frontend static files are served directly by bfm-server

# Don't exit on error in cleanup - we want to clean up everything
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# PID files
PID_DIR="/tmp"
SERVER_PID_FILE="${PID_DIR}/bfm-server.pid"
WORKER_PID_FILE="${PID_DIR}/bfm-worker.pid"

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Shutting down services...${NC}"
    
    # Stop worker if running
    if [ -f "$WORKER_PID_FILE" ]; then
        WORKER_PID=$(cat "$WORKER_PID_FILE")
        if kill -0 "$WORKER_PID" 2>/dev/null; then
            echo -e "${YELLOW}Stopping BfM Worker (PID: $WORKER_PID)...${NC}"
            kill -TERM "$WORKER_PID" 2>/dev/null || true
            wait "$WORKER_PID" 2>/dev/null || true
        fi
        rm -f "$WORKER_PID_FILE"
    fi
    
    # Stop server
    if [ -f "$SERVER_PID_FILE" ]; then
        SERVER_PID=$(cat "$SERVER_PID_FILE")
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            echo -e "${YELLOW}Stopping BfM Server (PID: $SERVER_PID)...${NC}"
            kill -TERM "$SERVER_PID" 2>/dev/null || true
            wait "$SERVER_PID" 2>/dev/null || true
        fi
        rm -f "$SERVER_PID_FILE"
    fi
    
    echo -e "${GREEN}All services stopped.${NC}"
    exit 0
}

# Set up signal handlers
trap cleanup SIGTERM SIGINT

# Create PID directory
mkdir -p "$PID_DIR"

# Start BfM Server
echo -e "${GREEN}Starting BfM Server...${NC}"
/app/bin/bfm-server &
SERVER_PID=$!
echo "$SERVER_PID" > "$SERVER_PID_FILE"
echo -e "${GREEN}BfM Server started (PID: $SERVER_PID)${NC}"

# Wait for server to be ready
echo -e "${YELLOW}Waiting for BfM Server to be ready...${NC}"
SERVER_STARTED=false
for i in {1..60}; do
    if curl -s http://localhost:7070/health > /dev/null 2>&1; then
        echo -e "${GREEN}BfM Server is ready!${NC}"
        SERVER_STARTED=true
        break
    fi
    # Check if server process is still running
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        echo -e "${RED}Error: BfM Server process (PID: $SERVER_PID) has exited${NC}"
        # Don't exit immediately - let the wait loop continue to show logs
        SERVER_STARTED=false
    fi
    if [ $i -eq 60 ]; then
        if [ "$SERVER_STARTED" != "true" ]; then
            echo -e "${RED}Error: BfM Server failed to start within 60 seconds${NC}"
            echo -e "${YELLOW}Server process status:${NC}"
            ps aux | grep bfm-server || echo "Server process not found"
            cleanup
            exit 1
        fi
    fi
    sleep 1
done

# Start BfM Worker conditionally
if [ "${BFM_QUEUE_ENABLED:-false}" = "true" ]; then
    echo -e "${GREEN}Starting BfM Worker (queue enabled)...${NC}"
    /app/bin/bfm-worker &
    WORKER_PID=$!
    echo "$WORKER_PID" > "$WORKER_PID_FILE"
    echo -e "${GREEN}BfM Worker started (PID: $WORKER_PID)${NC}"
else
    echo -e "${YELLOW}BfM Worker not started (BFM_QUEUE_ENABLED is not 'true')${NC}"
fi

# Generate runtime config for frontend from BFM_* environment variables
echo -e "${GREEN}Generating runtime config for frontend...${NC}"
cat > /app/frontend/runtime-config.js << EOF
window.__RUNTIME_CONFIG__ = {
  BFM_API_URL: '${BFM_FRONTEND_API_URL:-/api}',
  BFM_API_TOKEN: '${BFM_FRONTEND_API_TOKEN:-}',
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
    echo -e "${GREEN}Frontend runtime config generated${NC}"
else
    echo -e "${YELLOW}Warning: Frontend directory not found, skipping runtime config injection${NC}"
fi

echo -e "\n${GREEN}========================================${NC}"
echo -e "${GREEN}All services started successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "  Frontend & API: http://localhost:7070"
echo -e "  API (gRPC):     localhost:9090"
echo -e "${GREEN}========================================${NC}\n"

# Wait for server (foreground process)
wait "$SERVER_PID" 2>/dev/null || true

# If server exits, trigger cleanup
cleanup

