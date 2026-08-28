import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";
import wrtc from "@roamhq/wrtc";

import { createPeerRPCClient } from "@gizclaw/gizclaw/rpc";
import {
  PEER_EVENT_VERSION,
  PeerEventType,
  StreamKind,
  beginPeerStream,
  createContinuousAudioRouteRearm,
  createPeerEvent,
  endPeerStream,
  sendPeerEvent,
  subscribePeerEvents,
  type DecodedPeerStreamEvent,
} from "@gizclaw/gizclaw/events";
import {
  closePeerConnection,
  connectPeerWithTransports,
  loadIdentity,
  repoRoot,
} from "../common/webrtc.ts";

const identityDir =
  process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ??
  path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");
const directEndpoint = process.env.GIZCLAW_E2E_SERVER_ENDPOINT;
const registrationToken = process.env.GIZCLAW_TEST_REGISTRATION_TOKEN;
const expectedRuntimeProfile =
  process.env.GIZCLAW_E2E_RUNTIME_PROFILE ?? "e2e-giztest";
const inputMode = process.env.GIZCLAW_E2E_INPUT_MODE ?? "audio";
const workflowName =
  process.env.GIZCLAW_E2E_WORKFLOW_NAME ??
  (inputMode === "audio-reload"
    ? "doubao-realtime-conversation"
    : "flowcraft-voice-assistant");
const inputVoice = process.env.GIZCLAW_E2E_INPUT_VOICE ?? "narrator";
const inputPCMPath = process.env.GIZCLAW_E2E_INPUT_PCM_PATH;
const turnTimeoutMs = 90_000;
const realtimeTailSilenceMs = 4_000;

type ConnectionProbe = {
  events: DecodedPeerStreamEvent[];
  timeline: string[];
};

async function main(): Promise<void> {
  assert.ok(
    directEndpoint,
    "GIZCLAW_E2E_SERVER_ENDPOINT is required to verify Edge routing",
  );
  assert.ok(
    registrationToken,
    "GIZCLAW_TEST_REGISTRATION_TOKEN is required for an isolated JS peer",
  );
  const edgeEndpoint =
    process.env.GIZCLAW_E2E_EDGE_ENDPOINT ??
    (await loadIdentity(identityDir)).endpoint;
  assert.notEqual(
    edgeEndpoint,
    directEndpoint,
    "the JS conversation lifecycle test requires a Peer identity routed through Edge",
  );
  const clientPrivateKey = loadPrivateKey();
  const workspaceName = `e2e-js-conversation-${process.pid}-${Date.now()}`;
  let testError: unknown;
  try {
    await withConnection(
      "edge",
      edgeEndpoint,
      clientPrivateKey,
      async (rpc, probe, eventChannel, pc) => {
        await rpc.call("server.info.put", {
          name: "JavaScript conversation lifecycle E2E",
        });
        const registration = await rpc.call("server.register", {
          token: registrationToken,
        });
        assert.equal(registration.runtime_profile_name, expectedRuntimeProfile);
        await rpc.call("server.workspace.create", {
          name: workspaceName,
          collection: "assistants",
          workflow_name: workflowName,
          parameters: {
            agent_type:
              inputMode === "audio-reload" ? "doubao-realtime" : "flowcraft",
            input: inputMode === "audio-reload" ? "realtime" : "push-to-talk",
          },
        });
        await rpc.call("server.run.workspace.set", {
          workspace_name: workspaceName,
        });
        if (inputMode === "audio") {
          await runTwoAudioTurns(rpc, probe, eventChannel, pc);
        } else if (inputMode === "audio-reload") {
          await runContinuousAudioAcrossReload(rpc, probe, eventChannel, pc);
        } else {
          assert.equal(
            inputMode,
            "text",
            `unsupported input mode ${inputMode}`,
          );
          await runTwoTextTurns("edge", probe, eventChannel);
        }
        await rpc.call("server.run.stop", {});
        await rpc.call("server.workspace.delete", { name: workspaceName });
      },
      async (rpc) => rpc.call("server.peer.delete", {}),
    );
  } catch (error) {
    testError = error;
  }
  await new Promise((resolve) => setTimeout(resolve, 250));
  if (testError != null) throw testError;
}

