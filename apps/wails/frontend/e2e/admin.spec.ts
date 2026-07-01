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
    let session = { active: false };
    const views = [
      { description: "Manage GizClaw server resources.", id: "admin", title: "Admin" },
      { description: "Use workspaces, chat history, social, and firmware flows.", id: "play", title: "Play" },
    ];
    const pageResponse = (items) => ({ has_next: false, items, next_cursor: null });
    const json = (data) =>
      new Response(JSON.stringify(data), {
        headers: { "content-type": "application/json" },
        status: 200,
      });
    const data = {
      "/acl/policy-bindings": pageResponse([{ id: "binding-admin", role: "admin-role", view: "default-view" }]),
      "/acl/roles": pageResponse([{ name: "admin-role", permissions: [] }]),
      "/acl/views": pageResponse([{ name: "default-view", resources: [] }]),
      "/badges": pageResponse([{ id: "badge-helper", name: "Helper Badge", updated_at: "2026-07-01T00:00:00Z" }]),
      "/credentials": pageResponse([{ body: { api_key: "set" }, name: "fake-openai-credential-000", provider: "openai", updated_at: "2026-07-01T00:00:00Z" }]),
      "/firmwares": pageResponse([{ name: "devkit-firmware-main", slots: { beta: {}, develop: {}, pending: {}, stable: {} }, updated_at: "2026-07-01T00:00:00Z" }]),
      "/models": pageResponse([{ id: "fake-openai-chat-000", kind: "chat", name: "Fake OpenAI chat model", provider: { kind: "openai-tenant", name: "openai-tenant" }, updated_at: "2026-07-01T00:00:00Z" }]),
      "/peers": pageResponse([{ auto_registered: false, public_key: "peer-public-key-1", role: "peer", status: "approved", updated_at: "2026-07-01T00:00:00Z" }]),
      "/pet-species": pageResponse([{ id: "pet-cat", name: "Pet Cat", updated_at: "2026-07-01T00:00:00Z" }]),
      "/server-info": { build_commit: "test-build", public_key: "server-public-key" },
      "/social/contacts": pageResponse([{ id: "contact-admin", name: "Admin Contact", owner_public_key: "peer-public-key-1" }]),
      "/social/friend-groups": pageResponse([{ id: "group-main", name: "Main Group", my_role: "owner", workspace_name: "group-workspace" }]),
      "/social/friends": pageResponse([{ id: "peer-b", owner_public_key: "peer-a", peer_public_key: "peer-b", workspace_name: "friend-workspace" }]),
      "/voices": pageResponse([{ id: "volc-voice-000", name: "Volc Voice", provider: { kind: "volc-tenant", name: "volc-tenant" }, source: "sync", updated_at: "2026-07-01T00:00:00Z" }]),
      "/volc-tenants": pageResponse([{ credential_name: "volc-credential", name: "volc-tenant", updated_at: "2026-07-01T00:00:00Z" }]),
      "/workflows": pageResponse([{ metadata: { name: "openai-chat" }, spec: { driver: "workflow" } }]),
      "/workspaces": pageResponse([{ name: "main-workspace", workflow_name: "openai-chat", updated_at: "2026-07-01T00:00:00Z" }]),
    };
    window.__GIZCLAW_DESKTOP_TEST_API__ = {
      async Bootstrap() {
        return { contexts: [context], state: { last_context: "local", last_view: "admin" }, view_session: session, views };
      },
      async CreateContext() {
        return context;
      },
      async EndViewSession() {
        session = { active: false };
        return session;
      },
      async GetViewSession() {
        return session;
      },
      async InjectedRuntime() {
        return { context, private_key_base64: "cHJpdmF0ZS1rZXktbWF0ZXJpYWw=", signaling_url: "http://127.0.0.1:9820/webrtc/v1/offer" };
      },
      async ListContexts() {
        return [context];
      },
      async ListViews() {
        return views;
      },
      async SelectContext() {
        return context;
      },
      async StartViewSession(req) {
        session = { active: true, context_name: req.context_name, view: req.view };
        return session;
      },
    };
    window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH__ = async (input) => {
      const url = new URL(typeof input === "string" ? input : input.url);
      return json(data[url.pathname] ?? pageResponse([]));
    };
  });
});

test("admin view renders full resource manager pages", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Get Started" }).click();

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByText("test-build")).toBeVisible();
  await expect(page.getByRole("link", { name: "Peers" }).first()).toBeVisible();

  await page.getByRole("link", { name: "Peers" }).first().click();
  await expect(page.getByRole("heading", { name: "Peers" })).toBeVisible();
  await expect(page.getByText("peer-public-key-1")).toBeVisible();

  await page.getByRole("link", { name: "Workflows" }).click();
  await expect(page.getByRole("heading", { name: "Workflows" })).toBeVisible();
  await expect(page.getByText("openai-chat")).toBeVisible();

  await page.getByRole("link", { name: "Firmwares" }).click();
  await expect(page.getByRole("heading", { name: "Firmwares" })).toBeVisible();
  await expect(page.getByText("devkit-firmware-main")).toBeVisible();

  await page.getByRole("link", { name: "Friends" }).click();
  await expect(page.getByRole("heading", { name: "Friends" })).toBeVisible();
  await expect(page.getByText("peer-a <-> peer-b")).toBeVisible();
});
