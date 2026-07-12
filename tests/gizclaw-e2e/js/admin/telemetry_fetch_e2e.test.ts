import assert from "node:assert/strict";
import path from "node:path";

import { batteryTelemetry, gnssTelemetry, networkTelemetry, sendTelemetryPacket, systemTelemetry } from "@gizclaw/gizclaw";
import { aggregatePeerTelemetry, createAdminAPIClient, getPeerTelemetryLatest, queryPeerTelemetry } from "@gizclaw/gizclaw/admin";
import {
  assertSetupServerAvailable,
  closePeerConnection,
  connectSetupPeer,
  connectSetupPeerWithPacketChannel,
  loadIdentity,
  repoRoot,
} from "../common/webrtc.ts";

const adminIdentityDir = process.env.GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/admin");
const peerIdentityDir = process.env.GIZCLAW_E2E_JS_IDENTITY_DIR ?? path.join(repoRoot, "tests/gizclaw-e2e/testdata/identities/peer");

async function main(): Promise<void> {
  const adminIdentity = await loadIdentity(adminIdentityDir);
  const peerIdentity = await loadIdentity(peerIdentityDir);
  await assertSetupServerAvailable(adminIdentity.endpoint);

  const { packetChannel, pc: peerPC } = await connectSetupPeerWithPacketChannel(peerIdentityDir);
  const adminPC = await connectSetupPeer(adminIdentityDir);
  try {
    const client = createAdminAPIClient(adminPC as unknown as RTCPeerConnection, { requestTimeoutMs: 10_000 });
    const base = Date.now() - 40 * 60 * 1000;
    for (let index = 0; index < 12; index += 1) {
      sendTelemetryPacket(packetChannel, telemetryFrame(base, index));
    }

    await pollLatest(client, peerIdentity.publicKey);

    const start = base - 60_000;
    const end = Date.now() + 60_000;
    const ranged = await queryPeerTelemetry({
      client,
      path: { publicKey: peerIdentity.publicKey },
      query: {
        field: "battery.percent",
        start_time_ms: start,
        end_time_ms: end,
        step_ms: 120_000,
        limit: 100,
        order: "asc",
      },
      throwOnError: true,
    });
    assert.ok(ranged.data.points.length >= 6, `range point count ${ranged.data.points.length}`);

    const aggregate = await aggregatePeerTelemetry({
      client,
      path: { publicKey: peerIdentity.publicKey },
      query: {
        field: "battery.percent",
        start_time_ms: start,
        end_time_ms: end,
        bucket_ms: 600_000,
        aggregate: "last",
      },
      throwOnError: true,
    });
    assert.ok(aggregate.data.points.length > 0, "aggregate points should not be empty");
  } finally {
    closePeerConnection(peerPC);
    closePeerConnection(adminPC);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

function telemetryFrame(base: number, index: number) {
  return {
    observedAtUnixMs: base + index * 120_000,
    observations: [
      batteryTelemetry({ charging: index % 2 === 0, percent: 60 + index, voltageMv: 3700 + index * 7 }),
      gnssTelemetry({ accuracyM: 3.5 + index / 10, altitudeM: 12 + index, latitude: 37.77 + index / 1000, longitude: -122.42 + index / 1000 }),
      networkTelemetry({ connected: true, rssiDbm: -72 + index, signalLevel: 2 + (index % 4) }),
      systemTelemetry({ freeMemoryBytes: 64 * 1024 * 1024 - index * 128 * 1024, temperatureC: 35.5 + index / 10, uptimeSeconds: 3600 + index * 120 }),
    ],
    sequence: index + 1,
  };
}

async function pollLatest(client: ReturnType<typeof createAdminAPIClient>, peerPublicKey: string): Promise<void> {
  const deadline = Date.now() + 10_000;
  for (;;) {
    const latest = await getPeerTelemetryLatest({
      client,
      path: { publicKey: peerPublicKey },
      query: { fields: "battery.percent,gnss.latitude,gnss.longitude,network.rssi_dbm,system.temperature_c" },
      throwOnError: true,
    });
    const battery = latest.data.values.find((value) => value.field === "battery.percent");
    const latitude = latest.data.values.find((value) => value.field === "gnss.latitude");
    const rssi = latest.data.values.find((value) => value.field === "network.rssi_dbm");
    const temperature = latest.data.values.find((value) => value.field === "system.temperature_c");
    if (battery?.value === 71 && latitude != null && Math.abs(latitude.value - 37.781) < 0.000001 && rssi?.value === -61 && temperature?.value === 36.6) {
      return;
    }
    if (Date.now() > deadline) {
      throw new Error(`latest telemetry did not become ready: ${JSON.stringify(latest.data.values)}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
}

main().then(
  () => {
    console.log("ok - Node admin SDK queries peer telemetry written through telemetry packets");
    process.exit(0);
  },
  (err: unknown) => {
    console.error(err);
    process.exit(1);
  },
);
