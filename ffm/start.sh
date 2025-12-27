#!/bin/sh
set -e

echo "[ffm] Starting FFM frontend..."

# Check if node_modules exists and contains required packages
# Also check if react version matches between package.json and installed version
REINSTALL_NEEDED=false
if [ ! -d "/app/node_modules" ] || \
   [ ! -f "/app/node_modules/tailwindcss/package.json" ] || \
   [ ! -f "/app/node_modules/react-is/package.json" ]; then
  REINSTALL_NEEDED=true
fi

# Check React version mismatch
if [ -f "/app/node_modules/react/package.json" ] && [ -f "/app/package.json" ]; then
  INSTALLED_REACT=$(grep -o '"version": "[^"]*"' /app/node_modules/react/package.json | cut -d'"' -f4)
  EXPECTED_REACT=$(grep -o '"react": "[^"]*"' /app/package.json | cut -d'"' -f4 | sed 's/\^//' | sed 's/~//')
  if [ "$INSTALLED_REACT" != "$EXPECTED_REACT" ]; then
    echo "[ffm] React version mismatch detected (installed: $INSTALLED_REACT, expected: $EXPECTED_REACT)"
    REINSTALL_NEEDED=true
  fi
fi

if [ "$REINSTALL_NEEDED" = "true" ]; then
  echo "[ffm] Installing/updating dependencies..."
  # Use npm install in development to allow lock file updates
  npm install
  if [ $? -ne 0 ]; then
    echo "[ffm] Failed to install dependencies. Exiting."
    exit 1
  fi
  echo "[ffm] Dependencies installed successfully"
else
  echo "[ffm] Dependencies already installed."
fi

# Create runtime config for development (similar to production entrypoint.sh)
echo "[ffm] Creating runtime configuration..."
mkdir -p /app/public
cat > /app/public/runtime-config.js << EOF
window.__RUNTIME_CONFIG__ = {
  BFM_API_URL: '${BFM_FRONTEND_API_URL:-/api}',
  BFM_API_TOKEN: '${BFM_API_TOKEN:-}',
  BFM_AUTH_ENABLED: '${BFM_FRONTEND_AUTH_ENABLED:-true}',
  BFM_AUTH_USERNAME: '${BFM_FRONTEND_AUTH_USERNAME:-admin}',
  BFM_AUTH_PASSWORD: '${BFM_FRONTEND_AUTH_PASSWORD:-admin123}'
};
EOF

# Inject the script tag into index.html if it doesn't exist
if ! grep -q "runtime-config.js" /app/index.html; then
  echo "[ffm] Injecting runtime config script into index.html..."
  sed -i 's|</head>|<script src="/runtime-config.js"></script></head>|' /app/index.html
fi

# Add node_modules/.bin to PATH
export PATH="/app/node_modules/.bin:$PATH"

echo "[ffm] Starting Vite dev server..."
# Start Vite development server
exec npm run dev -- --host 0.0.0.0
