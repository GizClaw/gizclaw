import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { create } from "@bufbuild/protobuf";
import {
  beginPeerAudioInput,
  createContinuousAudioRouteRearm,
  decodePeerEvent,
  encodePeerEvent,
  encodePeerEventFrame,
  endPeerStream,
  FriendGroupChange,
  FriendRelationshipChange,
  PeerEventFrameDecoder,
  PeerEventSchema,
  PeerEventType,
  StreamKind,
  validatePeerEvent,
  WorkspaceKind,
  type PeerEvent,
  sendPeerEvent,
  subscribePeerEvents,
} from "./events.ts";
import {
  encodeFrame,
  RPC_FRAME_TYPE_BINARY,
  RPC_FRAME_TYPE_JSON,
  type WebRTCRPCDataChannel,
} from "./index.ts";

const events: PeerEvent[] = [
  peerEvent(PeerEventType.BOS, "bos", {
    streamId: "stream-a",
    sequence: 1n,
    timestampUnixMs: 2n,
    kind: StreamKind.AUDIO,
    label: "user",
    mimeType: "audio/opus",
  }),
  peerEvent(PeerEventType.EOS, "eos", {
    streamId: "stream-a",
    sequence: 2n,
    timestampUnixMs: 3n,
    kind: StreamKind.AUDIO,
    label: "assistant",
    error: {
      code: "MEMBER_REMOVED",
      message: "removed",
      retryable: false,
    },
  }),
  peerEvent(PeerEventType.TEXT_DELTA, "textDelta", {
    streamId: "stream-b",
    sequence: 1n,
    timestampUnixMs: 2n,
    label: "assistant",
    text: "hel",
  }),
  peerEvent(PeerEventType.TEXT_DONE, "textDone", {
    streamId: "stream-b",
    sequence: 2n,
    timestampUnixMs: 3n,
    label: "assistant",
    text: "hello",
  }),
  peerEvent(
    PeerEventType.WORKSPACE_HISTORY_UPDATED,
    "workspaceHistoryUpdated",
    {
      workspaceName: "direct-a-b",
      workspaceKind: WorkspaceKind.WORKFLOW,
      lastUpdatedAtUnixMs: 4n,
    },
  ),
  peerEvent(
    PeerEventType.FRIEND_RELATIONSHIP_UPDATED,
    "friendRelationshipUpdated",
    {
      peerPublicKey: "peer-b",
      workspaceName: "direct-a-b",
      change: FriendRelationshipChange.DELETED,
      revisionUnixMs: 5n,
    },
  ),
  peerEvent(PeerEventType.FRIEND_GROUP_UPDATED, "friendGroupUpdated", {
    friendGroupName: "group-a",
    workspaceName: "group-a",
    change: FriendGroupChange.MEMBER_REMOVED,
    revisionUnixMs: 6n,
  }),
  peerEvent(PeerEventType.GAMEPLAY_REWARD_UPDATED, "gameplayRewardUpdated", {
    workspaceName: "workflow-a",
    rewardGrantName: "grant-a",
    revisionUnixMs: 7n,
  }),
];

test("round-trips every Peer Event oneof arm", () => {
  for (const event of events) {
    const decoded = decodePeerEvent(encodePeerEvent(event));
    assert.equal(decoded.type, event.type);
    assert.equal(decoded.payload.case, event.payload.case);
  }
});

test("decodes split and coalesced binary frames", () => {
  const bytes = events
    .slice(0, 2)
    .map((event) => new Uint8Array(encodePeerEventFrame(event)));
  const joined = new Uint8Array(bytes[0].length + bytes[1].length);
  joined.set(bytes[0]);
  joined.set(bytes[1], bytes[0].length);
  const decoder = new PeerEventFrameDecoder();
  assert.deepEqual(decoder.push(joined.slice(0, 3)), []);
  const decoded = decoder.push(joined.slice(3));
  assert.deepEqual(
    decoded.map((event) => event.type),
    ["bos", "eos"],
  );
  assert.equal(decoded[1]?.errorCode, "MEMBER_REMOVED");
  decoder.finish();
});

