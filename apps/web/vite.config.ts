import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { mkdir, readdir, rm } from "node:fs/promises";
import { fileURLToPath, URL } from "node:url";

const configuredWebDevPort = process.env.WEB_DEV_PORT;
const webDevPort = Number(configuredWebDevPort ?? "5173");
const apiTarget = process.env.CETS_DEV_API_TARGET ?? "http://localhost:8080";
const staticOutDirURL = new URL(
  "../../services/api/internal/httpapi/static/",
  import.meta.url,
);
const staticOutDir = fileURLToPath(staticOutDirURL);
const watchOptions =
  process.env.CHOKIDAR_USEPOLLING === "true"
    ? { usePolling: true, interval: 100 }
    : undefined;

export default defineConfig({
  plugins: [cleanGeneratedStaticAssets(), react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: staticOutDir,
    emptyOutDir: false,
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

function cleanGeneratedStaticAssets() {
  return {
    name: "clean-generated-static-assets",
    apply: "build" as const,
    async buildStart() {
      await mkdir(staticOutDirURL, { recursive: true });
      const entries = await readdir(staticOutDirURL);
      await Promise.all(
        entries
          .filter((entry) => entry !== "placeholder.txt")
          .map((entry) =>
            rm(new URL(entry, staticOutDirURL), {
              force: true,
              recursive: true,
            }),
          ),
      );
    },
  };
}
