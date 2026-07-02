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
    const actions: string[] = [];
    let session = { active: false };
    const views = [
      { description: "Manage GizClaw server resources.", id: "admin", title: "Admin" },
      { description: "Use workspaces, chat history, social, and firmware flows.", id: "play", title: "Play" },
    ];
    const snapshot = {
      contacts: [{ id: "contact-main", name: "Main Contact", title: "Main Contact" }],
      credentials: [{ id: "fake-openai-credential-000", title: "Fake OpenAI Credential" }],
      firmwares: [{ id: "devkit-firmware-main", name: "devkit-firmware-main", slots: { beta: {}, develop: {}, pending: {}, stable: { description: "stable" } }, title: "Devkit Firmware" }],
      friendGroups: [{ id: "story-group", my_role: "member", name: "Story Group", workspace_name: "story-group-workspace" }],
      friends: [{ id: "peer-b", peer_public_key: "peer-b", name: "Peer B", workspace_name: "friend-workspace" }],
      history: [
        { created_at: "2026-07-01T00:00:00Z", id: "20260701T000000Z-1", name: "transcript", replay_available: true, text: "你好，开始测试。", type: "gear", updated_at: "2026-07-01T00:00:00Z" },
        { created_at: "2026-07-01T00:00:01Z", id: "20260701T000001Z-2", name: "answer", replay_available: true, text: "收到，我们继续。", type: "agent", updated_at: "2026-07-01T00:00:01Z" },
      ],
      memoryStats: { total: 2 },
      models: [{ id: "fake-openai-chat-000", name: "Fake OpenAI Chat", title: "Fake OpenAI Chat" }],
      pets: [{ id: "pet-main", name: "Main Pet", title: "Main Pet" }],
      rewards: [{ id: "reward-claim", prompt: "Reward Claim", title: "Reward Claim" }],
      runWorkspace: {
        active_workspace_name: "flowcraft-chat",
        mode: "push",
        workspace_mode: "push",
        workspace_name: "flowcraft-chat",
      },
      voices: [{ id: "volc-voice-000", name: "Volc Voice", provider: { kind: "volc-tenant", name: "volc-tenant" }, source: "sync" }],
      wallet: { id: "wallet-main", point_balance: 10, title: "Main Wallet", token_balance: 0 },
      walletTransactions: [{ id: "wallet-tx-1", reason: "seed", title: "Wallet Transaction" }],
      warnings: [],
      workflows: [{ id: "flowcraft-chat", name: "Flowcraft Chat Workflow", title: "Flowcraft Chat Workflow" }],
      workspaces: [{ id: "flowcraft-chat", name: "flowcraft-chat", title: "Flowcraft Chat Workspace", workflow_name: "flowcraft-chat" }],
    };
    window.__GIZCLAW_DESKTOP_TEST_API__ = {
      async Bootstrap() {
        return { contexts: [context], state: { last_context: "local", last_view: "play" }, view_session: session, views };
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
    window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__ = {
      async loadSnapshot() {
        return snapshot;
      },
      async playHistory(historyID) {
        actions.push(`play:${historyID}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { accepted: true };
      },
      async recallMemory(query) {
        actions.push(`recall:${query}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { hits: [{ id: "memory-hit-1", score: 0.95, snippet: `Memory Hit: ${query}`, source_id: "memory-hit-1" }] };
      },
      async reloadWorkspace() {
        actions.push("reload");
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return snapshot.runWorkspace;
      },
      async setWorkspace(workspaceName) {
        actions.push(`set:${workspaceName}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        snapshot.runWorkspace.workspace_name = workspaceName;
        snapshot.runWorkspace.active_workspace_name = workspaceName;
        return snapshot.runWorkspace;
      },
    };
  });
});

test("play view renders the full desktop play surface", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Get Started" }).click();

  await expect(page.getByText("OpenAI Gateway")).toBeVisible();
  await expect(page.getByRole("button", { name: /Workspaces/ })).toBeVisible();
  await expect(page.getByText("wallet-main")).toBeVisible();

  await page.getByRole("button", { name: /Workspaces/ }).click();
  await expect(page.getByRole("heading", { name: "Workspaces" })).toBeVisible();
  await expect(page.getByText("flowcraft-chat").first()).toBeVisible();

  await page.getByRole("button", { name: /Friends/ }).click();
  await expect(page.getByRole("heading", { name: "Friends" })).toBeVisible();
  await expect(page.getByText("peer-b").first()).toBeVisible();

  await page.getByRole("button", { name: /Firmwares/ }).click();
  await expect(page.getByRole("heading", { name: "Firmwares" })).toBeVisible();
  await expect(page.getByText("devkit-firmware-main")).toBeVisible();
});

test("play workspace drawer sends direct RPC-backed actions", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Get Started" }).click();

  await page.getByRole("button", { name: /^Workspace$/ }).click();
  await expect(page.getByRole("heading", { name: "Workspace" })).toBeVisible();
  await page.getByRole("button", { name: /Reload/ }).click();
  await expect.poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? [])).toContain("reload");

  await page.getByRole("tab", { name: /Recall/ }).click();
  await page.getByPlaceholder("Recall query").fill("route");
  await page.getByRole("button", { name: "Run Recall" }).click();
  await expect(page.getByText("Memory Hit: route")).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? [])).toContain("recall:route");
});

test("play workspace history replay uses direct peer RPC", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Get Started" }).click();

  await page.getByRole("button", { name: /^Workspace$/ }).click();
  await page.getByRole("tab", { name: "History" }).click();
  await expect(page.getByText("你好，开始测试。")).toBeVisible();

  const firstHistoryRow = page.getByRole("row").filter({ hasText: "你好，开始测试。" });
  await firstHistoryRow.getByRole("button", { name: "Play" }).click();

  await expect.poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? [])).toContain("play:20260701T000000Z-1");
});

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__?: string[];
  }
}
