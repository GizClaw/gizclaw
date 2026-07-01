import assert from "node:assert/strict";
import test from "node:test";

import {
  GIZCLAW_SERVICE_RPC,
  GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL,
  GIZNET_WEBRTC_SIGNALING_PATH,
  RPC_FRAME_TYPE_EOS,
  RPC_FRAME_TYPE_JSON,
  WebRTCRPCClient,
  WebRTCRPCError,
  WorkspaceRPC,
  createWebRTCFetch,
  decodeFrames,
  encodeRPCResponse,
  giznetServiceDataChannelLabel,
  prepareGiznetWebRTCPeerConnection,
  sendGiznetWebRTCOffer,
} from "./index.ts";

test("WebRTCRPCClient sends JSON-RPC over an rpc data channel", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-1" });

  const promise = client.call<{ ok: boolean }>("server.run.workspace.get", {});
  const channel = pc.lastChannel();
  channel.open();

  assert.equal(channel.label, giznetServiceDataChannelLabel(GIZCLAW_SERVICE_RPC));
  const frames = decodeFrames(channel.sent[0] ?? new ArrayBuffer(0));
  assert.equal(frames.length, 2);
  assert.equal(frames[0]?.type, RPC_FRAME_TYPE_JSON);
  assert.equal(frames[1]?.type, RPC_FRAME_TYPE_EOS);
  assert.deepEqual(JSON.parse(new TextDecoder().decode(frames[0]?.payload)), {
    id: "req-1",
    method: "server.run.workspace.get",
    params: {},
    v: 1,
  });

  channel.receive(encodeRPCResponse({ id: "req-1", result: { ok: true }, v: 1 }));

  assert.deepEqual(await promise, { ok: true });
  assert.equal(channel.closed, true);
});

test("WebRTCRPCClient reassembles response frames split across messages", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-split" });

  const promise = client.call<{ ok: boolean }>("server.run.workspace.get", {});
  const channel = pc.lastChannel();
  channel.open();

  const response = new Uint8Array(encodeRPCResponse({ id: "req-split", result: { ok: true }, v: 1 }));
  channel.receiveBytes(response.slice(0, 5));
  channel.receiveBytes(response.slice(5));

  assert.deepEqual(await promise, { ok: true });
});

test("WebRTCRPCClient rejects RPC error responses", async () => {
  const pc = new FakePeerConnection();
  const client = new WebRTCRPCClient(pc, { createID: () => "req-2" });

  const promise = client.call("server.run.workspace.reload");
  const channel = pc.lastChannel();
  channel.open();
  channel.receive(encodeRPCResponse({ error: { code: -32000, message: "boom" }, id: "req-2", v: 1 }));

  await assert.rejects(promise, (err) => {
    assert.equal(err instanceof WebRTCRPCError, true);
    assert.equal((err as WebRTCRPCError).code, -32000);
    assert.equal((err as WebRTCRPCError).message, "boom");
    return true;
  });
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

test("WorkspaceRPC exposes workspace-related RPC methods", async () => {
  const calls: Array<{ method: string; params: unknown }> = [];
  const client = {
    call: async (method: string, params: unknown) => {
      calls.push({ method, params });
      return { accepted: true };
    },
  } as unknown as WebRTCRPCClient;
  const workspace = new WorkspaceRPC(client);

  await workspace.setRunWorkspace({ workspace_name: "main" });
  await workspace.playRunWorkspaceHistory({ history_id: "h1" });
  await workspace.recallRunWorkspaceMemory({ query: "hello" });
  await workspace.listWorkspaceHistory({ cursor: "c1", workspace_name: "main" });

  assert.deepEqual(calls, [
    { method: "server.run.workspace.set", params: { workspace_name: "main" } },
    { method: "server.run.workspace.history.play", params: { history_id: "h1" } },
    { method: "server.run.workspace.recall", params: { query: "hello" } },
    { method: "server.workspace.history.list", params: { cursor: "c1", workspace_name: "main" } },
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

test("prepareGiznetWebRTCPeerConnection creates packet channel and audio transceiver", () => {
  const pc = new FakePeerConnection();

  prepareGiznetWebRTCPeerConnection(pc as unknown as RTCPeerConnection);

  assert.equal(pc.channels[0]?.label, GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL);
  assert.deepEqual(pc.channels[0]?.options, { maxRetransmits: 0, ordered: false });
  assert.deepEqual(pc.transceivers, [{ kind: "audio", init: { direction: "sendrecv" } }]);
});

test("sendGiznetWebRTCOffer posts the server public signaling request", async () => {
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

  open(): void {
    this.readyState = "open";
    this.emit("open");
  }

  receive(data: ArrayBuffer): void {
    this.emit("message", { data });
  }

  receiveBytes(data: Uint8Array): void {
    this.emit("message", { data });
  }

  private emit(type: string, event?: unknown): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}
