import assert from "node:assert/strict";
import test from "node:test";

import type { SyncEvent } from "./index.ts";

import {
  GizClawControlError,
  classifyGizClawControlError,
  createGizClawControlClient,
  type GizClawControlErrorKind,
} from "./index.ts";

const apiKey = "gizclaw_sk_v1_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG";
const baseUrl = "https://ap.gizclaw.com";

const apiKeyJson = {
  name: "key_0123456789abcdefghijkl",
  display_name: "LiteLink",
  prefix: "gizclaw_sk_v1_0123",
  api_key: apiKey,
  manage_api_keys: true,
  created_at: "2026-09-03T01:02:03Z",
};

const contactJson = {
  name: "alice",
  display_name: "Alice",
  phone_number: "+8613800000000",
};

interface Seen {
  method: string;
  url: URL;
  headers: Headers;
  body: string;
}

type Answer = (seen: Seen) => Response | Promise<Response>;

function json(status: number, body: unknown): Answer {
  return () =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "content-type": "application/json" },
    });
}

function noContent(): Answer {
  return () => new Response(null, { status: 204 });
}

function syncAnswer(events: SyncEvent[]): Answer {
  return () =>
    new Response(
      events
        .map(
          (event) =>
            `event: ${event.event}\ndata: ${JSON.stringify(event)}\n\n`,
        )
        .join(""),
      { headers: { "content-type": "text/event-stream" } },
    );
}

async function collectSync(
  events: AsyncGenerator<SyncEvent>,
): Promise<SyncEvent[]> {
  const result: SyncEvent[] = [];
  for await (const event of events) result.push(event);
  return result;
}

test("sync sends the owner API key and checkpoint and consumes finite SSE events", async () => {
  const events: SyncEvent[] = [
    { event: "reset" },
    {
      event: "upsert",
      key: "/gizclaw/v1/device/runtime",
      data: { online: true, last_seen_at: "2026-10-07T00:00:00Z" },
    },
    { event: "delete", key: "/gizclaw/v1/contacts/alice" },
    { event: "done", timestamp: 1791331200000 },
  ];
  const h = harness([syncAnswer(events)]);
  assert.deepEqual(await collectSync(h.client.sync(1791331199999)), events);
  const seen = h.single();
  assert.equal(seen.method, "GET");
  assert.equal(seen.url.pathname, "/gizclaw/v1/sync");
  assert.equal(seen.url.searchParams.get("timestamp"), "1791331199999");
  assert.equal(seen.headers.get("authorization"), `Bearer ${apiKey}`);
});

test("sync rejects HTTP errors with the existing control error classification and no retry", async () => {
  const h = harness([
    errorResponse(401, "INVALID_API_KEY", "revoked", {
      "x-request-id": "sync-request",
    }),
  ]);
  await assert.rejects(collectSync(h.client.sync()), (error) => {
    assert.ok(error instanceof GizClawControlError);
    assert.equal(error.kind, "unauthorized");
    assert.equal(error.code, "INVALID_API_KEY");
    assert.equal(error.requestId, "sync-request");
    return true;
  });
  h.single();
});

test("sync treats an incomplete batch as a failure and permits retry from the completed checkpoint", async () => {
  const change: SyncEvent = {
    event: "upsert",
    key: "/gizclaw/v1/device",
    data: { name: "Changed" },
  };
  const h = harness([
    syncAnswer([change]),
    syncAnswer([{ event: "done", timestamp: 1001 }]),
  ]);
  const committed = new Map<string, unknown>([
    ["/gizclaw/v1/device", { name: "Original" }],
  ]);
  const staged = new Map(committed);
  let timestamp = 1000;
  await assert.rejects(
    (async () => {
      for await (const event of h.client.sync(timestamp)) {
        if (event.event === "upsert") staged.set(event.key, event.data);
        if (event.event === "done") timestamp = event.timestamp;
      }
    })(),
    /ended without done/u,
  );
  assert.equal(timestamp, 1000);
  assert.deepEqual(committed.get("/gizclaw/v1/device"), { name: "Original" });
  assert.deepEqual(await collectSync(h.client.sync(timestamp)), [
    { event: "done", timestamp: 1001 },
  ]);
  assert.equal(h.seen[1]!.url.searchParams.get("timestamp"), "1000");
});

test("sync rejects malformed events and an out-of-order reset", async () => {
  const malformed = harness([
    () =>
      new Response('event: upsert\ndata: {"event":"upsert"}\n\n', {
        headers: { "content-type": "text/event-stream" },
      }),
  ]);
  await assert.rejects(
    collectSync(malformed.client.sync()),
    /invalid SSE event/u,
  );
  const reset = harness([
    syncAnswer([
      { event: "delete", key: "/gizclaw/v1/contacts/alice" },
      { event: "reset" },
      { event: "done", timestamp: 1000 },
    ]),
  ]);
  await assert.rejects(
    collectSync(reset.client.sync()),
    /reset must be the first event/u,
  );
});

