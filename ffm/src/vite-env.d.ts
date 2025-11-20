/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_BFM_API_URL: string;
  readonly VITE_BFM_API_TOKEN?: string;
  readonly VITE_AUTH_ENABLED: string;
  readonly VITE_AUTH_USERNAME: string;
  readonly VITE_AUTH_PASSWORD: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

