#!/bin/sh
# Entrypoint script to inject runtime environment variables into the built frontend

# Create a runtime config script that will be injected into the HTML
cat > /usr/share/nginx/html/runtime-config.js << EOF
window.__RUNTIME_CONFIG__ = {
  VITE_BFM_API_URL: '${VITE_BFM_API_URL:-/api}',
  VITE_BFM_API_TOKEN: '${VITE_BFM_API_TOKEN:-}',
  VITE_AUTH_ENABLED: '${VITE_AUTH_ENABLED:-true}',
  VITE_AUTH_USERNAME: '${VITE_AUTH_USERNAME:-admin}',
  VITE_AUTH_PASSWORD: '${VITE_AUTH_PASSWORD:-admin123}'
};
EOF

# Inject the script tag into index.html if it doesn't exist
if ! grep -q "runtime-config.js" /usr/share/nginx/html/index.html; then
  sed -i 's|</head>|<script src="/runtime-config.js"></script></head>|' /usr/share/nginx/html/index.html
fi

# Start nginx
exec nginx -g "daemon off;"

