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

# Add node_modules/.bin to PATH
export PATH="/app/node_modules/.bin:$PATH"

echo "[ffm] Starting Vite dev server..."
# Start Vite development server
exec npm run dev -- --host 0.0.0.0