test("sync surfaces a network failure without automatically replaying a partial batch", async () => {
  const h = harness([
    () => {
      throw new Error("disconnected");
    },
  ]);
  await assert.rejects(collectSync(h.client.sync()), (error) => {
    assert.ok(error instanceof GizClawControlError);
    assert.equal(error.kind, "network");
    assert.match(error.message, /disconnected/u);
    return true;
  });
  h.single();
});

test("sync validates timestamps and cancels the fetch when iteration stops", async () => {
  const h = harness([]);
  for (const value of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1, NaN]) {
    await assert.rejects(collectSync(h.client.sync(value)), TypeError);
  }
  assert.equal(h.seen.length, 0);
  let signal: AbortSignal | undefined;
  const client = createGizClawControlClient({
    baseUrl,
    apiKey,
    fetch: async (input, init) => {
      signal = new Request(input, init).signal;
      return new Response('event: reset\ndata: {"event":"reset"}\n\n', {
        headers: { "content-type": "text/event-stream" },
      });
    },
  });
  for await (const event of client.sync()) {
    assert.equal(event.event, "reset");
    break;
  }
  assert.equal(signal?.aborted, true);
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(collectSync(client.sync(0, controller.signal)), {
    name: "AbortError",
  });
});

function accepted(): Answer {
  return () => new Response(null, { status: 202 });
}

function errorResponse(
  status: number,
  code: string,
  message = "failed",
  headers: Record<string, string> = {},
): Answer {
  return () =>
    new Response(JSON.stringify({ error: { code, message } }), {
      status,
      headers: { "content-type": "application/json", ...headers },
    });
}

function harness(answers: Answer[], base = baseUrl) {
  const seen: Seen[] = [];
  const queue = [...answers];
  const fetchStub: typeof fetch = async (input, init) => {
    const request = new Request(input, init);
    seen.push({
      method: request.method,
      url: new URL(request.url),
      headers: request.headers,
      body: await request.text(),
    });
    const next = queue.shift();
    if (next === undefined) {
      throw new Error(`no answer queued for ${request.url}`);
    }
    return next(seen[seen.length - 1]!);
  };
  const client = createGizClawControlClient({
    baseUrl: base,
    apiKey,
    fetch: fetchStub,
  });
  return {
    client,
    seen,
    single(): Seen {
      assert.equal(seen.length, 1);
      return seen[0]!;
    },
  };
}

async function failure(
  promise: Promise<unknown>,
): Promise<GizClawControlError> {
  try {
    await promise;
  } catch (error) {
    assert.ok(
      error instanceof GizClawControlError,
      `expected GizClawControlError, got ${String(error)}`,
    );
    return error;
  }
  assert.fail("expected GizClawControlError");
}

test("rejects an empty API key and a non-http base URL", () => {
  assert.throws(
    () => createGizClawControlClient({ baseUrl, apiKey: "" }),
    TypeError,
  );
  assert.throws(
    () => createGizClawControlClient({ baseUrl: "ftp://x", apiKey }),
    TypeError,
  );
});

test("rejects a plaintext base URL unless it is allowed", () => {
  assert.throws(
    () =>
      createGizClawControlClient({ apiKey, baseUrl: "http://ap.gizclaw.com" }),
    /must use https/u,
  );
  // A local test deployment opts in explicitly.
  const control = createGizClawControlClient({
    allowInsecureTransport: true,
    apiKey,
    baseUrl: "http://127.0.0.1:9821",
  });
  assert.ok(control.client != null);
});

test("sends the bearer header on every request", async () => {
  const h = harness([
    json(200, apiKeyJson),
    noContent(),
    json(200, { tools: [] }),
  ]);
  await h.client.apiKeys.getSelf();
  await h.client.apiKeys.revokeSelf();
  await h.client.device.listTools();
  assert.equal(h.seen.length, 3);
  for (const request of h.seen) {
    assert.equal(request.headers.get("authorization"), `Bearer ${apiKey}`);
  }
  assert.equal(h.seen[0]!.method, "GET");
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/api-keys/self");
  assert.equal(h.seen[1]!.method, "DELETE");
  assert.equal(h.seen[1]!.url.pathname, "/gizclaw/v1/api-keys/self");
});

test("joins a base URL path prefix without doubling slashes", async () => {
  const h = harness(
    [json(200, { online: true, last_seen_at: "2026-09-03T00:00:00Z" })],
    "https://example.test/prefix/",
  );
  const runtime = await h.client.device.getRuntime();
  assert.equal(
    h.single().url.href,
    "https://example.test/prefix/gizclaw/v1/device/runtime",
  );
  assert.equal(runtime.online, true);
});

