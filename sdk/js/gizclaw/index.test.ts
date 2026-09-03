import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { x25519 } from "@noble/curves/ed25519.js";

import {
  GIZCLAW_SERVICE_ADMIN_HTTP,
  GIZCLAW_MAX_PACKET_MESSAGE_SIZE,
  GIZCLAW_EVENT_STREAM_TELEMETRY,
  GIZCLAW_EVENT_STREAM_AGENT,
  GIZCLAW_SERVICE_EDGE_RPC,
  GIZCLAW_SERVICE_PEER_RPC,
  GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL,
  GIZNET_WEBRTC_SIGNALING_PATH,
  RPC_FRAME_TYPE_EOS,
  RPC_FRAME_TYPE_BINARY,
  RPC_FRAME_TYPE_TEXT,
  STATUS_CODE_INTERNAL,
  STATUS_CODE_INVALID_ARGUMENT,
  STATUS_CODE_UNIMPLEMENTED,
  SPEECH_EXTRACTION_REQUEST_TIMEOUT_MS,
  SPEECH_SYNTHESIS_REQUEST_TIMEOUT_MS,
  SPEECH_TRANSCRIPTION_REQUEST_TIMEOUT_MS,
  GizClawDeviceControlError,
  type GizClawPeerRPCHandlers,
  type RPCResponse,
  WebRTCRPCClient,
  WebRTCRPCError,
  applyGiznetServerInfoICEServers,
  createAdminAPIFetch,
  batteryTelemetry,
  connectGiznetWebRTC,
  createWebRTCFetch,
  decodeFrames,
  encodeTelemetryPacket,
  encodeFrame,
  encodeRPCRequest,
  serveGiznetWebRTCRPC,
  encodeRPCResponse,
  fetchGiznetServerInfo,
  giznetServiceDataChannelLabel,
  isServerInfoHostPort,
  normalizeServerBaseURL,
  getGiznetWebRTCPacketDataChannel,
  getGiznetWebRTCPeerEventDataChannel,
  parseRPCResponse,
  prepareGiznetWebRTCPeerConnection,
  rewriteGiznetWebRTCAnswerForEndpoint,
  sendGiznetWebRTCTelemetry,
  sendGiznetWebRTCOffer,
  systemTelemetry,
  waitForICEGatheringComplete,
} from "./index.ts";
import { createSseClient } from "./generated/adminhttp/core/serverSentEvents.gen.ts";
import {
  decodeRPCRequestPayload,
  decodeRPCResponsePayload,
  encodeRPCRequestPayload,
  encodeRPCResponsePayload,
} from "./generated/rpc/payload-codec.ts";
import { RPC_METHOD_IDS, RPC_METHODS } from "./generated/rpc/method-map.ts";
import * as peerhttp from "./peerhttp.ts";
import { createEdgeRPCClient, createPeerRPCClient } from "./rpc.ts";
import {
  base58Decode,
  base58Encode,
  base64Decode,
  prepareEncryptedGiznetWebRTCOffer,
} from "./signaling.ts";

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

  const promise = client.call<{ server_time: number }>("all.ping", {
    client_send_time: 1,
  });
  const channel = pc.lastChannel();
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  assert.equal(
    channel.label,
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  const frames = decodeFrames(channel.sent[0] ?? new ArrayBuffer(0));
  assert.equal(frames.length, 2);
  assert.equal(frames[0]?.type, RPC_FRAME_TYPE_BINARY);
  assert.ok((frames[0]?.payload.length ?? 0) > 0);
  assert.equal(
    new TextDecoder()
      .decode(frames[0]?.payload ?? new Uint8Array())
      .includes("client_send_time"),
    false,
  );
  assert.equal(
    includesBytes(
      frames[0]?.payload ?? new Uint8Array(),
      [0x1a, 0x02, 0x08, 0x01],
    ),
    true,
  );
  assert.equal(frames[1]?.type, RPC_FRAME_TYPE_EOS);

  channel.receive(
    encodeRPCResponse(
      { id: "req-1", result: { server_time: 99 }, v: 1 },
      "all.ping",
    ),
  );

  assert.deepEqual(await promise, { server_time: 99 });
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient omits protobuf payload when params are absent", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-no-params" });

  const promise = client.call<{ server_time: number }>("all.ping");
  const channel = pc.lastChannel();
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  const frames = decodeFrames(channel.sent[0] ?? new ArrayBuffer(0));
  assert.equal(frames.length, 2);
  assert.equal(frames[0]?.type, RPC_FRAME_TYPE_BINARY);
  assert.equal(
    includesBytes(frames[0]?.payload ?? new Uint8Array(), [0x1a]),
    false,
  );
  assert.equal(frames[1]?.type, RPC_FRAME_TYPE_EOS);

  channel.receive(
    encodeRPCResponse(
      { id: "req-no-params", result: { server_time: 98 }, v: 1 },
      "all.ping",
    ),
  );

  assert.deepEqual(await promise, { server_time: 98 });
});

test("WebRTCRPCClient resumes service writes on bufferedamountlow", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-water" });
  const promise = client.call<{ server_time: number }>("all.ping");
  const channel = pc.lastChannel();
  channel.bufferedAmount = 1024 * 1024;
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  assert.equal(channel.sent.length, 0);
  assert.equal(channel.bufferedAmountLowThreshold, 256 * 1024);

  channel.bufferedAmount = 256 * 1024;
  channel.bufferedAmountLow();
  await new Promise<void>((resolve) => setImmediate(resolve));
  assert.equal(channel.sent.length, 1);
  channel.receive(
    encodeRPCResponse(
      { id: "req-water", result: { server_time: 97 }, v: 1 },
      "all.ping",
    ),
  );
  assert.deepEqual(await promise, { server_time: 97 });
});

test("WebRTCRPCClient handles a drain while installing the low-water waiter", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-water-race" });
  const promise = client.call<{ server_time: number }>("all.ping", {
    client_send_time: 1,
  });
  const channel = pc.lastChannel();
  channel.bufferedAmount = 1024 * 1024;
  channel.drainBufferedAmountOnRead = 2;
  channel.open();
  await channel.waitForSent();

  assert.equal(channel.sent.length, 1);
  channel.receive(
    encodeRPCResponse(
      { id: "req-water-race", result: { server_time: 96 }, v: 1 },
      "all.ping",
    ),
  );
  assert.deepEqual(await promise, { server_time: 96 });
});

test("WebRTCRPCClient service write timeouts ignore the wall clock", async () => {
  const originalNow = Date.now;
  let wallClockReads = 0;
  Date.now = () => {
    wallClockReads += 1;
    return wallClockReads % 2 === 0 ? 0 : Number.MAX_SAFE_INTEGER;
  };
  try {
    const pc = new FakePeerConnection();
    const client = new WebRTCRPCClient(pc, { createID: () => "req-monotonic" });
    const promise = client.call<{ server_time: number }>("all.ping", {
      client_send_time: 1,
    });
    const channel = pc.lastChannel();
    channel.open();
    await channel.waitForSent();
    channel.receive(
      encodeRPCResponse(
        { id: "req-monotonic", result: { server_time: 95 }, v: 1 },
        "all.ping",
      ),
    );

    assert.deepEqual(await promise, { server_time: 95 });
    assert.equal(wallClockReads, 0);
  } finally {
    Date.now = originalNow;
  }
});

test("WebRTCRPCClient serializes logical writes sharing one service channel", async () => {
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  const factory = { createDataChannel: () => channel };
  const firstController = new AbortController();
  const secondController = new AbortController();
  const firstRequest = {
    id: "first",
    method: "server.run.say",
    params: { text: "a".repeat(70000) },
    v: 1 as const,
  };
  const secondRequest = {
    id: "second",
    method: "all.ping",
    params: { client_send_time: 2 },
    v: 1 as const,
  };
  const firstClient = new WebRTCRPCClient(factory, {
    createID: () => firstRequest.id,
  });
  const secondClient = new WebRTCRPCClient(factory, {
    createID: () => secondRequest.id,
  });
  const first = firstClient.call(firstRequest.method, firstRequest.params, {
    signal: firstController.signal,
  });
  const second = secondClient.call(secondRequest.method, secondRequest.params, {
    signal: secondController.signal,
  });
  const expectedNativeMessages =
    Math.ceil(encodeRPCRequest(firstRequest).byteLength / (16 * 1024)) +
    Math.ceil(encodeRPCRequest(secondRequest).byteLength / (16 * 1024));

  channel.open();
  await channel.waitForSentCount(expectedNativeMessages);

  const frames = decodeFrames(concatBuffers(channel.sent));
  const eosIndexes = frames
    .map((frame, index) => (frame.type === RPC_FRAME_TYPE_EOS ? index : -1))
    .filter((index) => index >= 0);
  assert.equal(eosIndexes.length, 2);
  assert.equal(frames[eosIndexes[0]! + 1]?.type, RPC_FRAME_TYPE_BINARY);

  firstController.abort();
  secondController.abort();
  await Promise.allSettled([first, second]);
});

test("WebRTCRPCClient promptly cancels a write queued behind backpressure", async () => {
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  const factory = { createDataChannel: () => channel };
  const firstController = new AbortController();
  const queuedController = new AbortController();
  const firstClient = new WebRTCRPCClient(factory, {
    createID: () => "req-blocked",
  });
  const queuedClient = new WebRTCRPCClient(factory, {
    createID: () => "req-cancelled",
  });
  const first = firstClient.call(
    "server.run.say",
    { text: "x".repeat(70000) },
    {
      signal: firstController.signal,
    },
  );
  const queued = queuedClient.call(
    "all.ping",
    { client_send_time: 1 },
    {
      signal: queuedController.signal,
    },
  );
  channel.bufferedAmount = 1024 * 1024;
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  queuedController.abort();
  const outcome = await Promise.race([
    queued.then(
      () => "resolved",
      (error: unknown) => (error instanceof Error ? error.name : "rejected"),
    ),
    new Promise<string>((resolve) => setTimeout(() => resolve("pending"), 25)),
  ]);

  assert.equal(outcome, "AbortError");
  assert.equal(channel.sent.length, 0);
  firstController.abort();
  await Promise.allSettled([first, queued]);
});

test("WebRTCRPCClient closes after a non-first native send fails", async () => {
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  const factory = { createDataChannel: () => channel };
  const firstClient = new WebRTCRPCClient(factory, {
    createID: () => "req-send-failure",
  });
  const secondClient = new WebRTCRPCClient(factory, {
    createID: () => "req-queued",
  });
  const active = firstClient.call("server.run.say", {
    text: "x".repeat(70000),
  });
  const queued = secondClient.call("all.ping", { client_send_time: 1 });
  channel.failSendCall = 2;

  channel.open();

  await assert.rejects(active, /injected send failure/);
  await assert.rejects(queued, /readyState.*open|closed before response/);
  assert.equal(channel.sent.length, 1);
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient aborts a service write waiting for low water", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-water-abort" });
  const controller = new AbortController();
  const promise = client.call("all.ping", undefined, {
    signal: controller.signal,
  });
  const channel = pc.lastChannel();
  channel.bufferedAmount = 1024 * 1024;
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  controller.abort();

  await assert.rejects(
    promise,
    (error: unknown) => error instanceof Error && error.name === "AbortError",
  );
  assert.equal(channel.closed, true);
  assert.equal(channel.sent.length, 0);
});

