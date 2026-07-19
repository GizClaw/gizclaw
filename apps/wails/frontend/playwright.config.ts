import { defineConfig, devices } from "@playwright/test";

const port = process.env.GIZCLAW_DESKTOP_E2E_PORT ?? "4191";
const baseURL = process.env.GIZCLAW_E2E_DESKTOP_URL ?? `http://127.0.0.1:${port}`;
const webServer = process.env.GIZCLAW_E2E_DESKTOP_URL
  ? undefined
  : {
      command: `npm run dev -- --port ${port}`,
      url: baseURL,
      reuseExistingServer: false,
      stdout: "pipe" as const,
      stderr: "pipe" as const,
    };

export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  use: {
    ...devices["Desktop Chrome"],
    channel: "chrome",
    baseURL,
  },
  webServer,
});
