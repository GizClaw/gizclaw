import assert from "node:assert/strict";
import { test } from "node:test";

import { RunContext, type FunctionTool } from "@openai/agents-core";

import {
  ASSISTANT_APIS,
  HISTORY_LIMIT,
  LOG_RECORD_LIMIT,
} from "../src/apis.ts";
import { RUNTIME_METHODS } from "../src/runtime.ts";
import { createTools, type ActionRecord } from "../src/tools.ts";
import { FakeRuntime, type World } from "../testing/fake-runtime.ts";
import {
  BALCONY,
  KIDS_ROOM,
  LIVING_ROOM,
  SCENARIO_NOW,
  scenarios,
} from "../testing/scenarios.ts";

const baseWorld: World = scenarios[0].world;

function harness(world: Partial<World> = {}) {
  const runtime = new FakeRuntime({ ...baseWorld, ...world });
  const actions: ActionRecord[] = [];
  const tools = new Map<string, FunctionTool>(
    createTools({
      runtime,
      record: (action) => actions.push(action),
      now: () => SCENARIO_NOW,
    }).map((item) => [item.name, item]),
  );
  const call = async (name: string, args: unknown): Promise<any> => {
    const item = tools.get(name);
    assert.ok(item, `tool ${name} exists`);
    const output = await item.invoke(new RunContext(), JSON.stringify(args));
    return typeof output === "string" ? JSON.parse(output) : output;
  };
  return { runtime, actions, tools, call };
}

test("every runtime method is exposed by exactly one tool", () => {
  const used = ASSISTANT_APIS.flatMap((api) => api.uses);
  assert.deepEqual([...used].sort(), [...RUNTIME_METHODS].sort());
  assert.equal(
    new Set(ASSISTANT_APIS.map((api) => api.tool)).size,
    ASSISTANT_APIS.length,
  );
});

test("tools are generated one per catalog entry with non-strict schemas", () => {
  const { tools } = harness();
  assert.deepEqual(
    [...tools.keys()],
    ASSISTANT_APIS.map((api) => api.tool),
  );
  assert.deepEqual([...tools.keys()].sort(), [
    "find_device",
    "get_conversation_history",
    "get_current_page",
    "get_device_status",
    "get_device_telemetry",
    "get_device_wifi",
    "get_node_traffic",
    "list_device_workspaces",
    "list_devices",
    "list_nodes",
    "navigate",
    "open_link",
    "query_device_telemetry",
    "search_logs",
  ]);
  for (const item of tools.values()) {
    assert.equal(
      item.strict,
      false,
      `${item.name} declares a non-strict schema`,
    );
    assert.equal((item.parameters as { type: string }).type, "object");
  }
});

test("get_current_page reports the route, hash and page view", async () => {
  const { call } = harness({
    route: { page: "peer", publicKey: LIVING_ROOM },
    view: { page: "设备详情" },
  });
  assert.deepEqual(await call("get_current_page", {}), {
    route: { page: "peer", publicKey: LIVING_ROOM },
    hash: `#/peer/${LIVING_ROOM}`,
    view: { page: "设备详情" },
  });
});

test("navigate moves the page and validates required route fields", async () => {
  const { call, runtime, actions } = harness();
  assert.deepEqual(
    await call("navigate", {
      page: "server",
      node_id: "edge-bj",
      public_key: null,
      error_code: null,
    }),
    {
      navigated: true,
      route: { page: "server", id: "edge-bj" },
      hash: "#/server/edge-bj",
    },
  );
  assert.deepEqual(runtime.currentRoute, { page: "server", id: "edge-bj" });

  assert.equal(
    (await call("navigate", { page: "peer" })).error.code,
    "invalid_arguments",
  );
  assert.equal(
    (await call("navigate", { page: "somewhere" })).error.code,
    "invalid_arguments",
  );
  assert.equal(runtime.navigations.length, 1);
  assert.deepEqual(
    actions.map((action) => action.error?.code),
    [undefined, "invalid_arguments", "invalid_arguments"],
  );
});

test("navigate builds the console log query from structured filters", async () => {
  const { call, runtime } = harness();
  const result = await call("navigate", {
    page: "logs",
    public_key: LIVING_ROOM,
    error_code: "ASR_TIMEOUT",
    level: "error",
    text: "timed out",
  });
  const query = `peer_public_key:${LIVING_ROOM} error_code:ASR_TIMEOUT level:error timed out`;
  assert.deepEqual(result.route, { page: "logs", query });
  assert.deepEqual(runtime.currentRoute, { page: "logs", query });
  assert.deepEqual((await call("navigate", { page: "logs" })).route, {
    page: "logs",
  });
});

test("get_node_traffic returns the window and its range", async () => {
  const { call } = harness();
  const result = await call("get_node_traffic", {
    node_id: "edge-bj",
    window_minutes: 1,
  });
  assert.equal(result.samples.length, 13);
  assert.equal(result.summary.rx_bytes_per_second.last, 5_595_000);
  assert.equal(
    (await call("get_node_traffic", { node_id: "missing" })).error.code,
    "NOT_FOUND",
  );
});

test("query_device_telemetry reads a field over a window", async () => {
  const { call, runtime } = harness();
  const result = await call("query_device_telemetry", {
    public_key: BALCONY,
    field: "network.rssi_dbm",
    since_minutes: 180,
  });
  assert.deepEqual(
    result.points.map((point: { value: number }) => point.value),
    [-87, -88, -89],
  );
  assert.deepEqual(result.summary, {
    points: 3,
    min: -89,
    max: -87,
    average: -88,
    last: -89,
  });
  const [request] = runtime.called("devices.telemetryRange")[0].args as [
    { startMs: number; endMs: number },
  ];
  assert.equal(request.endMs - request.startMs, 180 * 60_000);
});