test("WebRTCRPCClient splits oversized request envelopes into continuation frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-large" });

  const promise = client.call<{ accepted: boolean }>("server.run.say", {
    text: "x".repeat(70000),
  });
  const channel = pc.lastChannel();
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  const frames = decodeFrames(concatBuffers(channel.sent));
  assert.deepEqual(
    channel.sent.map((message) => message.byteLength),
    [16 * 1024, 16 * 1024, 16 * 1024, 16 * 1024, 4497],
  );
  assert.equal(frames.length >= 3, true);
  assert.equal(frames[0]?.type, RPC_FRAME_TYPE_TEXT);
  assert.equal(frames[0]?.payload.length, 0xffff);
  assert.equal(frames[frames.length - 1]?.type, RPC_FRAME_TYPE_EOS);
  assert.equal(
    frames.slice(0, -1).every((frame) => frame.type === RPC_FRAME_TYPE_TEXT),
    true,
  );

  channel.receive(
    encodeRPCResponse(
      { id: "req-large", result: { accepted: true }, v: 1 },
      "server.run.say",
    ),
  );

  assert.deepEqual(await promise, { accepted: true });
});

test("WebRTCRPCClient decodes Go-compatible protobuf payload bytes", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-go" });

  const promise = client.call<{ server_time: number }>("all.ping", {
    client_send_time: 5,
  });
  const channel = pc.lastChannel();
  channel.open();

  channel.receive(
    concatBuffers([
      encodeFrame(
        RPC_FRAME_TYPE_BINARY,
        new Uint8Array([
          0x0a, 0x06, 0x72, 0x65, 0x71, 0x2d, 0x67, 0x6f, 0x12, 0x03, 0x08,
          0xe3, 0x07,
        ]),
      ),
      encodeFrame(RPC_FRAME_TYPE_EOS),
    ]),
  );

  assert.deepEqual(await promise, { server_time: 995 });
});

test("parseRPCResponse decodes framed protobuf responses", () => {
  const response = parseRPCResponse<{ server_time: number }>(
    encodeRPCResponse(
      { id: "req-low", result: { server_time: 77 }, v: 1 },
      "all.ping",
    ),
    "all.ping",
  );

  assert.deepEqual(response, {
    id: "req-low",
    result: { server_time: 77 },
    v: 1,
  });
});

test("RPC payload codec returns only the runtime profile after registration", () => {
  const payload = encodeRPCResponsePayload("server.register", {
    runtime_profile_name: "h106-production",
  });

  const decoded = decodeRPCResponsePayload("server.register", payload) as {
    runtime_profile_name?: string;
  };
  assert.deepEqual(decoded, {
    runtime_profile_name: "h106-production",
  });
});

test("RPC payload codec preserves caller-assigned Pet adoption names", () => {
  const payload = encodeRPCRequestPayload("runtime.adopt", {
    display_name: "Miso",
    name: "device-pet-01",
  });

  assert.deepEqual(decodeRPCRequestPayload("runtime.adopt", payload), {
    display_name: "Miso",
    name: "device-pet-01",
  });
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

  const decoded = decodeRPCRequestPayload(
    "server.workspace.create",
    payload,
  ) as {
    parameters?: { agent_type?: string; input?: string };
  };

  assert.equal(decoded.parameters?.agent_type, "doubao-realtime");
});

test("RPC payload codec round-trips system workspace classification", () => {
  const payload = encodeRPCResponsePayload("server.workspace.get", {
    runtime_profile_name: "default",
    runtime_profile_revision: "revision-1",
    value: {
      created_at: "2026-07-16T00:00:00Z",
      last_active_at: "2026-07-16T00:00:00Z",
      name: "friend-chat",
      system: true,
      updated_at: "2026-07-16T00:00:00Z",
      workflow_name: "chatroom",
    },
  });

  const decoded = decodeRPCResponsePayload("server.workspace.get", payload) as {
    runtime_profile_name?: string;
    value?: { system?: boolean; workflow_name?: string };
  };
  assert.equal(decoded.value?.system, true);
  assert.equal(decoded.value?.workflow_name, "chatroom");
  assert.equal(decoded.runtime_profile_name, "default");
});

test("RPC payload codec enforces typed model provider-data oneof", () => {
  const response = {
    runtime_profile_name: "default",
    runtime_profile_revision: "revision-1",
    value: {
      name: "chat",
      deepseek_tenant: {
        api_mode: "chat_completions",
        thinking_levels: [],
        upstream_model: "deepseek-chat",
      },
      i18n: {},
      kind: "llm",
      provider_kind: "deepseek-tenant",
    },
  };
  const payload = encodeRPCResponsePayload("server.model.get", response);
  assert.deepEqual(
    decodeRPCResponsePayload("server.model.get", payload),
    response,
  );

  const dashScopeResponse = {
    ...response,
    value: {
      name: "qwen",
      dashscope_tenant: {
        api_mode: "chat_completions",
        thinking_levels: [],
        upstream_model: "qwen-plus",
      },
      i18n: {},
      kind: "llm",
      provider_kind: "dashscope-tenant",
    },
  };
  const dashScopePayload = encodeRPCResponsePayload(
    "server.model.get",
    dashScopeResponse,
  );
  assert.deepEqual(
    decodeRPCResponsePayload("server.model.get", dashScopePayload),
    dashScopeResponse,
  );

  const duplexResponse = {
    ...response,
    value: {
      name: "duplex",
      i18n: {},
      kind: "realtime-duplex",
      provider_kind: "volc-tenant",
      volc_tenant: {
        api_mode: "realtime_duplex",
        thinking_levels: [],
        upstream_model: "1.2.6.0",
      },
    },
  };
  const duplexPayload = encodeRPCResponsePayload(
    "server.model.get",
    duplexResponse,
  );
  assert.deepEqual(
    decodeRPCResponsePayload("server.model.get", duplexPayload),
    duplexResponse,
  );

  assert.throws(
    () =>
      encodeRPCResponsePayload("server.model.get", {
        ...response,
        value: {
          ...response.value,
          openai_tenant: { thinking_levels: [], upstream_model: "gpt-test" },
        },
      }),
    /protobuf message Model has multiple oneof values/,
  );

  assert.throws(
    () =>
      encodeRPCResponsePayload("server.model.get", {
        ...response,
        value: {
          ...response.value,
          deepseek_tenant: undefined,
          openai_tenant: { thinking_levels: [], upstream_model: "gpt-test" },
        },
      }),
    /requires provider_data field deepseek_tenant, got openai_tenant/,
  );

  const mismatchedPayload = payload.slice();
  let providerKindTag = -1;
  for (let index = 0; index + 1 < mismatchedPayload.length; index++) {
    if (
      mismatchedPayload[index] === 0x50 &&
      mismatchedPayload[index + 1] === 0x06
    ) {
      providerKindTag = index;
      break;
    }
  }
  assert.notEqual(providerKindTag, -1);
  mismatchedPayload[providerKindTag + 1] = 0x01;
  assert.throws(
    () => decodeRPCResponsePayload("server.model.get", mismatchedPayload),
    /requires provider_data field openai_tenant, got deepseek_tenant/,
  );
});

test("RPC payload codec rejects ambiguous numeric workspace discriminators", () => {
  assert.throws(
    () =>
      encodeRPCRequestPayload("server.workspace.create", {
        created_at: "now",
        last_active_at: "now",
        name: "main",
        parameters: {
          agent_type: 1,
        },
        updated_at: "now",
        workflow_name: "chat",
      }),
    /no protobuf oneof candidate for WorkspaceParameters/,
  );
});

test("RPC payload codec selects every new workflow workspace discriminator", () => {
  for (const agentType of [
    "dashscope-realtime",
    "doubao-realtime-duplex",
    "eino",
    "pet",
  ]) {
    const request = {
      collection: "assistants",
      name: `workspace-${agentType}`,
      parameters: { agent_type: agentType },
      workflow_name: `workflow-${agentType}`,
    };
    const payload = encodeRPCRequestPayload("server.workspace.create", request);
    const decoded = decodeRPCRequestPayload(
      "server.workspace.create",
      payload,
    ) as { parameters?: { agent_type?: string } };
    assert.equal(decoded.parameters?.agent_type, agentType);
  }
});

test("RPC payload codec round-trips DashScope float options", () => {
  const request = {
    collection: "assistants",
    name: "workspace-dashscope",
    parameters: {
      agent_type: "dashscope-realtime",
      temperature: 0.7,
    },
    workflow_name: "workflow-dashscope",
  };
  const payload = encodeRPCRequestPayload("server.workspace.create", request);
  const decoded = decodeRPCRequestPayload(
    "server.workspace.create",
    payload,
  ) as { parameters?: { temperature?: number } };
  assert.ok(Math.abs((decoded.parameters?.temperature ?? 0) - 0.7) < 0.000001);
});

test("RPC payload codec excludes peer resource mutation methods", () => {
  for (const method of [
    "server.workflow.create",
    "server.model.create",
    "server.credential.create",
    "server.tool.create",
  ]) {
    assert.throws(
      () => encodeRPCRequestPayload(method, {}),
      new RegExp(`unknown RPC method: ${method.replaceAll(".", "\\.")}`),
    );
  }
});

test("RPC method map preserves generated payload types", () => {
  const source = readFileSync(
    new URL("./generated/rpc/method-map.ts", import.meta.url),
    "utf8",
  );

  assert.match(source, /request: PingRequest;/);
  assert.match(source, /response: PingResponse;/);
  assert.doesNotMatch(source, /request: unknown;/);
  assert.doesNotMatch(source, /response: unknown;/);
});

test("RPC API key root management methods preserve IDs and payloads", () => {
  assert.equal(RPC_METHOD_IDS["server.api_key.create"], 96);
  assert.equal(RPC_METHOD_IDS["server.api_key.list"], 97);
  assert.equal(RPC_METHOD_IDS["server.api_key.revoke"], 98);
  assert.equal(RPC_METHOD_IDS["client.device.status.get"], 100);
  assert.equal(RPC_METHOD_IDS["client.device.volume.set"], 101);
  assert.equal(RPC_METHOD_IDS["client.device.sound.play"], 102);
  assert.equal(RPC_METHOD_IDS["client.device.reboot"], 103);
  assert.equal(RPC_METHOD_IDS["client.wifi.status.get"], 104);
  assert.equal(RPC_METHOD_IDS["client.wifi.saved.list"], 105);
  assert.equal(RPC_METHOD_IDS["client.wifi.saved.forget"], 106);

  const list = { cursor: "key_cursor", limit: 25 };
  assert.deepEqual(
    decodeRPCRequestPayload(
      "server.api_key.list",
      encodeRPCRequestPayload("server.api_key.list", list),
    ),
    list,
  );
  const completeKey =
    "gizclaw_sk_v1_0123456789012345678901234567890123456789012";
  const listResponse = {
    items: [
      {
        name: "key_name",
        display_name: "",
        prefix: "",
        api_key: completeKey,
        manage_api_keys: false,
        created_at: "",
      },
    ],
  };
  assert.deepEqual(
    decodeRPCResponsePayload(
      "server.api_key.list",
      encodeRPCResponsePayload("server.api_key.list", listResponse),
    ),
    listResponse,
  );
  const revoke = { name: "key_name" };
  assert.deepEqual(
    decodeRPCRequestPayload(
      "server.api_key.revoke",
      encodeRPCRequestPayload("server.api_key.revoke", revoke),
    ),
    revoke,
  );
});