test("rejects mismatched payloads and JSON frames", () => {
  const mismatched = peerEvent(PeerEventType.EOS, "bos", {
    streamId: "stream-a",
    kind: StreamKind.AUDIO,
  });
  assert.throws(() => validatePeerEvent(mismatched), /requires eos payload/);
  assert.throws(
    () => new PeerEventFrameDecoder().push(encodeFrame(RPC_FRAME_TYPE_JSON)),
    /expected Peer Event binary frame/,
  );
});

test("matches every cross-language Peer Event golden vector", () => {
  const vectors = JSON.parse(
    readFileSync(
      new URL(
        "../../../api/proto/events/testdata/peer_event_vectors.json",
        import.meta.url,
      ),
      "utf8",
    ),
  ) as { hex: string; name: string }[];
  assert.equal(vectors.length, 9);
  for (const vector of vectors) {
    const bytes = Uint8Array.from(Buffer.from(vector.hex, "hex"));
    const event = decodePeerEvent(bytes);
    assert.equal(
      Buffer.from(encodePeerEvent(event)).toString("hex"),
      vector.hex,
    );
  }
});

test("keeps a future event type consumable", () => {
  // A future producer sends type=99 with a oneof arm unknown to this SDK.
  const bytes = Uint8Array.from([0x08, 0x01, 0x10, 0x63, 0x9a, 0x01, 0x00]);
  const decoded = decodePeerEvent(bytes);
  assert.equal(decoded.type, 99);
  assert.equal(
    new PeerEventFrameDecoder().push(
      encodeFrame(RPC_FRAME_TYPE_BINARY, bytes),
    )[0]?.type,
    "unknown",
  );
});

test("rejects a future type that reuses a known payload arm", () => {
  assert.throws(
    () =>
      decodePeerEvent(
        // version=1, type=99, bos={}
        Uint8Array.from([0x08, 0x01, 0x10, 0x63, 0x52, 0x00]),
      ),
    /must not use a known payload/,
  );
});

test("rejects domain events with missing resource identifiers", () => {
  const event = peerEvent(
    PeerEventType.WORKSPACE_HISTORY_UPDATED,
    "workspaceHistoryUpdated",
    { workspaceName: " " },
  );
  assert.throws(() => encodePeerEvent(event), /requires workspaceName/);
});

test("rejects stream events with missing stream identifiers", () => {
  const event = peerEvent(PeerEventType.BOS, "bos", {
    streamId: " ",
    kind: StreamKind.AUDIO,
  });
  assert.throws(() => encodePeerEvent(event), /requires streamId/);
});

test("drops an invalid timestamp frame before reporting the error", () => {
  const invalid = peerEvent(
    PeerEventType.WORKSPACE_HISTORY_UPDATED,
    "workspaceHistoryUpdated",
    {
      workspaceName: "direct-a-b",
      workspaceKind: WorkspaceKind.WORKFLOW,
      lastUpdatedAtUnixMs: 9223372036854775807n,
    },
  );
  const validPrefix = new Uint8Array(encodePeerEventFrame(events[0]));
  const invalidFrame = new Uint8Array(encodePeerEventFrame(invalid));
  const validSuffix = new Uint8Array(encodePeerEventFrame(events[1]));
  const joined = new Uint8Array(
    validPrefix.length + invalidFrame.length + validSuffix.length,
  );
  joined.set(validPrefix);
  joined.set(invalidFrame, validPrefix.length);
  joined.set(validSuffix, validPrefix.length + invalidFrame.length);
  const decoder = new PeerEventFrameDecoder();

  assert.throws(() => decoder.push(joined), /timestamp is out of range/);
  assert.deepEqual(
    decoder.push(new Uint8Array()).map((event) => event.type),
    ["bos", "eos"],
  );
  decoder.finish();
});

