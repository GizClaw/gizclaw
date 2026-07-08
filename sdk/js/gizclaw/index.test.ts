import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { x25519 } from "@noble/curves/ed25519.js";

import {
  GIZCLAW_SERVICE_ADMIN_HTTP,
  GIZCLAW_MAX_PACKET_MESSAGE_SIZE,
  GIZCLAW_EVENT_STREAM_TELEMETRY,
  GIZCLAW_SERVICE_PEER_RPC,
  GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL,
  GIZNET_WEBRTC_SIGNALING_PATH,
  RPC_FRAME_TYPE_EOS,
  RPC_FRAME_TYPE_BINARY,
  RPC_FRAME_TYPE_TEXT,
  WebRTCRPCClient,
  WebRTCRPCError,
  createAdminAPIFetch,
  batteryTelemetry,
  createWebRTCFetch,
  decodeFrames,
  encodeTelemetryPacket,
  encodeFrame,
  encodeRPCResponse,
  fetchGiznetServerInfo,
  giznetServiceDataChannelLabel,
  getGiznetWebRTCPacketDataChannel,
  prepareGiznetWebRTCPeerConnection,
  sendGiznetWebRTCTelemetry,
  sendGiznetWebRTCOffer,
  systemTelemetry,
  waitForICEGatheringComplete,
} from "./index.ts";
import { createSseClient } from "./generated/adminhttp/core/serverSentEvents.gen.ts";
import { decodeRPCRequestPayload, decodeRPCResponsePayload, encodeRPCRequestPayload } from "./generated/rpc/payload-codec.ts";
import { createPeerRPCClient } from "./rpc.ts";
import { base58Decode, base58Encode, base64Decode, prepareEncryptedGiznetWebRTCOffer } from "./signaling.ts";

function concatBuffers(parts: Array<ArrayBuffer | Uint8Array>): ArrayBuffer {
  let total = 0;
  for (const part of parts) {
    total += part.byteLength;
  }
  const out = new Uint8Array(total);
  let offset = 0;
  for (const part of parts) {
    const bytes = part instanceof Uint8Array ? part : new Uint8Array(part);
    out.set(bytes, offset);
    offset += bytes.byteLength;
  }
  return out.buffer;
}

function includesBytes(bytes: Uint8Array, needle: number[]): boolean {
  if (needle.length === 0) {
    return true;
  }
  for (let i = 0; i <= bytes.length - needle.length; i += 1) {
    let matches = true;
    for (let j = 0; j < needle.length; j += 1) {
      if (bytes[i + j] !== needle[j]) {
        matches = false;
        break;
      }
    }
    if (matches) {
      return true;
    }
  }
  return false;
}

test("WebRTCRPCClient sends protobuf RPC over an rpc data channel", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-1" });

  const promise = client.call<{ server_time: number }>("all.ping", { client_send_time: 1 });
  const channel = pc.lastChannel();
  channel.open();

  assert.equal(channel.label, giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC));
  const frames = decodeFrames(channel.sent[0] ?? new ArrayBuffer(0));
  assert.equal(frames.length, 2);
  assert.equal(frames[0]?.type, RPC_FRAME_TYPE_BINARY);
  assert.ok((frames[0]?.payload.length ?? 0) > 0);
  assert.equal(new TextDecoder().decode(frames[0]?.payload ?? new Uint8Array()).includes("client_send_time"), false);
  assert.equal(includesBytes(frames[0]?.payload ?? new Uint8Array(), [0x1a, 0x02, 0x08, 0x01]), true);
  assert.equal(frames[1]?.type, RPC_FRAME_TYPE_EOS);

  channel.receive(encodeRPCResponse({ id: "req-1", result: { server_time: 99 }, v: 1 }, "all.ping"));

  assert.deepEqual(await promise, { server_time: 99 });
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient splits oversized request envelopes into continuation frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-large" });

  const promise = client.call<{ accepted: boolean }>("server.run.say", { text: "x".repeat(70000) });
  const channel = pc.lastChannel();
  channel.open();

  const frames = decodeFrames(channel.sent[0] ?? new ArrayBuffer(0));
  assert.equal(frames.length >= 3, true);
  assert.equal(frames[0]?.type, RPC_FRAME_TYPE_TEXT);
  assert.equal(frames[0]?.payload.length, 0xffff);
  assert.equal(frames[frames.length - 1]?.type, RPC_FRAME_TYPE_EOS);
  assert.equal(frames.slice(0, -1).every((frame) => frame.type === RPC_FRAME_TYPE_TEXT), true);

  channel.receive(encodeRPCResponse({ id: "req-large", result: { accepted: true }, v: 1 }, "server.run.say"));

  assert.deepEqual(await promise, { accepted: true });
});