test("RPC payload codec decodes omitted proto3 defaults", () => {
  assert.deepEqual(
    decodeRPCResponsePayload("all.speed_test.run", new Uint8Array()),
    {
      down_content_length: 0,
      up_content_length: 0,
    },
  );
  assert.deepEqual(
    decodeRPCResponsePayload("server.workspace.list", new Uint8Array()),
    {
      has_next: false,
      items: [],
      runtime_profile_name: "",
      runtime_profile_revision: "",
    },
  );
});

test("RPC payload codec round-trips device control payloads", () => {
  const volume = { level: 35, muted: true };
  assert.deepEqual(
    decodeRPCRequestPayload(
      "client.device.volume.set",
      encodeRPCRequestPayload("client.device.volume.set", volume),
    ),
    volume,
  );
  const status = { volume: 35, muted: true, battery_percent: 80 };
  const decodedStatus = decodeRPCResponsePayload(
    "client.device.volume.set",
    encodeRPCResponsePayload("client.device.volume.set", status),
  );
  assert.equal(decodedStatus.volume, 35);
  assert.equal(decodedStatus.muted, true);
  assert.equal(decodedStatus.battery_percent, 80);

  const sound = { sound: "chime", duration_ms: 1500 };
  assert.deepEqual(
    decodeRPCRequestPayload(
      "client.device.sound.play",
      encodeRPCRequestPayload("client.device.sound.play", sound),
    ),
    sound,
  );
  const wifi = {
    connected: true,
    ssid: "home",
    rssi_dbm: -55,
    ip: "192.0.2.10",
  };
  const decodedWifi = decodeRPCResponsePayload(
    "client.wifi.status.get",
    encodeRPCResponsePayload("client.wifi.status.get", wifi),
  );
  assert.equal(decodedWifi.connected, true);
  assert.equal(decodedWifi.ssid, "home");
  assert.equal(decodedWifi.rssi_dbm, -55);
  const saved = { networks: [{ ssid: "home" }, { ssid: "office" }] };
  assert.deepEqual(
    decodeRPCResponsePayload(
      "client.wifi.saved.list",
      encodeRPCResponsePayload("client.wifi.saved.list", saved),
    ),
    saved,
  );
  assert.deepEqual(
    decodeRPCRequestPayload(
      "client.wifi.saved.forget",
      encodeRPCRequestPayload("client.wifi.saved.forget", { ssid: "office" }),
    ),
    { ssid: "office" },
  );
});

test("peer HTTP SDK exposes device and contact operations", () => {
  for (const name of [
    "getDevice",
    "getDeviceRuntime",
    "getDeviceStatus",
    "getDeviceTelemetryLatest",
    "queryDeviceTelemetry",
    "aggregateDeviceTelemetry",
    "setDeviceVolume",
    "playDeviceSound",
    "rebootDevice",
    "getDeviceWifi",
    "listDeviceSavedWifi",
    "forgetDeviceSavedWifi",
    "listContacts",
    "createContact",
    "getContact",
    "putContact",
    "deleteContact",
  ]) {
    assert.equal(
      typeof peerhttp[name as keyof typeof peerhttp],
      "function",
      name,
    );
  }
});

test("RPC payload codec preserves Tool invocation JSON strings", () => {
  const value = { data_json: `{"ok":true}` };
  const payload = encodeRPCResponsePayload("client.tool.invoke", value);

  assert.deepEqual(
    decodeRPCResponsePayload("client.tool.invoke", payload),
    value,
  );
});

test("RPC payload codec preserves safe Tool JSON schema fields", () => {
  const payload = encodeRPCResponsePayload("server.tool.get", {
    runtime_profile_name: "default",
    runtime_profile_revision: "revision-1",
    value: {
      name: "lookup",
      invoke_name: "canonical_lookup",
      i18n: {},
      input_schema: {
        type: "string",
        additionalProperties: false,
        minLength: 1,
      },
    },
  });

  const decoded = decodeRPCResponsePayload("server.tool.get", payload) as {
    value?: {
      input_schema?: Record<string, unknown>;
      invoke_name?: string;
      name?: string;
    };
  };
  const parameters = decoded.value?.input_schema;

  assert.equal(decoded.value?.name, "lookup");
  assert.equal(decoded.value?.invoke_name, "canonical_lookup");
  assert.equal(parameters?.additionalProperties, false);
  assert.equal(parameters?.minLength, 1);
  assert.equal(parameters?.type, "string");
  assert.equal(
    Object.prototype.hasOwnProperty.call(parameters ?? {}, "enum"),
    false,
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(parameters ?? {}, "required"),
    false,
  );
  assert.equal(
    Object.prototype.hasOwnProperty.call(parameters ?? {}, "anyOf"),
    false,
  );
});

test("RPC payload codec exposes only runtime workflow names", () => {
  const workflow = {
    runtime_profile_name: "default",
    runtime_profile_revision: "revision-1",
    value: {
      name: "assistant",
      collection: "assistants",
      driver: "doubao-realtime",
      i18n: {
        en: { display_name: "Assistant" },
      },
    },
  };
  const payload = encodeRPCResponsePayload("server.workflow.get", workflow);
  const decoded = decodeRPCResponsePayload("server.workflow.get", payload) as {
    value?: {
      name?: string;
      collection?: string;
      owner_public_key?: string;
      spec?: unknown;
    };
  };

  assert.equal(decoded.value?.name, "assistant");
  assert.equal(decoded.value?.collection, "assistants");
  assert.equal(decoded.value?.owner_public_key, undefined);
  assert.equal(decoded.value?.spec, undefined);
});

test("RPC payload codec addresses workflows by peer-visible name", () => {
  const payload = encodeRPCRequestPayload("server.workflow.get", {
    name: "assistant",
  });
  assert.deepEqual(decodeRPCRequestPayload("server.workflow.get", payload), {
    name: "assistant",
  });
});

test("RPC payload codec rejects string values for bool fields", () => {
  assert.throws(
    () =>
      encodeRPCRequestPayload("server.workspace.create", {
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

test("RPC payload codec rejects unknown enum strings", () => {
  assert.throws(
    () =>
      encodeRPCRequestPayload("server.firmware.get", {
        channel: "stabel",
      }),
    /unknown protobuf enum value for FirmwareChannelName: stabel/,
  );
});

test("Firmware RPC generated contract round-trips every channel and field", () => {
  for (const channel of ["stable", "beta", "develop"] as const) {
    const request = { channel };
    assert.deepEqual(
      decodeRPCRequestPayload(
        "server.firmware.get",
        encodeRPCRequestPayload("server.firmware.get", request),
      ),
      request,
    );
  }

  const response = {
    channel: "stable" as const,
    description: "stable package",
    sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    size: Number.MAX_SAFE_INTEGER,
    url: "https://firmware.example.invalid/devkit/stable.tar.zlib",
  };
  assert.deepEqual(
    decodeRPCResponsePayload(
      "server.firmware.get",
      encodeRPCResponsePayload("server.firmware.get", response),
    ),
    response,
  );
  const withoutDescription = { ...response };
  delete (withoutDescription as { description?: string }).description;
  assert.equal(
    (
      decodeRPCResponsePayload(
        "server.firmware.get",
        encodeRPCResponsePayload("server.firmware.get", withoutDescription),
      ) as { description?: string }
    ).description,
    undefined,
  );

  assert.equal(RPC_METHOD_IDS["server.firmware.get"], 22);
  assert.equal("server.firmware.download" in RPC_METHODS, false);
  assert.throws(
    () =>
      encodeRPCRequestPayload("server.firmware.get", {
        channel: "pending" as never,
      }),
    /unknown protobuf enum value for FirmwareChannelName: pending/,
  );
});

test("WebRTCRPCClient reassembles response frames split across messages", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-split" });

  const promise = client.call<{ server_time: number }>("all.ping", {
    client_send_time: 2,
  });
  const channel = pc.lastChannel();
  channel.open();

  const response = new Uint8Array(
    encodeRPCResponse(
      { id: "req-split", result: { server_time: 100 }, v: 1 },
      "all.ping",
    ),
  );
  channel.receiveBytes(response.slice(0, 5));
  channel.receiveBytes(response.slice(5));

  assert.deepEqual(await promise, { server_time: 100 });
});

test("WebRTCRPCClient reassembles oversized protobuf envelope continuation frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-continuation",
  });

  const promise = client.call<{ server_time: number }>("all.ping", {
    client_send_time: 3,
  });
  const channel = pc.lastChannel();
  channel.open();

  const responseFrames = decodeFrames(
    encodeRPCResponse(
      { id: "req-continuation", result: { server_time: 101 }, v: 1 },
      "all.ping",
    ),
  );
  const envelope = responseFrames[0]?.payload;
  assert.ok(envelope);
  const split = Math.floor(envelope.length / 2);
  channel.receive(encodeFrame(RPC_FRAME_TYPE_TEXT, envelope.slice(0, split)));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_TEXT, envelope.slice(split)));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));

  assert.deepEqual(await promise, { server_time: 101 });
});

test("WebRTCRPCClient rejects oversized protobuf response continuation frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-continuation-too-large",
  });

  const promise = client.call("all.ping", { client_send_time: 3 });
  const channel = pc.lastChannel();
  channel.open();

  const chunk = new Uint8Array(0xffff);
  for (let i = 0; i < 17; i += 1) {
    channel.receive(encodeFrame(RPC_FRAME_TYPE_TEXT, chunk));
  }

  await assert.rejects(promise, /RPC protobuf envelope too large/);
});

test("WebRTCRPCClient resolves queued response before close", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-close" });

  const promise = client.call<{ server_time: number }>("all.ping", {
    client_send_time: 4,
  });
  const channel = pc.lastChannel();
  channel.open();
  channel.receive(
    encodeRPCResponse(
      { id: "req-close", result: { server_time: 102 }, v: 1 },
      "all.ping",
    ),
  );
  channel.remoteClose();

  assert.deepEqual(await promise, { server_time: 102 });
});