test("drops invalid frame types before reporting the error", () => {
  const validPrefix = new Uint8Array(encodePeerEventFrame(events[0]));
  const validSuffix = new Uint8Array(encodePeerEventFrame(events[1]));
  const invalidFrames = [
    {
      bytes: new Uint8Array(encodeFrame(RPC_FRAME_TYPE_JSON)),
      error: /expected Peer Event binary frame/,
    },
    {
      // length=1, type=EOS, payload=0xff. encodeFrame intentionally rejects
      // this malformed frame, so construct the wire bytes directly.
      bytes: Uint8Array.from([1, 0, 0, 0, 0xff]),
      error: /Peer Event EOS frame must be empty/,
    },
  ];

  for (const invalid of invalidFrames) {
    const joined = new Uint8Array(
      validPrefix.length + invalid.bytes.length + validSuffix.length,
    );
    joined.set(validPrefix);
    joined.set(invalid.bytes, validPrefix.length);
    joined.set(validSuffix, validPrefix.length + invalid.bytes.length);
    const decoder = new PeerEventFrameDecoder();

    assert.throws(() => decoder.push(joined), invalid.error);
    assert.deepEqual(
      decoder.push(new Uint8Array()).map((event) => event.type),
      ["bos", "eos"],
    );
    decoder.finish();
  }
});

test("shares the connection-owned Peer Event channel across subscribers", async () => {
  const channel = new FakePeerEventChannel();
  const first: string[] = [];
  const second: string[] = [];
  const shared = channel as unknown as WebRTCRPCDataChannel;
  const unsubscribeFirst = subscribePeerEvents(shared, (event) => {
    first.push(event.type);
  });
  subscribePeerEvents(shared, (event) => {
    second.push(event.type);
  });

  channel.receive(encodePeerEventFrame(events[0]));
  await new Promise<void>((resolve) => setImmediate(resolve));
  unsubscribeFirst();
  channel.receive(encodePeerEventFrame(events[1]));
  await new Promise<void>((resolve) => setImmediate(resolve));

  assert.deepEqual(first, ["bos"]);
  assert.deepEqual(second, ["bos", "eos"]);
  channel.readyState = "open";
  sendPeerEvent(shared, events[2]);
  assert.equal(channel.sent.length, 1);
});

test("continuous audio route owner subscribes and re-arms without client event wiring", async () => {
  const channel = new FakePeerEventChannel();
  channel.readyState = "open";
  const rearmed: string[] = [];
  const owner = createContinuousAudioRouteRearm(
    channel as unknown as WebRTCRPCDataChannel,
    () => "audio-new",
    { onRearmed: (streamId) => rearmed.push(streamId) },
  );
  owner.activate({
    streamId: "audio-old",
    label: "user",
    mimeType: "audio/opus; rate=16000",
  });

  channel.receive(reloadEOSFrame());
  await flushPeerEvents();

  assert.deepEqual(rearmed, []);
  channel.receive(inputReadyFrame("audio-new"));
  await flushPeerEvents();
  assert.deepEqual(rearmed, ["audio-new"]);
  assert.equal(channel.sent.length, 1);
  const [bos] = new PeerEventFrameDecoder().push(channel.sent[0]!);
  assert.deepEqual(bos, {
    type: "bos",
    streamId: "audio-new",
    kind: "audio",
    label: "user",
    mimeType: "audio/opus",
  });
  owner.close();
  assert.throws(
    () => owner.activate({ streamId: "audio-later" }),
    /owner is closed/,
  );
});

