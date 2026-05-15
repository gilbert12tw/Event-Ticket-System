import { defineConfig, devices } from "@playwright/test";

const isCI = process.env.CI === "true";
const baseURL = process.env.PLAYWRIGHT_LIVE_BASE_URL || "http://127.0.0.1:8080";

export default defineConfig({
  testDir: "./e2e-live",
  timeout: 120_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  workers: 1,
  retries: isCI ? 1 : 0,
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: "playwright-live-report" }],
    ["json", { outputFile: "test-results/playwright-live-results.json" }],
  ],
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  forbidOnly: isCI,
  projects: [375, 768, 1024, 1440].map((width) => ({
    name: `chromium-live-${width}`,
    use: {
      ...devices["Desktop Chrome"],
      viewport: { width, height: 900 },
    },
  })),
});
