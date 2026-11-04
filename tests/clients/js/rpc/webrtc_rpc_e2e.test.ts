import assert from "node:assert/strict";
import path from "node:path";

import { batteryTelemetry, sendTelemetryPacket } from "@gizclaw/gizclaw";
import { createPeerRPCClient } from "@gizclaw/gizclaw/rpc";
import {
  assertSetupServerAvailable,
  closePeerConnection,
  connectSetupPeerWithPacketChannel,
  loadIdentity,
  repoRoot,
} from "../common/webrtc.ts";

const identityDir =
  process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ??
  path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");

async function main(): Promise<void> {
  const identity = await loadIdentity(identityDir);
  await assertSetupServerAvailable(identity.endpoint);

  const { packetChannel, pc } =
    await connectSetupPeerWithPacketChannel(identityDir);
  try {
    const rpc = createPeerRPCClient(pc as unknown as RTCPeerConnection, {
      requestTimeoutMs: 10_000,
    });
    const result = await rpc.call("all.ping", {
      client_send_time: Date.now(),
    });

    assert.equal(typeof result.server_time, "number");
    assert.ok(result.server_time > 0);

    sendTelemetryPacket(packetChannel, {
      observations: [batteryTelemetry({ charging: true, percent: 88 })],
      sequence: 1,
    });
    const status = await pollServerStatus(rpc);
    assert.equal(status.battery_percent, 88);
    assert.equal(status.charging, true);
  } finally {
    closePeerConnection(pc);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

async function pollServerStatus(
  rpc: ReturnType<typeof createPeerRPCClient>,
): Promise<{ battery_percent?: number; charging?: boolean }> {
  const deadline = Date.now() + 5000;
  for (;;) {
    const status = (await rpc.call("server.status.get", {})) as {
      battery_percent?: number;
      charging?: boolean;
    };
    if (status.battery_percent === 88 && status.charging === true) {
      return status;
    }
    if (Date.now() > deadline) {
      throw new Error(
        `server.status.get did not reflect telemetry: ${JSON.stringify(status)}`,
      );
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
}

main().then(
  () => {
    console.log(
      "ok - Node WebRTC SDK connects to setup server, runs all.ping, and reports telemetry",
    );
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