test("WebRTCRPCClient reads metadata plus binary response frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-binary" });

  const promise = client.callBinary<{ mime_type: string; size_bytes: number }>(
    "server.workspace.history.audio.download",
    {
      history_name: "h1",
      workspace_name: "main",
    },
  );
  const channel = pc.lastChannel();
  channel.open();

  channel.receive(
    encodeRPCResponse(
      {
        id: "req-binary",
        result: { mime_type: "audio/ogg", size_bytes: 5 },
        v: 1,
      },
      "server.workspace.history.audio.download",
    ).slice(0, -4),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([1, 2])));
  channel.receive(
    encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([3, 4, 5])),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));

  const result = await promise;
  assert.deepEqual(result.result, {
    history_name: "",
    mime_type: "audio/ogg",
    size_bytes: 5,
    workspace_name: "",
  });
  assert.deepEqual(result.body, new Uint8Array([1, 2, 3, 4, 5]));
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient streams transcription audio before request EOS", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "speech-transcribe",
  });
  let releaseUpload!: () => void;
  const uploadGate = new Promise<void>((resolve) => {
    releaseUpload = resolve;
  });
  async function* audio(): AsyncIterable<Uint8Array> {
    yield new Uint8Array([1, 2]);
    await uploadGate;
    yield new Uint8Array([3, 4]);
  }

  const promise = client.transcribeSpeech(
    {
      model_name: "asr-main",
      content_type: "audio/L16;rate=16000;channels=1",
    },
    audio(),
  );
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSentCount(2);

  assert.equal(
    decodeFrames(channel.sent[0] ?? new ArrayBuffer(0))[0]?.type,
    RPC_FRAME_TYPE_BINARY,
  );
  assert.deepEqual(decodeFrames(channel.sent[1] ?? new ArrayBuffer(0)), [
    { payload: new Uint8Array([1, 2]), type: RPC_FRAME_TYPE_BINARY },
  ]);
  assert.equal(
    channel.sent.some(
      (message) => decodeFrames(message)[0]?.type === RPC_FRAME_TYPE_EOS,
    ),
    false,
  );

  releaseUpload();
  await channel.waitForSentCount(4);
  assert.equal(
    decodeFrames(channel.sent[3] ?? new ArrayBuffer(0))[0]?.type,
    RPC_FRAME_TYPE_EOS,
  );
  channel.receive(
    encodeRPCResponse(
      {
        id: "speech-transcribe",
        result: { transcript: "hello" },
        v: 1,
      },
      "server.speech.transcribe",
    ),
  );

  assert.deepEqual(await promise, { transcript: "hello" });
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient streams extraction audio before request EOS", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "speech-extract",
  });
  let releaseUpload!: () => void;
  const uploadGate = new Promise<void>((resolve) => {
    releaseUpload = resolve;
  });
  async function* audio(): AsyncIterable<Uint8Array> {
    yield new Uint8Array([1, 2]);
    await uploadGate;
    yield new Uint8Array([3, 4]);
  }

  const promise = client.extractSpeech(
    {
      asr_model_name: "asr-main",
      extract_model_name: "extract-main",
      content_type: "audio/L16;rate=16000;channels=1",
      schema_json:
        '{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}',
    },
    audio(),
  );
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSentCount(2);

  assert.equal(
    decodeFrames(channel.sent[0] ?? new ArrayBuffer(0))[0]?.type,
    RPC_FRAME_TYPE_BINARY,
  );
  assert.deepEqual(decodeFrames(channel.sent[1] ?? new ArrayBuffer(0)), [
    { payload: new Uint8Array([1, 2]), type: RPC_FRAME_TYPE_BINARY },
  ]);
  releaseUpload();
  await channel.waitForSentCount(4);
  assert.equal(
    decodeFrames(channel.sent[3] ?? new ArrayBuffer(0))[0]?.type,
    RPC_FRAME_TYPE_EOS,
  );
  channel.receive(
    encodeRPCResponse(
      {
        id: "speech-extract",
        result: {
          transcript: "name is GizClaw",
          result_json: '{"name":"GizClaw"}',
        },
        v: 1,
      },
      "server.speech.extract",
    ),
  );

  assert.deepEqual(await promise, {
    result_json: '{"name":"GizClaw"}',
    transcript: "name is GizClaw",
  });
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient delimits a split transcription envelope before audio", async () => {
  const largeID = "r".repeat(0xffff + 1024);
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => largeID });
  const promise = client.transcribeSpeech(
    {
      model_name: "asr-main",
      content_type: "audio/L16;rate=16000;channels=1",
    },
    [new Uint8Array([1, 2])],
  );
  const channel = pc.lastChannel();
  channel.open();
  await new Promise<void>((resolve) => setImmediate(resolve));

  const sentFrames = decodeFrames(concatBuffers(channel.sent));
  assert.equal(sentFrames[0]?.type, RPC_FRAME_TYPE_TEXT);
  assert.equal(sentFrames[1]?.type, RPC_FRAME_TYPE_TEXT);
  assert.equal(sentFrames[2]?.type, RPC_FRAME_TYPE_EOS);
  assert.deepEqual(sentFrames[3], {
    payload: new Uint8Array([1, 2]),
    type: RPC_FRAME_TYPE_BINARY,
  });
  assert.equal(sentFrames[4]?.type, RPC_FRAME_TYPE_EOS);

  channel.receive(
    encodeRPCResponse(
      {
        id: largeID,
        result: { transcript: "hello" },
        v: 1,
      },
      "server.speech.transcribe",
    ),
  );
  assert.deepEqual(await promise, { transcript: "hello" });
});

test("WebRTCRPCClient stops a live transcription upload on an early response", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "speech-early-error",
  });
  let reads = 0;
  let returned = false;
  const audio: AsyncIterable<Uint8Array> = {
    [Symbol.asyncIterator]() {
      return {
        next: async () => {
          reads += 1;
          if (reads === 1)
            return { done: false, value: new Uint8Array([1, 2]) };
          return await new Promise<IteratorResult<Uint8Array>>(() => {});
        },
        return: async () => {
          returned = true;
          return { done: true, value: undefined };
        },
      };
    },
  };

  const promise = client.transcribeSpeech(
    {
      model_name: "missing",
      content_type: "audio/L16;rate=16000;channels=1",
    },
    audio,
  );
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSentCount(2);
  channel.receive(
    encodeRPCResponse(
      {
        error: {
          code: STATUS_CODE_INVALID_ARGUMENT,
          message: "model alias is invalid",
        },
        id: "speech-early-error",
        v: 1,
      },
      "server.speech.transcribe",
    ),
  );

  await assert.rejects(promise, /model alias is invalid/);
  assert.equal(returned, true);
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient stops a live extraction upload on an early response", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "speech-extract-early-error",
  });
  let reads = 0;
  let returned = false;
  const audio: AsyncIterable<Uint8Array> = {
    [Symbol.asyncIterator]() {
      return {
        next: async () => {
          reads += 1;
          if (reads === 1)
            return { done: false, value: new Uint8Array([1, 2]) };
          return await new Promise<IteratorResult<Uint8Array>>(() => {});
        },
        return: async () => {
          returned = true;
          return { done: true, value: undefined };
        },
      };
    },
  };

  const promise = client.extractSpeech(
    {
      asr_model_name: "asr-main",
      extract_model_name: "missing",
      content_type: "audio/L16;rate=16000;channels=1",
      schema_json: '{"type":"object"}',
    },
    audio,
  );
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSentCount(2);
  channel.receive(
    encodeRPCResponse(
      {
        error: { code: 5, message: "speech alias not found" },
        id: "speech-extract-early-error",
        v: 1,
      },
      "server.speech.extract",
    ),
  );

  await assert.rejects(promise, /speech alias not found/);
  assert.equal(returned, true);
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient cancels a live extraction upload with AbortSignal", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "speech-extract-cancel",
  });
  const controller = new AbortController();
  let returned = false;
  const audio: AsyncIterable<Uint8Array> = {
    [Symbol.asyncIterator]() {
      let first = true;
      return {
        next: async () => {
          if (first) {
            first = false;
            return { done: false, value: new Uint8Array([1, 2]) };
          }
          return await new Promise<IteratorResult<Uint8Array>>(() => {});
        },
        return: async () => {
          returned = true;
          return { done: true, value: undefined };
        },
      };
    },
  };

  const promise = client.extractSpeech(
    {
      asr_model_name: "asr-main",
      extract_model_name: "extract-main",
      content_type: "audio/L16;rate=16000;channels=1",
      schema_json: '{"type":"object"}',
    },
    audio,
    { signal: controller.signal },
  );
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSentCount(2);
  controller.abort();

  await assert.rejects(promise, { name: "AbortError" });
  assert.equal(returned, true);
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient exposes synthesized audio before response EOS", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "speech-synthesize",
  });
  const promise = client.synthesizeSpeech({
    voice_name: "narrator",
    text: "hello",
    accepted_content_types: ["audio/pcm"],
  });
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSent();

  channel.receive(
    encodeRPCResponse(
      {
        id: "speech-synthesize",
        result: {
          content_type: "audio/pcm",
          sample_rate_hz: 16000,
          channels: 1,
        },
        v: 1,
      },
      "server.speech.synthesize",
    ).slice(0, -4),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([1, 2])));

  const result = await promise;
  assert.deepEqual(result.result, {
    channels: 1,
    content_type: "audio/pcm",
    sample_rate_hz: 16000,
  });
  const iterator = result.body[Symbol.asyncIterator]();
  assert.deepEqual(await iterator.next(), {
    done: false,
    value: new Uint8Array([1, 2]),
  });
  assert.equal(channel.closed, false);

  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([3, 4])));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));
  assert.deepEqual(await iterator.next(), {
    done: false,
    value: new Uint8Array([3, 4]),
  });
  assert.deepEqual(await iterator.next(), { done: true, value: undefined });
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient gives speech calls speech-specific default timeouts", async () => {
  assert.equal(SPEECH_EXTRACTION_REQUEST_TIMEOUT_MS, 125000);
  assert.equal(SPEECH_TRANSCRIPTION_REQUEST_TIMEOUT_MS, 80000);
  assert.equal(SPEECH_SYNTHESIS_REQUEST_TIMEOUT_MS, 125000);

  const transcriptionPC = new FakePeerConnection();
  const transcriptionClient = new WebRTCRPCClient(transcriptionPC, {
    createID: () => "slow-transcription",
    requestTimeoutMs: 1,
  });
  const transcription = transcriptionClient.transcribeSpeech(
    {
      model_name: "asr-main",
      content_type: "audio/L16;rate=16000;channels=1",
    },
    [new Uint8Array([1, 2])],
  );
  const transcriptionChannel = transcriptionPC.lastChannel();
  transcriptionChannel.open();
  await new Promise((resolve) => setTimeout(resolve, 20));
  transcriptionChannel.receive(
    encodeRPCResponse(
      {
        id: "slow-transcription",
        result: { transcript: "hello" },
        v: 1,
      },
      "server.speech.transcribe",
    ),
  );
  assert.deepEqual(await transcription, { transcript: "hello" });

  const synthesisPC = new FakePeerConnection();
  const synthesisClient = new WebRTCRPCClient(synthesisPC, {
    createID: () => "slow-synthesis",
    requestTimeoutMs: 1,
  });
  const synthesis = synthesisClient.synthesizeSpeech({
    voice_name: "narrator",
    text: "hello",
    accepted_content_types: ["audio/pcm"],
  });
  const synthesisChannel = synthesisPC.lastChannel();
  synthesisChannel.open();
  await new Promise((resolve) => setTimeout(resolve, 20));
  synthesisChannel.receive(
    encodeRPCResponse(
      {
        id: "slow-synthesis",
        result: {
          content_type: "audio/pcm",
          sample_rate_hz: 16000,
          channels: 1,
        },
        v: 1,
      },
      "server.speech.synthesize",
    ).slice(0, -4),
  );
  synthesisChannel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));
  const result = await synthesis;
  assert.equal(result.result.content_type, "audio/pcm");
});

test("WebRTCRPCClient reads continuation metadata plus binary response frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-binary-continuation",
  });

  const promise = client.callBinary<{
    mime_type: string;
    size_bytes: number;
    workspace_name: string;
  }>("server.workspace.history.audio.download", {
    history_name: "h1",
    workspace_name: "main",
  });
  const channel = pc.lastChannel();
  channel.open();

  const workspaceName = "w".repeat(70000);
  channel.receive(
    encodeRPCResponse(
      {
        id: "req-binary-continuation",
        result: {
          mime_type: "audio/ogg",
          size_bytes: 2,
          workspace_name: workspaceName,
        },
        v: 1,
      },
      "server.workspace.history.audio.download",
    ),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array([8, 9])));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));

  const result = await promise;
  assert.equal(result.result.workspace_name, workspaceName);
  assert.deepEqual(result.body, new Uint8Array([8, 9]));
});

