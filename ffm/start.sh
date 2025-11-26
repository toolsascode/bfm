#!/bin/sh
set -e

echo "[ffm] Starting FFM frontend..."

# Check if node_modules exists and contains tailwindcss
if [ ! -d "/app/node_modules" ] || [ ! -f "/app/node_modules/tailwindcss/package.json" ]; then
  echo "[ffm] Tailwind CSS not found, installing/updating dependencies..."
  if [ -f "/app/package-lock.json" ]; then
    npm ci
  else
    npm install
  fi
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
