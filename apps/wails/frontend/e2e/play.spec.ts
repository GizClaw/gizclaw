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
            selected_view: "play",
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
    window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__ = {
      async loadSnapshot() {
        return {
          contacts: [{ id: "contact-main", title: "Main Contact" }],
          credentials: [{ id: "fake-openai-credential-000", title: "Fake OpenAI Credential" }],
          firmwares: [{ id: "devkit-firmware-main", subtitle: "stable", title: "Devkit Firmware" }],
          friendGroups: [{ id: "story-group", subtitle: "member", title: "Story Group" }],
          friends: [{ id: "peer-b", subtitle: "peer-a <-> peer-b", title: "Peer B" }],
          history: [
            {
              id: "20260701T000000Z-1",
              name: "transcript",
              text: "你好，开始测试。",
              type: "gear",
              updated_at: "2026-07-01T00:00:00Z",
            },
            {
              id: "20260701T000001Z-2",
              name: "answer",
              text: "收到，我们继续。",
              type: "agent",
              updated_at: "2026-07-01T00:00:01Z",
            },
          ],
          memoryStats: { total: 2 },
          models: [{ id: "fake-openai-chat-000", title: "Fake OpenAI Chat" }],
          pets: [{ id: "pet-main", title: "Main Pet" }],
          rewards: [{ id: "reward-claim", title: "Reward Claim" }],
          runWorkspace: {
            mode: "push-to-talk",
            workspace_name: "flowcraft-chat",
          },
          wallet: { id: "wallet-main", title: "Main Wallet" },
          walletTransactions: [{ id: "wallet-tx-1", title: "Wallet Transaction" }],
          warnings: [],
          workflows: [{ id: "flowcraft-chat", title: "Flowcraft Chat Workflow" }],
          workspaces: [{ id: "flowcraft-chat", title: "Flowcraft Chat Workspace" }],
        };
      },
      async playHistory(historyID) {
        actions.push(`play:${historyID}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { accepted: true };
      },
      async recallMemory(query) {
        actions.push(`recall:${query}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return {
          hits: [{ id: "memory-hit-1", subtitle: query, title: "Memory Hit" }],
          raw: { query },
        };
      },
      async reloadWorkspace() {
        actions.push("reload");
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { accepted: true };
      },
      async setWorkspace(workspaceName) {
        actions.push(`set:${workspaceName}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { workspace_name: workspaceName };
      },
    };
  });
});

test("play view uses direct WebRTC RPC client data", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByText("Play Console")).toBeVisible();
  await expect(page.locator(".card-title", { hasText: "Play RPC" })).toBeVisible();
  await expect(page.getByText("WebRTC RPC")).toBeVisible();
  await expect(page.getByText("flowcraft-chat").first()).toBeVisible();
  await expect(page.getByText("push-to-talk")).toBeVisible();
  await expect(page.getByText("你好，开始测试。")).toBeVisible();
  await expect(page.getByText("Peer B")).toBeVisible();
  await expect(page.getByText("Story Group")).toBeVisible();
  await expect(page.getByText("Flowcraft Chat Workspace")).toBeVisible();
  await expect(page.getByText("Fake OpenAI Chat")).toBeVisible();
  await expect(page.getByText("Main Wallet")).toBeVisible();
  await expect(page.getByText("Reward Claim")).toBeVisible();
  await expect(page.getByText("Devkit Firmware")).toBeVisible();

  await page.locator(".card", { hasText: "Workspace History" }).getByRole("button", { name: /^Play$/ }).first().click();
  await expect(page.getByText("History 20260701T000000Z-1 replay requested.")).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []))
    .toContain("play:20260701T000000Z-1");
});

test("play view sends workspace and memory actions through client", async ({ page }) => {
  await page.goto("/");

  await page.locator(".card", { hasText: "Workspace" }).getByRole("button", { name: "Reload" }).click();
  await expect(page.getByText("Workspace reloaded.")).toBeVisible();

  await page.getByLabel("Workspace name").fill("story-group-workspace");
  await page.getByRole("button", { name: "Set" }).click();
  await expect(page.getByText("Workspace set to story-group-workspace.")).toBeVisible();

  await page.getByLabel("Memory recall query").fill("route");
  await page.getByRole("button", { name: "Recall" }).click();
  await expect(page.getByText("Memory Hit")).toBeVisible();

  await expect
    .poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []))
    .toEqual(expect.arrayContaining(["reload", "set:story-group-workspace", "recall:route"]));
});

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__?: string[];
  }
}
