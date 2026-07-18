import { expect, test } from "@playwright/test";

const requiredEnvNames = [
  "GIZCLAW_E2E_ADMIN_ENDPOINT",
  "GIZCLAW_E2E_ADMIN_PUBLIC_KEY",
  "GIZCLAW_E2E_ADMIN_PRIVATE_KEY_BASE64",
];
const missingEnv = requiredEnvNames.filter((name) => (process.env[name] ?? "").trim() === "");
const endpoint = process.env.GIZCLAW_E2E_ADMIN_ENDPOINT ?? "";
const localPublicKey = process.env.GIZCLAW_E2E_ADMIN_PUBLIC_KEY ?? "";
const privateKeyBase64 = process.env.GIZCLAW_E2E_ADMIN_PRIVATE_KEY_BASE64 ?? "";

test.skip(missingEnv.length > 0, `real Desktop connection e2e requires ${missingEnv.join(", ")}`);

test.beforeEach(async ({ page }) => {
  await page.addInitScript(
    ({ endpoint, localPublicKey, privateKeyBase64 }) => {
      window.__GIZCLAW_DESKTOP_TEST_RUNTIME__ = {
        context: {
          endpoint,
          local_public_key: localPublicKey,
          name: "Local Server",
        },
        private_key_base64: privateKeyBase64,
      };
    },
    { endpoint, localPublicKey, privateKeyBase64 },
  );
});

test("admin connects to the real peer and admin HTTP services", async ({ page }) => {
  await page.goto("/admin.html");

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("No public key")).toHaveCount(0, { timeout: 20_000 });
  await page.getByRole("button", { name: "Models" }).click();
  await expect(page.getByRole("heading", { name: "Models" })).toBeVisible();
  await expect(page.locator("tbody tr")).not.toHaveCount(0, { timeout: 20_000 });
});

test("play connects to the real peer RPC service", async ({ page }) => {
  await page.goto("/play.html");

  await expect(page.getByRole("button", { name: /Workspaces/ })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("heading", { name: "Play connection failed" })).toHaveCount(0);
});