test("api keys: create posts the body and decodes 201", async () => {
  const h = harness([json(201, { value: apiKeyJson, api_key: apiKey })]);
  const result = await h.client.apiKeys.create({
    display_name: "LiteLink",
    manage_api_keys: false,
  });
  const request = h.single();
  assert.equal(request.method, "POST");
  assert.equal(request.url.pathname, "/gizclaw/v1/api-keys");
  assert.match(
    request.headers.get("content-type") ?? "",
    /^application\/json/u,
  );
  assert.deepEqual(JSON.parse(request.body), {
    display_name: "LiteLink",
    manage_api_keys: false,
  });
  assert.equal(result.api_key, apiKey);
  assert.equal(result.value.name, apiKeyJson.name);
});

test("api keys: list passes cursor and limit, named routes encode the name", async () => {
  const h = harness([
    json(200, { items: [apiKeyJson] }),
    json(200, apiKeyJson),
    noContent(),
  ]);
  const list = await h.client.apiKeys.list({
    cursor: "key_0123456789abcdefghijkl",
    limit: 10,
  });
  await h.client.apiKeys.get("key_0123456789abcdefghijkl");
  await h.client.apiKeys.revoke("key_0123456789abcdefghijkl");
  assert.equal(list.items.length, 1);
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/api-keys");
  assert.deepEqual(Object.fromEntries(h.seen[0]!.url.searchParams), {
    cursor: "key_0123456789abcdefghijkl",
    limit: "10",
  });
  assert.equal(
    h.seen[1]!.url.pathname,
    "/gizclaw/v1/api-keys/key_0123456789abcdefghijkl",
  );
  assert.equal(h.seen[2]!.method, "DELETE");
  assert.equal(
    h.seen[2]!.url.pathname,
    "/gizclaw/v1/api-keys/key_0123456789abcdefghijkl",
  );
});

test("device reads: get, status, telemetry", async () => {
  const h = harness([
    json(200, { name: "Kitchen", firmware: { version: "9" } }),
    json(200, { volume: 35, muted: false, future_field: 1 }),
    json(200, { peer_public_key: "pk", values: [] }),
    json(200, { peer_public_key: "pk", values: [] }),
    json(200, {
      peer_public_key: "pk",
      field: "battery.percent",
      start_time_ms: 0,
      end_time_ms: 1000,
      step_ms: 100,
      points: [{ observed_at_unix_ms: 0, value: 1.5 }],
    }),
    json(200, {
      peer_public_key: "pk",
      field: "battery.percent",
      aggregate: "avg",
      bucket_ms: 60000,
      points: [],
    }),
  ]);
  const device = await h.client.device.get();
  const status = await h.client.device.getStatus();
  await h.client.device.getTelemetryLatest("battery.percent");
  await h.client.device.getTelemetryLatest("network.rssi_dbm");
  const range = await h.client.device.queryTelemetry({
    field: "battery.percent",
    start_time_ms: 0,
    end_time_ms: 1000,
    step_ms: 100,
    limit: 5,
    order: "desc",
  });
  await h.client.device.aggregateTelemetry({
    field: "battery.percent",
    start_time_ms: 0,
    end_time_ms: 1000,
    bucket_ms: 60000,
    aggregate: "avg",
  });

  assert.equal(device.name, "Kitchen");
  assert.deepEqual((device as Record<string, unknown>).firmware, {
    version: "9",
  });
  assert.equal(status.volume, 35);
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device");
  assert.equal(h.seen[1]!.url.pathname, "/gizclaw/v1/device/status");
  assert.equal(
    h.seen[2]!.url.pathname,
    "/gizclaw/v1/device/telemetry/battery.percent/latest",
  );
  assert.equal(h.seen[2]!.url.search, "");
  assert.equal(
    h.seen[3]!.url.pathname,
    "/gizclaw/v1/device/telemetry/network.rssi_dbm/latest",
  );
  assert.equal(h.seen[3]!.url.search, "");
  assert.equal(h.seen[4]!.url.pathname, "/gizclaw/v1/device/telemetry");
  assert.deepEqual(Object.fromEntries(h.seen[4]!.url.searchParams), {
    field: "battery.percent",
    start_time_ms: "0",
    end_time_ms: "1000",
    step_ms: "100",
    limit: "5",
    order: "desc",
  });
  assert.equal(range.points[0]!.value, 1.5);
  assert.equal(
    h.seen[5]!.url.pathname,
    "/gizclaw/v1/device/telemetry/aggregate",
  );
  assert.deepEqual(Object.fromEntries(h.seen[5]!.url.searchParams), {
    field: "battery.percent",
    start_time_ms: "0",
    end_time_ms: "1000",
    bucket_ms: "60000",
    aggregate: "avg",
  });
});

