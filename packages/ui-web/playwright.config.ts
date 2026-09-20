import { defineConfig, devices } from "@playwright/test";

const PORT = process.env.PLAYWRIGHT_PORT || "3100";
const BASE_URL = `http://localhost:${PORT}`;

export default defineConfig({
  testDir: "./e2e",
  testMatch: /.*\.spec\.ts$/,
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [["list"]],
  use: {
    baseURL: BASE_URL,
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: process.env.SKIP_WEBSERVER
    ? undefined
    : {
        command: "bun run src/server.ts",
        url: BASE_URL,
        reuseExistingServer: !process.env.CI,
        timeout: 15_000,
        env: {
          PORT,
          NODE_ENV: "test",
        },
      },
});
