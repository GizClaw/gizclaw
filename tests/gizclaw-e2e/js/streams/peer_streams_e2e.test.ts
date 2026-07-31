import assert from "node:assert/strict";
import path from "node:path";
import wrtc from "@roamhq/wrtc";

import {
  GIZCLAW_EVENT_STREAM_AGENT,
  GIZCLAW_SERVICE_PEER_HTTP,
  GIZCLAW_SERVICE_PEER_RPC,
  GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL,
  batteryTelemetry,
  createWebRTCServiceFetch,
  giznetServiceDataChannelLabel,
  sendGiznetWebRTCTelemetry,
  type WebRTCRPCDataChannelFactory,
} from "@gizclaw/gizclaw";
import { createPeerRPCClient } from "@gizclaw/gizclaw/rpc";
import { createAdminAPIClient, getPeerRuntime } from "@gizclaw/gizclaw/admin";
import {
  assertSetupServerAvailable,
  closePeerConnection,
  connectSetupPeer,
  connectSetupPeerWithTransports,
  loadIdentity,
  repoRoot,
  waitForDataChannelOpen,
} from "../common/webrtc.ts";

const identityDir =
  process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ??
  path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");
const adminIdentityDir =
  process.env.GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR ??
  path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/admin");

async function main(): Promise<void> {
  const identity = await loadIdentity(identityDir);
  await assertSetupServerAvailable(identity.endpoint);
  const transports = await connectSetupPeerWithTransports(identityDir);
  const { createdChannels, eventChannel, packetChannel, pc } = transports;
  let uplinkTrack: MediaStreamTrack | undefined;
  try {
    const mandatoryLabels = createdChannels.map(({ label }) => label);
    assert.deepEqual(mandatoryLabels, [
      GIZNET_WEBRTC_PACKET_DATA_CHANNEL_LABEL,
      giznetServiceDataChannelLabel(GIZCLAW_EVENT_STREAM_AGENT),
    ]);
    assert.equal(packetChannel.ordered, false);
    assert.equal(packetChannel.maxRetransmits, 0);
    assert.equal(eventChannel.ordered, true);

    const audioTransceivers = pc
      .getTransceivers()
      .filter(({ receiver }) => receiver.track.kind === "audio");
    assert.equal(audioTransceivers.length, 1);
    const [audio] = audioTransceivers;
    assert.equal(audio.direction, "sendrecv");
    assert.ok(audio.sender);
    const audioSource = new wrtc.nonstandard.RTCAudioSource();
    uplinkTrack = audioSource.createTrack();
    await audio.sender.replaceTrack(uplinkTrack);
    assert.equal(audio.sender.track, uplinkTrack);
    assert.equal(audio.receiver.track.kind, "audio");
    const rpcLabel = giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_RPC);
    const httpLabel = giznetServiceDataChannelLabel(GIZCLAW_SERVICE_PEER_HTTP);
    const firstRPCChannel = pc.createDataChannel(rpcLabel, {
      ordered: true,
    }) as unknown as RTCDataChannel;
    const secondRPCChannel = pc.createDataChannel(rpcLabel, {
      ordered: true,
    }) as unknown as RTCDataChannel;
    const httpChannel = pc.createDataChannel(httpLabel, {
      ordered: true,
    }) as unknown as RTCDataChannel;
    await Promise.all([
      waitForDataChannelOpen(firstRPCChannel),
      waitForDataChannelOpen(secondRPCChannel),
      waitForDataChannelOpen(httpChannel),
    ]);
    assert.notEqual(firstRPCChannel.id, secondRPCChannel.id);
    assert.notEqual(firstRPCChannel.id, httpChannel.id);
    assert.notEqual(secondRPCChannel.id, httpChannel.id);

    const firstRPC = createPeerRPCClient(
      oneChannelFactory(firstRPCChannel, rpcLabel),
      { requestTimeoutMs: 10_000 },
    );
    const secondRPC = createPeerRPCClient(
      oneChannelFactory(secondRPCChannel, rpcLabel),
      { requestTimeoutMs: 10_000 },
    );
    const peerFetch = createWebRTCServiceFetch(
      oneChannelFactory(httpChannel, httpLabel),
      {
        requestTimeoutMs: 10_000,
        service: GIZCLAW_SERVICE_PEER_HTTP,
      },
    );
    const firstPingPromise = firstRPC.call("all.ping", {
      client_send_time: Date.now(),
    });
    const secondPingPromise = secondRPC.call("all.ping", {
      client_send_time: Date.now(),
    });
    const serverInfoPromise = peerFetch("http://gizclaw/server-info");
    const remainingResponses = Promise.all([
      secondPingPromise,
      serverInfoPromise,
    ]);
    const firstPing = await firstPingPromise;
    assert.ok(firstPing.server_time > 0);
    await waitForDataChannelClosed(firstRPCChannel);

    const [secondPing, serverInfo] = await remainingResponses;
    assert.ok(secondPing.server_time > 0);
    assert.equal(serverInfo.status, 200);
    const serverInfoBody = (await serverInfo.json()) as {
      public_key?: string;
    };
    assert.equal(typeof serverInfoBody.public_key, "string");
    assert.equal(pc.connectionState, "connected");
    assert.equal(eventChannel.readyState, "open");

    for (let index = 0; index < 3; index++) {
      audioSource.onData({
        bitsPerSample: 16,
        channelCount: 1,
        numberOfFrames: 480,
        sampleRate: 48_000,
        samples: new Int16Array(480),
      });
    }

    await sendGiznetWebRTCTelemetry(
      pc as unknown as RTCPeerConnection,
      {
        observations: [batteryTelemetry({ charging: true, percent: 87 })],
        sequence: 666,
      },
      { timeoutMs: 10_000 },
    );
    const statusRPC = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
      requestTimeoutMs: 10_000,
    });
    const status = await pollTelemetry(statusRPC);
    assert.equal(status.battery_percent, 87);
    assert.equal(status.charging, true);

    assert.equal(eventChannel.readyState, "open");
    assert.equal(pc.connectionState, "connected");
  } finally {
    uplinkTrack?.stop();
    closePeerConnection(pc);
    await requirePeerOffline(identity.publicKey);
  }
}