test("WebRTCRPCClient decodes Go-compatible protobuf payload bytes", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-go" });

  const promise = client.call<{ server_time: number }>("all.ping", { client_send_time: 5 });
  const channel = pc.lastChannel();
  channel.open();

  channel.receive(concatBuffers([
    encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([
      0x0a, 0x06, 0x72, 0x65, 0x71, 0x2d, 0x67, 0x6f,
      0x12, 0x03, 0x08, 0xe3, 0x07,
    ])),
    encodeFrame(RPC_FRAME_TYPE_EOS),
  ]));

  assert.deepEqual(await promise, { server_time: 995 });
});

test("RPC payload codec selects workspace oneofs from discriminators", () => {
  const payload = encodeRPCRequestPayload("server.workspace.create", {
    created_at: "now",
    last_active_at: "now",
    name: "main",
    parameters: {
      agent_type: "doubao-realtime",
    },
    updated_at: "now",
    workflow_name: "chat",
  });

  const decoded = decodeRPCRequestPayload("server.workspace.create", payload) as {
    parameters?: { agent_type?: string; input?: string };
  };

  assert.equal(decoded.parameters?.agent_type, "doubao-realtime");
});

test("RPC method map preserves generated payload types", () => {
  const source = readFileSync(new URL("./generated/rpc/method-map.ts", import.meta.url), "utf8");

  assert.match(source, /request: PingRequest;/);
  assert.match(source, /response: PingResponse;/);
  assert.doesNotMatch(source, /request: unknown;/);
  assert.doesNotMatch(source, /response: unknown;/);
});

test("RPC payload codec decodes omitted proto3 defaults", () => {
  assert.deepEqual(decodeRPCResponsePayload("all.speed_test.run", new Uint8Array()), {
    down_content_length: 0,
    up_content_length: 0,
  });
  assert.deepEqual(decodeRPCResponsePayload("server.workspace.list", new Uint8Array()), {
    has_next: false,
    items: [],
  });
});

test("RPC payload codec rejects string values for bool fields", () => {
  assert.throws(
    () => encodeRPCRequestPayload("server.workspace.create", {
      created_at: "now",
      last_active_at: "now",
      name: "main",
      parameters: {
        agent_type: "doubao-realtime",
        e2e: "false",
      },
      updated_at: "now",
      workflow_name: "chat",
    }),
    /protobuf bool field expects boolean/,
  );
});

test("WebRTCRPCClient reassembles response frames split across messages", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-split" });

  const promise = client.call<{ server_time: number }>("all.ping", { client_send_time: 2 });
  const channel = pc.lastChannel();
  channel.open();

  const response = new Uint8Array(encodeRPCResponse({ id: "req-split", result: { server_time: 100 }, v: 1 }, "all.ping"));
  channel.receiveBytes(response.slice(0, 5));
  channel.receiveBytes(response.slice(5));

  assert.deepEqual(await promise, { server_time: 100 });
});

test("WebRTCRPCClient reassembles oversized protobuf envelope continuation frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-continuation" });

  const promise = client.call<{ server_time: number }>("all.ping", { client_send_time: 3 });
  const channel = pc.lastChannel();
  channel.open();

  const responseFrames = decodeFrames(encodeRPCResponse({ id: "req-continuation", result: { server_time: 101 }, v: 1 }, "all.ping"));
  const envelope = responseFrames[0]?.payload;
  assert.ok(envelope);
  const split = Math.floor(envelope.length / 2);
  channel.receive(encodeFrame(RPC_FRAME_TYPE_TEXT, envelope.slice(0, split)));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_TEXT, envelope.slice(split)));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));

  assert.deepEqual(await promise, { server_time: 101 });
});