test("WebRTCRPCClient rejects oversized binary metadata continuation frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-binary-continuation-too-large",
  });

  const promise = client.callBinary("server.workspace.history.audio.download", {
    history_name: "h1",
    workspace_name: "main",
  });
  const channel = pc.lastChannel();
  channel.open();

  const chunk = new Uint8Array(0xffff);
  for (let i = 0; i < 17; i += 1) {
    channel.receive(encodeFrame(RPC_FRAME_TYPE_TEXT, chunk));
  }

  await assert.rejects(promise, /RPC protobuf envelope too large/);
});

test("WebRTCRPCClient rejects continuation binary RPC errors without body frames", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-binary-error",
  });

  const promise = client.callBinary("server.workspace.history.audio.download", {
    history_name: "h1",
    workspace_name: "main",
  });
  const channel = pc.lastChannel();
  channel.open();

  channel.receive(
    encodeRPCResponse(
      {
        error: { code: STATUS_CODE_INTERNAL, message: "x".repeat(70000) },
        id: "req-binary-error",
        v: 1,
      },
      "server.workspace.history.audio.download",
    ),
  );

  await assert.rejects(promise, (err) => {
    assert.equal(err instanceof WebRTCRPCError, true);
    assert.equal((err as WebRTCRPCError).code, STATUS_CODE_INTERNAL);
    assert.equal((err as WebRTCRPCError).message, "x".repeat(70000));
    return true;
  });
});

test("WebRTCRPCClient rejects RPC error responses", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-2" });

  const promise = client.call("server.run.workspace.reload");
  const channel = pc.lastChannel();
  channel.open();
  channel.receive(
    encodeRPCResponse(
      {
        error: { code: STATUS_CODE_INTERNAL, message: "boom" },
        id: "req-2",
        v: 1,
      },
      "server.run.workspace.reload",
    ),
  );

  await assert.rejects(promise, (err) => {
    assert.equal(err instanceof WebRTCRPCError, true);
    assert.equal((err as WebRTCRPCError).code, STATUS_CODE_INTERNAL);
    assert.equal((err as WebRTCRPCError).message, "boom");
    return true;
  });
});

test("WebRTCRPCClient rejects unknown methods before data channel open", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-unknown",
    requestTimeoutMs: 0,
  });

  await assert.rejects(
    client.call("peer.unknown", {}),
    /unknown RPC method: peer\.unknown/,
  );
  assert.equal(pc.lastChannel().closed, true);
  assert.equal(pc.lastChannel().sent.length, 0);
});

test("WebRTCRPCClient rejects unknown binary methods before data channel open", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-unknown-binary",
    requestTimeoutMs: 0,
  });

  await assert.rejects(
    client.callBinary("peer.unknown", {}),
    /unknown RPC method: peer\.unknown/,
  );
  assert.equal(pc.lastChannel().closed, true);
  assert.equal(pc.lastChannel().sent.length, 0);
});

test("WebRTCRPCClient honors AbortSignal", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, {
    createID: () => "req-3",
    requestTimeoutMs: 0,
  });
  const ac = new AbortController();

  const promise = client.call(
    "server.run.workspace.get",
    {},
    { signal: ac.signal },
  );
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
    transcribeSpeech: async () => {
      throw new Error("unexpected transcription call");
    },
    extractSpeech: async () => {
      throw new Error("unexpected extraction call");
    },
    synthesizeSpeech: async () => {
      throw new Error("unexpected synthesis call");
    },
  } as unknown as WebRTCRPCClient;
  const rpc = createPeerRPCClient(client);

  await rpc.call("server.run.workspace.set", { workspace_name: "main" });
  await rpc.call("server.run.workspace.history.play", { history_name: "h1" });
  await rpc.call("server.firmware.get", {
    channel: "stable",
  });
  await rpc.call("server.friend_group.messages.list", {
    friend_group_name: "group-a",
  });

  assert.deepEqual(calls, [
    { method: "server.run.workspace.set", params: { workspace_name: "main" } },
    {
      method: "server.run.workspace.history.play",
      params: { history_name: "h1" },
    },
    {
      method: "server.firmware.get",
      params: { channel: "stable" },
    },
    {
      method: "server.friend_group.messages.list",
      params: { friend_group_name: "group-a" },
    },
  ]);
});

test("createPeerRPCClient rejects legacy callers without speech methods", () => {
  const legacyCaller = {
    call: async () => ({}),
    callBinary: async () => ({ body: new Uint8Array(), result: {} }),
  };

  assert.throws(
    () => createPeerRPCClient(legacyCaller as never),
    /must implement call, callBinary, extractSpeech, transcribeSpeech, and synthesizeSpeech/,
  );
});

test("createEdgeRPCClient uses the edge RPC service channel", async () => {
  const pc = new FakePeerConnection();
  const rpc = createEdgeRPCClient(pc, { createID: () => "req-edge" });

  const promise = rpc.call("server.route.resolve", {
    target_peer_public_key: "peer-b",
  });
  const channel = pc.lastChannel();
  channel.open();

  assert.equal(
    channel.label,
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_EDGE_RPC),
  );
  channel.receive(
    encodeRPCResponse(
      {
        id: "req-edge",
        result: {
          assignment: {
            role: "edge-node",
            server_endpoint: "https://edge.example",
            server_public_key: "server-pk",
            version: 1,
          },
        },
        v: 1,
      },
      "server.route.resolve",
    ),
  );

  assert.deepEqual(await promise, {
    assignment: {
      peer_public_key: "",
      role: "edge-node",
      server_endpoint: "https://edge.example",
      server_public_key: "server-pk",
      updated_at: "",
      version: 1,
    },
  });
});

test("createEdgeRPCClient calls generated edge RPC methods", async () => {
  const calls: Array<{ method: string; params: unknown }> = [];
  const client = {
    call: async (method: string, params: unknown) => {
      calls.push({ method, params });
      return { assignment: { server_public_key: "server-pk" } };
    },
    callBinary: async () => {
      throw new Error("unexpected binary call");
    },
  } as unknown as WebRTCRPCClient;
  const rpc = createEdgeRPCClient(client);

  await rpc.call("server.peer.lookup", { peer_public_key: "peer-a" });
  await rpc.call("server.peer.assign", { peer_public_key: "peer-a" });
  await rpc.call("server.route.resolve", { target_peer_public_key: "peer-a" });
  await rpc.call("server.api_key.resolve", { api_key: "gzk_test" });

  assert.deepEqual(calls, [
    { method: "server.peer.lookup", params: { peer_public_key: "peer-a" } },
    { method: "server.peer.assign", params: { peer_public_key: "peer-a" } },
    {
      method: "server.route.resolve",
      params: { target_peer_public_key: "peer-a" },
    },
    { method: "server.api_key.resolve", params: { api_key: "gzk_test" } },
  ]);
});

test("createPeerRPCClient forwards dedicated streaming speech methods", async () => {
  const audio = [new Uint8Array([1, 2])];
  const calls: string[] = [];
  const client = {
    call: async () => ({}),
    callBinary: async () => ({ body: new Uint8Array(), result: {} }),
    transcribeSpeech: async (
      params: { model_name: string },
      input: Iterable<Uint8Array>,
    ) => {
      calls.push(
        `transcribe:${params.model_name}:${Array.from(input)[0]?.byteLength ?? 0}`,
      );
      return { transcript: "hello" };
    },
    extractSpeech: async (
      params: { extract_model_name: string },
      input: Iterable<Uint8Array>,
    ) => {
      calls.push(
        `extract:${params.extract_model_name}:${Array.from(input)[0]?.byteLength ?? 0}`,
      );
      return {
        transcript: "name is GizClaw",
        result_json: '{"name":"GizClaw"}',
      };
    },
    synthesizeSpeech: async (params: { voice_name: string }) => {
      calls.push(`synthesize:${params.voice_name}`);
      return {
        body: (async function* () {
          yield new Uint8Array([3, 4]);
        })(),
        cancel: () => {},
        result: { content_type: "audio/pcm" },
      };
    },
  } as unknown as WebRTCRPCClient;
  const rpc = createPeerRPCClient(client);

  assert.equal(
    (
      await rpc.transcribeSpeech(
        {
          model_name: "2fa-asr",
          content_type: "audio/L16;rate=16000;channels=1",
        },
        audio,
      )
    ).transcript,
    "hello",
  );
  assert.equal(
    (
      await rpc.extractSpeech(
        {
          asr_model_name: "2fa-asr",
          extract_model_name: "2fa-extract",
          content_type: "audio/L16;rate=16000;channels=1",
          schema_json:
            '{"type":"object","properties":{"name":{"type":"string"}}}',
        },
        audio,
      )
    ).result_json,
    '{"name":"GizClaw"}',
  );
  const synthesis = await rpc.synthesizeSpeech({
    voice_name: "2fa-voice",
    text: "hello",
    accepted_content_types: ["audio/pcm"],
  });
  let synthesizedBytes = 0;
  for await (const chunk of synthesis.body)
    synthesizedBytes += chunk.byteLength;
  assert.equal(synthesizedBytes, 2);
  assert.deepEqual(calls, [
    "transcribe:2fa-asr:2",
    "extract:2fa-extract:2",
    "synthesize:2fa-voice",
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

  assert.equal(
    channel.label,
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_ADMIN_HTTP),
  );
  await channel.waitForSent();
  const requestText = new TextDecoder().decode(
    channel.sent[0] ?? new ArrayBuffer(0),
  );
  assert.match(requestText, /^GET \/peers\?limit=10 HTTP\/1\.1\r\n/);
  assert.match(requestText, /\r\nHost: gizclaw\r\n/);
  assert.match(requestText, /\r\nConnection: close\r\n/);

  const body = JSON.stringify({ has_next: false, items: [] });
  channel.receiveText(
    `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${body.length}\r\n\r\n${body}`,
  );

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
  channel.receiveText(
    `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: ${body.length}\r\n\r\n${body}`,
  );

  const response = await promise;
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { has_next: false, items: [] });
});

test("connectGiznetWebRTC waits for both mandatory data channels", async () => {
  const pc = new FakePeerConnection();
  let settled = false;
  const connected = connectGiznetWebRTC({
    pc: pc as unknown as RTCPeerConnection,
    prepareOffer: async () => ({
      body: new Blob(),
      clientPublicKey: "client",
      nonce: "nonce",
      openAnswer: async () => "v=0",
      timestamp: 1,
    }),
    sendOffer: async () => new Blob(),
  }).then((result) => {
    settled = true;
    return result;
  });
  await pc.waitForChannel(GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL);
  assert.equal(settled, false);

  pc.channel(GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL).open();
  await new Promise<void>((resolve) => setImmediate(resolve));
  assert.equal(settled, false);
  pc.channel(giznetServiceDataChannelLabel(0x20)).open();

  assert.equal(await connected, pc);
  assert.equal(settled, true);
});

