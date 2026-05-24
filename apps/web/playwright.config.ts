import { defineConfig, devices } from "@playwright/test";

const isCI = process.env.CI === "true";
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
  workers: isCI ? 4 : undefined,
  retries: isCI ? 2 : 0,
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: "playwright-report" }],
    ["json", { outputFile: "test-results/playwright-mock-results.json" }],
  ],
  use: {
    baseURL: "http://127.0.0.1:4173",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  forbidOnly: isCI,
  webServer: {
    command: "pnpm exec vite --host 127.0.0.1 --port 4173",
    url: "http://127.0.0.1:4173",
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