test("continuous audio re-arm ignores inactive, stale, output, ordinary, and closed events", async () => {
  const variants = [
    { streamId: "stale" },
    { label: "assistant" },
    { kind: StreamKind.VIDEO },
    { mimeType: "audio/aac" },
    { errorCode: "STREAM_ERROR" },
    { errorMessage: "ordinary completion" },
    { errorRetryable: false },
  ];
  for (const event of variants) {
    const channel = new FakePeerEventChannel();
    channel.readyState = "open";
    let allocations = 0;
    const owner = createContinuousAudioRouteRearm(
      channel as unknown as WebRTCRPCDataChannel,
      () => {
        allocations++;
        return "audio-new";
      },
    );
    owner.activate({ streamId: "audio-old" });
    channel.receive(reloadEOSFrame(event));
    await flushPeerEvents();
    assert.equal(allocations, 0);
    assert.equal(channel.sent.length, 0);
    owner.close();
  }

  const channel = new FakePeerEventChannel();
  let allocations = 0;
  const owner = createContinuousAudioRouteRearm(
    channel as unknown as WebRTCRPCDataChannel,
    () => {
      allocations++;
      return "audio-new";
    },
  );
  channel.readyState = "open";
  channel.receive(reloadEOSFrame());
  await flushPeerEvents();
  owner.activate({ streamId: "audio-old" });
  channel.readyState = "closed";
  channel.receive(reloadEOSFrame());
  await flushPeerEvents();
  channel.readyState = "open";
  owner.deactivate();
  channel.receive(reloadEOSFrame());
  await flushPeerEvents();
  assert.equal(allocations, 0);
  assert.equal(channel.sent.length, 0);
  owner.close();
});

test("continuous audio re-arm reports failures and retains the old route", async () => {
  for (const invalid of ["", "audio-old"]) {
    const channel = new FakePeerEventChannel();
    channel.readyState = "open";
    const errors: string[] = [];
    const owner = createContinuousAudioRouteRearm(
      channel as unknown as WebRTCRPCDataChannel,
      () => invalid,
      { onError: (error) => errors.push(error.message) },
    );
    owner.activate({ streamId: "audio-old" });
    channel.receive(reloadEOSFrame());
    channel.receive(reloadEOSFrame());
    await flushPeerEvents();
    assert.deepEqual(errors, [
      "continuous audio route allocator must return a fresh non-empty stream ID",
      "continuous audio route allocator must return a fresh non-empty stream ID",
    ]);
    assert.equal(channel.sent.length, 0);
    owner.close();
  }

  const channel = new FakePeerEventChannel();
  channel.readyState = "open";
  channel.sendError = new Error("send failed");
  const errors: string[] = [];
  const rearmed: string[] = [];
  const owner = createContinuousAudioRouteRearm(
    channel as unknown as WebRTCRPCDataChannel,
    () => "audio-new",
    {
      onError: (error) => errors.push(error.message),
      onRearmed: (streamId) => rearmed.push(streamId),
    },
  );
  owner.activate({ streamId: "audio-old" });
  channel.receive(reloadEOSFrame());
  await flushPeerEvents();
  assert.deepEqual(errors, ["send failed"]);
  channel.sendError = undefined;
  channel.receive(reloadEOSFrame());
  await flushPeerEvents();
  assert.deepEqual(rearmed, []);
  channel.receive(inputReadyFrame("audio-new"));
  await flushPeerEvents();
  assert.deepEqual(rearmed, ["audio-new"]);
  assert.equal(channel.sent.length, 1);
  owner.close();
});

test("pending audio re-arm reports channel closure once", async () => {
  const channel = new FakePeerEventChannel();
  channel.readyState = "open";
  const errors: string[] = [];
  const owner = createContinuousAudioRouteRearm(
    channel as unknown as WebRTCRPCDataChannel,
    () => "new",
    { onError: (error) => errors.push(error.message) },
  );
  owner.activate({ streamId: "audio-old" });
  channel.receive(reloadEOSFrame());
  await flushPeerEvents();
  channel.dispatch("close");
  await flushPeerEvents();
  assert.deepEqual(errors, ["Peer Event channel closed."]);
  owner.close();
});

function inputReadyFrame(streamId: string): ArrayBuffer {
  return encodePeerEventFrame(
    peerEvent(PeerEventType.AUDIO_INPUT_READY, "audioInputReady", { streamId }),
  );
}