test("rewriteGiznetWebRTCAnswerForEndpoint uses a loopback signaling endpoint for host candidates", () => {
  const answer = [
    "v=0",
    "a=candidate:1 1 udp 2130706431 100.100.100.100 63933 typ host generation 0",
    "a=candidate:2 1 udp 1694498815 198.51.100.10 50000 typ srflx raddr 100.100.100.100 rport 63933",
    "",
  ].join("\r\n");

  const rewritten = rewriteGiznetWebRTCAnswerForEndpoint(
    answer,
    "127.0.0.1:63933",
  );

  assert.match(
    rewritten,
    /a=candidate:1 1 udp 2130706431 127\.0\.0\.1 63933 typ host/,
  );
  assert.match(
    rewritten,
    /a=candidate:2 1 udp 1694498815 198\.51\.100\.10 50000 typ srflx/,
  );
  assert.equal(
    rewriteGiznetWebRTCAnswerForEndpoint(answer, "192.168.1.20:63933"),
    answer,
  );
});

test("createAdminAPIFetch reads chunked HTTP service responses", async () => {
  const pc = new FakePeerConnection();
  const adminFetch = createAdminAPIFetch(pc);
  const promise = adminFetch("http://gizclaw/server");
  const channel = pc.lastChannel();
  channel.open();
  await channel.waitForSent();

  channel.receiveText(
    'HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nB\r\n{"ok":t',
  );
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

  channel.receiveText(
    "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nhello",
  );
  channel.remoteClose();

  const response = await promise;
  assert.equal(response.status, 200);
  assert.equal(await response.text(), "hello");
});

test("SSE client parses HTTP JSON errors without retrying", async () => {
  const errorBody = {
    error: {
      code: "LOG_QUERY_NOT_CONFIGURED",
      message: "server log query backend is not configured",
    },
  };
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
  const errorBody = {
    error: { code: "LOG_BACKEND_ERROR", message: "backend search failed" },
  };
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
  assert.deepEqual(seenErrors, [
    "temporarily unavailable",
    "temporarily unavailable",
  ]);
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

test("prepareGiznetWebRTCPeerConnection creates all mandatory transports", () => {
  const pc = new FakePeerConnection();

  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  assert.equal(pc.channels[0]?.label, GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL);
  assert.deepEqual(pc.channels[0]?.options, {
    maxRetransmits: 0,
    ordered: false,
  });
  assert.equal(
    getGiznetWebRTCPacketDataChannel(pc as unknown as RTCPeerConnection),
    pc.channels[0],
  );
  assert.equal(
    getGiznetWebRTCPeerEventDataChannel(pc as unknown as RTCPeerConnection),
    pc.channels[1],
  );
  assert.equal(pc.channels[1]?.label, giznetServiceDataChannelLabel(0x20));
  assert.deepEqual(pc.channels[1]?.options, { ordered: true });
  assert.deepEqual(pc.transceivers, [
    { kind: "audio", init: { direction: "sendrecv" } },
  ]);
});

test("closing or failing either mandatory data channel closes the peer connection", () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  pc.channels[0]?.fail();
  assert.equal(pc.closeCalls, 1);
  assert.equal(pc.connectionState, "closed");

  pc.channels[1]?.remoteClose();
  assert.equal(pc.closeCalls, 1);
});

test("failing the peer connection closes its mandatory transports", () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  pc.failConnection();

  assert.equal(pc.closeCalls, 1);
  assert.equal(pc.connectionState, "closed");
});

test("prepared peer closes duplicate remote mandatory channels", () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  for (const label of [
    GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL,
    giznetServiceDataChannelLabel(GIZCLAW_EVENT_STREAM_AGENT),
  ]) {
    const duplicate = new FakeDataChannel(label);
    pc.receiveDataChannel(duplicate);
    assert.equal(duplicate.closed, true);
  }
});

test("prepared WebRTC peer serves server-initiated protobuf ping", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  channel.open();
  pc.receiveDataChannel(channel);

  channel.receive(
    encodeRPCRequest({
      id: "server-ping",
      method: "all.ping",
      params: { client_send_time: 123 },
      v: 1,
    }),
  );
  await channel.waitForSentCount(2);

  const response = parseRPCResponse<{ server_time: number }>(
    concatBuffers(channel.sent),
    "all.ping",
  );
  assert.equal(response.id, "server-ping");
  assert.ok((response.result?.server_time ?? 0) > 0);
});

test("prepared WebRTC peer serves full-duplex server-initiated speed test", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  channel.open();
  pc.receiveDataChannel(channel);

  const request = decodeFrames(
    encodeRPCRequest({
      id: "server-speed",
      method: "all.speed_test.run",
      params: {
        down_content_length: 32 * 1024 + 7,
        up_content_length: 32 * 1024 + 5,
      },
      v: 1,
    }),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, request[0]?.payload));
  channel.receive(
    encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array(32 * 1024)),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array(5)));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));
  await channel.waitForSentCount(4);

  const frames = decodeFrames(concatBuffers(channel.sent));
  assert.deepEqual(
    frames.map((frame) => frame.type),
    [
      RPC_FRAME_TYPE_BINARY,
      RPC_FRAME_TYPE_BINARY,
      RPC_FRAME_TYPE_BINARY,
      RPC_FRAME_TYPE_EOS,
    ],
  );
  assert.deepEqual(
    frames.slice(1, 3).map((frame) => frame.payload.length),
    [32 * 1024, 7],
  );
});

test("prepared WebRTC peer finishes a continued server-initiated ping on its envelope EOS", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  channel.open();
  pc.receiveDataChannel(channel);
  const id = "p".repeat(70_000);

  channel.receive(
    encodeRPCRequest({
      id,
      method: "all.ping",
      params: { client_send_time: 123 },
      v: 1,
    }),
  );
  await channel.waitForSentCount(3);

  const response = parseRPCResponse<{ server_time: number }>(
    concatBuffers(channel.sent),
    "all.ping",
  );
  assert.equal(response.id, id);
  assert.ok((response.result?.server_time ?? 0) > 0);
});

test("prepared WebRTC peer delimits a continued speed-test response envelope", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const channel = new FakeDataChannel(
    giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
  );
  channel.open();
  pc.receiveDataChannel(channel);
  const id = "s".repeat(70_000);

  channel.receive(
    encodeRPCRequest({
      id,
      method: "all.speed_test.run",
      params: { down_content_length: 7, up_content_length: 5 },
      v: 1,
    }),
  );
  channel.receive(encodeFrame(RPC_FRAME_TYPE_BINARY, new Uint8Array(5)));
  channel.receive(encodeFrame(RPC_FRAME_TYPE_EOS));
  await channel.waitForSentCount(5);

  const frames = decodeFrames(concatBuffers(channel.sent));
  assert.deepEqual(
    frames.map((frame) => frame.type),
    [
      RPC_FRAME_TYPE_TEXT,
      RPC_FRAME_TYPE_TEXT,
      RPC_FRAME_TYPE_EOS,
      RPC_FRAME_TYPE_BINARY,
      RPC_FRAME_TYPE_EOS,
    ],
  );
  assert.equal(frames[3]?.payload.length, 7);
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
  const promise = sendGiznetWebRTCTelemetry(
    pc as unknown as RTCPeerConnection,
    {
      observedAtUnixMs: 1000,
      observations: [batteryTelemetry({ percent: 82 })],
    },
  );

  assert.equal(pc.channels[0]?.sent.length, 0);
  pc.channels[0]?.open();
  await promise;

  assert.equal(pc.channels[0]?.sent.length, 1);
});

test("sendGiznetWebRTCTelemetry waits beyond the RPC send retry budget", async () => {
  const pc = new FakePeerConnection();
  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);
  const promise = sendGiznetWebRTCTelemetry(
    pc as unknown as RTCPeerConnection,
    {
      observedAtUnixMs: 1000,
      observations: [batteryTelemetry({ percent: 82 })],
    },
    { timeoutMs: 1000 },
  );

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
    sendGiznetWebRTCTelemetry(
      pc as unknown as RTCPeerConnection,
      {
        observedAtUnixMs: 1000,
        observations: [batteryTelemetry({ percent: 82 })],
      },
      { timeoutMs: 1 },
    ),
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
  assert.deepEqual(
    Array.from(packet.slice(1)),
    [8, 7, 16, 232, 7, 26, 13, 82, 11, 9, 0, 0, 0, 0, 0, 128, 84, 64, 16, 1],
  );
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
    assert.deepEqual(Array.from(packet.slice(1, 4)), [16, 210, 9]);
  } finally {
    Date.now = originalNow;
  }
});

test("encodeTelemetryPacket rejects observations with multiple bodies", () => {
  assert.throws(
    () =>
      encodeTelemetryPacket({
        observations: [
          {
            battery: { percent: 82 },
            system: { temperatureC: 33 },
          } as any,
        ],
      }),
    /exactly one body/,
  );
});

test("encodeTelemetryPacket rejects empty frames before encoding", () => {
  assert.throws(() => encodeTelemetryPacket({}), /at least one observation/);
});