function loadPrivateKey(): Uint8Array {
  const hex = process.env.GIZCLAW_E2E_JS_PRIVATE_KEY_HEX;
  if (hex == null) return randomBytes(32);
  if (!/^[0-9a-f]{64}$/i.test(hex)) {
    throw new Error(
      "GIZCLAW_E2E_JS_PRIVATE_KEY_HEX must contain 64 hex digits",
    );
  }
  return Buffer.from(hex, "hex");
}

async function withConnection(
  name: string,
  endpoint: string,
  clientPrivateKey: Uint8Array,
  run: (
    rpc: ReturnType<typeof createPeerRPCClient>,
    probe: ConnectionProbe,
    eventChannel: RTCDataChannel,
    pc: wrtc.RTCPeerConnection,
  ) => Promise<void>,
  afterAssertions?: (
    rpc: ReturnType<typeof createPeerRPCClient>,
  ) => Promise<unknown>,
): Promise<void> {
  const { eventChannel, packetChannel, pc } = await connectPeerWithTransports(
    clientPrivateKey,
    endpoint,
  );
  const probe: ConnectionProbe = {
    events: [],
    timeline: [`${name}:connected`],
  };
  const mark = (value: string): void => {
    probe.timeline.push(`${Date.now()}:${name}:${value}`);
  };
  for (const [label, channel] of [
    ["packet", packetChannel],
    ["event", eventChannel],
  ] as const) {
    channel.addEventListener("close", () => mark(`${label}:close`));
    channel.addEventListener("error", (event) =>
      mark(`${label}:error:${describeRTCError(event)}`),
    );
  }
  pc.addEventListener("connectionstatechange", () =>
    mark(`pc:${pc.connectionState}`),
  );
  const unsubscribe = subscribePeerEvents(
    eventChannel,
    (event) => {
      probe.events.push(event);
      mark(`peer-event:${event.type}:${event.label ?? ""}`);
    },
    (error) => mark(`peer-event-error:${error.message}`),
  );
  const rpc = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
    requestTimeoutMs: 10_000,
  });
  let runError: unknown;
  try {
    await run(rpc, probe, eventChannel, pc);
    assert.equal(
      eventChannel.readyState,
      "open",
      `${name} Event channel closed: ${probe.timeline.join(" | ")}`,
    );
    assert.equal(
      packetChannel.readyState,
      "open",
      `${name} Packet channel closed: ${probe.timeline.join(" | ")}`,
    );
    assert.equal(
      pc.connectionState,
      "connected",
      `${name} PeerConnection left connected state: ${probe.timeline.join(" | ")}`,
    );
  } catch (error) {
    runError = error;
  }
  try {
    await afterAssertions?.(rpc);
  } catch (cleanupError) {
    if (runError != null) {
      runError = new AggregateError(
        [runError, cleanupError],
        `${name} assertions and cleanup both failed`,
      );
    } else {
      runError = cleanupError;
    }
  }
  try {
    if (runError != null) throw runError;
  } catch (error) {
    throw new Error(
      `${name} conversation lifecycle failed; timeline=${probe.timeline.join(" | ")}`,
      { cause: error },
    );
  } finally {
    unsubscribe();
    closePeerConnection(pc);
  }
}

async function runTwoTextTurns(
  name: string,
  probe: ConnectionProbe,
  eventChannel: RTCDataChannel,
): Promise<void> {
  for (let turn = 1; turn <= 2; turn++) {
    const eventOffset = probe.events.length;
    const streamID = `${name}-${process.pid}-${Date.now()}-${turn}`;
    sendTextTurn(eventChannel, streamID, `第 ${turn} 轮，请只回答收到。`);
    await waitForAssistantTurn(probe, eventOffset, streamID);
    assert.equal(
      eventChannel.readyState,
      "open",
      `${name} Event channel closed after turn ${turn}`,
    );
  }
}

