import assert from "node:assert/strict";
import test from "node:test";

import type { PeerRPCClient } from "@gizclaw/gizclaw/rpc";

import { createRPCPlayDataClient } from "./gizclaw/play.ts";

test("snapshot keeps workspaces and workflows when a fixed collection is absent", async () => {
  const workspaceCalls: Array<Record<string, unknown>> = [];
  const workflowCalls: Array<Record<string, unknown>> = [];
  const rpc = {
    call: async (method: string, params: Record<string, unknown>) => {
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
              alias: "assistant-first",
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
            alias: "assistant-second",
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
      (workspace) => (workspace.raw as { alias: string }).alias,
    ),
    ["assistant-first", "assistant-second"],
  );
  assert.deepEqual(snapshot.runtimeProfiles?.workspaces, {
    runtime_profile_name: "default",
    runtime_profile_revision: "revision-a",
  });
  assert.deepEqual(
    snapshot.workflows.map(({ alias, driver, id, title }) => ({
      alias,
      driver,
      id,
      title,
    })),
    [
      {
        alias: "assistant-first",
        driver: "flowcraft",
        id: "assistant-first",
        title: "First assistant",
      },
      {
        alias: "assistant-second",
        driver: "flowcraft",
        id: "assistant-second",
        title: "Second assistant",
      },
    ],
  );
  assert.deepEqual(
    snapshot.workflows.map(
      (workflow) => (workflow.raw as { alias: string }).alias,
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
      if (
        method !== "server.workspace.list" &&
        method !== "server.workflow.list"
      )
        return { items: [] };
      return {
        items: [{ alias: `${String(params.collection)}-item` }],
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