test("encodeTelemetryPacket rejects oversized packets", () => {
  assert.throws(
    () =>
      encodeTelemetryPacket({
        observations: [
          systemTelemetry({
            firmwareVersion: "x".repeat(GIZCLAW_MAX_PACKET_MESSAGE_SIZE),
          }),
        ],
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

test("waitForICEGatheringComplete resolves on the end-of-candidates marker", async () => {
  const pc = new FakeICEPeerConnection();
  const controller = new AbortController();
  const abortTimer = setTimeout(() => controller.abort(), 10);

  try {
    const gathering = waitForICEGatheringComplete(
      pc as unknown as RTCPeerConnection,
      controller.signal,
    );
    pc.emitEndOfCandidates();
    await gathering;
  } finally {
    clearTimeout(abortTimer);
  }

  assert.equal(pc.iceGatheringState, "gathering");
});

test("waitForICEGatheringComplete uses a gathered candidate after the bounded window", async () => {
  const pc = new FakeICEPeerConnection();
  pc.localDescription = {
    sdp: "v=0\r\na=candidate:1 1 UDP 1 192.0.2.1 5000 typ relay\r\n",
    type: "offer",
  } as RTCSessionDescription;
  const controller = new AbortController();
  const abortTimer = setTimeout(() => controller.abort(), 20);

  try {
    await waitForICEGatheringComplete(
      pc as unknown as RTCPeerConnection,
      controller.signal,
      1,
    );
  } finally {
    clearTimeout(abortTimer);
  }
});

test("waitForICEGatheringComplete rejects a host-only partial offer after the bounded window", async () => {
  const pc = new FakeICEPeerConnection();
  pc.localDescription = {
    sdp: "v=0\r\na=candidate:1 1 UDP 1 192.0.2.1 5000 typ host\r\n",
    type: "offer",
  } as RTCSessionDescription;

  await assert.rejects(
    waitForICEGatheringComplete(
      pc as unknown as RTCPeerConnection,
      undefined,
      1,
    ),
    /before producing a relay candidate/,
  );
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
        return new Response(answer, {
          headers: { "content-type": "application/octet-stream" },
          status: 200,
        });
      },
      url: `http://localhost${GIZNET_WEBRTC_SIGNALING_PATH}`,
    },
  );

  assert.deepEqual(
    new Uint8Array(await result.arrayBuffer()),
    new Uint8Array([4, 5]),
  );
  assert.equal(result.type, "application/octet-stream");
  assert.equal(
    captured?.url,
    `http://localhost${GIZNET_WEBRTC_SIGNALING_PATH}`,
  );
  assert.equal(captured?.method, "POST");
  assert.equal(
    captured?.headers.get("content-type"),
    "application/octet-stream",
  );
  assert.equal(captured?.headers.get("x-giznet-public-key"), "peer-pk");
  assert.equal(captured?.headers.get("x-giznet-timestamp"), "123");
  assert.equal(captured?.headers.get("x-giznet-nonce"), "nonce");
});

test("fetchGiznetServerInfo validates server metadata", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
  let captured: Request | undefined;

  const info = await fetchGiznetServerInfo({
    baseUrl: "http://localhost:9820",
    fetch: async (input, init) => {
      captured = new Request(input, init);
      return Response.json({
        build_commit: "deadbeef",
        ice_servers: [
          {
            credential: "pass",
            urls: [" turn:edge.example.com:3478?transport=udp "],
            username: "user",
          },
        ],
        protocol: "gizclaw-webrtc",
        public_key: serverPublicKey,
        signaling_path: "/custom/offer",
        version: "0.2.5",
      });
    },
  });

  assert.equal(captured?.url, "http://localhost:9820/server-info");
  assert.equal(info.public_key, serverPublicKey);
  assert.equal(info.version, "0.2.5");
  assert.equal(info.build_commit, "deadbeef");
  assert.equal(info.signaling_path, "/custom/offer");
  assert.deepEqual(info.ice_servers, [
    {
      credential: "pass",
      urls: ["turn:edge.example.com:3478?transport=udp"],
      username: "user",
    },
  ]);
});

test("fetchGiznetServerInfo preserves Server identity and selects Edge transport", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
  const edgePublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(3)),
  );
  const info = await fetchGiznetServerInfo({
    baseUrl: "http://server.example:9820",
    fetch: async () =>
      Response.json({
        ice_servers: [{ urls: ["turn:server.example:3478"] }],
        public_key: serverPublicKey,
        transport: {
          endpoint: "edge.example:9821",
          mode: "edge-gateway",
          public_key: edgePublicKey,
          signaling_path: "/edge/offer",
        },
      }),
  });
  assert.equal(info.public_key, serverPublicKey);
  assert.deepEqual(info.transport, {
    endpoint: "edge.example:9821",
    mode: "edge-gateway",
    public_key: edgePublicKey,
    signaling_path: "/edge/offer",
  });
  assert.equal(info.ice_servers, undefined);
});

test("fetchGiznetServerInfo rejects invalid Edge transport metadata", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
  const edgePublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(3)),
  );
  for (const transport of [
    "bad",
    {
      endpoint: "edge.example:9821",
      mode: "future",
      public_key: edgePublicKey,
      signaling_path: "/offer",
    },
    {
      endpoint: "ftp://edge.example",
      mode: "edge-gateway",
      public_key: edgePublicKey,
      signaling_path: "/offer",
    },
    {
      endpoint: "edge.example:9821",
      mode: "edge-gateway",
      public_key: "bad",
      signaling_path: "/offer",
    },
    {
      endpoint: "edge.example:9821",
      mode: "edge-gateway",
      public_key: serverPublicKey,
      signaling_path: "/offer",
    },
    {
      endpoint: "edge.example:9821",
      mode: "edge-gateway",
      public_key: edgePublicKey,
      signaling_path: "offer",
    },
  ]) {
    await assert.rejects(
      fetchGiznetServerInfo({
        fetch: async () =>
          Response.json({ public_key: serverPublicKey, transport }),
      }),
      /transport|signaling_path|base58/,
    );
  }
});

test("normalizeServerBaseURL keeps https and defaults a bare host:port to http", () => {
  assert.equal(
    normalizeServerBaseURL("127.0.0.1:9820"),
    "http://127.0.0.1:9820",
  );
  assert.equal(
    normalizeServerBaseURL("https://ap.gizclaw.com"),
    "https://ap.gizclaw.com",
  );
  assert.equal(
    normalizeServerBaseURL("https://ap.gizclaw.com/"),
    "https://ap.gizclaw.com",
  );
  assert.equal(
    normalizeServerBaseURL("https://ap.gizclaw.com/prefix/"),
    "https://ap.gizclaw.com/prefix",
  );
  for (const value of [
    "",
    "ftp://ap.gizclaw.com",
    "http:",
    "https:",
    "mailto:device@ap.gizclaw.com",
    "https://user@ap.gizclaw.com",
    "https://ap.gizclaw.com?probe=1",
    "https://ap.gizclaw.com#fragment",
    "ap.gizclaw.com:",
    "ap.gizclaw.com:port",
    "ap.gizclaw.com:998877",
    ":9820",
    "[not-an-ip]",
    "[:::]:9820",
    "[1:2:3:4:5:6:7]",
    "[1:2:3:4:5:6:7:8:9]",
    "[12345::1]",
    "[::1%eth0]",
    "[::ffff:999.1.1.1]",
    "[]:9820",
    "[::1:9820",
  ]) {
    assert.throws(() => normalizeServerBaseURL(value), /server URL/u);
  }
});

test("fetchGiznetServerInfo rejects an ICE endpoint that is not host:port", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
  for (const endpoint of [
    "https://ice.example",
    "ice.example/path",
    "ice.example?probe=1",
    "ice.example#fragment",
    "user@ice.example",
    "ice.example:",
    "ice.example:port",
    "ice.example:998877",
    ":9820",
    "[not-an-ip]",
    "[:::]:9820",
    "[1:2:3:4:5:6:7]",
    "[1:2:3:4:5:6:7:8:9]",
    "[12345::1]",
    "[::1%eth0]",
    "[::ffff:999.1.1.1]",
    "[]:9820",
    "[::1:9820",
    42,
  ]) {
    await assert.rejects(
      fetchGiznetServerInfo({
        fetch: async () =>
          Response.json({ endpoint, public_key: serverPublicKey }),
      }),
      /endpoint/u,
    );
  }
  for (const endpoint of [
    "ice.example",
    "ice.example:9820",
    "[::]",
    "[::1]:9820",
    "[1:2:3:4:5:6:7:8]",
    "[::ffff:192.168.1.10]:9820",
  ]) {
    const info = await fetchGiznetServerInfo({
      fetch: async () =>
        Response.json({ endpoint, public_key: serverPublicKey }),
    });
    assert.equal(info.endpoint, endpoint);
    assert.ok(isServerInfoHostPort(endpoint));
  }
});

test("fetchGiznetServerInfo reaches an https access point and preserves a path prefix", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
  const requested: string[] = [];
  const fetchImpl = async (input: Request | string | URL) => {
    requested.push(String(input));
    return Response.json({
      endpoint: "ice.example:9820",
      public_key: serverPublicKey,
    });
  };
  const info = await fetchGiznetServerInfo({
    endpoint: "https://ap.gizclaw.com/prefix/",
    fetch: fetchImpl as unknown as typeof fetch,
  });
  assert.equal(info.endpoint, "ice.example:9820");
  assert.deepEqual(requested, ["https://ap.gizclaw.com/prefix/server-info"]);

  await fetchGiznetServerInfo({
    endpoint: "127.0.0.1:9820",
    fetch: fetchImpl as unknown as typeof fetch,
  });
  assert.equal(requested[1], "http://127.0.0.1:9820/server-info");
});

test("fetchGiznetServerInfo rejects invalid ICE server metadata", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
  for (const iceServers of [
    {},
    [{}],
    [{ urls: [""] }],
    [{ urls: ["https://edge.example.com"] }],
    [{ urls: ["turn:edge.example.com", 42] }],
  ]) {
    await assert.rejects(
      fetchGiznetServerInfo({
        baseUrl: "http://localhost:9820",
        fetch: async () =>
          Response.json({
            ice_servers: iceServers,
            public_key: serverPublicKey,
          }),
      }),
      /invalid ice_servers/,
    );
  }
});

test("applyGiznetServerInfoICEServers updates peer connection configuration", () => {
  const pc = new FakeConfigurablePeerConnection();
  const iceServers = [
    {
      credential: "pass",
      urls: ["turn:edge.example.com:3478?transport=udp"],
      username: "user",
    },
  ];

  applyGiznetServerInfoICEServers(pc as unknown as RTCPeerConnection, {
    ice_servers: iceServers,
  });

  assert.deepEqual(pc.configuration, {
    bundlePolicy: "balanced",
    iceServers,
  });
});