async function runTwoAudioTurns(
  rpc: ReturnType<typeof createPeerRPCClient>,
  probe: ConnectionProbe,
  eventChannel: RTCDataChannel,
  pc: wrtc.RTCPeerConnection,
): Promise<void> {
  const audio = pc
    .getTransceivers()
    .find(({ receiver }) => receiver.track.kind === "audio");
  assert.ok(audio, "registered JS Peer has no audio transceiver after connect");
  const source = new wrtc.nonstandard.RTCAudioSource();
  const uplinkTrack = source.createTrack();
  const sink = new wrtc.nonstandard.RTCAudioSink(audio.receiver.track);
  let downlinkFrames = 0;
  sink.ondata = () => {
    downlinkFrames++;
  };
  await audio.sender.replaceTrack(uplinkTrack);
  try {
    const { pcm, sampleRate } = await loadInputPCM(rpc);
    for (let turn = 1; turn <= 2; turn++) {
      const eventOffset = probe.events.length;
      const downlinkOffset = downlinkFrames;
      const streamID = `edge-audio-${process.pid}-${Date.now()}-${turn}`;
      sendPeerEvent(
        eventChannel,
        beginPeerStream({
          streamId: streamID,
          kind: StreamKind.AUDIO,
          label: "user",
          mimeType: "audio/opus",
        }),
      );
      await sendPCM(source, pcm, sampleRate);
      sendPeerEvent(
        eventChannel,
        endPeerStream({
          streamId: streamID,
          kind: StreamKind.AUDIO,
          label: "user",
          mimeType: "audio/opus",
        }),
      );
      await waitForAssistantTurn(probe, eventOffset, streamID);
      assert.ok(
        downlinkFrames > downlinkOffset,
        `edge audio turn ${turn} produced no WebRTC downlink frames`,
      );
      assert.equal(
        eventChannel.readyState,
        "open",
        `edge Event channel closed after audio turn ${turn}`,
      );
    }
  } finally {
    sink.stop();
    uplinkTrack.stop();
  }
}

