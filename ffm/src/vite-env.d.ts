/// <reference types="vite/client" />

// Note: Frontend configuration supports both:
// - BFM_* prefix (production via runtime config injection - see shared/start.sh)
// - VITE_* prefix (development via Vite's environment variables)
interface ImportMetaEnv {
  // Vite dev server environment variables (for development)
  readonly VITE_BFM_API_URL?: string;
  readonly VITE_BFM_API_TOKEN?: string;
  readonly VITE_AUTH_ENABLED?: string;
  readonly VITE_AUTH_USERNAME?: string;
  readonly VITE_AUTH_PASSWORD?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