test("device control: MHS HWD and typed procedures", async () => {
  const h = harness([
    json(200, {
      id: "speaker.main",
      hwd: "speaker",
      value: { volume_percent: 35 },
    }),
    json(200, { result: {} }),
    json(200, { result: {} }),
    json(200, { result: {} }),
    json(200, { result: { networks: [{ ssid: "Home" }] } }),
    json(200, { result: {} }),
  ]);
  const volume = await h.client.device.writeMhsHwd({
    id: "speaker.main",
    hwd: "speaker",
    value: { volume_percent: 35 },
  });
  await h.client.device.playSound({ sound: "chime", duration_ms: 500 });
  await h.client.device.reboot();
  await h.client.device.reboot({ delay_ms: 3000 });
  const saved = await h.client.device.listSavedWifi();
  await h.client.device.forgetSavedWifi("Café Wi-Fi/5G #2");
  assert.equal(volume.value.volume_percent, 35);
  assert.equal(saved.networks[0]!.ssid, "Home");
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/mhs/v0/write");
  assert.deepEqual(JSON.parse(h.seen[0]!.body), {
    id: "speaker.main",
    hwd: "speaker",
    value: { volume_percent: 35 },
  });
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    tool: "sound.play",
    args: { sound: "chime", duration_ms: 500 },
  });
});

test("device find: ring time and device default", async () => {
  const h = harness([json(200, { result: {} }), json(200, { result: {} })]);
  await h.client.device.find({ duration_ms: 8000 });
  await h.client.device.find();
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/tool/v0/invoke");
  assert.equal(h.seen[0]!.headers.get("authorization"), `Bearer ${apiKey}`);
  assert.deepEqual(JSON.parse(h.seen[0]!.body), {
    tool: "device.find",
    args: { duration_ms: 8000 },
  });
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    tool: "device.find",
    args: {},
  });
});

test("device find: an unsupported device surfaces DEVICE_UNSUPPORTED", async () => {
  const h = harness([
    errorResponse(501, "DEVICE_UNSUPPORTED", "device does not support find"),
  ]);
  await assert.rejects(h.client.device.find(), (error: unknown) => {
    assert.ok(error instanceof GizClawControlError);
    assert.equal(error.status, 501);
    assert.equal(error.kind, "deviceUnsupported");
    assert.equal(error.code, "DEVICE_UNSUPPORTED");
    return true;
  });
});

test("device wifi: scan and connect through tool/v0", async () => {
  const h = harness([
    json(200, {
      result: {
        networks: [
          {
            ssid: "Office",
            rssi_dbm: -42,
            frequency_mhz: 5180,
            security: "wpa3",
          },
        ],
      },
    }),
    json(200, { result: {} }),
    json(200, { result: {} }),
  ]);
  const scan = await h.client.device.scanWifi({ timeout_ms: 8000 });
  await h.client.device.connectWifi({
    ssid: "Office",
    passphrase: "correct-horse",
  });
  await h.client.device.connectWifi({ ssid: "Open Network" });
  assert.equal(scan.networks[0]!.ssid, "Office");
  assert.equal(scan.networks[0]!.rssi_dbm, -42);
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/tool/v0/invoke");
  assert.deepEqual(JSON.parse(h.seen[0]!.body), {
    tool: "wifi.scan",
    args: { timeout_ms: 8000 },
  });
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    tool: "wifi.connect",
    args: { ssid: "Office", passphrase: "correct-horse" },
  });
  assert.deepEqual(JSON.parse(h.seen[2]!.body), {
    tool: "wifi.connect",
    args: { ssid: "Open Network" },
  });
});

test("rejects an empty path parameter before sending", async () => {
  const h = harness([]);
  await assert.rejects(h.client.device.forgetSavedWifi(""), TypeError);
  await assert.rejects(h.client.contacts.get(""), TypeError);
  assert.equal(h.seen.length, 0);
});

test("device workspaces: filters, alias identity, delete", async () => {
  const workspace = {
    id: "ws-aesop",
    name: "aesop-save",
    workflow_name: "story.aesop",
    available: true,
    system: false,
    created_at: "2026-09-01T08:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    last_active_at: "2026-09-01T10:00:00Z",
  };
  const h = harness([
    json(200, [workspace]),
    json(200, []),
    accepted(),
    errorResponse(409, "SYSTEM_WORKSPACE_DELETE_FORBIDDEN"),
  ]);
  const items = await h.client.device.listWorkspaces({
    workflow_name: "story.aesop",
  });
  await h.client.device.listWorkspaces();
  await h.client.device.deleteWorkspace("ws/aesop");
  const error = await failure(h.client.device.deleteWorkspace("ws-pet"));

  assert.deepEqual(items, [workspace]);
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/workspaces");
  assert.deepEqual(Object.fromEntries(h.seen[0]!.url.searchParams), {
    workflow_name: "story.aesop",
  });
  assert.equal(h.seen[1]!.url.search, "");
  assert.equal(h.seen[2]!.method, "DELETE");
  assert.equal(
    h.seen[2]!.url.pathname,
    "/gizclaw/v1/device/workspaces/ws%2Faesop",
  );
  assert.equal(error.status, 409);
  assert.equal(error.code, "SYSTEM_WORKSPACE_DELETE_FORBIDDEN");
  await assert.rejects(h.client.device.deleteWorkspace(""), TypeError);
});