async function runContinuousAudioAcrossReload(
  rpc: ReturnType<typeof createPeerRPCClient>,
  probe: ConnectionProbe,
  eventChannel: RTCDataChannel,
  pc: wrtc.RTCPeerConnection,
): Promise<void> {
  const audio = pc
    .getTransceivers()
    .find(({ receiver }) => receiver.track.kind === "audio");
  assert.ok(audio, "registered JS Peer has no audio transceiver after connect");
  const source = new wrtc.nonstandard.RTCAudioSource();
  const uplinkTrack = source.createTrack();
  const sink = new wrtc.nonstandard.RTCAudioSink(audio.receiver.track);
  let downlinkFrames = 0;
  sink.ondata = () => {
    downlinkFrames++;
  };
  await audio.sender.replaceTrack(uplinkTrack);
  let routeSequence = 0;
  const allocateStreamID = (): string =>
    `edge-audio-${process.pid}-${Date.now()}-${++routeSequence}`;
  const routeRearm = createContinuousAudioRouteRearm(
    eventChannel,
    allocateStreamID,
  );
  let activeStreamID = allocateStreamID();
  const initialStreamID = activeStreamID;
  let reloadEOSObserved = false;
  let resolveRearmed!: (streamID: string) => void;
  let rejectRearmed!: (error: Error) => void;
  const rearmed = new Promise<string>((resolve, reject) => {
    resolveRearmed = resolve;
    rejectRearmed = reject;
  });
  const unsubscribeRearm = subscribePeerEvents(
    eventChannel,
    (event) => {
      if (
        event.type === "eos" &&
        event.streamId === activeStreamID &&
        event.kind === "audio" &&
        event.label === "user" &&
        event.mimeType?.split(";", 1)[0]?.trim().toLowerCase() ===
          "audio/opus" &&
        event.errorCode === "INPUT_ROUTE_RELOADED" &&
        event.errorMessage === "input route reloaded" &&
        event.errorRetryable === true
      ) {
        reloadEOSObserved = true;
      }
      const replacement = routeRearm.handle(event);
      if (replacement != null) {
        activeStreamID = replacement;
        resolveRearmed(replacement);
      }
    },
    rejectRearmed,
  );
  try {
    const { pcm, sampleRate } = await loadInputPCM(rpc);
    sendPeerEvent(
      eventChannel,
      beginPeerStream({
        streamId: activeStreamID,
        kind: StreamKind.AUDIO,
        label: "user",
        mimeType: "audio/opus",
      }),
    );
    routeRearm.activate({
      streamId: activeStreamID,
      label: "user",
      mimeType: "audio/opus",
    });

    const firstEventOffset = probe.events.length;
    const firstDownlinkOffset = downlinkFrames;
    await sendPCM(source, pcm, sampleRate);
    await sendSilence(source, sampleRate, realtimeTailSilenceMs);
    await waitForAssistantTurn(probe, firstEventOffset, activeStreamID);
    assert.ok(
      downlinkFrames > firstDownlinkOffset,
      "continuous audio produced no WebRTC downlink before reload",
    );

    const originalPC = pc;
    const originalEventChannel = eventChannel;
    const originalTrack = audio.sender.track;
    const replacementStreamID = await Promise.race([
      Promise.all([rpc.call("server.run.workspace.reload", {}), rearmed]).then(
        ([, streamID]) => streamID,
      ),
      new Promise<never>((_, reject) =>
        setTimeout(
          () =>
            reject(
              new Error(
                reloadEOSObserved
                  ? "JavaScript SDK observed INPUT_ROUTE_RELOADED EOS but did not send a replacement BOS"
                  : "Server did not send INPUT_ROUTE_RELOADED EOS for the active JavaScript SDK route",
              ),
            ),
          10_000,
        ),
      ),
    ]);
    assert.notEqual(replacementStreamID, initialStreamID);
    assert.equal(pc, originalPC, "reload replaced the PeerConnection");
    assert.equal(
      eventChannel,
      originalEventChannel,
      "reload replaced the Event channel",
    );
    assert.equal(
      audio.sender.track,
      originalTrack,
      "reload replaced the audio track",
    );

    const secondEventOffset = probe.events.length;
    const secondDownlinkOffset = downlinkFrames;
    await sendPCM(source, pcm, sampleRate);
    await sendSilence(source, sampleRate, realtimeTailSilenceMs);
    await waitForAssistantTurn(probe, secondEventOffset, activeStreamID);
    assert.ok(
      downlinkFrames > secondDownlinkOffset,
      "continuous audio produced no WebRTC downlink after reload",
    );
    assert.equal(eventChannel.readyState, "open");
    assert.equal(pc.connectionState, "connected");
  } finally {
    routeRearm.deactivate();
    unsubscribeRearm();
    if (eventChannel.readyState === "open") {
      sendPeerEvent(
        eventChannel,
        endPeerStream({
          streamId: activeStreamID,
          kind: StreamKind.AUDIO,
          label: "user",
          mimeType: "audio/opus",
        }),
      );
    }
    sink.stop();
    uplinkTrack.stop();
  }
}

async function loadInputPCM(
  rpc: ReturnType<typeof createPeerRPCClient>,
): Promise<{ pcm: Buffer; sampleRate: number }> {
  if (inputPCMPath != null) {
    const pcm = await readFile(inputPCMPath);
    const sampleRate = Number(
      process.env.GIZCLAW_E2E_INPUT_PCM_SAMPLE_RATE ?? "16000",
    );
    assert.equal(
      Number.isInteger(sampleRate) && sampleRate > 0 && sampleRate % 100 === 0,
      true,
      "GIZCLAW_E2E_INPUT_PCM_SAMPLE_RATE must be a positive integer divisible by 100",
    );
    assert.equal(
      pcm.byteLength > 0 && pcm.byteLength % 2 === 0,
      true,
      "input PCM fixture must contain non-empty 16-bit mono samples",
    );
    return { pcm, sampleRate };
  }

  const synthesis = await rpc.synthesizeSpeech({
    voice_name: inputVoice,
    text: "请回答收到。",
    accepted_content_types: ["audio/pcm"],
  });
  const chunks: Uint8Array[] = [];
  for await (const chunk of synthesis.body) chunks.push(chunk);
  const pcm = Buffer.concat(chunks.map((chunk) => Buffer.from(chunk)));
  assert.equal(pcm.byteLength % 2, 0, "synthesized PCM is not 16-bit aligned");
  const sampleRate = synthesis.result.sample_rate_hz ?? 16_000;
  const channels = synthesis.result.channels ?? 1;
  assert.equal(channels, 1, "audio probe requires mono PCM");
  return { pcm, sampleRate };
}

