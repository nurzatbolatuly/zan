/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string;
  readonly VITE_ENABLE_VERBOSE_LOGS?: string;
  readonly VITE_FILE_MAX_SIZE_BYTES: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
