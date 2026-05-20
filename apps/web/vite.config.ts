import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

const configuredWebDevPort = process.env.WEB_DEV_PORT;
const webDevPort = Number(configuredWebDevPort ?? "5173");
const apiTarget = process.env.CETS_DEV_API_TARGET ?? "http://localhost:8080";
const buildOutDir =
  process.env.CETS_WEB_BUILD_OUT_DIR ??
  "../../services/api/internal/httpapi/static";
const watchOptions =
  process.env.CHOKIDAR_USEPOLLING === "true"
    ? { usePolling: true, interval: 100 }
    : undefined;

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: buildOutDir,
    emptyOutDir: true,
  },
  server: {
    host: "0.0.0.0",
    port: webDevPort,
    strictPort: true,
    hmr: configuredWebDevPort ? { clientPort: webDevPort } : undefined,
    watch: watchOptions,
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true,
      },
      "/healthz": {
        target: apiTarget,
        changeOrigin: true,
      },
      "/readyz": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    exclude: [
      "**/node_modules/**",
      "**/dist/**",
      "**/e2e/**",
      "**/e2e-live/**",
      "**/playwright-report/**",
      "**/playwright-live-report/**",
    ],
  },
});
