import { expect, test } from "@playwright/test";

const requiredEnvNames = [
  "GIZCLAW_E2E_ADMIN_TELEMETRY_CONTEXT_NAME",
  "GIZCLAW_E2E_ADMIN_TELEMETRY_ENDPOINT",
  "GIZCLAW_E2E_ADMIN_TELEMETRY_PUBLIC_KEY",
  "GIZCLAW_E2E_ADMIN_TELEMETRY_PRIVATE_KEY_BASE64",
  "GIZCLAW_E2E_ADMIN_TELEMETRY_PEER_PUBLIC_KEY",
];
const missingEnv = requiredEnvNames.filter(
  (name) => (process.env[name] ?? "").trim() === "",
);
const contextName = process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_CONTEXT_NAME ?? "";
const endpoint = process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_ENDPOINT ?? "";
const localPublicKey = process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_PUBLIC_KEY ?? "";
const privateKeyBase64 =
  process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_PRIVATE_KEY_BASE64 ?? "";
const peerPublicKey =
  process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_PEER_PUBLIC_KEY ?? "";
const screenshotPath =
  process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_SCREENSHOT ??
  "test-results/admin-telemetry/peer-telemetry-tab.png";
const telemetryPanelScreenshotPath =
  process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_PANEL_SCREENSHOT ??
  "test-results/admin-telemetry/peer-telemetry-panel.png";
const telemetryMapScreenshotPath =
  process.env.GIZCLAW_E2E_ADMIN_TELEMETRY_MAP_SCREENSHOT ??
  "test-results/admin-telemetry/peer-telemetry-map.png";

test.skip(
  missingEnv.length > 0,
  `real admin telemetry e2e requires ${missingEnv.join(", ")}`,
);

test.beforeEach(async ({ page }) => {
  await page.addInitScript(
    ({ contextName, endpoint, localPublicKey, privateKeyBase64 }) => {
      const context = {
        current: true,
        description: "Local telemetry e2e server",
        endpoint,
        local_public_key: localPublicKey,
        name: contextName,
      };
      window.__GIZCLAW_DESKTOP_TEST_RUNTIME__ = {
        context,
        private_key_base64: privateKeyBase64,
      };
    },
    { contextName, endpoint, localPublicKey, privateKeyBase64 },
  );
});

test("admin telemetry tab renders seeded peer telemetry and writes screenshot artifact", async ({
  page,
}) => {
  await page.setViewportSize({ height: 1400, width: 1800 });
  await page.goto("/admin.html");

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible({
    timeout: 20_000,
  });
  await page.getByRole("button", { name: "Peers" }).first().click();
  await expect(page.getByRole("heading", { name: "Peers" })).toBeVisible();
  await expect(page.getByText(peerPublicKey)).toBeVisible();

  await page.getByRole("link").filter({ hasText: peerPublicKey }).click();
  await expect(page.getByText(peerPublicKey).first()).toBeVisible();
  await page.getByRole("tab", { name: "Telemetry" }).click();

  await expect(page.getByText("Latest").first()).toBeVisible({
    timeout: 20_000,
  });
  await expect(page.getByText("Battery").first()).toBeVisible();
  await expect(page.getByText("GNSS").first()).toBeVisible();
  await expect(page.getByText("Network").first()).toBeVisible();
  await expect(page.getByText("System").first()).toBeVisible();
  await expect(page.getByText("71 %").first()).toBeVisible();
  await expect(page.getByText("-61 dBm").first()).toBeVisible();
  await expect(page.getByText("Trajectory").first()).toBeVisible();
  await expect(page.getByText(/sampled points|No points/)).toBeVisible();

  const telemetryPanel = page.getByRole("tabpanel", { name: "Telemetry" });
  const mapCard = telemetryPanel
    .getByText("Trajectory")
    .first()
    .locator('xpath=ancestor::*[@data-slot="card"][1]');
  await mapCard.scrollIntoViewIfNeeded();

  await page.screenshot({ fullPage: true, path: screenshotPath });
  await telemetryPanel.screenshot({ path: telemetryPanelScreenshotPath });
  await mapCard.screenshot({ path: telemetryMapScreenshotPath });
  console.log(`admin telemetry screenshot: ${screenshotPath}`);
  console.log(
    `admin telemetry panel screenshot: ${telemetryPanelScreenshotPath}`,
  );
  console.log(`admin telemetry map screenshot: ${telemetryMapScreenshotPath}`);
});
