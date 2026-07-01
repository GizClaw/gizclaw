import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  use: {
    ...devices["Desktop Chrome"],
    channel: "chrome",
    baseURL: "http://127.0.0.1:4191",
  },
  webServer: {
    command: "npm run dev -- --port 4191",
    url: "http://127.0.0.1:4191",
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
  },
});
