/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string;
  readonly VITE_ENABLE_VERBOSE_LOGS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
