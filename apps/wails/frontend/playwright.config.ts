import { defineConfig, devices } from "@playwright/test";

const port = process.env.GIZCLAW_DESKTOP_E2E_PORT ?? "4191";
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  use: {
    ...devices["Desktop Chrome"],
    channel: "chrome",
    baseURL,
  },
  webServer: {
    command: `npm run dev -- --port ${port}`,
    url: baseURL,
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
  },
});