test("contacts: list, create, named routes", async () => {
  const h = harness([
    json(200, { items: [contactJson], has_next: true, next_cursor: "alice" }),
    json(201, contactJson),
    json(200, contactJson),
    json(200, contactJson),
    noContent(),
  ]);
  const list = await h.client.contacts.list({ cursor: "aaron", limit: 1 });
  await h.client.contacts.create({ name: "alice", display_name: "Alice" });
  await h.client.contacts.get("爱丽丝/1");
  await h.client.contacts.put("爱丽丝/1", { phone_number: "+8613900000000" });
  await h.client.contacts.delete("爱丽丝/1");

  assert.equal(list.has_next, true);
  assert.deepEqual(Object.fromEntries(h.seen[0]!.url.searchParams), {
    cursor: "aaron",
    limit: "1",
  });
  assert.equal(h.seen[1]!.method, "POST");
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    name: "alice",
    display_name: "Alice",
  });
  const encoded = "/gizclaw/v1/contacts/%E7%88%B1%E4%B8%BD%E4%B8%9D%2F1";
  assert.equal(h.seen[2]!.url.pathname, encoded);
  assert.equal(h.seen[3]!.method, "PUT");
  assert.equal(h.seen[3]!.url.pathname, encoded);
  assert.deepEqual(JSON.parse(h.seen[3]!.body), {
    phone_number: "+8613900000000",
  });
  assert.equal(h.seen[4]!.method, "DELETE");
  assert.equal(h.seen[4]!.url.pathname, encoded);
});

const friendJson = {
  name: "7hwyLuAApKGgUbVCd8xYnLEiugfniMFfAsZ7RH8cPYAM",
  peer_public_key: "7hwyLuAApKGgUbVCd8xYnLEiugfniMFfAsZ7RH8cPYAM",
  workspace_name: "social-direct-1",
  created_at: "2026-09-12T01:02:03Z",
  updated_at: "2026-09-12T01:02:03Z",
  info: { display_name: "Kitchen", emoji: "🔊" },
};

const inviteTokenJson = {
  invite_token: "0123456789abcdef",
  expires_at: "2026-09-19T01:02:03Z",
};

test("friends: invite token ttl, add, list, named routes", async () => {
  const h = harness([
    errorResponse(404, "INVITE_TOKEN_NOT_FOUND"),
    json(200, inviteTokenJson),
    json(200, inviteTokenJson),
    noContent(),
    json(201, friendJson),
    errorResponse(409, "FRIEND_ALREADY_EXISTS"),
    json(200, { items: [friendJson], has_next: false }),
    json(200, friendJson),
    noContent(),
  ]);
  const missing = await h.client.friends.getInviteToken().catch((e) => e);
  assert.ok(missing instanceof GizClawControlError);
  assert.equal(missing.kind, "notFound");
  assert.equal(missing.code, "INVITE_TOKEN_NOT_FOUND");
  await h.client.friends.createInviteToken();
  const token = await h.client.friends.createInviteToken({
    ttl_seconds: 604800,
  });
  await h.client.friends.clearInviteToken();
  const friend = await h.client.friends.add({ invite_token: "0123" });
  const duplicate = await h.client.friends
    .add({ invite_token: "0123" })
    .catch((e) => e);
  const list = await h.client.friends.list({ limit: 10 });
  await h.client.friends.get(friendJson.name);
  await h.client.friends.delete(friendJson.name);

  assert.equal(h.seen[1]!.method, "POST");
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {});
  assert.deepEqual(JSON.parse(h.seen[2]!.body), { ttl_seconds: 604800 });
  assert.equal(token.invite_token, "0123456789abcdef");
  assert.equal(h.seen[3]!.method, "DELETE");
  assert.equal(h.seen[3]!.url.pathname, "/gizclaw/v1/friends/invite-token");
  assert.equal(friend.info?.display_name, "Kitchen");
  assert.equal(duplicate.kind, "conflict");
  assert.equal(duplicate.code, "FRIEND_ALREADY_EXISTS");
  assert.equal(list.items.length, 1);
  assert.equal(h.seen[6]!.url.searchParams.get("limit"), "10");
  assert.equal(
    h.seen[7]!.url.pathname,
    `/gizclaw/v1/friends/${friendJson.name}`,
  );
  assert.equal(h.seen[8]!.method, "DELETE");
  await assert.rejects(h.client.friends.get(""), TypeError);
});

