import assert from "node:assert/strict";
import test from "node:test";

import type { PeerRPCClient } from "@gizclaw/gizclaw/rpc";

import { createRPCPlayDataClient } from "./gizclaw/play.ts";

test("production Play data client requests the selected Firmware channel", async () => {
  const calls: Array<{ method: string; params: Record<string, unknown> }> = [];
  const response = {
    channel: "develop" as const,
    sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    size: 16384,
    url: "https://firmware.example.invalid/devkit/develop.tar.zlib",
  };
  const rpc = {
    call: async (method: string, params: Record<string, unknown>) => {
      calls.push({ method, params });
      return response;
    },
  } as unknown as PeerRPCClient;

  const got = await createRPCPlayDataClient(rpc).getFirmware({
    channel: "develop",
  });

  assert.deepEqual(got, response);
  assert.deepEqual(calls, [
    { method: "server.firmware.get", params: { channel: "develop" } },
  ]);
});

test("snapshot keeps workspaces and workflows when a fixed collection is absent", async () => {
  const workspaceCalls: Array<Record<string, unknown>> = [];
  const workflowCalls: Array<Record<string, unknown>> = [];
  const rpc = {
    call: async (method: string, params: Record<string, unknown>) => {
      if (method === "server.firmware.get") {
        return {
          channel: "stable",
          sha256:
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
          size: 4096,
          url: "https://firmware.example.invalid/devkit/stable.tar.zlib",
        };
      }
      if (
        method !== "server.workspace.list" &&
        method !== "server.workflow.list"
      ) {
        return { items: [] };
      }
      const calls =
        method === "server.workspace.list" ? workspaceCalls : workflowCalls;
      calls.push(params);
      if (params.collection === "role-play") {
        throw Object.assign(new Error(`${method} collection not found`), {
          code: 404,
        });
      }
      if (params.collection !== "assistants") {
        return { items: [] };
      }
      if (params.cursor == null) {
        return {
          has_next: true,
          items: [
            {
              name: "assistant-first",
              driver: "flowcraft",
              i18n: { en: { display_name: "First assistant" } },
            },
          ],
          next_cursor: "assistant-next",
          runtime_profile_name: "default",
          runtime_profile_revision: "revision-a",
        };
      }
      return {
        items: [
          {
            name: "assistant-second",
            driver: "flowcraft",
            i18n: { en: { display_name: "Second assistant" } },
          },
        ],
        runtime_profile_name: "default",
        runtime_profile_revision: "revision-a",
      };
    },
  } as unknown as PeerRPCClient;

  const snapshot = await createRPCPlayDataClient(rpc).loadSnapshot();

  assert.equal(
    snapshot.warnings.some((warning) =>
      warning.startsWith("server.workspace.list:"),
    ),
    false,
  );
  assert.equal(
    snapshot.warnings.some((warning) =>
      warning.startsWith("server.workflow.list:"),
    ),
    false,
  );
  assert.deepEqual(
    snapshot.workspaces.map(
      (workspace) => (workspace.raw as { name: string }).name,
    ),
    ["assistant-first", "assistant-second"],
  );
  assert.deepEqual(snapshot.runtimeProfiles?.workspaces, {
    runtime_profile_name: "default",
    runtime_profile_revision: "revision-a",
  });
  assert.deepEqual(
    snapshot.workflows.map(({ driver, id, title }) => ({
      driver,
      id,
      title,
    })),
    [
      {
        driver: "flowcraft",
        id: "assistant-first",
        title: "First assistant",
      },
      {
        driver: "flowcraft",
        id: "assistant-second",
        title: "Second assistant",
      },
    ],
  );
  assert.deepEqual(
    snapshot.workflows.map(
      (workflow) => (workflow.raw as { name: string }).name,
    ),
    ["assistant-first", "assistant-second"],
  );
  assert.deepEqual(
    workspaceCalls
      .filter((call) => call.collection === "assistants")
      .map((call) => call.cursor ?? ""),
    ["", "assistant-next"],
  );
  assert.deepEqual(
    workflowCalls
      .filter((call) => call.collection === "assistants")
      .map((call) => call.cursor ?? ""),
    ["", "assistant-next"],
  );
});

test("snapshot rejects mixed runtime profile revisions across collections", async () => {
  const rpc = {
    call: async (method: string, params: Record<string, unknown>) => {
      if (method === "server.firmware.get") {
        return {
          channel: "stable",
          sha256:
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
          size: 4096,
          url: "https://firmware.example.invalid/devkit/stable.tar.zlib",
        };
      }
      if (
        method !== "server.workspace.list" &&
        method !== "server.workflow.list"
      )
        return { items: [] };
      return {
        items: [{ name: `${String(params.collection)}-item` }],
        runtime_profile_name: "default",
        runtime_profile_revision:
          params.collection === "assistants" ? "revision-a" : "revision-b",
      };
    },
  } as unknown as PeerRPCClient;

  const snapshot = await createRPCPlayDataClient(rpc).loadSnapshot();

  assert.deepEqual(snapshot.workspaces, []);
  assert.deepEqual(snapshot.workflows, []);
  assert.equal(
    snapshot.warnings.filter((warning) =>
      warning.includes("runtime profile changed"),
    ).length,
    2,
  );
});

test("snapshot rejects Peer history without a name", async () => {
  const rpc = {
    call: async (method: string) => {
      if (method === "server.firmware.get") {
        return {
          channel: "stable",
          sha256:
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
          size: 4096,
          url: "https://firmware.example.invalid/devkit/stable.tar.zlib",
        };
      }
      if (method === "server.run.workspace.history") {
        return {
          items: [
            {
              actor_name: "gear-a",
              created_at: "2026-08-06T00:00:00Z",
              text: "missing identity",
              type: "gear",
            },
          ],
        };
      }
      return { items: [] };
    },
  } as unknown as PeerRPCClient;

  await assert.rejects(
    createRPCPlayDataClient(rpc).loadSnapshot(),
    /history: Peer response is missing name/,
  );
});