function oneChannelFactory(
  channel: RTCDataChannel,
  expectedLabel: string,
): WebRTCRPCDataChannelFactory {
  let used = false;
  return {
    createDataChannel(label): RTCDataChannel {
      assert.equal(label, expectedLabel);
      assert.equal(used, false, `channel ${channel.id} was requested twice`);
      used = true;
      return channel;
    },
  } as unknown as WebRTCRPCDataChannelFactory;
}

async function waitForDataChannelClosed(
  channel: RTCDataChannel,
): Promise<void> {
  if (channel.readyState === "closed") return;
  await new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      cleanup();
      reject(
        new Error(
          `data channel ${channel.id} state=${channel.readyState}, want closed`,
        ),
      );
    }, 5000);
    const onClose = (): void => {
      cleanup();
      resolve();
    };
    const cleanup = (): void => {
      clearTimeout(timeout);
      channel.removeEventListener("close", onClose);
    };
    channel.addEventListener("close", onClose);
  });
}

async function pollTelemetry(
  rpc: ReturnType<typeof createPeerRPCClient>,
): Promise<{ battery_percent?: number; charging?: boolean }> {
  const deadline = Date.now() + 5000;
  for (;;) {
    const status = await rpc.call("server.status.get", {});
    if (status.battery_percent === 87 && status.charging === true) {
      return status;
    }
    if (Date.now() >= deadline) {
      throw new Error(
        `server.status.get did not reflect telemetry: ${JSON.stringify(status)}`,
      );
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
}

async function requirePeerOffline(peerPublicKey: string): Promise<void> {
  const adminPC = await connectSetupPeer(adminIdentityDir);
  try {
    const client = createAdminAPIClient(
      adminPC as unknown as RTCPeerConnection,
      { requestTimeoutMs: 10_000 },
    );
    const deadline = Date.now() + 10_000;
    for (;;) {
      const runtime = await getPeerRuntime({
        client,
        path: { publicKey: peerPublicKey },
        throwOnError: true,
      });
      if (!runtime.data.online) return;
      if (Date.now() >= deadline) {
        throw new Error(
          `Peer ${peerPublicKey} remained online after closing the JavaScript SDK`,
        );
      }
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  } finally {
    closePeerConnection(adminPC);
  }
}

main().then(
  () => {
    console.log(
      "ok - Node WebRTC keeps mandatory transports while concurrent service channels coexist",
    );
    process.exit(0);
  },
  (error: unknown) => {
    console.error(error);
    process.exit(1);
  },
);
