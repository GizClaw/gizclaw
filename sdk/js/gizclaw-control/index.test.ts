import assert from "node:assert/strict";
import test from "node:test";

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
    json(200, { connected: false }),
  ]);
  await h.client.apiKeys.getSelf();
  await h.client.apiKeys.revokeSelf();
  await h.client.device.getWifi();
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

test("device control: volume, sound, reboot, wifi", async () => {
  const h = harness([
    json(200, { status: { volume: 35, muted: false } }),
    noContent(),
    noContent(),
    noContent(),
    json(200, { connected: true, ssid: "Home", rssi_dbm: -50 }),
    json(200, { networks: [{ ssid: "Home" }] }),
    noContent(),
  ]);
  const result = await h.client.device.setVolume({ level: 35, muted: false });
  await h.client.device.playSound({ sound: "chime", duration_ms: 500 });
  await h.client.device.reboot();
  await h.client.device.reboot({ delay_ms: 3000 });
  const wifi = await h.client.device.getWifi();
  const saved = await h.client.device.listSavedWifi();
  await h.client.device.forgetSavedWifi("Café Wi-Fi/5G #2");

  assert.equal(result.status.volume, 35);
  assert.equal(h.seen[0]!.method, "PUT");
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/volume");
  assert.deepEqual(JSON.parse(h.seen[0]!.body), { level: 35, muted: false });
  assert.equal(h.seen[1]!.method, "POST");
  assert.equal(
    h.seen[1]!.url.pathname,
    "/gizclaw/v1/device/actions/play-sound",
  );
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    sound: "chime",
    duration_ms: 500,
  });
  assert.equal(h.seen[2]!.url.pathname, "/gizclaw/v1/device/actions/reboot");
  assert.deepEqual(JSON.parse(h.seen[2]!.body), {});
  assert.deepEqual(JSON.parse(h.seen[3]!.body), { delay_ms: 3000 });
  assert.equal(wifi.rssi_dbm, -50);
  assert.equal(saved.networks[0]!.ssid, "Home");
  assert.equal(h.seen[4]!.url.pathname, "/gizclaw/v1/device/wifi");
  assert.equal(h.seen[5]!.url.pathname, "/gizclaw/v1/device/wifi/saved");
  assert.equal(h.seen[6]!.method, "DELETE");
  assert.equal(
    h.seen[6]!.url.pathname,
    "/gizclaw/v1/device/wifi/saved/Caf%C3%A9%20Wi-Fi%2F5G%20%232",
  );
});

test("device wifi: scan and connect", async () => {
  const h = harness([
    json(200, {
      networks: [
        {
          ssid: "Office",
          bssid: "aa:bb:cc:dd:ee:ff",
          rssi_dbm: -42,
          frequency_mhz: 5180,
          security: "wpa3",
        },
      ],
    }),
    accepted(),
    accepted(),
  ]);
  const scan = await h.client.device.scanWifi({ timeout_ms: 8000 });
  await h.client.device.connectWifi({
    ssid: "Office",
    passphrase: "correct-horse",
  });
  await h.client.device.connectWifi({ ssid: "Open Network" });

  assert.equal(h.seen[0]!.method, "POST");
  assert.equal(h.seen[0]!.url.pathname, "/gizclaw/v1/device/wifi/scan");
  assert.deepEqual(JSON.parse(h.seen[0]!.body), { timeout_ms: 8000 });
  assert.equal(scan.networks[0]!.ssid, "Office");
  assert.equal(scan.networks[0]!.rssi_dbm, -42);
  assert.equal(scan.networks[0]!.frequency_mhz, 5180);
  assert.equal(scan.networks[0]!.security, "wpa3");
  assert.equal(h.seen[1]!.method, "PUT");
  assert.equal(h.seen[1]!.url.pathname, "/gizclaw/v1/device/wifi");
  assert.deepEqual(JSON.parse(h.seen[1]!.body), {
    ssid: "Office",
    passphrase: "correct-horse",
  });
  assert.deepEqual(JSON.parse(h.seen[2]!.body), { ssid: "Open Network" });
});

test("rejects an empty path parameter before sending", async () => {
  const h = harness([]);
  await assert.rejects(h.client.device.forgetSavedWifi(""), TypeError);
  await assert.rejects(h.client.contacts.get(""), TypeError);
  assert.equal(h.seen.length, 0);
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
    const error = await failure(
      h.client.device.setVolume({ level: 1, muted: false }),
    );
    assert.equal(error.kind, kind);
    assert.equal(error.status, status);
    assert.equal(error.code, code);
    assert.equal(error.requestId, "req-1");
    assert.equal(error.message, "setDeviceVolume: m");
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

test("audioplayer routes preserve playlist order and explicit zero index", async () => {
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
    json(200, { status }),
    json(200, { items, playlist_revision: 3 }),
    ...Array.from({ length: 5 }, () => json(200, { status })),
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
  assert.deepEqual(
    h.seen.map((request) => [request.method, request.url.pathname]),
    [
      ["GET", "/gizclaw/v1/device/audioplayer"],
      ["GET", "/gizclaw/v1/device/audioplayer/playlist"],
      ["PUT", "/gizclaw/v1/device/audioplayer/playlist"],
      ["POST", "/gizclaw/v1/device/audioplayer/playlist/append"],
      ["POST", "/gizclaw/v1/device/audioplayer/actions/play"],
      ["POST", "/gizclaw/v1/device/audioplayer/actions/stop"],
      ["PUT", "/gizclaw/v1/device/audioplayer/mode"],
    ],
  );
  assert.deepEqual(JSON.parse(h.seen[2]!.body), { items });
  assert.deepEqual(JSON.parse(h.seen[3]!.body), { items });
  assert.deepEqual(JSON.parse(h.seen[4]!.body), { index: 0 });
});
