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
import {
  createAdminAPIClient,
  createRegistrationToken,
  deletePeer,
  deleteRegistrationToken,
  getPeerRuntime,
  listRuntimeProfiles,
} from "@gizclaw/gizclaw/admin";
import {
  FriendGroupChange,
  subscribePeerEvents,
} from "@gizclaw/gizclaw/events";
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
  const adminPC = await connectSetupPeer(adminIdentityDir);
  const admin = createAdminAPIClient(adminPC as unknown as RTCPeerConnection, {
    requestTimeoutMs: 10_000,
  });
  const registrationTokenName = `e2e-js-concurrent-stream-${process.pid}-${Date.now()}`;
  const eventProbeGroupCreateName = `${registrationTokenName}-group`;
  let registrationTokenID: string | undefined;
  let eventProbeGroupName: string | undefined;
  let eventProbePeerRegistered = false;
  let uplinkTrack: MediaStreamTrack | undefined;
  let testError: unknown;
  let cleanupError: unknown;
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
    const setupRPC = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
      requestTimeoutMs: 10_000,
    });
    await setupRPC.call("server.info.put", {
      name: "JavaScript concurrent service Event probe",
    });
    eventProbePeerRegistered = true;
    const runtimeProfileID = await resolveRuntimeProfileID(
      admin,
      "default-gameplay",
    );
    const registrationToken = await createRegistrationToken({
      client: admin,
      body: {
        name: registrationTokenName,
        token: registrationTokenName,
        runtime_profile_id: runtimeProfileID,
      },
      throwOnError: true,
    });
    registrationTokenID = registrationToken.data.id;
    await setupRPC.call("server.register", { token: registrationTokenName });
    await deleteRegistrationToken({
      client: admin,
      path: { id: registrationTokenID },
      throwOnError: true,
    });
    registrationTokenID = undefined;
    const eventProbeGroup = await setupRPC.call("server.friend_group.create", {
      name: eventProbeGroupCreateName,
      display_name: "JavaScript concurrent service Event probe",
    });
    assert.equal(eventProbeGroup.name, eventProbeGroupCreateName);
    eventProbeGroupName = eventProbeGroup.name;

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
    requireDataChannelCloseStarted(firstRPCChannel);
    await requirePeerEventAfterServiceClose(
      pc,
      eventChannel,
      eventProbeGroupName,
    );

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
  } catch (error) {
    testError = error;
  } finally {
    if (registrationTokenID !== undefined) {
      try {
        await deleteRegistrationToken({
          client: admin,
          path: { id: registrationTokenID },
          throwOnError: true,
        });
      } catch (error) {
        cleanupError ??= error;
      }
    }
    if (eventProbePeerRegistered) {
      try {
        await deleteEventProbeGroup(pc, identityDir, eventProbeGroupName);
      } catch (error) {
        cleanupError ??= error;
      }
    }
    uplinkTrack?.stop();
    closePeerConnection(pc);
    try {
      await requirePeerOffline(identity.publicKey, admin);
    } catch (error) {
      cleanupError ??= error;
    }
    if (eventProbePeerRegistered) {
      try {
        await deletePeer({
          client: admin,
          path: { publicKey: identity.publicKey },
          throwOnError: true,
        });
      } catch (error) {
        cleanupError ??= error;
      }
    }
    closePeerConnection(adminPC);
  }
  if (testError != null) throw testError;
  if (cleanupError != null) throw cleanupError;
}

