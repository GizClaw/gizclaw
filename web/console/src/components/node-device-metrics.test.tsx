import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NodeDeviceMetrics } from "@/components/node-device-metrics";
import type { ConsoleServer } from "@/lib/config";
import type { ServerState } from "@/hooks/use-fleet";

vi.mock("@/components/node-metric-chart", () => ({
  NodeMetricChart: () => null,
}));
afterEach(cleanup);

function source(
  role: "server" | "edge",
  online?: number,
  key = role,
): { server: ConsoleServer; state: ServerState } {
  return {
    server: {
      id: key,
      name: key,
      role,
      url: `https://${key}.example.com`,
      monitorToken: "test",
    },
    state: {
      status: "online",
      samples: [],
      snapshot: {
        public_key: key,
        role,
        time: new Date(10000).toISOString(),
        uptime_seconds: 10,
        goroutines: 1,
        heap_bytes: 1,
        transport: {
          connections: 72,
          services: 0,
          inbound_service_channels: 0,
          rx_bytes: 0,
          tx_bytes: 0,
        },
        ...(online === undefined
          ? {}
          : {
              device_metrics: {
                online_devices: online,
                transports: [
                  {
                    peer_kind: "device",
                    rtp_tracks: { rx: 2, tx: 2 },
                    data_channels: { packet: 2, service: 3, native: 0 },
                    rtp_packets_total: { rx: 100, tx: 200 },
                    data_channel_messages_total: { rx: 300, tx: 400 },
                  },
                ],
              },
            }),
      },
    },
  };
}

describe("node device dashboard", () => {
  it("shows an explicit unsupported state for deployed old nodes", () => {
    render(
      <NodeDeviceMetrics
        nodes={[source("server"), source("edge")]}
        windowSeconds={120}
      />,
    );
    expect(screen.getByText("设备指标尚未接入")).toBeTruthy();
    expect(
      screen.getByText(/2 个在线节点的当前版本尚未提供设备指标/),
    ).toBeTruthy();
    expect(screen.getByText("在线设备")).toBeTruthy();
    expect(screen.getByText("消息速率")).toBeTruthy();
    expect(screen.queryByText("—")).toBeNull();
    expect(screen.queryByText("72")).toBeNull();
  });

  it("keeps Server and Edge device counts separate and deduplicates node aliases", () => {
    const server = source("server", 7);
    const edge = source("edge", 11);
    render(
      <NodeDeviceMetrics
        nodes={[
          server,
          edge,
          {
            ...server,
            server: {
              ...server.server,
              id: "alias",
              url: "https://alias.example.com",
            },
          },
        ]}
        windowSeconds={120}
      />,
    );
    expect(screen.getByText("已覆盖 2/2 节点")).toBeTruthy();
    expect(screen.getByText("7")).toBeTruthy();
    expect(screen.getByText("11")).toBeTruthy();
    expect(screen.queryByText("18")).toBeNull();
    expect(screen.getByText("10")).toBeTruthy();
  });

  it("excludes stale values and converts measured rates to a minute estimate", () => {
    const current = source("server", 7);
    current.state.deviceRates = {
      messagesRx: 2,
      messagesTx: 3,
      rtpRx: 4,
      rtpTx: 5,
    };
    const failed = source("edge", 900);
    failed.state.status = "error";
    failed.state.deviceRates = {
      messagesRx: 900,
      messagesTx: 900,
      rtpRx: 900,
      rtpTx: 900,
    };
    render(<NodeDeviceMetrics nodes={[current, failed]} windowSeconds={120} />);
    expect(screen.queryByText("900")).toBeNull();
    expect(screen.getByText("2/s · 3/s")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("消息速率单位"), {
      target: { value: "min" },
    });
    expect(screen.getByText("120/min · 180/min")).toBeTruthy();
  });
});
