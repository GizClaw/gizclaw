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
                raw: { public_key: "peer-public-key-1", role: "peer", status: "approved" },
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
                raw: { metadata: { name: "openai-chat" }, kind: "Workflow" },
                title: "OpenAI Chat",
              },
              {
                id: "doubao-realtime",
                raw: { metadata: { name: "doubao-realtime" }, kind: "Workflow" },
                title: "Doubao Realtime",
              },
            ],
            title: "Workflows",
          },
          { description: "Workspace instances.", key: "workspaces", rows: [{ id: "main-workspace", title: "Main Workspace" }], title: "Workspaces" },
          { description: "Model catalog.", key: "models", rows: [{ id: "fake-openai-chat-000", title: "Fake OpenAI chat model" }], title: "Models" },
          { description: "Credentials.", key: "credentials", rows: [{ id: "fake-openai-credential-000", title: "Fake OpenAI Credential" }], title: "Credentials" },
          { description: "Voices.", key: "voices", rows: [{ id: "volc-voice-000", title: "Volc Voice" }], title: "Voices" },
          { description: "Firmware.", key: "firmwares", rows: [{ id: "devkit-firmware-main", title: "Devkit Firmware" }], title: "Firmwares" },
          { description: "Badges.", key: "badges", rows: [{ id: "badge-helper", title: "Helper Badge" }], title: "Badges" },
          { description: "Pet species.", key: "pet-species", rows: [{ id: "pet-cat", title: "Pet Cat" }], title: "Pet Species" },
          { description: "Social contacts.", key: "contacts", rows: [{ id: "contact-admin", title: "Admin Contact" }], title: "Contacts" },
          { description: "Friends.", key: "friends", rows: [{ id: "friend-peer", subtitle: "peer-a <-> peer-b", title: "Friend Pair" }], title: "Friends" },
          { description: "Friend groups.", key: "friend-groups", rows: [{ id: "group-main", title: "Main Group" }], title: "Friend Groups" },
          { description: "ACL views.", key: "acl-views", rows: [{ id: "default-view", title: "Default View" }], title: "ACL Views" },
          { description: "ACL roles.", key: "acl-roles", rows: [{ id: "admin-role", title: "Admin Role" }], title: "ACL Roles" },
          {
            description: "ACL policy bindings.",
            key: "acl-policy-bindings",
            rows: [{ id: "binding-admin", title: "Admin Binding" }],
            title: "ACL Policy Bindings",
          },
          { description: "Gemini tenants.", key: "gemini-tenants", rows: [{ id: "gemini-tenant", title: "Gemini Tenant" }], title: "Gemini Tenants" },
          {
            description: "DashScope tenants.",
            key: "dashscope-tenants",
            rows: [{ id: "dashscope-tenant", title: "DashScope Tenant" }],
            title: "DashScope Tenants",
          },
          { description: "OpenAI tenants.", key: "openai-tenants", rows: [{ id: "openai-tenant", title: "OpenAI Tenant" }], title: "OpenAI Tenants" },
          { description: "MiniMax tenants.", key: "minimax-tenants", rows: [{ id: "minimax-tenant", title: "MiniMax Tenant" }], title: "MiniMax Tenants" },
          { description: "Volc tenants.", key: "volc-tenants", rows: [{ id: "volc-tenant", title: "Volc Tenant" }], title: "Volc Tenants" },
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
  await expect(page.getByRole("cell", { name: "Kitchen Device" })).toBeVisible();
  await expect(page.getByRole("button", { name: "peer-public-key-1" })).toBeVisible();
  await expect(page.getByText("Peers Detail")).toBeVisible();
  await expect(page.getByText("gizclaw admin --context <admin-cli-context> show Peers 'peer-public-key-1'")).toBeVisible();
  await page.getByLabel("Filter Peers").fill("missing");
  await expect(page.getByText("No matching peers found.")).toBeVisible();
  await page.getByLabel("Filter Peers").fill("kitchen");
  await expect(page.getByRole("cell", { name: "Kitchen Device" })).toBeVisible();

  await page.getByRole("button", { name: /Workflows/ }).click();
  await expect(page.locator(".card-title").filter({ hasText: /^Workflows$/ })).toBeVisible();
  await expect(page.getByRole("cell", { name: "OpenAI Chat" })).toBeVisible();
  await expect(page.getByRole("cell", { name: "Doubao Realtime" })).toBeVisible();
  await expect(page.getByText("Workflows Detail")).toBeVisible();
  await page.getByRole("button", { name: /Models/ }).click();
  await expect(page.getByRole("cell", { name: "Fake OpenAI chat model" })).toBeVisible();
  await page.getByRole("button", { name: /Firmwares/ }).click();
  await expect(page.getByRole("button", { name: "devkit-firmware-main" })).toBeVisible();
  await page.getByRole("button", { name: /Badges/ }).click();
  await expect(page.getByRole("cell", { name: "Helper Badge" })).toBeVisible();
  await page.getByRole("button", { name: /Pet Species/ }).click();
  await expect(page.getByRole("cell", { name: "Pet Cat" })).toBeVisible();
  await page.getByRole("button", { name: /ACL Policy Bindings/ }).click();
  await expect(page.getByRole("button", { name: "binding-admin" })).toBeVisible();
  await page.getByRole("button", { name: /Gemini Tenants/ }).click();
  await expect(page.getByRole("cell", { name: "Gemini Tenant" })).toBeVisible();
  await page.getByRole("button", { name: /DashScope Tenants/ }).click();
  await expect(page.getByRole("cell", { name: "DashScope Tenant" })).toBeVisible();
  await page.getByRole("button", { name: /Volc Tenants/ }).click();
  await expect(page.getByRole("cell", { name: "Volc Tenant" })).toBeVisible();
});