test("friend groups: join, leave, members, invite token", async () => {
  const groupJson = { name: "家/1", my_role: "owner" };
  const memberJson = {
    name: friendJson.name,
    peer_public_key: friendJson.name,
    role: "member",
    info: { display_name: "Kitchen" },
  };
  const h = harness([
    json(200, { items: [groupJson], has_next: false }),
    json(201, groupJson),
    json(200, { group: groupJson, member: memberJson }),
    json(200, groupJson),
    json(200, groupJson),
    json(200, inviteTokenJson),
    json(200, inviteTokenJson),
    noContent(),
    json(200, { items: [memberJson], has_next: false }),
    json(201, memberJson),
    json(200, memberJson),
    noContent(),
    errorResponse(409, "FRIEND_GROUP_OWNER_CANNOT_LEAVE"),
    errorResponse(403, "FRIEND_GROUP_PERMISSION_DENIED"),
  ]);
  const groups = h.client.friendGroups;
  await groups.list({ cursor: "c", limit: 2 });
  await groups.create({ name: "家/1", display_name: "Family" });
  const joined = await groups.join({ invite_token: "abc", name: "家/1" });
  await groups.get("家/1");
  await groups.put("家/1", { description: "weekend" });
  await groups.getInviteToken("家/1");
  await groups.createInviteToken("家/1", { ttl_seconds: 3600 });
  await groups.clearInviteToken("家/1");
  const members = await groups.listMembers("家/1", { limit: 5 });
  await groups.addMember("家/1", {
    peer_public_key: friendJson.name,
    member_name: "kids",
    role: "member",
  });
  await groups.putMember("家/1", friendJson.name, { role: "admin" });
  await groups.deleteMember("家/1", friendJson.name);
  const leave = await groups.leave("家/1").catch((e) => e);
  const dissolve = await groups.delete("家/1").catch((e) => e);

  const encoded = "/gizclaw/v1/friend-groups/%E5%AE%B6%2F1";
  assert.equal(h.seen[2]!.url.pathname, "/gizclaw/v1/friend-groups/@join");
  assert.equal(joined.member.info?.display_name, "Kitchen");
  assert.equal(h.seen[3]!.url.pathname, encoded);
  assert.equal(h.seen[4]!.method, "PUT");
  assert.equal(h.seen[5]!.url.pathname, `${encoded}/invite-token`);
  assert.deepEqual(JSON.parse(h.seen[6]!.body), { ttl_seconds: 3600 });
  assert.equal(h.seen[7]!.method, "DELETE");
  assert.equal(members.items[0]!.role, "member");
  assert.equal(h.seen[8]!.url.pathname, `${encoded}/members`);
  assert.equal(h.seen[9]!.method, "POST");
  assert.equal(
    h.seen[10]!.url.pathname,
    `${encoded}/members/${friendJson.name}`,
  );
  assert.equal(h.seen[11]!.method, "DELETE");
  assert.equal(h.seen[12]!.url.pathname, `${encoded}/@leave`);
  assert.equal(leave.kind, "conflict");
  assert.equal(leave.code, "FRIEND_GROUP_OWNER_CANNOT_LEAVE");
  assert.equal(dissolve.kind, "forbidden");
  await assert.rejects(groups.deleteMember("家/1", ""), TypeError);
});

const cases: Array<[number, string, GizClawControlErrorKind]> = [
  [401, "UNAUTHORIZED", "unauthorized"],
  [403, "FORBIDDEN", "forbidden"],
  [404, "NOT_FOUND", "notFound"],
  [409, "DEVICE_OFFLINE", "deviceOffline"],
  [504, "DEVICE_TIMEOUT", "deviceTimeout"],
  [400, "DEVICE_REJECTED", "deviceRejected"],
  [501, "DEVICE_UNSUPPORTED", "deviceUnsupported"],
  [502, "DEVICE_ERROR", "deviceError"],
  [409, "PENDING_DELETION", "conflict"],
  [400, "INVALID_ARGUMENT", "invalidRequest"],
  [500, "INTERNAL", "server"],
  [503, "UNAVAILABLE", "server"],
  [418, "TEAPOT", "unexpectedStatus"],
];

for (const [status, code, kind] of cases) {
  test(`error mapping: ${status} ${code} -> ${kind}`, async () => {
    const h = harness([
      errorResponse(status, code, "m", { "x-request-id": "req-1" }),
    ]);
    const error = await failure(h.client.device.find());
    assert.equal(error.kind, kind);
    assert.equal(error.status, status);
    assert.equal(error.code, code);
    assert.equal(error.requestId, "req-1");
    assert.equal(error.message, "invokeClientTool: m");
    assert.equal(error.name, "GizClawControlError");
  });
}