test("WebRTCRPCClient resolves queued response before close", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-close" });

  const promise = client.call<{ server_time: number }>("all.ping", { client_send_time: 4 });
  const channel = pc.lastChannel();
  channel.open();
  channel.receive(encodeRPCResponse({ id: "req-close", result: { server_time: 102 }, v: 1 }, "all.ping"));
  channel.remoteClose();

  assert.deepEqual(await promise, { server_time: 102 });
});

test("WebRTCRPCClient reads metadata plus binary response frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-binary" });

  const promise = client.callBinary<{ mime_type: string; size_bytes: number }>("server.workspace.history.audio.get", {
    history_id: "h1",
    workspace_name: "main",
  });
  const channel = pc.lastChannel();
  channel.open();

  channel.receive(encodeRPCResponse({ id: "req-binary", result: { mime_type: "audio/ogg", size_bytes: 5 }, v: 1 }, "server.workspace.history.audio.get").slice(0, -4));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([1, 2])));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([3, 4, 5])));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));

  const result = await promise;
  assert.deepEqual(result.result, {
    history_id: "",
    mime_type: "audio/ogg",
    size_bytes: 5,
    workspace_name: "",
  });
  assert.deepEqual(result.body, new Uint8Array([1, 2, 3, 4, 5]));
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient rejects RPC error responses", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-2" });

  const promise = client.call("server.run.workspace.reload");
  const channel = pc.lastChannel();
  channel.open();
  channel.receive(encodeRPCResponse({ error: { code: -32000, message: "boom" }, id: "req-2", v: 1 }, "server.run.workspace.reload"));

  await assert.rejects(promise, (err) => {
    assert.equal(err instanceof WebRTCRPCError, true);
    assert.equal((err as WebRTCRPCError).code, -32000);
    assert.equal((err as WebRTCRPCError).message, "boom");
    return true;
  });
});

test("WebRTCRPCClient rejects unknown methods before data channel open", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-unknown", requestTimeoutMs: 0 });

  await assert.rejects(client.call("peer.unknown", {}), /unknown RPC method: peer\.unknown/);
  assert.equal(pc.lastChannel().closed, true);
  assert.equal(pc.lastChannel().sent.length, 0);
});

test("WebRTCRPCClient rejects unknown binary methods before data channel open", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-unknown-binary", requestTimeoutMs: 0 });

  await assert.rejects(client.callBinary("peer.unknown", {}), /unknown RPC method: peer\.unknown/);
  assert.equal(pc.lastChannel().closed, true);
  assert.equal(pc.lastChannel().sent.length, 0);
});

test("WebRTCRPCClient honors AbortSignal", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-3", requestTimeoutMs: 0 });
  const ac = new AbortController();

  const promise = client.call("server.run.workspace.get", {}, { signal: ac.signal });
  const channel = pc.lastChannel();
  ac.abort();

  await assert.rejects(promise, { name: "AbortError" });
  assert.equal(channel.closed, true);
});

test("createPeerRPCClient calls generated typed RPC methods", async () => {
  const calls: Array<{ method: string; params: unknown }> = [];
  const client = {
    call: async (method: string, params: unknown) => {
      calls.push({ method, params });
      return { accepted: true };
    },
    callBinary: async () => {
      throw new Error("unexpected binary call");
    },
  } as unknown as WebRTCRPCClient;
  const rpc = createPeerRPCClient(client);

  await rpc.call("server.run.workspace.set", { workspace_name: "main" });
  await rpc.call("server.run.workspace.history.play", { history_id: "h1" });
  await rpc.call("server.firmware.files.download", { channel: "stable", firmware_id: "devkit", path: "firmware.bin" });
  await rpc.call("server.friend_group.messages.send", { friend_group_id: "group-a", text: "hello" });

  assert.deepEqual(calls, [
    { method: "server.run.workspace.set", params: { workspace_name: "main" } },
    { method: "server.run.workspace.history.play", params: { history_id: "h1" } },
    { method: "server.firmware.files.download", params: { channel: "stable", firmware_id: "devkit", path: "firmware.bin" } },
    { method: "server.friend_group.messages.send", params: { friend_group_id: "group-a", text: "hello" } },
  ]);
});

