import { describe, expect, it } from "vitest";
import {
  nodeDeviceMetricsSchema,
  type NodeDeviceMetrics,
  type NodeSnapshot,
} from "@/lib/api";
import { messageRates, metricTrend, transportTotals } from "@/lib/node-metrics";

function metrics(online = 4, packets = 10): NodeDeviceMetrics {
  return {
    online_devices: online,
    transports: ["device", "upstream", "admin", "unknown"].map((kind) => ({
      peer_kind:
        kind === "device"
          ? "device"
          : kind === "upstream"
            ? "upstream"
            : kind === "admin"
              ? "admin"
              : "unknown",
      rtp_tracks: { rx: 1, tx: 2 },
      data_channels: { packet: 1, service: 2, native: 3 },
      rtp_packets_total: { rx: packets, tx: packets * 2 },
      data_channel_messages_total: { rx: packets * 3, tx: packets * 4 },
    })),
  };
}
function node(at: number, value = metrics(), uptime = at / 1000): NodeSnapshot {
  return {
    public_key: "node",
    role: "server",
    time: new Date(at).toISOString(),
    uptime_seconds: uptime,
    goroutines: 1,
    heap_bytes: 0,
    transport: {
      connections: 1,
      services: 0,
      inbound_service_channels: 0,
      rx_bytes: 0,
      tx_bytes: 0,
    },
    device_metrics: value,
  };
}

describe("node device metrics", () => {
  it("adds physical channels without confusing them with devices", () => {
    const totals = transportTotals(metrics());
    expect(totals.rtpRxTracks).toBe(4);
    expect(
      totals.packetChannels + totals.serviceChannels + totals.nativeChannels,
    ).toBe(24);
    expect(transportTotals(metrics(), "device").messagesRx).toBe(30);
  });

  it("calculates rates only from consecutive samples of the same process", () => {
    const before = node(10000, metrics(4, 10));
    const after = node(15000, metrics(4, 20));
    expect(messageRates(before, after)).toEqual({
      rtpRx: 8,
      rtpTx: 16,
      messagesRx: 24,
      messagesTx: 32,
    });
    expect(messageRates(before, after, "device")?.rtpRx).toBe(2);
    expect(messageRates(undefined, after)).toBeUndefined();
    expect(
      messageRates({ ...before, device_metrics: undefined }, after),
    ).toBeUndefined();
    expect(
      messageRates(before, { ...after, public_key: "another-node" }),
    ).toBeUndefined();
    expect(messageRates(before, node(15000, metrics(4, 1)))).toBeUndefined();
    expect(
      messageRates(before, node(15000, metrics(4, 20), 2)),
    ).toBeUndefined();
    // Restart with a higher counter and uptime still must not make a spike.
    expect(
      messageRates(
        node(10000, metrics(4, 1), 1),
        node(15000, metrics(4, 20), 2),
      ),
    ).toBeUndefined();
  });

  it("does not turn missing nodes and network gaps into zeroes or partial totals", () => {
    const points = metricTrend(
      [
        {
          role: "server",
          samples: [
            { timestamp: 10000, onlineDevices: 3 },
            { timestamp: 15000 },
            { timestamp: 20000, onlineDevices: 0 },
          ],
        },
        {
          role: "server",
          samples: [
            { timestamp: 10000, onlineDevices: 2 },
            { timestamp: 15000, onlineDevices: 4 },
            { timestamp: 20000, onlineDevices: 0 },
          ],
        },
        { role: "edge", samples: [{ timestamp: 10000, onlineDevices: 9 }] },
      ],
      "online",
      120,
    );
    expect(points.map((point) => point.server)).toEqual([5, null, 0]);
    expect(points.map((point) => point.edge)).toEqual([9, null, null]);
    expect(points[0].server).not.toBe(points[0].edge);
  });

  it("rejects invalid counts and duplicate peer-kind rows", () => {
    expect(nodeDeviceMetricsSchema.safeParse(metrics()).success).toBe(true);
    expect(
      nodeDeviceMetricsSchema.safeParse({ ...metrics(), online_devices: -1 })
        .success,
    ).toBe(false);
    const duplicate = metrics();
    duplicate.transports[1] = duplicate.transports[0];
    expect(nodeDeviceMetricsSchema.safeParse(duplicate).success).toBe(false);
  });

  it("leaves a gap when the whole fleet stops polling", () => {
    const points = metricTrend(
      [
        {
          role: "server",
          samples: [
            { timestamp: 10000, onlineDevices: 3 },
            { timestamp: 50000, onlineDevices: 4 },
          ],
        },
      ],
      "online",
      120,
    );
    expect(points.map((point) => point.server)).toEqual([3, null, 4]);
  });
});
