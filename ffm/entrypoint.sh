#!/bin/sh
# Entrypoint script to inject runtime environment variables into the built frontend

# Create a runtime config script that will be injected into the HTML
cat > /usr/share/nginx/html/runtime-config.js << EOF
window.__RUNTIME_CONFIG__ = {
  BFM_API_URL: '${BFM_FRONTEND_API_URL:-/api}',
  BFM_API_TOKEN: '${BFM_FRONTEND_API_TOKEN:-}',
  BFM_AUTH_ENABLED: '${BFM_FRONTEND_AUTH_ENABLED:-true}',
  BFM_AUTH_USERNAME: '${BFM_FRONTEND_AUTH_USERNAME:-admin}',
  BFM_AUTH_PASSWORD: '${BFM_FRONTEND_AUTH_PASSWORD:-admin123}'
};
EOF

# Inject the script tag into index.html if it doesn't exist
if ! grep -q "runtime-config.js" /usr/share/nginx/html/index.html; then
  sed -i 's|</head>|<script src="/runtime-config.js"></script></head>|' /usr/share/nginx/html/index.html
fi

# Start nginx
exec nginx -g "daemon off;"
