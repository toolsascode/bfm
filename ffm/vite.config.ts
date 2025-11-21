import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  // Development server configuration (only used in dev mode)
  // These settings are NOT used in production builds
  server: {
    port: 4040,
    host: true, // Listen on all addresses
    strictPort: false,
    hmr: {
      port: 4040,
      host: 'localhost',
      protocol: 'ws',
    },
    watch: {
      usePolling: true, // Useful for Docker/WSL
    },
    proxy: {
      '/api': {
        // In Docker, use service name; locally, use localhost
        // The proxy forwards /api/* requests to the backend
        // e.g., /api/v1/migrations -> http://bfm-server:7070/api/v1/migrations (Docker)
        //      /api/v1/migrations -> http://localhost:7070/api/v1/migrations (local)
        target: process.env.DOCKER === 'true' ? 'http://bfm-server:7070' : 'http://localhost:7070',
        changeOrigin: true,
        ws: true, // Enable WebSocket proxying for HMR
      }
    }
  },
  // Production build configuration
  build: {
    sourcemap: true,
  },
})