test("error mapping: classifies on status alone when the body is not an error", async () => {
  const h = harness([() => new Response("gateway", { status: 502 })]);
  const error = await failure(h.client.device.get());
  assert.equal(error.kind, "server");
  assert.equal(error.status, 502);
  assert.equal(error.code, undefined);
  assert.equal(error.requestId, undefined);
  assert.equal(error.message, "getDevice: HTTP 502");
});

test("error mapping: keeps details", async () => {
  const h = harness([
    () =>
      new Response(
        JSON.stringify({
          error: {
            code: "DEVICE_REJECTED",
            message: "unknown sound",
            details: { sound: "nope" },
          },
        }),
        { status: 400 },
      ),
  ]);
  const error = await failure(h.client.device.playSound({ sound: "nope" }));
  assert.equal(error.kind, "deviceRejected");
  assert.deepEqual(error.details, { sound: "nope" });
});

test("error mapping: transport failures become network", async () => {
  const h = harness([
    () => {
      throw new TypeError("fetch failed");
    },
  ]);
  const error = await failure(h.client.device.get());
  assert.equal(error.kind, "network");
  assert.equal(error.status, undefined);
  assert.ok(error.cause instanceof TypeError);
});

test("classifyGizClawControlError: device codes win over status", () => {
  assert.equal(
    classifyGizClawControlError(409, "DEVICE_OFFLINE"),
    "deviceOffline",
  );
  assert.equal(classifyGizClawControlError(409), "conflict");
  assert.equal(classifyGizClawControlError(504, "UPSTREAM"), "server");
  assert.equal(classifyGizClawControlError(302), "unexpectedStatus");
});

test("GNSS reporting control invokes get/set and returns the device value", async () => {
  const h = harness([
    json(200, { result: { enabled: true } }),
    json(200, { result: { enabled: false } }),
    json(200, { result: { enabled: false } }),
  ]);
  assert.equal(await h.client.device.getGnssReporting(), true);
  assert.equal(await h.client.device.setGnssReporting(false), false);
  assert.equal(await h.client.device.setGnssReporting(true), false);
  assert.deepEqual(
    h.seen.map((request) => JSON.parse(request.body)),
    [
      { tool: "gnss.reporting.get", args: {} },
      { tool: "gnss.reporting.set", args: { enabled: false } },
      { tool: "gnss.reporting.set", args: { enabled: true } },
    ],
  );
  assert.ok(
    h.seen.every(
      (request) =>
        request.method === "POST" &&
        request.url.pathname === "/gizclaw/v1/device/tool/v0/invoke" &&
        request.headers.get("authorization") === `Bearer ${apiKey}`,
    ),
  );
});

test("GNSS reporting control rejects absent or nonboolean device values", async () => {
  for (const result of [
    {},
    { enabled: null },
    { enabled: 0 },
    { enabled: "false" },
  ]) {
    const h = harness([json(200, { result })]);
    await assert.rejects(h.client.device.getGnssReporting());
  }
  const h = harness([]);
  assert.throws(
    () => h.client.device.setGnssReporting(undefined as never),
    TypeError,
  );
  assert.equal(h.seen.length, 0);
});

test("audioplayer procedures preserve playlist order and explicit zero index", async () => {
  const status = {
    state: "buffering",
    current_index: 0,
    position_ms: 0,
    repeat: "all",
    playlist_length: 1,
    playlist_revision: 3,
    observed_at_unix_ms: 1700000000000,
  };
  const items = [
    {
      url: "https://media.example/music.mp3",
      title: "music",
      source_ref: "catalog/song",
    },
  ];
  const h = harness([
    json(200, { result: status }),
    json(200, { result: { items, playlist_revision: 3 } }),
    ...Array.from({ length: 5 }, () => json(200, { result: status })),
  ]);
  assert.deepEqual((await h.client.device.getAudioPlayer()).status, status);
  assert.deepEqual(
    (await h.client.device.getAudioPlayerPlaylist()).items,
    items,
  );
  await h.client.device.setAudioPlayerPlaylist({ items });
  await h.client.device.appendAudioPlayerPlaylist({ items });
  await h.client.device.playAudioPlayer({ index: 0 });
  await h.client.device.stopAudioPlayer();
  await h.client.device.setAudioPlayerMode({ repeat: "all" });
  assert.ok(
    h.seen.every(
      (request) => request.url.pathname === "/gizclaw/v1/device/tool/v0/invoke",
    ),
  );
  assert.deepEqual(JSON.parse(h.seen[2]!.body), {
    tool: "audioplayer.playlist.set",
    args: { items },
  });
  assert.deepEqual(JSON.parse(h.seen[4]!.body), {
    tool: "audioplayer.play",
    args: { index: 0 },
  });
});

