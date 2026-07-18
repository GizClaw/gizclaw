import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const context = {
      current: true,
      description: "Local server",
      endpoint: "127.0.0.1:9820",
      local_public_key: "local-public-key",
      name: "local",
    };
    window.__GIZCLAW_DESKTOP_TEST_RUNTIME__ = { context, private_key_base64: "cHJpdmF0ZS1rZXktbWF0ZXJpYWw=" };
    const actions: string[] = [];
    const snapshot = {
      badges: [{ active: true, badge_def_id: "badge-basic", exp: 125, id: "badge-basic", level: 1, ruleset_name: "default-gameplay" }],
      contacts: [{ id: "contact-main", name: "Main Contact", title: "Main Contact" }],
      credentials: [{ id: "fake-openai-credential-000", title: "Fake OpenAI Credential" }],
      firmwares: [{ id: "devkit-firmware-main", name: "devkit-firmware-main", slots: { beta: {}, develop: {}, pending: {}, stable: { description: "stable" } }, title: "Devkit Firmware" }],
      friendGroups: [{ id: "story-group", my_role: "member", name: "Story Group", workspace_name: "story-group-workspace" }],
      friends: [{ id: "peer-b", peer_public_key: "peer-b", name: "Peer B", workspace_name: "friend-workspace" }],
      gameResults: [],
      grants: [],
      history: [
        { created_at: "2026-07-01T00:00:00Z", id: "20260701T000000Z-1", name: "transcript", replay_available: true, text: "你好，开始测试。", type: "gear", updated_at: "2026-07-01T00:00:00Z" },
        { created_at: "2026-07-01T00:00:01Z", id: "20260701T000001Z-2", name: "answer", replay_available: true, text: "收到，我们继续。", type: "agent", updated_at: "2026-07-01T00:00:01Z" },
      ],
      memoryStats: { total: 2 },
      models: [{ id: "fake-openai-chat-000", name: "Fake OpenAI Chat", title: "Fake OpenAI Chat" }],
      pets: [
        {
          display_name: "Starter Pet",
          id: "pet-main",
          life: { clean: 80, hunger: 90 },
          petdef_id: "petdef-basic",
          progression: { xp: 90 },
          ruleset_name: "default-gameplay",
          workspace_name: "pet-pet-main",
        },
      ],
      points: { balance: 100, id: "default-gameplay", ruleset_name: "default-gameplay", updated_at: "2026-07-01T00:00:00Z" },
      pointsTransactions: [],
      runWorkspace: {
        active_workspace_name: "flowcraft-chat",
        mode: "push",
        workspace_mode: "push",
        workspace_name: "flowcraft-chat",
      },
      ruleset: {
        name: "default-gameplay",
        spec: {
          enabled: true,
          pet_pool: [{ adoption_cost: 10, petdef_id: "petdef-basic", weight: 100 }],
          points: { initial_balance: 100 },
        },
      },
      voices: [{ id: "volc-voice-000", name: "Volc Voice", provider: { kind: "volc-tenant", name: "volc-tenant" }, source: "sync" }],
      warnings: [],
      workflows: [
        {
          i18n: { description: "General-purpose chat workflow", name: "Flowcraft Chat Assistant" },
          name: "flowcraft-chat",
          spec: { driver: "flowcraft" },
        },
      ],
      workspaces: [{ id: "flowcraft-chat", name: "flowcraft-chat", title: "Flowcraft Chat Workspace", workflow_name: "flowcraft-chat" }],
    };
    const pageResponse = (items) => ({ has_next: false, items, next_cursor: null });
    const findByID = (items, id) => items.find((item) => item.id === id) ?? null;
    window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__ = {
      async adoptPet(req) {
        const displayName = String(req.display_name ?? "Adopted Pet");
        const id = `pet-${snapshot.pets.length + 1}`;
        const pet = {
          display_name: displayName,
          id,
          life: { clean: 100, hunger: 100 },
          petdef_id: "petdef-basic",
          progression: { xp: 0 },
          ruleset_name: "default-gameplay",
          workspace_name: `pet-${id}`,
        };
        snapshot.pets.push(pet);
        snapshot.points.balance -= 10;
        snapshot.pointsTransactions.push({
          balance_after: snapshot.points.balance,
          created_at: "2026-07-01T00:00:02Z",
          delta: -10,
          id: "txn-adopt-1",
          reason: "adopt",
          source_id: id,
          source_type: "pet_adoption",
        });
        actions.push(`adopt:${displayName}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return pet;
      },
      async deletePet(req) {
        const index = snapshot.pets.findIndex((pet) => pet.id === req.id);
        const [pet] = index >= 0 ? snapshot.pets.splice(index, 1) : [];
        return pet ?? null;
      },
      async drivePet(req) {
        const pet = findByID(snapshot.pets, req.pet_id);
        if (pet == null) {
          throw new Error("pet not found");
        }
        pet.progression.xp += 20;
        pet.life.clean = Math.min(100, pet.life.clean + 10);
        snapshot.points.balance += 5;
        snapshot.pointsTransactions.push({
          balance_after: snapshot.points.balance,
          created_at: "2026-07-01T00:00:03Z",
          delta: 5,
          id: "txn-drive-1",
          reason: String(req.action ?? "drive"),
          source_id: String(req.pet_id),
          source_type: "pet_drive",
        });
        let gameResult = null;
        if (req.game_result != null) {
          gameResult = {
            duration_ms: req.game_result.duration_ms,
            game_def_id: req.game_result.game_def_id,
            id: "game-result-1",
            idempotency_key: req.game_result.idempotency_key,
            max_score: req.game_result.max_score,
            occurred_at: "2026-07-01T00:00:03Z",
            outcome: req.game_result.outcome,
            pet_id: req.pet_id,
            score: req.game_result.score,
          };
          snapshot.gameResults.push(gameResult);
        }
        snapshot.grants.push({
          created_at: "2026-07-01T00:00:03Z",
          id: "reward-grant-1",
          pet_exp_delta: 20,
          pet_id: req.pet_id,
          points_delta: 5,
          reason: String(req.action ?? "drive"),
          source_id: gameResult?.id ?? String(req.pet_id),
          source_type: gameResult == null ? "pet_drive" : "game_result",
        });
        actions.push(`drive:${req.pet_id}:${req.action ?? ""}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { game_result: gameResult, pet, rewards: snapshot.grants.slice(-1) };
      },
      async getBadge(req) {
        return findByID(snapshot.badges, req.id);
      },
      async getGameResult(req) {
        return findByID(snapshot.gameResults, req.id);
      },
      async getGameRuleset() {
        return snapshot.ruleset;
      },
      async getPet(req) {
        return findByID(snapshot.pets, req.id);
      },
      async getPetActions(req) {
        const pet = findByID(snapshot.pets, req.id);
        if (pet == null) {
          throw new Error("pet not found");
        }
        return {
          default_locale: "en",
          actions: [
            { cost: 0, id: "idle", pixa_clip_name: "idle", visual_clip_id: "idle" },
            { cost: 5, id: "bath", pixa_clip_name: "bath", visual_clip_id: "bath" },
          ],
          i18n: {
            en: {
              actions: {
                idle: { name: "Idle" },
                bath: { name: "Bath" },
              },
            },
          },
          pet_id: pet.id,
          petdef_id: pet.petdef_id,
          petdef_updated_at: "2026-07-01T00:00:00Z",
        };
      },
      async getPoints() {
        return snapshot.points;
      },
      async getPointsTransaction(req) {
        return findByID(snapshot.pointsTransactions, req.id);
      },
      async getRewardGrant(req) {
        return findByID(snapshot.grants, req.id);
      },
      async loadSnapshot() {
        return snapshot;
      },
      async listBadges() {
        return pageResponse(snapshot.badges);
      },
      async listGameResults() {
        return pageResponse(snapshot.gameResults);
      },
      async listPets() {
        return pageResponse(snapshot.pets);
      },
      async listPointsTransactions() {
        return pageResponse(snapshot.pointsTransactions);
      },
      async listRewardGrants() {
        return pageResponse(snapshot.grants);
      },
      async playHistory(historyID) {
        actions.push(`play:${historyID}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { accepted: true };
      },
      async putPet(req) {
        const pet = findByID(snapshot.pets, req.id);
        if (pet != null && req.display_name != null) {
          pet.display_name = String(req.display_name);
        }
        return pet;
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
  await page.goto("/play.html");

  await expect(page.getByText("OpenAI Gateway")).toBeVisible();
  await expect(page.getByRole("button", { name: /Workspaces/ })).toBeVisible();
  await expect(page.getByText("ACL-controlled resources are listed in the resource sections.")).toBeVisible();
  await expect(page.getByRole("button", { name: /Models 1/ })).toBeVisible();

  await page.getByRole("button", { name: /Workspaces/ }).click();
  await expect(page.getByRole("heading", { name: "Workspaces" })).toBeVisible();
  await expect(page.getByText("flowcraft-chat").first()).toBeVisible();

  await page.getByRole("button", { name: /Friends/ }).click();
  await expect(page.getByRole("heading", { name: "Friends" })).toBeVisible();
  await expect(page.getByText("peer-b").first()).toBeVisible();

  await page.getByRole("button", { name: /Firmwares/ }).click();
  await expect(page.getByRole("heading", { name: "Firmwares" })).toBeVisible();
  await expect(page.getByText("devkit-firmware-main")).toBeVisible();

  await page.getByRole("button", { name: /Workflows/ }).click();
  await expect(page.getByRole("heading", { name: "Workflows" })).toBeVisible();
  const workflowRow = page.getByRole("row").filter({ hasText: "flowcraft-chat" });
  await expect(workflowRow).toContainText("Flowcraft Chat Assistant");
  await expect(workflowRow).toContainText("flowcraft");
  await expect(workflowRow).toContainText("General-purpose chat workflow");
});

test("play workspace drawer sends direct RPC-backed actions", async ({ page }) => {
  await page.goto("/play.html");

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
  await page.goto("/play.html");

  await page.getByRole("button", { name: /^Workspace$/ }).click();
  await page.getByRole("tab", { name: "History" }).click();
  await expect(page.getByText("你好，开始测试。")).toBeVisible();

  const firstHistoryRow = page.getByRole("row").filter({ hasText: "你好，开始测试。" });
  await firstHistoryRow.getByRole("button", { name: "Play" }).click();

  await expect.poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? [])).toContain("play:20260701T000000Z-1");
});

test("play gameplay panel adopts and drives pets through peer RPC", async ({ page }) => {
  await page.goto("/play.html");

  await page.getByRole("button", { name: /Gameplay/ }).click();
  await expect(page.getByText("default-gameplay").first()).toBeVisible();
  await expect(page.getByText("Starter Pet").first()).toBeVisible();
  await expect(page.getByText("badge-basic").first()).toBeVisible();

  await page.getByPlaceholder("Display name").fill("Test Pet");
  await page.getByRole("button", { name: "Adopt Pet" }).click();
  await expect(page.getByText("Test Pet")).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? [])).toContain("adopt:Test Pet");

  await page.getByPlaceholder("Action").fill("bath");
  await page.getByPlaceholder("Game ID").fill("game-basic");
  await page.getByPlaceholder("Score", { exact: true }).fill("42");
  await page.getByPlaceholder("Max score").fill("100");
  await page.getByPlaceholder("Difficulty").fill("normal");
  await page.getByPlaceholder("Outcome").fill("win");
  await page.getByPlaceholder("Duration ms").fill("1200");
  await page.getByPlaceholder("Idempotency key").fill("ui-e2e-result-1");
  await page.getByRole("button", { name: "Drive" }).click();

  await expect(page.getByText("ui-e2e-result-1")).toBeVisible();
  await expect(page.getByText("game-result-1").first()).toBeVisible();
  await expect(page.getByText("game_result").first()).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? [])).toContain("drive:pet-main:bath");
});

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__?: string[];
  }
}
