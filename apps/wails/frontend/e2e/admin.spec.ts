import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const context = {
      current: true,
      description: "Local server",
      endpoint: "127.0.0.1:9820",
      local_public_key: "local-public-key",
      name: "local",
      server_public_key: "server-public-key",
    };
    window.__GIZCLAW_DESKTOP_TEST_API__ = {
      async Bootstrap() {
        return {
          contexts: [context],
          paths: {
            config_root: "/tmp/gizclaw-desktop",
            context_dir: "/tmp/gizclaw-desktop/contexts",
            state_file: "/tmp/gizclaw-desktop/state.json",
          },
          runtime: {
            context,
            private_key_base64: "cHJpdmF0ZS1rZXktbWF0ZXJpYWw=",
            signaling_url: "http://127.0.0.1:9820/webrtc/v1/offer",
          },
          state: {
            selected_context: "local",
            selected_view: "admin",
          },
        };
      },
      async CreateContext() {
        return { context };
      },
      async ListContexts() {
        return [context];
      },
      async RuntimeContext() {
        return { context };
      },
      async SelectContext() {
        return { context };
      },
      async SetSelectedView(view) {
        return { selected_context: "local", selected_view: view };
      },
    };
    window.__GIZCLAW_DESKTOP_TEST_ADMIN_CLIENT__ = {
      async listSections() {
        return [
          {
            description: "Registered peers and runtime metadata.",
            key: "peers",
            rows: [
              {
                id: "peer-public-key-1",
                status: "approved",
                subtitle: "developer badge",
                title: "Kitchen Device",
                updated_at: "2026-07-01T00:00:00Z",
              },
            ],
            title: "Peers",
          },
          {
            description: "Workspace workflow definitions.",
            key: "workflows",
            rows: [
              {
                id: "openai-chat",
                title: "OpenAI Chat",
              },
              {
                id: "doubao-realtime",
                title: "Doubao Realtime",
              },
            ],
            title: "Workflows",
          },
        ];
      },
    };
  });
});

test("admin view renders generated-client resource sections", async ({ page }) => {
  await page.goto("/");

  await expect(page.locator(".card-title", { hasText: "Admin API" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Peers/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /Workflows/ })).toBeVisible();
  await expect(page.getByText("Kitchen Device")).toBeVisible();
  await expect(page.getByText("peer-public-key-1")).toBeVisible();

  await page.getByRole("button", { name: /Workflows/ }).click();
  await expect(page.locator(".card-title", { hasText: "Workflows" })).toBeVisible();
  await expect(page.getByText("OpenAI Chat")).toBeVisible();
  await expect(page.getByText("Doubao Realtime")).toBeVisible();
});