test("audio input waits for its own ready event", async () => {
  const channel = new FakePeerEventChannel();
  channel.readyState = "open";
  let ready = false;
  const opening = beginPeerAudioInput(
    channel as unknown as WebRTCRPCDataChannel,
    { streamId: "turn" },
  ).then(() => {
    ready = true;
  });
  await flushPeerEvents();
  assert.equal(channel.sent.length, 1);
  assert.equal(ready, false);
  channel.receive(inputReadyFrame("stale"));
  await flushPeerEvents();
  assert.equal(ready, false);
  channel.receive(inputReadyFrame("turn"));
  await opening;
  assert.equal(ready, true);
});

test("audio input exposes cancellation, denial, close and channel errors", async () => {
  for (const mode of ["abort", "denied", "close", "error"]) {
    const channel = new FakePeerEventChannel();
    channel.readyState = "open";
    const controller = new AbortController();
    const opening = beginPeerAudioInput(
      channel as unknown as WebRTCRPCDataChannel,
      { streamId: "turn" },
      controller.signal,
    );
    const rejected = assert.rejects(
      opening,
      mode === "abort"
        ? /cancelled/
        : mode === "denied"
          ? /MEMBER_REMOVED/
          : /Peer Event channel/,
    );
    if (mode === "abort") controller.abort(new Error("cancelled"));
    else if (mode === "denied")
      channel.receive(
        reloadEOSFrame({ streamId: "turn", errorCode: "MEMBER_REMOVED" }),
      );
    else channel.dispatch(mode);
    await rejected;
    assert.equal(channel.sent.length, 1);
  }
});

function reloadEOSFrame(
  input: {
    errorCode?: string;
    errorMessage?: string;
    errorRetryable?: boolean;
    kind?: StreamKind;
    label?: string;
    mimeType?: string;
    streamId?: string;
  } = {},
): ArrayBuffer {
  return encodePeerEventFrame(
    endPeerStream({
      streamId: input.streamId ?? "audio-old",
      kind: input.kind ?? StreamKind.AUDIO,
      label: input.label ?? "user",
      mimeType: input.mimeType ?? "audio/opus",
      error: {
        code: input.errorCode ?? "INPUT_ROUTE_RELOADED",
        message: input.errorMessage ?? "input route reloaded",
        retryable: input.errorRetryable ?? true,
      },
    }),
  );
}

async function flushPeerEvents(): Promise<void> {
  await new Promise<void>((resolve) => setImmediate(resolve));
}

class FakePeerEventChannel {
  readyState: RTCDataChannelState = "connecting";
  sendError?: Error;
  sent: ArrayBuffer[] = [];
  private readonly messages = new Set<(event: MessageEvent) => void>();
  private readonly other = new Map<
    string,
    Set<(event: MessageEvent) => void>
  >();

  dispatch(type: string): void {
    for (const listener of this.other.get(type) ?? [])
      listener({} as MessageEvent);
  }

  addEventListener(
    type: string,
    listener: (event: MessageEvent) => void,
  ): void {
    if (type === "message") this.messages.add(listener);
    else {
      const listeners = this.other.get(type) ?? new Set();
      listeners.add(listener);
      this.other.set(type, listeners);
    }
  }

  removeEventListener(
    type: string,
    listener: (event: MessageEvent) => void,
  ): void {
    if (type === "message") this.messages.delete(listener);
    else this.other.get(type)?.delete(listener);
  }

  send(data: ArrayBuffer | ArrayBufferView | Blob | string): void {
    if (this.sendError != null) throw this.sendError;
    assert.ok(data instanceof ArrayBuffer);
    this.sent.push(data);
  }

  receive(data: ArrayBuffer): void {
    for (const listener of this.messages) {
      listener({ data } as MessageEvent);
    }
  }
}

function peerEvent(
  type: PeerEventType,
  payloadCase: Exclude<PeerEvent["payload"]["case"], undefined>,
  value: object,
): PeerEvent {
  return create(PeerEventSchema, {
    version: 1,
    type,
    payload: { case: payloadCase, value },
  });
}