test("createWebRTCFetch turns generated-client fetch calls into RPC calls", async () => {
  const calls: Array<{ method: string; params: unknown }> = [];
  const client = {
    call: async (method: string, params: unknown) => {
      calls.push({ method, params });
      return { workspace_name: "main" };
    },
  } as unknown as WebRTCRPCClient;
  const rpcFetch = createWebRTCFetch(client, {
    router: async (request) => {
      assert.equal(new URL(request.url).pathname, "/peer-run/workspace");
      return { method: "server.run.workspace.get", params: {} };
    },
  });

  const response = await rpcFetch("http://gizclaw.local/peer-run/workspace");

  assert.equal(response.status, 200);
  assert.equal(response.headers.get("content-type"), "application/json");
  assert.deepEqual(await response.json(), { workspace_name: "main" });
  assert.deepEqual(calls, [{ method: "server.run.workspace.get", params: {} }]);
});

test("createAdminAPIFetch sends HTTP over the admin HTTP service channel", async () => {
  const pc = new FakePeerConnection();
  const adminFetch = createAdminAPIFetch(pc);
  const promise = adminFetch("http://gizclaw/peers?limit=10", {
    headers: {
      Accept: "application/json",
    },
  });
  const channel = pc.lastChannel();
  channel.open();

  assert.equal(channel.label, giznetServiceDataChannelLabel(GIZCLAW_SERVICE_ADMIN_HTTP));
  await channel.waitForSent();
  const requestText = new TextDecoder().decode(channel.sent[0] ?? new ArrayBuffer(0));
  assert.match(requestText, /^GET \/peers\?limit=10 HTTP\/1\.1\r\n/);
  assert.match(requestText, /\r\nHost: gizclaw\r\n/);
  assert.match(requestText, /\r\nConnection: close\r\n/);

  const body = JSON.stringify({ has_next: false, items: [] });
  channel.receiveText(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${body.length}\r\n\r\n${body}`);

  const response = await promise;
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { has_next: false, items: [] });
  assert.equal(channel.closed, true);
});

test("createAdminAPIFetch waits for open readyState before sending", async () => {
  const pc = new FakePeerConnection();
  const adminFetch = createAdminAPIFetch(pc);
  const promise = adminFetch("http://gizclaw/peers");
  const channel = pc.lastChannel();

  channel.signalOpenWithoutReady();
  assert.equal(channel.sent.length, 0);
  channel.readyState = "open";
  await channel.waitForSent();

  const body = JSON.stringify({ has_next: false, items: [] });
  channel.receiveText(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${body.length}\r\n\r\n${body}`);

  const response = await promise;
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { has_next: false, items: [] });
});

test("createAdminAPIFetch reads chunked HTTP service responses", async () => {
  const pc = new FakePeerConnection();
  const adminFetch = createAdminAPIFetch(pc);
  const promise = adminFetch("http://gizclaw/server");
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSent();

  channel.receiveText("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nB\r\n{\"ok\":t");
  channel.receiveText("rue}\r\n0\r\n\r\n");

  const response = await promise;
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { ok: true });
});

test("createAdminAPIFetch resolves close-delimited HTTP service responses", async () => {
  const pc = new FakePeerConnection();
  const adminFetch = createAdminAPIFetch(pc);
  const promise = adminFetch("http://gizclaw/server");
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSent();

  channel.receiveText("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nhello");
  channel.remoteClose();

  const response = await promise;
  assert.equal(response.status, 200);
  assert.equal(await response.text(), "hello");
});