test("fetchGiznetServerInfo defaults signaling path and reports HTTP failures", async () => {
  const serverPublicKey = base58Encode(
    x25519.getPublicKey(new Uint8Array(32).fill(2)),
  );
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
      fetch: async () =>
        new Response("not ready", {
          status: 503,
          statusText: "Service Unavailable",
        }),
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

  assert.deepEqual(
    base58Decode(base58Encode(clientPublicKey)),
    clientPublicKey,
  );
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
  closeCalls = 0;
  connectionState: RTCPeerConnectionState = "new";
  iceGatheringState: RTCIceGatheringState = "complete";
  localDescription: RTCSessionDescription | null = null;
  transceivers: Array<{ init?: RTCRtpTransceiverInit; kind: string }> = [];
  readonly listeners = new Map<string, Set<(event: unknown) => void>>();

  createDataChannel(
    label: string,
    options?: RTCDataChannelInit,
  ): FakeDataChannel {
    const channel = new FakeDataChannel(label, options);
    this.channels.push(channel);
    return channel;
  }

  close(): void {
    this.closeCalls += 1;
    this.connectionState = "closed";
  }

  async createOffer(): Promise<RTCSessionDescriptionInit> {
    return { sdp: "v=0", type: "offer" };
  }

  async setLocalDescription(
    description: RTCLocalSessionDescriptionInit,
  ): Promise<void> {
    this.localDescription = description as RTCSessionDescription;
  }

  async setRemoteDescription(
    _description: RTCSessionDescriptionInit,
  ): Promise<void> {}

  addTransceiver(kind: string, init?: RTCRtpTransceiverInit): void {
    this.transceivers.push({ kind, init });
  }

  addEventListener(type: string, listener: (event: unknown) => void): void {
    let listeners = this.listeners.get(type);
    if (listeners == null) {
      listeners = new Set();
      this.listeners.set(type, listeners);
    }
    listeners.add(listener);
  }

  receiveDataChannel(channel: FakeDataChannel): void {
    for (const listener of this.listeners.get("datachannel") ?? []) {
      listener({ channel });
    }
  }

  failConnection(): void {
    this.connectionState = "failed";
    for (const listener of this.listeners.get("connectionstatechange") ?? []) {
      listener({});
    }
  }

  lastChannel(): FakeDataChannel {
    const channel = this.channels.at(-1);
    if (channel == null) {
      throw new Error("no channel created");
    }
    return channel;
  }

  channel(label: string): FakeDataChannel {
    const channel = this.channels.find((item) => item.label === label);
    if (channel == null) {
      throw new Error(`channel ${label} was not created`);
    }
    return channel;
  }

  async waitForChannel(label: string): Promise<void> {
    for (let i = 0; i < 50; i += 1) {
      if (this.channels.some((channel) => channel.label === label)) {
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    throw new Error(`channel ${label} was not created`);
  }
}

class FakeICEPeerConnection {
  completeAfterFirstListener = false;
  iceGatheringState: RTCIceGatheringState = "gathering";
  localDescription: RTCSessionDescription | null = null;
  readonly listeners = new Map<string, Set<(event?: unknown) => void>>();

  addEventListener(type: string, listener: (event?: unknown) => void): void {
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

  removeEventListener(type: string, listener: (event?: unknown) => void): void {
    this.listeners.get(type)?.delete(listener);
  }

  emitEndOfCandidates(): void {
    for (const listener of this.listeners.get("icecandidate") ?? []) {
      listener({ candidate: null });
    }
  }
}

class FakeConfigurablePeerConnection {
  configuration: RTCConfiguration = { bundlePolicy: "balanced" };

  getConfiguration(): RTCConfiguration {
    return this.configuration;
  }

  setConfiguration(configuration: RTCConfiguration): void {
    this.configuration = configuration;
  }
}

class FakeDataChannel {
  binaryType?: BinaryType;
  bufferedAmountLowThreshold?: number;
  bufferedAmountReads = 0;
  closed = false;
  drainBufferedAmountOnRead?: number;
  failSendCall?: number;
  readonly label: string;
  readonly options?: RTCDataChannelInit;
  readyState: RTCDataChannelState = "connecting";
  sent: ArrayBuffer[] = [];
  readonly listeners = new Map<string, Set<(event?: unknown) => void>>();
  private bufferedAmountValue = 0;
  private sendCalls = 0;

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

  get bufferedAmount(): number {
    this.bufferedAmountReads += 1;
    if (this.bufferedAmountReads === this.drainBufferedAmountOnRead) {
      this.bufferedAmountValue = 256 * 1024;
    }
    return this.bufferedAmountValue;
  }

  set bufferedAmount(value: number) {
    this.bufferedAmountValue = value;
  }

  send(data: ArrayBuffer | ArrayBufferView | Blob | string): void {
    this.sendCalls += 1;
    if (this.sendCalls === this.failSendCall) {
      throw new Error("injected send failure");
    }
    if (data instanceof ArrayBuffer) {
      this.sent.push(data);
      return;
    }
    if (ArrayBuffer.isView(data)) {
      this.sent.push(
        data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength),
      );
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

  fail(): void {
    this.emit("error");
  }

  open(): void {
    this.readyState = "open";
    this.emit("open");
  }

  signalOpenWithoutReady(): void {
    this.emit("open");
  }

  bufferedAmountLow(): void {
    this.emit("bufferedamountlow");
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

  async waitForSentCount(count: number): Promise<void> {
    for (let i = 0; i < 50; i += 1) {
      if (this.sent.length >= count) {
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
    throw new Error(
      `fake data channel sent ${this.sent.length} messages, want ${count}`,
    );
  }

  private emit(type: string, event?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

type FakeChannel = {
  binaryType: BinaryType;
  bufferedAmount: number;
  close(): void;
  label: string;
  readyState: RTCDataChannelState;
  addEventListener(type: string, listener: (event: unknown) => void): void;
  removeEventListener(type: string, listener: (event: unknown) => void): void;
  send(data: ArrayBuffer): void;
};

// serveInboundClientRPC drives one inbound client.* request through the SDK's
// built-in RPC server and returns the decoded response.
async function serveInboundClientRPC(
  method: string,
  params: unknown,
  handlers?: GizClawPeerRPCHandlers,
): Promise<RPCResponse> {
  const sent: ArrayBuffer[] = [];
  const listeners = new Map<string, Set<(event: unknown) => void>>();
  const channel: FakeChannel = {
    binaryType: "arraybuffer",
    bufferedAmount: 0,
    close(): void {
      channel.readyState = "closed";
    },
    label: giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC),
    readyState: "open",
    addEventListener(type, listener): void {
      const set = listeners.get(type) ?? new Set();
      set.add(listener);
      listeners.set(type, set);
    },
    removeEventListener(type, listener): void {
      listeners.get(type)?.delete(listener);
    },
    send(data): void {
      sent.push(data);
    },
  };
  let openChannel: ((event: { channel: unknown }) => void) | undefined;
  const pc = {
    addEventListener(_type: "datachannel", listener: typeof openChannel): void {
      openChannel = listener;
    },
  };
  serveGiznetWebRTCRPC(
    pc as unknown as Parameters<typeof serveGiznetWebRTCRPC>[0],
    handlers,
  );
  assert.ok(openChannel != null);
  openChannel({ channel });
  for (const listener of listeners.get("message") ?? []) {
    listener({
      data: encodeRPCRequest({ id: "inbound-1", method, params, v: 1 }),
    });
  }
  for (let tick = 0; tick < 50; tick++) {
    await new Promise((resolve) => setImmediate(resolve));
  }
  assert.ok(sent.length > 0, `no response for ${method}`);
  const total = sent.reduce((sum, frame) => sum + frame.byteLength, 0);
  const merged = new Uint8Array(total);
  let offset = 0;
  for (const frame of sent) {
    merged.set(new Uint8Array(frame), offset);
    offset += frame.byteLength;
  }
  return parseRPCResponse(merged, method);
}

test("inbound client.device.volume.set answers from the handler", async () => {
  let seen: { level: number; muted: boolean } | undefined;
  const response = await serveInboundClientRPC(
    "client.device.volume.set",
    { level: 35, muted: true },
    {
      deviceControl: {
        setVolume: (level, muted) => {
          seen = { level, muted };
          return { battery_percent: 88, muted, volume: level };
        },
      },
    },
  );
  assert.deepEqual(seen, { level: 35, muted: true });
  assert.equal(response.error, undefined);
  assert.deepEqual(response.result, {
    battery_percent: 88,
    labels: {},
    muted: true,
    volume: 35,
  });
});

test("inbound client.wifi.status.get and saved.list answer from handlers", async () => {
  const status = await serveInboundClientRPC(
    "client.wifi.status.get",
    {},
    {
      deviceControl: { wifiStatus: () => ({ connected: true, ssid: "home" }) },
    },
  );
  assert.deepEqual(status.result, { connected: true, ssid: "home" });

  const saved = await serveInboundClientRPC(
    "client.wifi.saved.list",
    {},
    {
      deviceControl: {
        savedWifi: () => [{ ssid: "home" }, { ssid: "office" }],
      },
    },
  );
  assert.deepEqual(saved.result, {
    networks: [{ ssid: "home" }, { ssid: "office" }],
  });
});

test("inbound client.wifi.scan and connect answer from handlers", async () => {
  let connected: { passphrase?: string; ssid: string } | undefined;
  const scan = await serveInboundClientRPC(
    "client.wifi.scan",
    { timeout_ms: 8000 },
    {
      deviceControl: {
        scanWifi: (timeoutMs) => {
          assert.equal(timeoutMs, 8000);
          return [{ rssi_dbm: -42, security: "wpa3", ssid: "office" }];
        },
      },
    },
  );
  assert.deepEqual(scan.result, {
    networks: [{ rssi_dbm: -42, security: "wpa3", ssid: "office" }],
  });

  const connect = await serveInboundClientRPC(
    "client.wifi.connect",
    { passphrase: "correct-horse", ssid: "office" },
    {
      deviceControl: {
        connectWifi: (ssid, passphrase) => {
          connected = { passphrase, ssid };
        },
      },
    },
  );
  assert.equal(connect.error, undefined);
  assert.deepEqual(connect.result, {});
  assert.deepEqual(connected, {
    passphrase: "correct-horse",
    ssid: "office",
  });
});

test("inbound client.* without a handler answers METHOD_NOT_FOUND", async () => {
  const response = await serveInboundClientRPC(
    "client.device.volume.set",
    { level: 1, muted: false },
    { deviceControl: {} },
  );
  assert.equal(response.error?.code, STATUS_CODE_UNIMPLEMENTED);
  assert.match(response.error?.message ?? "", /unsupported method/u);
});

test("inbound device control rejects out-of-range and oversized params", async () => {
  const volume = await serveInboundClientRPC(
    "client.device.volume.set",
    { level: 101, muted: false },
    { deviceControl: { setVolume: () => ({ volume: 1 }) } },
  );
  assert.equal(volume.error?.code, STATUS_CODE_INVALID_ARGUMENT);

  const sound = await serveInboundClientRPC(
    "client.device.sound.play",
    { sound: "s".repeat(33) },
    { deviceControl: { playSound: () => {} } },
  );
  assert.equal(sound.error?.code, STATUS_CODE_INVALID_ARGUMENT);

  const forget = await serveInboundClientRPC(
    "client.wifi.saved.forget",
    { ssid: "" },
    { deviceControl: { forgetWifi: () => {} } },
  );
  assert.equal(forget.error?.code, STATUS_CODE_INVALID_ARGUMENT);

  const scan = await serveInboundClientRPC(
    "client.wifi.scan",
    { timeout_ms: 999 },
    { deviceControl: { scanWifi: () => [] } },
  );
  assert.equal(scan.error?.code, STATUS_CODE_INVALID_ARGUMENT);

  const connect = await serveInboundClientRPC(
    "client.wifi.connect",
    { passphrase: "short", ssid: "home" },
    { deviceControl: { connectWifi: () => {} } },
  );
  assert.equal(connect.error?.code, STATUS_CODE_INVALID_ARGUMENT);
});

// A proto3 bool always decodes to a value, so `muted` cannot arrive malformed,
// and the codec renders an explicit null optional field as absent. The
// remaining reachable malformation is an out-of-range duration.
test("inbound device control rejects out-of-range durations", async () => {
  const badDuration = await serveInboundClientRPC(
    "client.device.sound.play",
    { duration_ms: -1, sound: "chime" },
    { deviceControl: { playSound: () => {} } },
  );
  assert.equal(badDuration.error?.code, STATUS_CODE_INVALID_ARGUMENT);

  const badDelay = await serveInboundClientRPC(
    "client.device.reboot",
    { delay_ms: -5 },
    { deviceControl: { reboot: () => {} } },
  );
  assert.equal(badDelay.error?.code, STATUS_CODE_INVALID_ARGUMENT);
});

test("inbound device control surfaces a scripted error code", async () => {
  const response = await serveInboundClientRPC(
    "client.device.sound.play",
    { sound: "nope" },
    {
      deviceControl: {
        playSound: () => {
          throw new GizClawDeviceControlError(
            STATUS_CODE_INVALID_ARGUMENT,
            "unknown sound",
          );
        },
      },
    },
  );
  assert.equal(response.error?.code, STATUS_CODE_INVALID_ARGUMENT);
  assert.equal(response.error?.message, "unknown sound");
});

test("inbound client.device.reboot and sound.play answer empty results", async () => {
  const calls: string[] = [];
  const reboot = await serveInboundClientRPC(
    "client.device.reboot",
    { delay_ms: 3000 },
    {
      deviceControl: {
        reboot: (delayMs) => {
          calls.push(`reboot:${delayMs}`);
        },
      },
    },
  );
  assert.equal(reboot.error, undefined);
  const play = await serveInboundClientRPC(
    "client.device.sound.play",
    { duration_ms: 500, sound: "chime" },
    {
      deviceControl: {
        playSound: (sound, durationMs) => {
          calls.push(`play:${sound}:${durationMs}`);
        },
      },
    },
  );
  assert.equal(play.error, undefined);
  assert.deepEqual(calls, ["reboot:3000", "play:chime:500"]);
});
