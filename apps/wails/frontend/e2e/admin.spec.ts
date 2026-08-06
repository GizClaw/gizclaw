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
    const pageResponse = (items) => ({
      has_next: false,
      items,
      next_cursor: null,
    });
    const adminFetchPaths: string[] = [];
    const adminFetchRequests: Array<{
      body?: unknown;
      method: string;
      path: string;
    }> = [];
    const json = (data) =>
      new Response(JSON.stringify(data), {
        headers: { "content-type": "application/json" },
        status: 200,
      });
    const ogg = () =>
      new Response(new Uint8Array([0x4f, 0x67, 0x67, 0x53]), {
        headers: { "content-type": "audio/ogg" },
        status: 200,
      });
    const friend = {
      id: "peer-a:peer-b",
      owner_public_key: "peer-a",
      peer_public_key: "peer-b",
      workspace_id: "workspace-friend-000",
    };
    const firmware = {
      created_at: "2026-07-01T00:00:00Z",
      description: "Devkit firmware line",
      id: "devkit-firmware-main",
      slots: {
        beta: {},
        develop: {},
        pending: {},
        stable: {
          description: "stable package",
          package: {
            sha256:
              "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
            size: 4096,
            url: "https://firmware.example.invalid/devkit/stable.tar.zlib",
          },
        },
      },
      updated_at: "2026-07-01T00:00:00Z",
    };
    const data = {
      "/credentials": pageResponse([
        {
          body: { api_key: "set" },
          id: "fake-openai-credential-000",
          provider: "openai",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/dashscope-tenants": pageResponse([
        {
          credential_id: "dashscope-credential",
          id: "dashscope-tenant",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/firmwares": pageResponse([firmware]),
      "/firmwares/devkit-firmware-main": firmware,
      "/resources/Firmware/devkit-firmware-main": {
        apiVersion: "gizclaw.admin/v1alpha1",
        kind: "Firmware",
        metadata: { id: "devkit-firmware-main" },
        spec: {
          description: firmware.description,
          slots: firmware.slots,
        },
      },
      "/gemini-tenants": pageResponse([
        {
          credential_id: "gemini-credential",
          id: "gemini-tenant",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/minimax-tenants": pageResponse([
        {
          credential_id: "minimax-credential",
          group_id: "minimax-group",
          id: "minimax-tenant",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/models": pageResponse([
        {
          id: "fake-openai-chat-000",
          kind: "chat",
          display_name: "Fake OpenAI chat model",
          provider: { kind: "openai-tenant", id: "openai-tenant" },
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/openai-tenants": pageResponse([
        {
          credential_id: "openai-credential",
          id: "openai-tenant",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/peers": pageResponse([
        {
          auto_registered: false,
          created_at: "2026-07-01T00:00:00Z",
          public_key: "peer-public-key-1",
          role: "client",
          status: "active",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/peers/peer-public-key-1": {
        approved_at: "2026-07-01T00:00:00Z",
        auto_registered: false,
        created_at: "2026-07-01T00:00:00Z",
        public_key: "peer-public-key-1",
        role: "client",
        status: "active",
        updated_at: "2026-07-01T00:00:00Z",
      },
      "/peers/peer-public-key-1/info": {
        emoji: "📍",
        name: "Telemetry Peer",
      },
      "/peers/peer-public-key-1/runtime": {
        last_addr: "127.0.0.1:9820",
        last_seen_at: "2026-07-01T00:00:00Z",
        online: true,
        rx_bytes: 1024,
        tx_bytes: 2048,
      },
      "/server-info": {
        build_commit: "test-build",
        public_key: "server-public-key",
      },
      "/social/contacts": pageResponse([
        {
          id: "contact-admin",
          name: "Admin Contact",
          owner_public_key: "peer-public-key-1",
        },
      ]),
      "/social/friend-groups": pageResponse([
        {
          id: "group-main",
          name: "Main Group",
          my_role: "owner",
          workspace_name: "group-workspace",
        },
      ]),
      "/social/friends": pageResponse([friend]),
      "/social/friends/peer-a/peer-a:peer-b": friend,
      "/workspaces/workspace-friend-000/history": pageResponse([
        {
          created_at: "2026-07-01T00:00:00Z",
          id: "20260701T000000Z-1",
          name: "transcript",
          replay_available: true,
          text: "你好，开始测试。",
          type: "gear",
        },
      ]),
      "/voices": pageResponse([
        {
          id: "volc-voice-000",
          display_name: "Volc Voice",
          provider: { kind: "volc-tenant", id: "volc-tenant" },
          source: "sync",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/volc-tenants": pageResponse([
        {
          credential_id: "volc-credential",
          id: "volc-tenant",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
      "/workflows": pageResponse([
        {
          id: "openai-chat",
          spec: { driver: "flowcraft" },
        },
      ]),
      "/workspaces": pageResponse([
        {
          id: "main-workspace",
          name: "primary-workspace",
          workflow_id: "openai-chat",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ]),
    };
    window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_PATHS__ = adminFetchPaths;
    window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH__ = async (input) => {
      const request =
        typeof input === "string" ? new Request(input) : input.clone();
      const url = new URL(request.url);
      const path = decodeURIComponent(url.pathname);
      adminFetchPaths.push(path);
      const body =
        request.method === "GET" || request.method === "HEAD"
          ? undefined
          : await request.json();
      adminFetchRequests.push({ body, method: request.method, path });
      window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_REQUESTS__ = adminFetchRequests;
      if (
        request.method === "PUT" &&
        path === "/firmwares/devkit-firmware-main"
      ) {
        Object.assign(firmware, body, {
          id: firmware.id,
          updated_at: "2026-07-01T00:00:01Z",
        });
        data["/resources/Firmware/devkit-firmware-main"] = {
          apiVersion: "gizclaw.admin/v1alpha1",
          kind: "Firmware",
          metadata: { id: firmware.id },
          spec: {
            description: firmware.description,
            slots: firmware.slots,
          },
        };
        return json(firmware);
      }
      if (
        path ===
        "/workspaces/workspace-friend-000/history/20260701T000000Z-1/audio.ogg"
      ) {
        return ogg();
      }
      if (path === "/peers/peer-public-key-1/telemetry/latest") {
        return json({
          peer_public_key: "peer-public-key-1",
          values: [
            {
              field: "battery.percent",
              observed_at_unix_ms: 1782864000000,
              value: 82,
            },
          ],
        });
      }
      if (path === "/peers/peer-public-key-1/telemetry") {
        const field = url.searchParams.get("field");
        const points =
          field === "gnss.latitude"
            ? [
                { observed_at_unix_ms: 1782864000000, value: 39.9042 },
                { observed_at_unix_ms: 1782864060000, value: 39.9052 },
              ]
            : field === "gnss.longitude"
              ? [
                  { observed_at_unix_ms: 1782864000000, value: 116.4074 },
                  { observed_at_unix_ms: 1782864060000, value: 116.4094 },
                ]
              : [
                  { observed_at_unix_ms: 1782864000000, value: 80 },
                  { observed_at_unix_ms: 1782864060000, value: 82 },
                ];
        return json({
          end_time_ms: 1782864060000,
          field,
          peer_public_key: "peer-public-key-1",
          points,
          start_time_ms: 1782864000000,
          step_ms: 60000,
        });
      }
      if (path === "/peers/peer-public-key-1/telemetry/aggregate") {
        return json({
          aggregate: url.searchParams.get("aggregate"),
          bucket_ms: Number(url.searchParams.get("bucket_ms")),
          field: url.searchParams.get("field"),
          peer_public_key: "peer-public-key-1",
          points: [{ bucket_start_time_ms: 1782864000000, value: 81 }],
        });
      }
      return json(data[path] ?? pageResponse([]));
    };
  });
});

test("admin view renders full resource manager pages", async ({ page }) => {
  await page.goto("/admin.html");

  await expect(page.getByRole("heading", { name: "Dashboard" })).toBeVisible();
  await expect(page.getByText("test-build")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Peers" }).first(),
  ).toBeVisible();

  await page.getByRole("button", { name: "Peers" }).first().click();
  await expect(page.getByRole("heading", { name: "Peers" })).toBeVisible();
  await expect(page.getByText("peer-public-key-1")).toBeVisible();

  await page.getByRole("button", { name: "Workflows" }).click();
  await expect(page.getByRole("heading", { name: "Workflows" })).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Display name" }),
  ).toHaveCount(0);
  await expect(page.getByText("openai-chat").first()).toBeVisible();

  await page.getByRole("button", { name: "Firmwares" }).click();
  await expect(page.getByRole("heading", { name: "Firmwares" })).toBeVisible();
  await expect(page.getByText("devkit-firmware-main")).toBeVisible();

  await page.getByRole("button", { name: "Friends" }).click();
  await expect(page.getByRole("heading", { name: "Friends" })).toBeVisible();
  await expect(page.getByText("peer-a <-> peer-b")).toBeVisible();
});

test("admin firmware editor persists and renders an exact channel package", async ({
  page,
}) => {
  await page.goto("/admin.html");
  await page.getByRole("button", { name: "Firmwares" }).click();
  await page.getByText("devkit-firmware-main").click();
  await expect(
    page.getByRole("heading", { name: "devkit-firmware-main" }),
  ).toBeVisible();

  await page.getByRole("tab", { name: "Edit" }).click();
  await page.getByRole("button", { name: "Edit beta slot" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox").nth(0).fill("beta package");
  await dialog
    .getByRole("textbox")
    .nth(1)
    .fill("https://firmware.example.invalid:/devkit/beta.tar.zlib");
  await dialog
    .getByRole("textbox")
    .nth(2)
    .fill("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789");
  await dialog.getByRole("spinbutton").fill("8192");
  await dialog.getByRole("button", { name: "Save Slot" }).click();
  await expect(
    dialog.getByText("Package requires a valid HTTPS URL"),
  ).toBeVisible();
  await dialog
    .getByRole("textbox")
    .nth(1)
    .fill("https://firmware.example.invalid/devkit/beta.tar.zlib");
  await dialog.getByRole("button", { name: "Save Slot" }).click();

  await expect
    .poll(() =>
      page.evaluate(() =>
        (window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_REQUESTS__ ?? []).find(
          (request) =>
            request.method === "PUT" &&
            request.path === "/firmwares/devkit-firmware-main",
        ),
      ),
    )
    .toMatchObject({
      body: {
        slots: {
          beta: {
            description: "beta package",
            package: {
              sha256:
                "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
              size: 8192,
              url: "https://firmware.example.invalid/devkit/beta.tar.zlib",
            },
          },
        },
      },
    });

  const firmwarePutBody = await page.evaluate(
    () =>
      (window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_REQUESTS__ ?? []).find(
        (request) =>
          request.method === "PUT" &&
          request.path === "/firmwares/devkit-firmware-main",
      )?.body,
  );
  expect(firmwarePutBody).not.toHaveProperty("name");

  await page.getByRole("tab", { name: "Summary" }).click();
  await expect(
    page.getByText("https://firmware.example.invalid/devkit/beta.tar.zlib"),
  ).toBeVisible();
  await expect(page.getByText("8.0 KiB")).toBeVisible();
});

test("admin peer telemetry renders the MapLibre route", async ({ page }) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));

  await page.goto("/admin.html");
  await page.getByRole("button", { name: "Peers" }).first().click();
  await page.getByRole("link").filter({ hasText: "peer-public-key-1" }).click();
  await page.getByRole("tab", { name: "Telemetry" }).click();

  await expect(page.getByText("2 points")).toBeVisible();
  await expect(page.getByText("2 sampled points")).toBeVisible();
  await expect(page.locator("canvas.maplibregl-canvas")).toBeVisible();
  await expect(page.locator(".maplibregl-ctrl-zoom-in")).toBeVisible();
  await expect(page.locator(".maplibregl-ctrl-zoom-out")).toBeVisible();
  await expect(page.getByText("Map unavailable", { exact: true })).toHaveCount(
    0,
  );
  await expect(
    page.getByText("Map rendering unavailable", { exact: true }),
  ).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});

test("admin view covers provider, AI, social, and settings sections", async ({
  page,
}) => {
  await page.goto("/admin.html");

  await page.getByRole("button", { name: "Credentials" }).click();
  await expect(
    page.getByRole("heading", { name: "Credentials" }),
  ).toBeVisible();
  await expect(page.getByText("fake-openai-credential-000")).toBeVisible();

  await page.getByRole("button", { name: "OpenAI Tenants" }).click();
  await expect(
    page.getByRole("heading", { name: "OpenAI Tenants" }),
  ).toBeVisible();
  await expect(page.getByText("openai-tenant")).toBeVisible();

  await page.getByRole("button", { name: "Gemini Tenants" }).click();
  await expect(
    page.getByRole("heading", { name: "Gemini Tenants" }),
  ).toBeVisible();
  await expect(page.getByText("gemini-tenant")).toBeVisible();

  await page.getByRole("button", { name: "DashScope Tenants" }).click();
  await expect(
    page.getByRole("heading", { name: "DashScope Tenants" }),
  ).toBeVisible();
  await expect(page.getByText("dashscope-tenant")).toBeVisible();

  await page.getByRole("button", { name: "MiniMax Tenants" }).click();
  await expect(
    page.getByRole("heading", { name: "MiniMax Tenants" }),
  ).toBeVisible();
  await expect(page.getByText("minimax-tenant")).toBeVisible();

  await page.getByRole("button", { name: "Volcengine Tenants" }).click();
  await expect(
    page.getByRole("heading", { name: "Volcengine Tenants" }),
  ).toBeVisible();
  await expect(page.getByText("volc-tenant")).toBeVisible();

  await page.getByRole("button", { name: "Voices" }).click();
  await expect(page.getByRole("heading", { name: "Voices" })).toBeVisible();
  await expect(page.getByText("volc-voice-000")).toBeVisible();

  await page.getByRole("button", { name: "Models" }).click();
  await expect(page.getByRole("heading", { name: "Models" })).toBeVisible();
  await expect(page.getByText("fake-openai-chat-000")).toBeVisible();

  await page.getByRole("button", { name: "Workspaces" }).click();
  await expect(page.getByRole("heading", { name: "Workspaces" })).toBeVisible();
  await expect(page.getByText("primary-workspace")).toBeVisible();
  await expect(page.getByText("main-workspace")).toBeVisible();

  await page.getByRole("button", { name: "Contacts" }).click();
  await expect(page.getByRole("heading", { name: "Contacts" })).toBeVisible();
  await expect(
    page.getByRole("button", {
      exact: true,
      name: "peer-public-key-1:contact-admin",
    }),
  ).toBeVisible();

  await page.getByRole("button", { name: "Friend Groups" }).click();
  await expect(
    page.getByRole("heading", { name: "Friend Groups" }),
  ).toBeVisible();
  await expect(page.getByText("group-main")).toBeVisible();

  await page.getByRole("button", { name: "Resources" }).click();
  await expect(page.getByRole("heading", { name: "Resources" })).toBeVisible();
  const resourceJSON = page.getByRole("textbox").last();
  await page.getByRole("combobox").click();
  await page.getByRole("option", { name: "RuntimeProfile" }).click();
  await expect(resourceJSON).toHaveValue(/"kind": "RuntimeProfile"/);
  await expect(resourceJSON).toHaveValue(/"friend_chatroom": "chatroom"/);
  await expect(resourceJSON).toHaveValue(/"group_chatroom": "chatroom"/);
  await expect(resourceJSON).toHaveValue(/"pet": "pet-care"/);
  await expect(resourceJSON).toHaveValue(/"collections"/);
  await expect(resourceJSON).toHaveValue(/"resource_id": "general-chat"/);
  await expect(resourceJSON).toHaveValue(/"resource_id": "petdef-starter"/);
  await expect(resourceJSON).toHaveValue(/"reward-evaluator"/);
  await expect(resourceJSON).toHaveValue(/"workspace_reward"/);
  await expect(resourceJSON).toHaveValue(/"qualifying_score": 80/);
  await expect(resourceJSON).toHaveValue(/"rolling_budget"/);
  await expect(resourceJSON).toHaveValue(/"zh-CN"/);
  await page.getByRole("combobox").click();
  await page.getByRole("option", { name: "RegistrationToken" }).click();
  await expect(resourceJSON).toHaveValue(/"kind": "RegistrationToken"/);
  await expect(resourceJSON).not.toHaveValue(/"firmware_name"/);
  await expect(resourceJSON).toHaveValue(
    /"runtime_profile_id": "runtime-profile-default"/,
  );
  await expect(resourceJSON).not.toHaveValue(/"runtime_profile_name"/);
  await page.getByRole("combobox").click();
  await page.getByRole("option", { name: "Tool" }).click();
  await expect(resourceJSON).toHaveValue(/"kind": "Tool"/);
  await expect(resourceJSON).toHaveValue(/"invoke_name": "tool_example"/);
  await expect(resourceJSON).toHaveValue(/"type": "client_rpc"/);
  await expect(resourceJSON).not.toHaveValue(/"executor"/);
  await page.getByRole("combobox").click();
  await page.getByRole("option", { name: "PetDef" }).click();
  await expect(resourceJSON).toHaveValue(/"kind": "PetDef"/);
  await expect(resourceJSON).not.toHaveValue(/"default_locale"/);
  await expect(resourceJSON).not.toHaveValue(/"i18n"/);
  await page.getByRole("combobox").click();
  await page.getByRole("option", { name: "BadgeDef" }).click();
  await expect(resourceJSON).toHaveValue(/"kind": "BadgeDef"/);
  await expect(resourceJSON).toHaveValue(/"reward_prompt"/);
});

test("admin social friend detail loads workspace history and downloads audio", async ({
  page,
}) => {
  await page.goto("/admin.html");

  await page.getByRole("button", { name: "Friends" }).click();
  await page.getByRole("link", { name: /peer-a <-> peer-b/ }).click();

  await expect(page.getByRole("heading", { name: "peer-a" })).toBeVisible();
  await expect(page.getByText("workspace-friend-000").first()).toBeVisible();
  await expect(page.getByText("Workspace History")).toBeVisible();
  await expect(page.getByText("你好，开始测试。")).toBeVisible();

  await page
    .getByRole("row")
    .filter({ hasText: "你好，开始测试。" })
    .getByRole("button", { name: "Play" })
    .click();

  await expect
    .poll(() =>
      page.evaluate(
        () => window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_PATHS__ ?? [],
      ),
    )
    .toContain("/workspaces/workspace-friend-000/history");
  await expect
    .poll(() =>
      page.evaluate(
        () => window.__GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_PATHS__ ?? [],
      ),
    )
    .toContain(
      "/workspaces/workspace-friend-000/history/20260701T000000Z-1/audio.ogg",
    );
});

declare global {
  interface Window {
    __GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_PATHS__?: string[];
    __GIZCLAW_DESKTOP_TEST_ADMIN_FETCH_REQUESTS__?: Array<{
      body?: unknown;
      method: string;
      path: string;
    }>;
  }
}