test("SSE client parses HTTP JSON errors without retrying", async () => {
  const errorBody = { error: { code: "LOG_QUERY_NOT_CONFIGURED", message: "server log query backend is not configured" } };
  const seenErrors: Array<unknown> = [];
  let attempts = 0;
  const result = createSseClient({
    fetch: async () => {
      attempts++;
      return new Response(JSON.stringify(errorBody), {
        status: 501,
        headers: { "Content-Type": "application/json" },
      });
    },
    onSseError: (error) => seenErrors.push(error),
    sseSleepFn: async () => {
      throw new Error("SSE HTTP errors must not be retried");
    },
    url: "http://gizclaw/logs/stream",
  });

  await assert.rejects(
    async () => {
      for await (const _event of result.stream) {
        // noop
      }
    },
    (error) => {
      assert.deepEqual(error, errorBody);
      return true;
    },
  );
  assert.equal(attempts, 1);
  assert.deepEqual(seenErrors, [errorBody]);
});

test("SSE client does not retry backend JSON errors", async () => {
  const errorBody = { error: { code: "LOG_BACKEND_ERROR", message: "backend search failed" } };
  let attempts = 0;
  const result = createSseClient({
    fetch: async () => {
      attempts++;
      return new Response(JSON.stringify(errorBody), {
        status: 502,
        headers: { "Content-Type": "application/json" },
      });
    },
    sseSleepFn: async () => {
      throw new Error("SSE JSON backend errors must not be retried");
    },
    url: "http://gizclaw/logs/stream",
  });

  await assert.rejects(
    async () => {
      for await (const _event of result.stream) {
        // noop
      }
    },
    (error) => {
      assert.deepEqual(error, errorBody);
      return true;
    },
  );
  assert.equal(attempts, 1);
});

test("SSE client retries transient HTTP errors", async () => {
  const seenErrors: Array<unknown> = [];
  let attempts = 0;
  let sleeps = 0;
  const result = createSseClient<{ 200: { message: string } }>({
    fetch: async () => {
      attempts++;
      if (attempts < 3) {
        return new Response("temporarily unavailable", { status: 503 });
      }
      return new Response('event: log\ndata: {"message":"ready"}\n\n', {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      });
    },
    onSseError: (error) => seenErrors.push(error),
    sseMaxRetryAttempts: 3,
    sseSleepFn: async () => {
      sleeps++;
    },
    url: "http://gizclaw/logs/stream",
  });

  const events: Array<unknown> = [];
  for await (const event of result.stream) {
    events.push(event);
  }

  assert.equal(attempts, 3);
  assert.equal(sleeps, 2);
  assert.deepEqual(seenErrors, ["temporarily unavailable", "temporarily unavailable"]);
  assert.deepEqual(events, [{ message: "ready" }]);
});

test("SSE client yields parsed JSON event objects", async () => {
  const result = createSseClient<{ 200: { message: string } }>({
    fetch: async () =>
      new Response('event: log\ndata: {"message":"ready"}\n\n', {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      }),
    url: "http://gizclaw/logs/stream",
  });

  const events: Array<unknown> = [];
  for await (const event of result.stream) {
    events.push(event);
  }

  assert.deepEqual(events, [{ message: "ready" }]);
});

test("prepareGiznetWebRTCPeerConnection creates packet channel and audio transceiver", () => {
  const pc = new FakePeerConnection();

  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  assert.equal(pc.channels[0]?.label, GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL);
  assert.deepEqual(pc.channels[0]?.options, { maxRetransmits: 0, ordered: false });
  assert.equal(getGiznetWebRTCPacketDataChannel(pc as unknown as RTCPeerConnection), pc.channels[0]);
  assert.deepEqual(pc.transceivers, [{ kind: "audio", init: { direction: "sendrecv" } }]);
});

