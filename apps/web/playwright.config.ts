import { defineConfig, devices } from "@playwright/test";

const isCI = process.env.CI === "true";
const port = Number(process.env.PLAYWRIGHT_MOCK_PORT || "4173");
const baseURL = `http://127.0.0.1:${port}`;
const workerCount = process.env.PLAYWRIGHT_WORKERS
  ? Number(process.env.PLAYWRIGHT_WORKERS)
  : isCI
    ? 4
    : undefined;
const viewports = [
  { name: "375", width: 375, height: 812 },
  { name: "768", width: 768, height: 1024 },
  { name: "1024", width: 1024, height: 768 },
  { name: "1440", width: 1440, height: 900 },
];

export default defineConfig({
  testDir: "./e2e",
  timeout: 90_000,
  fullyParallel: true,
  expect: {
    timeout: 8_000,
  },
  testIgnore: ["**/*.snapshots/**"],
  workers: workerCount,
  retries: isCI ? 2 : 0,
  reporter: [
    ["list"],
    [
      "html",
      {
        open: "never",
        outputFolder: process.env.PLAYWRIGHT_HTML_REPORT || "playwright-report",
      },
    ],
    [
      "json",
      {
        outputFile:
          process.env.PLAYWRIGHT_JSON_OUTPUT ||
          "test-results/playwright-mock-results.json",
      },
    ],
  ],
  outputDir: process.env.PLAYWRIGHT_OUTPUT_DIR || "test-results",
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  forbidOnly: isCI,
  webServer: {
    command: `pnpm exec vite --host 127.0.0.1 --port ${port}`,
    url: baseURL,
    reuseExistingServer: !isCI,
    timeout: 120_000,
  },
  projects: viewports.map((viewport) => ({
    name: `chromium-${viewport.name}`,
    use: {
      ...devices["Desktop Chrome"],
      viewport: {
        width: viewport.width,
        height: viewport.height,
      },
    },
  })),
});
