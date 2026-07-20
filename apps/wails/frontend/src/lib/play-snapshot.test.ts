import assert from "node:assert/strict";
import test from "node:test";

import type { PeerRPCClient } from "@gizclaw/gizclaw/rpc";

import { createRPCPlayDataClient } from "./gizclaw/play.ts";

test("snapshot keeps workflows when a fixed collection is absent", async () => {
  const workflowCalls: Array<Record<string, unknown>> = [];
  const rpc = {
    call: async (method: string, params: Record<string, unknown>) => {
      if (method !== "server.workflow.list") {
        return { items: [] };
      }
      workflowCalls.push(params);
      if (params.collection === "role-play") {
        throw Object.assign(new Error("workflow collection not found"), { code: 404 });
      }
      if (params.collection !== "assistants") {
        return { items: [] };
      }
      if (params.cursor == null) {
        return {
          has_next: true,
          items: [{ alias: "assistant-first" }],
          next_cursor: "assistant-next",
        };
      }
      return { items: [{ alias: "assistant-second" }] };
    },
  } as unknown as PeerRPCClient;

  const snapshot = await createRPCPlayDataClient(rpc).loadSnapshot();

  assert.equal(snapshot.warnings.some((warning) => warning.startsWith("server.workflow.list:")), false);
  assert.deepEqual(
    snapshot.workflows.map((workflow) => (workflow.raw as { alias: string }).alias),
    ["assistant-first", "assistant-second"],
  );
  assert.deepEqual(
    workflowCalls.filter((call) => call.collection === "assistants").map((call) => call.cursor ?? ""),
    ["", "assistant-next"],
  );
});