test("sendGiznetWebRTCTelemetry uses the prepared packet channel", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  pc.channels[0]?.open();

  await sendGiznetWebRTCTelemetry(pc as unknown as RTCPeerConnection, {
    observedAtUnixMs: 1000,
    observations: [batteryTelemetry({ percent: 82 })],
  });

  assert.equal(pc.channels[0]?.sent.length, 1);
  const packet = new Uint8Array(pc.channels[0]?.sent[0] ?? new ArrayBuffer(0));
  assert.equal(packet[0], GIZCLAW_EVENT_STREAM_TELEMETRY);
});

test("sendGiznetWebRTCTelemetry waits for the packet channel to open", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const promise = sendGiznetWebRTCTelemetry(pc as unknown as RTCPeerConnection, {
    observedAtUnixMs: 1000,
    observations: [batteryTelemetry({ percent: 82 })],
  });

  assert.equal(pc.channels[0]?.sent.length, 0);
  pc.channels[0]?.open();
  await promise;

  assert.equal(pc.channels[0]?.sent.length, 1);
});

test("sendGiznetWebRTCTelemetry waits beyond the RPC send retry budget", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const promise = sendGiznetWebRTCTelemetry(pc as unknown as RTCPeerConnection, {
    observedAtUnixMs: 1000,
    observations: [batteryTelemetry({ percent: 82 })],
  }, { timeoutMs: 1000 });

  await new Promise((resolve) => setTimeout(resolve, 150));
  assert.equal(pc.channels[0]?.sent.length, 0);
  pc.channels[0]?.open();
  await promise;

  assert.equal(pc.channels[0]?.sent.length, 1);
});

test("sendGiznetWebRTCTelemetry rejects when the packet channel does not open", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  await assert.rejects(
    sendGiznetWebRTCTelemetry(pc as unknown as RTCPeerConnection, {
      observedAtUnixMs: 1000,
      observations: [batteryTelemetry({ percent: 82 })],
    }, { timeoutMs: 1 }),
    /did not open/,
  );
});

test("encodeTelemetryPacket prefixes protobuf telemetry payload", () => {
  const packet = encodeTelemetryPacket({
    sequence: 7,
    observedAtUnixMs: 1000,
    observations: [batteryTelemetry({ percent: 82, charging: true })],
  });

  assert.equal(packet[0], GIZCLAW_EVENT_STREAM_TELEMETRY);
  assert.deepEqual([...packet.slice(1)], [
    8, 7,
    16, 232, 7,
    26, 13,
    82, 11,
    9, 0, 0, 0, 0, 0, 128, 84, 64,
    16, 1,
  ]);
});

test("encodeTelemetryPacket stamps frames before send", () => {
  const originalNow = Date.now;
  Date.now = () => 1234;
  try {
    const frame = {
      observedAtUnixMs: 0,
      observations: [batteryTelemetry({ percent: 82 })],
    };
    const packet = encodeTelemetryPacket(frame);

    assert.equal(frame.observations.length, 1);
    assert.equal(frame.observedAtUnixMs, 0);
    assert.deepEqual([...packet.slice(1, 4)], [16, 210, 9]);
  } finally {
    Date.now = originalNow;
  }
});

test("encodeTelemetryPacket rejects observations with multiple bodies", () => {
  assert.throws(
    () => encodeTelemetryPacket({
      observations: [{
        battery: { percent: 82 },
        system: { temperatureC: 33 },
      } as any],
    }),
    /exactly one body/,
  );
});

test("encodeTelemetryPacket rejects empty frames before encoding", () => {
  assert.throws(
    () => encodeTelemetryPacket({}),
    /at least one observation/,
  );
});

test("encodeTelemetryPacket rejects oversized packets", () => {
  assert.throws(
    () => encodeTelemetryPacket({
      observations: [systemTelemetry({ firmwareVersion: "x".repeat(GIZCLAW_MAX_PACKET_MESSAGE_SIZE) })],
    }),
    /maximum/,
  );
});

test("waitForICEGatheringComplete resolves when completion races listener registration", async () => {
  const pc = new FakeICEPeerConnection();
  pc.completeAfterFirstListener = true;

  await waitForICEGatheringComplete(pc as unknown as RTCPeerConnection);

  assert.equal(pc.iceGatheringState, "complete");
});

