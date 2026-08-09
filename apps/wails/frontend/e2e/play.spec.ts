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
    window.__GIZCLAW_DESKTOP_TEST_RUNTIME__ = {
      context,
      private_key_base64: "cHJpdmF0ZS1rZXktbWF0ZXJpYWw=",
    };
    const actions: string[] = [];
    const snapshot = {
      badges: [
        {
          active: true,
          badge_def_name: "badge-basic",
          created_at: "2026-07-01T00:00:00Z",
          exp: 125,
          level: 1,
          name: "badge-basic",
          progress: 0.25,
          updated_at: "2026-07-01T00:00:00Z",
        },
      ],
      contacts: [
        {
          display_name: "Main Contact",
          name: "contact-main",
          title: "Main Contact",
        },
      ],
      credentials: [
        {
          body: { api_key: "fake-openai-api-key" },
          created_at: "2026-07-01T00:00:00Z",
          description: "Fake OpenAI credential",
          name: "fake-openai-credential-000",
          provider: "openai",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ],
      firmwares: [
        {
          id: "stable",
          channel: "stable",
          slots: {
            beta: {},
            develop: {},
            stable: { description: "stable" },
          },
          title: "Stable Firmware",
        },
      ],
      friendGroups: [
        {
          display_name: "Story Group",
          my_role: "member",
          name: "story-group",
          workspace_name: "story-group-workspace",
        },
      ],
      friends: [
        {
          name: "peer-b",
          peer_public_key: "peer-b",
          workspace_name: "friend-workspace",
        },
      ],
      gameResults: [],
      grants: [],
      history: [
        {
          created_at: "2026-07-01T00:00:00Z",
          name: "20260701T000000Z-1",
          actor_name: "transcript",
          replay_available: true,
          text: "你好，开始测试。",
          type: "gear",
          updated_at: "2026-07-01T00:00:00Z",
        },
        {
          created_at: "2026-07-01T00:00:01Z",
          name: "20260701T000001Z-2",
          actor_name: "answer",
          replay_available: true,
          text: "收到，我们继续。",
          type: "agent",
          updated_at: "2026-07-01T00:00:01Z",
        },
      ],
      memoryStats: { total: 2 },
      models: [
        {
          i18n: {},
          kind: "asr",
          name: "asr",
          provider_kind: "volc-tenant",
          volc_tenant: { api_mode: "asr", support_text_only: false },
        },
        {
          i18n: {},
          kind: "llm",
          name: "chat",
          openai_tenant: {
            default_thinking_level: "disabled",
            support_temperature: true,
            support_thinking: true,
            thinking_levels: ["enabled", "disabled", "auto"],
            thinking_param: "thinking.type",
          },
          provider_kind: "openai-tenant",
        },
      ],
      pets: [
        {
          created_at: "2026-07-01T00:00:00Z",
          display_name: "Starter Pet",
          last_active_at: "2026-07-01T00:00:00Z",
          lifecycle: "alive",
          name: "pet-main",
          pet_def_name: "petdef-basic",
          progression: { experience: 90, level: 3 },
          runtime_profile_name: "default-gameplay",
          state_settled_at: "2026-07-01T00:00:00Z",
          stats: {
            life: 100,
            health: 100,
            satiety: 90,
            hygiene: 80,
            mood: 100,
            energy: 100,
          },
          updated_at: "2026-07-01T00:00:00Z",
          workspace_name: "pet-pet-main",
        },
      ],
      points: {
        balance: 100,
        created_at: "2026-07-01T00:00:00Z",
        owner_public_key: "peer-main",
        runtime_profile_name: "default-gameplay",
        updated_at: "2026-07-01T00:00:00Z",
      },
      pointsTransactions: [],
      runWorkspace: {
        active_workspace_name: "flowcraft-chat",
        mode: "push",
        workspace_mode: "push",
        workspace_name: "flowcraft-chat",
      },
      voices: [
        {
          i18n: { en: { display_name: "Pet" } },
          name: "pet",
        },
      ],
      warnings: [],
      workflows: [
        {
          collection: "assistants",
          driver: "flowcraft",
          i18n: {},
          name: "flowcraft-chat",
        },
      ],
      workspaces: [
        {
          created_at: "2026-07-01T00:00:00Z",
          last_active_at: "2026-07-01T00:00:01Z",
          name: "flowcraft-chat",
          system: false,
          updated_at: "2026-07-01T00:00:01Z",
          workflow_name: "flowcraft-chat",
        },
      ],
    };
    const pageResponse = (items) => ({
      has_next: false,
      items,
      next_cursor: null,
    });
    const findByName = (items, name) =>
      items.find((item) => item.name === name) ?? null;
    window.__GIZCLAW_DESKTOP_TEST_PLAY_CLIENT__ = {
      async adoptPet(req) {
        const displayName = String(req.display_name ?? "Adopted Pet");
        const name = String(req.name ?? `pet-${snapshot.pets.length + 1}`);
        const pet = {
          created_at: "2026-07-01T00:00:02Z",
          display_name: displayName,
          last_active_at: "2026-07-01T00:00:02Z",
          lifecycle: "alive",
          name,
          pet_def_name: "petdef-basic",
          progression: { experience: 0, level: 1 },
          runtime_profile_name: "default-gameplay",
          state_settled_at: "2026-07-01T00:00:02Z",
          stats: {
            life: 100,
            health: 100,
            satiety: 100,
            hygiene: 100,
            mood: 100,
            energy: 100,
          },
          updated_at: "2026-07-01T00:00:02Z",
          workspace_name: `pet-${name}`,
        };
        snapshot.pets.push(pet);
        snapshot.points.balance -= 10;
        snapshot.pointsTransactions.push({
          balance_after: snapshot.points.balance,
          created_at: "2026-07-01T00:00:02Z",
          delta: -10,
          name: "txn-adopt-1",
          owner_public_key: "peer-main",
          pet_name: name,
          reason: "adopt",
          runtime_profile_name: "default-gameplay",
          source_name: name,
          source_type: "pet_adoption",
        });
        actions.push(`adopt:${displayName}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return pet;
      },
      async deletePet(req) {
        const index = snapshot.pets.findIndex((pet) => pet.name === req.name);
        const [pet] = index >= 0 ? snapshot.pets.splice(index, 1) : [];
        return pet ?? null;
      },
      async drivePet(req) {
        const pet = snapshot.pets.find((item) => item.name === req.pet_name);
        if (pet == null) {
          throw new Error("pet not found");
        }
        let gameResult = null;
        if (req.game_result != null) {
          pet.progression.experience += 20;
          pet.stats.energy -= 10;
          snapshot.points.balance -= 10;
          snapshot.pointsTransactions.push({
            balance_after: snapshot.points.balance,
            created_at: "2026-07-01T00:00:03Z",
            delta: -10,
            name: "txn-drive-1",
            owner_public_key: "peer-main",
            pet_name: req.pet_name,
            reason: "game.play",
            runtime_profile_name: "default-gameplay",
            source_name: "game-result-1",
            source_type: "game_result",
          });
          gameResult = {
            duration_ms: req.game_result.duration_ms,
            game_def_name: req.game_result.game_name,
            name: "game-result-1",
            idempotency_key: req.game_result.idempotency_key,
            max_score: req.game_result.max_score,
            occurred_at: "2026-07-01T00:00:03Z",
            outcome: req.game_result.outcome,
            pet_name: req.pet_name,
            runtime_profile_name: "default-gameplay",
            score: req.game_result.score,
          };
          snapshot.gameResults.push(gameResult);
        } else {
          pet.progression.experience += 2;
          pet.stats.energy -= 10;
          if (req.behavior === "feed")
            pet.stats.satiety = Math.min(100, pet.stats.satiety + 10);
          if (req.behavior === "bathe")
            pet.stats.hygiene = Math.min(100, pet.stats.hygiene + 10);
          if (req.behavior === "play")
            pet.stats.mood = Math.min(100, pet.stats.mood + 10);
          if (req.behavior === "heal")
            pet.stats.health = Math.min(100, pet.stats.health + 10);
        }
        snapshot.grants.push({
          created_at: "2026-07-01T00:00:03Z",
          name: `reward-grant-${snapshot.grants.length + 1}`,
          owner_public_key: "peer-main",
          pet_exp_delta: 20,
          pet_name: req.pet_name,
          points_delta: 0,
          reason: String(req.behavior ?? "game reward"),
          runtime_profile_name: "default-gameplay",
          source_name: gameResult?.name ?? String(req.pet_name),
          source_type: gameResult == null ? "pet_behavior" : "game_result",
        });
        actions.push(
          `drive:${req.pet_name}:${req.behavior ?? ""}:${req.idempotency_key ?? req.game_result?.idempotency_key ?? ""}`,
        );
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return {
          badges: [],
          game_result: gameResult,
          pet,
          points: snapshot.points,
          reward_grants: snapshot.grants.slice(-1),
          transactions:
            req.game_result == null
              ? []
              : snapshot.pointsTransactions.slice(-1),
        };
      },
      async getBadge(req) {
        return snapshot.badges.find((badge) => badge.name === req.name) ?? null;
      },
      async getGameResult(req) {
        return findByName(snapshot.gameResults, req.name);
      },
      async getFirmware(req) {
        const channel = String(req.channel);
        actions.push(`firmware:${channel}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return {
          channel,
          description: `${channel} channel package`,
          sha256:
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
          size: 4096 + ["stable", "beta", "develop"].indexOf(channel),
          url: `https://firmware.example.invalid/devkit/${channel}.tar.zlib`,
        };
      },
      async getPet(req) {
        return snapshot.pets.find((pet) => pet.name === req.name) ?? null;
      },
      async getPetActions(req) {
        const pet = snapshot.pets.find((item) => item.name === req.name);
        if (pet == null) {
          throw new Error("pet not found");
        }
        return {
          bindings: {
            feed: "idle",
            bathe: "bath",
            play: "idle",
            heal: "idle",
            idle: "idle",
            sick: "idle",
            dead: "idle",
          },
          clip_names: { idle: "idle", bath: "bath" },
          pet_name: pet.name,
          pet_def_name: pet.pet_def_name,
          pet_def_updated_at: "2026-07-01T00:00:00Z",
        };
      },
      async getPoints() {
        return snapshot.points;
      },
      async getPointsTransaction(req) {
        return findByName(snapshot.pointsTransactions, req.name);
      },
      async getRewardGrant(req) {
        return findByName(snapshot.grants, req.name);
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
      async playHistory(historyName) {
        actions.push(`play:${historyName}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return { accepted: true };
      },
      async putPet(req) {
        const pet = snapshot.pets.find((item) => item.name === req.name);
        if (pet != null && req.display_name != null) {
          pet.display_name = String(req.display_name);
        }
        return pet;
      },
      async recallMemory(query) {
        actions.push(`recall:${query}`);
        window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ = actions;
        return {
          hits: [
            {
              name: "memory-hit-1",
              score: 0.95,
              snippet: `Memory Hit: ${query}`,
              source_name: "memory-hit-1",
            },
          ],
        };
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
  await expect(
    page.getByText(
      "RuntimeProfile and owner resources are listed in the resource sections.",
    ),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: /Models 2/ })).toBeVisible();

  await page.getByRole("button", { name: /Workspaces/ }).click();
  await expect(page.getByRole("heading", { name: "Workspaces" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Display name" }),
  ).toHaveCount(0);
  await expect(page.getByText("flowcraft-chat").first()).toBeVisible();

  await page.getByRole("button", { name: /Models 2/ }).click();
  await expect(page.getByRole("heading", { name: "Models" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "chat" })).toContainText(
    "thinking.type",
  );

  await page.getByRole("button", { name: /Friends/ }).click();
  await expect(page.getByRole("heading", { name: "Friends" })).toBeVisible();
  await expect(page.getByText("peer-b").first()).toBeVisible();

  await page.getByRole("button", { name: /Firmwares/ }).click();
  await expect(page.getByRole("heading", { name: "Firmwares" })).toBeVisible();
  await expect(
    page.getByRole("row").filter({ hasText: "stable channel package" }),
  ).toContainText("stable");
  await expect(page.getByText("devkit-firmware-main")).toHaveCount(0);
  await expect(
    page.getByText("https://firmware.example.invalid/devkit/stable.tar.zlib"),
  ).toBeVisible();

  await page.getByRole("combobox", { name: "Firmware channel" }).click();
  await expect(page.getByRole("option", { name: "pen" + "ding" })).toHaveCount(
    0,
  );
  await page.getByRole("option", { name: "beta" }).click();
  await expect(
    page.getByText("https://firmware.example.invalid/devkit/beta.tar.zlib"),
  ).toBeVisible();
  await expect(page.getByText("beta channel package")).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("firmware:beta");

  await page.getByRole("button", { name: "Open", exact: true }).click();
  await expect(page.getByText("beta", { exact: true })).toBeVisible();
  await expect(page.getByText("4.0 KiB")).toBeVisible();
  await expect(
    page.getByText(
      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    ),
  ).toBeVisible();

  await page.getByRole("button", { name: /Workflows/ }).click();
  await expect(page.getByRole("heading", { name: "Workflows" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "Name" })).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Display name" }),
  ).toHaveCount(0);
  const workflowRow = page
    .getByRole("row")
    .filter({ hasText: "flowcraft-chat" });
  await expect(workflowRow).toContainText("flowcraft");
});

test("OpenAI tester keeps the thinking toggle consistent with the model level", async ({
  page,
}) => {
  await page.goto("/play.html");
  await page.getByRole("button", { name: "OpenAI", exact: true }).click();

  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("chat", { exact: true })).toBeVisible();
  await expect(drawer.getByText("asr", { exact: true })).toHaveCount(0);
  const thinking = drawer.getByRole("checkbox", { name: "Think" });
  await expect(thinking).not.toBeChecked();
  await expect(drawer.getByText("disabled", { exact: true })).toBeVisible();

  await thinking.check();
  await expect(thinking).toBeChecked();
  await expect(drawer.getByText("enabled", { exact: true })).toBeVisible();
});

test("play workspace drawer sends direct RPC-backed actions", async ({
  page,
}) => {
  await page.goto("/play.html");

  await page.getByRole("button", { name: /^Workspace$/ }).click();
  await expect(page.getByRole("heading", { name: "Workspace" })).toBeVisible();
  await page.getByRole("button", { name: /Reload/ }).click();
  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("reload");

  await page.getByRole("tab", { name: /Recall/ }).click();
  await page.getByPlaceholder("Recall query").fill("route");
  await page.getByRole("button", { name: "Run Recall" }).click();
  await expect(page.getByText("Memory Hit: route")).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("recall:route");
});

test("play workspace history replay uses direct peer RPC", async ({ page }) => {
  await page.goto("/play.html");

  await page.getByRole("button", { name: /^Workspace$/ }).click();
  await page.getByRole("tab", { name: "History" }).click();
  await expect(page.getByText("你好，开始测试。")).toBeVisible();

  const firstHistoryRow = page
    .getByRole("row")
    .filter({ hasText: "你好，开始测试。" });
  await firstHistoryRow.getByRole("button", { name: "Play" }).click();

  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("play:20260701T000000Z-1");
});

test("play gameplay panel adopts and drives pets through peer RPC", async ({
  page,
}) => {
  await page.goto("/play.html");

  await page.getByRole("button", { name: /Gameplay/ }).click();
  await expect(page.getByText("default-gameplay").first()).toBeVisible();
  await expect(page.getByText("Starter Pet").first()).toBeVisible();
  await expect(page.getByText("badge-basic").first()).toBeVisible();

  await expect(page.getByRole("button", { name: "Adopt Pet" })).toBeDisabled();
  await page.getByPlaceholder("Display name").fill("Test Pet");
  await expect(page.getByRole("button", { name: "Adopt Pet" })).toBeEnabled();
  await page.getByRole("button", { name: "Adopt Pet" }).click();
  await expect(
    page.getByRole("cell", { name: "Test Pet", exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("adopt:Test Pet");

  await page.getByPlaceholder("Behavior (feed/bathe/play/heal)").fill("bathe");
  await page.getByPlaceholder("Idempotency key").fill("ui-e2e-care-1");
  await page.getByRole("button", { name: "Drive" }).click();
  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("drive:pet-main:bathe:ui-e2e-care-1");

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
  await expect
    .poll(() =>
      page.evaluate(() => window.__GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__ ?? []),
    )
    .toContain("drive:pet-main::ui-e2e-result-1");
});

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_PLAY_ACTIONS__?: string[];
  }
}
