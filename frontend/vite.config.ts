/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  // Порт зафиксирован: origin фронта должен совпадать с CORS_ALLOWED_ORIGINS
  // backend. strictPort — если порт занят, dev-сервер падает с ошибкой, а не
  // уходит молча на соседний, где backend отклонит запросы по CORS.
  server: {
    port: 5173,
    strictPort: true,
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: true,
  },
});