test("sendGiznetWebRTCOffer posts the peer HTTP signaling request", async () => {
  const body = new Blob([new Uint8Array([1, 2, 3])]);
  const answer = new Blob([new Uint8Array([4, 5])]);
  let captured: Request | undefined;

  const result = await sendGiznetWebRTCOffer(
    {
      body,
      clientPublicKey: "peer-pk",
      nonce: "nonce",
      openAnswer: async () => "v=0",
      timestamp: 123,
    },
    {
      fetch: async (input, init) => {
        captured = new Request(input, init);
        return new Response(answer, { headers: { "content-type": "application/octet-stream" }, status: 200 });
      },
      url: `http://localhost${GIZNET_WEBRTC_SIGNALING_PATH}`,
    },
  );

  assert.deepEqual(new Uint8Array(await result.arrayBuffer()), new Uint8Array([4, 5]));
  assert.equal(result.type, "application/octet-stream");
  assert.equal(captured?.url, `http://localhost${GIZNET_WEBRTC_SIGNALING_PATH}`);
  assert.equal(captured?.method, "POST");
  assert.equal(captured?.headers.get("content-type"), "application/octet-stream");
  assert.equal(captured?.headers.get("x-giznet-public-key"), "peer-pk");
  assert.equal(captured?.headers.get("x-giznet-timestamp"), "123");
  assert.equal(captured?.headers.get("x-giznet-nonce"), "nonce");
});

test("fetchGiznetServerInfo validates server metadata", async () => {
  const serverPublicKey = base58Encode(x25519.getPublicKey(new Uint8Array(32).fill(2)));
  let captured: Request | undefined;

  const info = await fetchGiznetServerInfo({
    baseUrl: "http://localhost:9820",
    fetch: async (input, init) => {
      captured = new Request(input, init);
      return Response.json({
        protocol: "gizclaw-webrtc",
        public_key: serverPublicKey,
        signaling_path: "/custom/offer",
      });
    },
  });

  assert.equal(captured?.url, "http://localhost:9820/server-info");
  assert.equal(info.public_key, serverPublicKey);
  assert.equal(info.signaling_path, "/custom/offer");
});

test("fetchGiznetServerInfo defaults signaling path and reports HTTP failures", async () => {
  const serverPublicKey = base58Encode(x25519.getPublicKey(new Uint8Array(32).fill(2)));
  const info = await fetchGiznetServerInfo({
    baseUrl: "http://localhost:9820",
    fetch: async () =>
      Response.json({
        protocol: "gizclaw-webrtc",
        public_key: serverPublicKey,
      }),
  });

  assert.equal(info.signaling_path, "/webrtc/v1/offer");
  await assert.rejects(
    fetchGiznetServerInfo({
      baseUrl: "http://localhost:9820",
      fetch: async () => new Response("not ready", { status: 503, statusText: "Service Unavailable" }),
    }),
    /server-info failed: 503 Service Unavailable: not ready/,
  );
});

test("fetchGiznetServerInfo rejects missing or invalid server public key", async () => {
  await assert.rejects(
    fetchGiznetServerInfo({
      baseUrl: "http://localhost:9820",
      fetch: async () => Response.json({ protocol: "gizclaw-webrtc" }),
    }),
    /missing public_key/,
  );
  await assert.rejects(
    fetchGiznetServerInfo({
      baseUrl: "http://localhost:9820",
      fetch: async () =>
        Response.json({
          protocol: "gizclaw-webrtc",
          public_key: "not-a-key",
        }),
    }),
    /invalid base58 character|invalid public_key/,
  );
});

