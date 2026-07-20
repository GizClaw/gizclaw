import assert from "node:assert/strict";
import test from "node:test";

import type { PeerRPCClient } from "@gizclaw/gizclaw/rpc";

import {
  clearPlayRPCClient,
  configurePlayRPCClient,
  listPeerWorkflows,
  listPeerWorkspaces,
} from "../views/play/full/peer-rpc-adapter.ts";

test("collection fan-out drains every workspace and workflow page", async (t) => {
  const calls: Array<{ method: string; params: Record<string, unknown> }> = [];
  const client = {
    call: async (method: string, params: Record<string, unknown>) => {
      calls.push({ method, params });
      const collection = String(params.collection);
      const cursor = params.cursor == null ? "" : String(params.cursor);
      if (method === "server.workflow.list" && collection === "role-play") {
        throw Object.assign(new Error("workflow collection not found"), { code: 404 });
      }
      const prefix = method === "server.workspace.list" ? "workspace" : "workflow";
      const paginated = collection === "assistants";
      return {
        has_next: paginated && cursor === "",
        items: paginated
          ? [{ name: `${prefix}-${cursor === "" ? "first" : "second"}`, alias: `${prefix}-${cursor === "" ? "first" : "second"}` }]
          : [],
        ...(paginated && cursor === "" ? { next_cursor: `${prefix}-cursor` } : {}),
        runtime_profile_name: "default",
        runtime_profile_revision: "revision-1",
      };
    },
  } as unknown as PeerRPCClient;
  configurePlayRPCClient(client);
  t.after(() => clearPlayRPCClient(client));

  const workspaces = await listPeerWorkspaces({ query: { limit: 50 } });
  const workflows = await listPeerWorkflows({ query: { limit: 50 } });

  assert.equal(workspaces.error, undefined);
  assert.deepEqual(workspaces.data?.items.map((item) => item.name), ["workspace-first", "workspace-second"]);
  assert.equal(workspaces.data?.has_next, false);
  assert.equal(workflows.error, undefined);
  assert.deepEqual(workflows.data?.items.map((item) => item.alias), ["workflow-first", "workflow-second"]);
  assert.equal(workflows.data?.has_next, false);
  assert.deepEqual(
    calls.filter((call) => call.params.collection === "assistants").map((call) => call.params.cursor ?? ""),
    ["", "workspace-cursor", "", "workflow-cursor"],
  );
  assert.equal(calls.some((call) => call.params.cursor === "workspace-cursor"), true);
  assert.equal(calls.some((call) => call.params.cursor === "workflow-cursor"), true);
});
