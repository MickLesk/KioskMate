import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "../tests/e2e",
  timeout: 30_000,
  expect: { timeout: 8_000 },
  fullyParallel: false,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: "http://127.0.0.1:33339",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "go run ./scripts/e2e-server -port 33339",
    cwd: "..",
    url: "http://127.0.0.1:33339/healthz",
    reuseExistingServer: false,
    timeout: 60_000,
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", use: { ...devices["Pixel 7"] } },
  ],
});