async function resolveRuntimeProfileID(
  client: ReturnType<typeof createAdminAPIClient>,
  name: string,
): Promise<string> {
  const matches: string[] = [];
  let cursor: string | undefined;
  do {
    const response = await listRuntimeProfiles({
      client,
      query: { cursor, limit: 200 },
      throwOnError: true,
    });
    for (const item of response.data.items) {
      if (item.name === name) matches.push(item.id);
    }
    cursor = response.data.has_next
      ? (response.data.next_cursor ?? undefined)
      : undefined;
  } while (cursor !== undefined);
  assert.equal(
    matches.length,
    1,
    `expected exactly one RuntimeProfile named ${JSON.stringify(name)}, found ${matches.length}`,
  );
  return matches[0]!;
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

async function requirePeerEventAfterServiceClose(
  pc: wrtc.RTCPeerConnection,
  channel: RTCDataChannel,
  groupName: string,
): Promise<void> {
  type EventCandidate = {
    change: FriendGroupChange | undefined;
    groupName: string | undefined;
    revisionUnixMs: bigint | undefined;
    type: string;
  };

  let revisionFloor: bigint | undefined;
  const candidates: EventCandidate[] = [];
  let newestMatchingCandidate: EventCandidate | undefined;
  let acceptCandidate = (_candidate: EventCandidate): void => {};
  let unsubscribe = (): void => {};
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    const eventPromise = new Promise<void>((resolve, reject) => {
      acceptCandidate = (candidate: EventCandidate): void => {
        if (
          revisionFloor != null &&
          candidate.type === "friend_group.updated" &&
          candidate.groupName === groupName &&
          candidate.change === FriendGroupChange.METADATA_UPDATED &&
          candidate.revisionUnixMs != null &&
          candidate.revisionUnixMs >= revisionFloor
        ) {
          resolve();
        }
      };
      unsubscribe = subscribePeerEvents(
        channel,
        (event) => {
          const update = event.friendGroupUpdated;
          const candidate: EventCandidate = {
            change: update?.change,
            groupName: update?.friendGroupName,
            revisionUnixMs: update?.revisionUnixMs,
            type: event.type,
          };
          if (candidates.length < 5) candidates.push(candidate);
          if (
            candidate.type === "friend_group.updated" &&
            candidate.groupName === groupName &&
            candidate.change === FriendGroupChange.METADATA_UPDATED &&
            candidate.revisionUnixMs != null &&
            (newestMatchingCandidate?.revisionUnixMs == null ||
              candidate.revisionUnixMs > newestMatchingCandidate.revisionUnixMs)
          ) {
            newestMatchingCandidate = candidate;
          }
          acceptCandidate(candidate);
        },
        (error) =>
          reject(
            new Error(`Peer Event channel or decoder failed: ${error.message}`),
          ),
      );
    });
    const timeoutPromise = new Promise<never>((_, reject) => {
      timeout = setTimeout(() => {
        const candidateSummary =
          candidates.length === 0
            ? "none"
            : candidates
                .map(
                  (candidate) =>
                    `type=${candidate.type} group=${candidate.groupName ?? "<none>"} ` +
                    `change=${candidate.change ?? "<none>"} ` +
                    `revision=${candidate.revisionUnixMs?.toString() ?? "<none>"}`,
                )
                .join("; ");
        reject(
          new Error(
            `Peer Event timeout after sibling RPC close: expected group=${groupName} ` +
              `change=${FriendGroupChange.METADATA_UPDATED} ` +
              `revision>=${revisionFloor?.toString() ?? "<mutation-pending>"}; ` +
              `candidates=${candidateSummary}`,
          ),
        );
      }, 10_000);
    });
    const eventRPC = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
      requestTimeoutMs: 10_000,
    });
    const groupPutPromise = eventRPC
      .call("server.friend_group.put", {
        name: groupName,
        display_name: "JavaScript concurrent service Event probe updated",
      })
      .then((groupPut) => {
        revisionFloor = parseServerRevisionUnixMs(groupPut.updated_at);
        if (newestMatchingCandidate != null) {
          acceptCandidate(newestMatchingCandidate);
        }
      })
      .catch((error: unknown) => {
        throw new Error(
          `Friend Group mutation failed after sibling RPC close: ${error instanceof Error ? error.message : String(error)}`,
        );
      });
    await Promise.all([
      groupPutPromise,
      Promise.race([eventPromise, timeoutPromise]),
    ]);
  } finally {
    if (timeout != null) clearTimeout(timeout);
    unsubscribe();
  }
}

function parseServerRevisionUnixMs(updatedAt: string | undefined): bigint {
  if (updatedAt == null || updatedAt === "") {
    throw new Error("Friend Group mutation response has no server updated_at");
  }
  const unixMs = Date.parse(updatedAt);
  if (!Number.isFinite(unixMs)) {
    throw new Error(
      `Friend Group mutation response has invalid server updated_at: ${updatedAt}`,
    );
  }
  return BigInt(unixMs);
}

async function deleteEventProbeGroup(
  pc: wrtc.RTCPeerConnection,
  identityDir: string,
  groupName: string | undefined,
): Promise<void> {
  const cleanupPC =
    pc.connectionState === "connected"
      ? pc
      : await connectSetupPeer(identityDir);
  try {
    const cleanupRPC = createPeerRPCClient(
      cleanupPC as unknown as RTCPeerConnection,
      { requestTimeoutMs: 10_000 },
    );
    if (groupName != null) {
      await cleanupRPC.call("server.friend_group.delete", { name: groupName });
    }
  } finally {
    if (cleanupPC !== pc) closePeerConnection(cleanupPC);
  }
}

function requireDataChannelCloseStarted(channel: RTCDataChannel): void {
  // The RPC client closes before resolving. wrtc can remain in "closing" while
  // its SCTP reset acknowledgement is pending, but the channel is no longer usable.
  assert.ok(
    channel.readyState === "closing" || channel.readyState === "closed",
    `data channel ${channel.id} state=${channel.readyState}, want closing or closed`,
  );
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

async function requirePeerOffline(
  peerPublicKey: string,
  client: ReturnType<typeof createAdminAPIClient>,
): Promise<void> {
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