test("prepareEncryptedGiznetWebRTCOffer builds a browser-safe encrypted offer", async () => {
  const clientPrivateKey = new Uint8Array(32).fill(1);
  const serverPrivateKey = new Uint8Array(32).fill(2);
  const clientPublicKey = x25519.getPublicKey(clientPrivateKey);
  const serverPublicKey = x25519.getPublicKey(serverPrivateKey);

  assert.deepEqual(base58Decode(base58Encode(clientPublicKey)), clientPublicKey);
  assert.deepEqual(base64Decode("AQID"), new Uint8Array([1, 2, 3]));

  const offer = await prepareEncryptedGiznetWebRTCOffer(
    {
      clientPrivateKey,
      clientPublicKey: base58Encode(clientPublicKey),
      serverPublicKey: base58Encode(serverPublicKey),
    },
    "v=0",
  );

  assert.equal(offer.clientPublicKey, base58Encode(clientPublicKey));
  assert.equal(typeof offer.nonce, "string");
  assert.equal(offer.timestamp > 0, true);
  assert.equal((await offer.body.arrayBuffer()).byteLength > 16, true);
});

class FakePeerConnection {
  channels: FakeDataChannel[] = [];
  transceivers: Array<{ init?: RTCRtpTransceiverInit; kind: string }> = [];

  createDataChannel(label: string, options?: RTCDataChannelInit): FakeDataChannel {
    const channel = new FakeDataChannel(label, options);
    this.channels.push(channel);
    return channel;
  }

  addTransceiver(kind: string, init?: RTCRtpTransceiverInit): void {
    this.transceivers.push({ kind, init });
  }

  lastChannel(): FakeDataChannel {
    const channel = this.channels.at(-1);
    if (channel == null) {
      throw new Error("no channel created");
    }
    return channel;
  }
}

class FakeICEPeerConnection {
  completeAfterFirstListener = false;
  iceGatheringState: RTCIceGatheringState = "gathering";
  readonly listeners = new Map<string, Set<() => void>>();

  addEventListener(type: string, listener: () => void): void {
    let listeners = this.listeners.get(type);
    if (listeners == null) {
      listeners = new Set();
      this.listeners.set(type, listeners);
    }
    listeners.add(listener);
    if (this.completeAfterFirstListener) {
      this.iceGatheringState = "complete";
    }
  }

  removeEventListener(type: string, listener: () => void): void {
    this.listeners.get(type)?.delete(listener);
  }
}

class FakeDataChannel {
  binaryType?: BinaryType;
  closed = false;
  readonly label: string;
  readonly options?: RTCDataChannelInit;
  readyState: RTCDataChannelState = "connecting";
  sent: ArrayBuffer[] = [];
  readonly listeners = new Map<string, Set<(event?: unknown) => void>>();

  constructor(label: string, options?: RTCDataChannelInit) {
    this.label = label;
    this.options = options;
  }

  addEventListener(type: string, listener: (event?: unknown) => void): void {
    let listeners = this.listeners.get(type);
    if (listeners == null) {
      listeners = new Set();
      this.listeners.set(type, listeners);
    }
    listeners.add(listener);
  }

  removeEventListener(type: string, listener: (event?: unknown) => void): void {
    this.listeners.get(type)?.delete(listener);
  }

  send(data: ArrayBuffer | ArrayBufferView | Blob | string): void {
    if (data instanceof ArrayBuffer) {
      this.sent.push(data);
      return;
    }
    if (ArrayBuffer.isView(data)) {
      this.sent.push(data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength));
      return;
    }
    if (typeof data === "string") {
      this.sent.push(new TextEncoder().encode(data).buffer);
      return;
    }
    throw new Error("fake data channel only supports synchronous data");
  }

  close(): void {
    this.closed = true;
    this.readyState = "closed";
  }

  remoteClose(): void {
    this.closed = true;
    this.readyState = "closed";
    this.emit("close");
  }

  open(): void {
    this.readyState = "open";
    this.emit("open");
  }

  signalOpenWithoutReady(): void {
    this.emit("open");
  }

  receive(data: ArrayBuffer): void {
    this.emit("message", { data });
  }

  receiveBytes(data: Uint8Array): void {
    this.emit("message", { data });
  }

  receiveText(data: string): void {
    this.receiveBytes(new TextEncoder().encode(data));
  }

  async waitForSent(): Promise<void> {
    for (let i = 0; i < 50; i += 1) {
      if (this.sent.length > 0) {
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    throw new Error("fake data channel did not send data");
  }

  private emit(type: string, event?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}