test("get_device_wifi and list_device_workspaces read the device", async () => {
  const { call } = harness();
  assert.deepEqual(await call("get_device_wifi", { public_key: BALCONY }), {
    status: {
      connected: true,
      ssid: "Home-5G",
      rssi_dbm: -89,
      ip: "192.168.1.42",
    },
    saved: ["Home-5G", "Home-2.4G"],
  });
  assert.equal(
    (await call("get_device_wifi", { public_key: KIDS_ROOM })).error.code,
    "DEVICE_UNSUPPORTED",
  );
  const { workspaces } = await call("list_device_workspaces", {
    public_key: LIVING_ROOM,
  });
  assert.deepEqual(
    workspaces.map((workspace: { id: string }) => workspace.id),
    ["ws-story"],
  );
});

test("get_conversation_history reads newest entries first", async () => {
  const { call, runtime } = harness();
  const result = await call("get_conversation_history", {
    public_key: LIVING_ROOM,
    workspace_id: "ws-story",
    text: "没听清",
  });
  assert.equal(result.entries.length, 3);
  assert.ok(
    result.entries.every(
      (entry: { actor_name: string }) => entry.actor_name === "assistant",
    ),
  );
  assert.ok(result.entries[0].created_at > result.entries[2].created_at);
  const [request] = runtime.called("devices.history")[0].args as [
    { order: string; limit: number },
  ];
  assert.equal(request.order, "desc");
  assert.equal(request.limit, HISTORY_LIMIT);
  assert.equal(
    (
      await call("get_conversation_history", {
        public_key: LIVING_ROOM,
        workspace_id: "missing",
      })
    ).error.code,
    "WORKSPACE_NOT_FOUND",
  );
});

test("open_link asks the user and only accepts http(s)", async () => {
  const accepted = harness();
  assert.deepEqual(
    await accepted.call("open_link", { url: "https://status.example.com/" }),
    { opened: true },
  );
  assert.equal(
    (await accepted.call("open_link", { url: "javascript:alert(1)" })).error
      .code,
    "invalid_arguments",
  );
  assert.equal(
    (await accepted.call("open_link", { url: "file:///etc/passwd" })).error
      .code,
    "invalid_arguments",
  );
  assert.deepEqual(accepted.runtime.links, ["https://status.example.com/"]);

  const declined = harness({ acceptLinks: false });
  assert.deepEqual(
    await declined.call("open_link", { url: "http://example.com" }),
    { opened: false },
  );
});

test("node and device tools read their sources", async () => {
  const { call } = harness();
  assert.equal((await call("list_nodes", {})).nodes.length, 3);
  assert.deepEqual(
    (await call("list_devices", {})).devices.map(
      (device: { label: string }) => device.label,
    ),
    ["客厅音箱", "儿童房故事机", "阳台音箱"],
  );
  assert.deepEqual(
    await call("find_device", {
      kind: "sn",
      value: "SN-BALCONY-03",
      serial: null,
    }),
    {
      public_keys: [BALCONY],
    },
  );
  assert.deepEqual(
    await call("find_device", { kind: "imei", value: "35", serial: "1" }),
    { public_keys: [] },
  );
  assert.equal(
    (await call("get_device_status", { public_key: BALCONY })).runtime
      .debug_mode,
    "readonly",
  );
  assert.deepEqual(
    await call("get_device_telemetry", {
      public_key: BALCONY,
      fields: ["network.rssi_dbm"],
    }),
    {
      values: [
        {
          field: "network.rssi_dbm",
          value: -89,
          observed_at_unix_ms: SCENARIO_NOW - 18 * 60_000,
        },
      ],
    },
  );
});

test("source failures reach the model as tool-result errors", async () => {
  const { call, actions } = harness();
  assert.deepEqual(await call("get_device_status", { public_key: KIDS_ROOM }), {
    error: {
      code: "DEBUG_ACCESS_FORBIDDEN",
      message: "device debug mode does not permit this operation",
    },
  });
  assert.equal(
    (await call("search_logs", { public_key: "missing" })).error.code,
    "NOT_FOUND",
  );
  assert.equal(
    (await call("get_device_status", {})).error.code,
    "invalid_arguments",
  );
  assert.deepEqual(
    actions.map((action) => [action.tool, action.error?.code]),
    [
      ["get_device_status", "DEBUG_ACCESS_FORBIDDEN"],
      ["search_logs", "NOT_FOUND"],
      ["get_device_status", "invalid_arguments"],
    ],
  );
});

test("search_logs aggregates, limits records and applies the time window", async () => {
  const { call, runtime } = harness();
  const all = await call("search_logs", { public_key: LIVING_ROOM });
  assert.equal(all.summary.total, 60);
  assert.equal(all.records.length, LOG_RECORD_LIMIT);
  assert.deepEqual(all.summary.top_errors[0], {
    error_code: "ASR_TIMEOUT",
    count: 17,
    last_message: "speech recognition timed out",
  });
  const [request] = runtime.called("logs.search")[0].args as [
    { sinceMs: number; untilMs: number },
  ];
  assert.equal(request.untilMs, SCENARIO_NOW);
  assert.equal(request.sinceMs, SCENARIO_NOW - 24 * 60 * 60_000);

  const recent = await call("search_logs", {
    public_key: LIVING_ROOM,
    level: "error",
    since_minutes: 60,
  });
  // Only the ASR timeout 30 minutes ago falls inside the last hour.
  assert.deepEqual(recent.summary.by_error_code, { ASR_TIMEOUT: 1 });
});