async function sendPCM(
  source: wrtc.nonstandard.RTCAudioSource,
  pcm: Buffer,
  sampleRate: number,
): Promise<void> {
  const samples = new Int16Array(
    pcm.buffer,
    pcm.byteOffset,
    pcm.byteLength / Int16Array.BYTES_PER_ELEMENT,
  );
  const framesPerChunk = sampleRate / 100;
  assert.equal(
    Number.isInteger(framesPerChunk),
    true,
    `unsupported PCM sample rate ${sampleRate}`,
  );
  for (let offset = 0; offset < samples.length; offset += framesPerChunk) {
    const frame = new Int16Array(framesPerChunk);
    frame.set(samples.subarray(offset, offset + framesPerChunk));
    source.onData({
      bitsPerSample: 16,
      channelCount: 1,
      numberOfFrames: framesPerChunk,
      sampleRate,
      samples: frame,
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
}

async function sendSilence(
  source: wrtc.nonstandard.RTCAudioSource,
  sampleRate: number,
  durationMs: number,
): Promise<void> {
  const framesPerChunk = sampleRate / 100;
  assert.equal(
    Number.isInteger(framesPerChunk),
    true,
    `unsupported PCM sample rate ${sampleRate}`,
  );
  const frame = new Int16Array(framesPerChunk);
  for (let elapsed = 0; elapsed < durationMs; elapsed += 10) {
    source.onData({
      bitsPerSample: 16,
      channelCount: 1,
      numberOfFrames: framesPerChunk,
      sampleRate,
      samples: frame,
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
}

function sendTextTurn(
  channel: RTCDataChannel,
  streamID: string,
  text: string,
): void {
  sendPeerEvent(
    channel,
    createPeerEvent({
      version: PEER_EVENT_VERSION,
      type: PeerEventType.BOS,
      payload: {
        case: "bos",
        value: {
          streamId: streamID,
          kind: StreamKind.TEXT,
          label: "user",
          mimeType: "text/plain",
        },
      },
    }),
  );
  sendPeerEvent(
    channel,
    createPeerEvent({
      version: PEER_EVENT_VERSION,
      type: PeerEventType.TEXT_DELTA,
      payload: {
        case: "textDelta",
        value: { streamId: streamID, label: "user", text },
      },
    }),
  );
  sendPeerEvent(
    channel,
    createPeerEvent({
      version: PEER_EVENT_VERSION,
      type: PeerEventType.TEXT_DONE,
      payload: {
        case: "textDone",
        value: { streamId: streamID, label: "user", text: "" },
      },
    }),
  );
}

async function waitForAssistantTurn(
  probe: ConnectionProbe,
  offset: number,
  inputStreamID: string,
): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const deadline = Date.now() + turnTimeoutMs;
    const poll = (): void => {
      const events = probe.events.slice(offset);
      const assistantTextDone = events.some(
        (event) =>
          event.type === "text.done" &&
          event.label === "assistant" &&
          event.streamId !== inputStreamID,
      );
      const assistantAudioDone = events.some(
        (event) =>
          event.type === "eos" &&
          event.kind === "audio" &&
          event.label === "assistant",
      );
      if (assistantTextDone && assistantAudioDone) {
        resolve();
        return;
      }
      if (Date.now() >= deadline) {
        reject(
          new Error(
            `assistant turn timed out; textDone=${assistantTextDone} audioDone=${assistantAudioDone}`,
          ),
        );
        return;
      }
      setTimeout(poll, 25);
    };
    poll();
  });
}

function describeRTCError(event: Event): string {
  const rtcError = (event as RTCErrorEvent).error;
  if (rtcError == null) return event.type;
  return [rtcError.errorDetail, rtcError.sctpCauseCode, rtcError.message]
    .filter((value) => value != null && value !== "")
    .join(":");
}

// Avoid @roamhq/wrtc's intermittent native finalizer crash after all
// PeerConnections have already been closed by this standalone E2E process.
try {
  await main();
} catch (error) {
  console.error(error);
  process.exit(1);
}
process.exit(0);