test("MHS HWD, reset, tool list and Workspace switch", async () => {
  const h = harness([
    json(200, {
      id: "display.main",
      hwd: "display",
      value: { brightness_percent: 0 },
    }),
    json(200, {
      id: "display.main",
      hwd: "display",
      value: { enabled: false },
    }),
    json(200, { result: {} }),
    json(200, { tools: ["device.factory_reset", "run.workspace.set"] }),
    json(200, { result: {} }),
  ]);
  const read = await h.client.device.readMhsHwd({
    id: "display.main",
    hwd: "display",
  });
  const written = await h.client.device.writeMhsHwd({
    id: "display.main",
    hwd: "display",
    value: { enabled: false },
  });
  assert.equal(read.value.brightness_percent, 0);
  assert.equal(written.value.enabled, false);
  await h.client.device.factoryReset({ keep_network: true });
  const tools = await h.client.device.listTools();
  await h.client.device.setRunWorkspace({
    workflow_name: "bedtime",
    kickoff: true,
  });
  assert.deepEqual(tools.tools, ["device.factory_reset", "run.workspace.set"]);
  assert.deepEqual(JSON.parse(h.seen[2]!.body), {
    tool: "device.factory_reset",
    args: { keep_network: true },
  });
  assert.deepEqual(JSON.parse(h.seen[4]!.body), {
    tool: "run.workspace.set",
    args: { workflow_name: "bedtime", kickoff: true },
  });
});

test("device tools: list installed procedures and invoke a typed tool", async () => {
  const h = harness([
    json(200, { tools: ["device.find"] }),
    json(200, { result: {} }),
    errorResponse(501, "DEVICE_UNSUPPORTED"),
  ]);
  const tools = await h.client.device.listTools();
  assert.deepEqual(tools.tools, ["device.find"]);
  await h.client.device.find({ duration_ms: 8000 });
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/tool/v0/tools");
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    tool: "device.find",
    args: { duration_ms: 8000 },
  });
  const error = await failure(h.client.device.playSound({ sound: "chime" }));
  assert.equal(error.kind, "deviceUnsupported");
});

test("MHS control uses generated HWD routes and preserves device errors", async () => {
  const value = {
    id: "led.status",
    hwd: "led",
    value: { enabled: false },
  } as const;
  const { client, seen } = harness([
    json(200, { devices: [{ id: "led.status", hwd: "led" }] }),
    json(200, value),
    json(200, value),
    errorResponse(404, "MHS_HWD_NOT_FOUND"),
  ]);
  assert.deepEqual(await client.device.getMhsManifest(), {
    devices: [{ id: "led.status", hwd: "led" }],
  });
  assert.deepEqual(
    await client.device.readMhsHwd({ id: "led.status", hwd: "led" }),
    value,
  );
  assert.deepEqual(await client.device.writeMhsHwd(value), value);
  assert.deepEqual(
    seen.slice(0, 3).map((r) => [r.method, r.url.pathname]),
    [
      ["GET", "/gizclaw/v1/device/mhs/v0/manifest"],
      ["POST", "/gizclaw/v1/device/mhs/v0/read"],
      ["POST", "/gizclaw/v1/device/mhs/v0/write"],
    ],
  );
  assert.deepEqual(JSON.parse(seen[2]!.body), value);
  assert.ok(
    seen.every((r) => r.headers.get("Authorization") === `Bearer ${apiKey}`),
  );
  await assert.rejects(
    client.device.readMhsHwd({ id: "led.status", hwd: "led" }),
    (error: unknown) =>
      error instanceof GizClawControlError &&
      error.kind === "notFound" &&
      error.code === "MHS_HWD_NOT_FOUND",
  );
});

test("Lua archive control sends one raw body with metadata and no retry", async () => {
  const result = { app: { app_id: "demo", version: "1.0.0" } };
  const h = harness([json(200, result)]);
  assert.deepEqual(
    await h.client.installLuaApp(
      new Blob(["*".repeat(524288)]),
      "a".repeat(64),
    ),
    result,
  );
  const seen = h.single();
  assert.equal(seen.method, "POST");
  assert.equal(seen.url.pathname, "/gizclaw/v1/device/lua-app/install");
  assert.equal(seen.url.searchParams.get("content_length"), "524288");
  assert.equal(seen.headers.get("content-type"), "application/octet-stream");
  assert.equal(seen.body.length, 524288);
});

test("Lua Binary control rejects oversize before sending and classifies 413", () => {
  const client = createGizClawControlClient({
    baseUrl,
    apiKey,
    fetch: async () => {
      assert.fail("oversized request was sent");
    },
  });
  assert.throws(
    () =>
      client.installLuaApp(new Blob([new Uint8Array(524289)]), "a".repeat(64)),
    /512 KiB.*URL/,
  );
  assert.equal(
    classifyGizClawControlError(413, "LUA_APP_PACKAGE_TOO_LARGE"),
    "invalidRequest",
  );
});
