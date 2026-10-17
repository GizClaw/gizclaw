import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConsoleServer } from "@/lib/config";
import type { NodeSnapshot } from "@/lib/api";

const loadNode = vi.fn();
vi.mock("@/lib/api", () => ({
  loadNode: (...args: unknown[]) => loadNode(...args),
  nodeErrorMessage: (error: unknown) => String(error),
}));

const { useFleet } = await import("@/hooks/use-fleet");

const snapshot: NodeSnapshot = {
  public_key: "node-key",
  role: "server",
  time: new Date(1700000000000).toISOString(),
  uptime_seconds: 10,
  goroutines: 3,
  heap_bytes: 1024,
  transport: {
    connections: 1,
    services: 1,
    inbound_service_channels: 0,
    rx_bytes: 10,
    tx_bytes: 20,
  },
};

const server = (overrides: Partial<ConsoleServer> = {}): ConsoleServer => ({
  id: "server-1",
  name: "Server",
  url: "https://old.example.com",
  role: "server",
  monitorToken: `gizclaw_mk_${"x".repeat(32)}`,
  ...overrides,
});

describe("useFleet", () => {
  beforeEach(() => {
    loadNode.mockReset();
    loadNode.mockResolvedValue(snapshot);
  });

  it("polls the node the configuration currently names", async () => {
    const { result } = renderHook(() => useFleet([server()], 60000));
    await waitFor(() =>
      expect(result.current["server-1"]?.status).toBe("online"),
    );
    expect(loadNode.mock.calls[0][0]).toMatchObject({
      url: "https://old.example.com",
    });
  });

  it("re-reads a node whose url or token changed under the same id", async () => {
    const { rerender } = renderHook(
      ({ servers }: { servers: ConsoleServer[] }) => useFleet(servers, 60000),
      { initialProps: { servers: [server()] } },
    );
    await waitFor(() => expect(loadNode).toHaveBeenCalled());
    const rotated = server({
      url: "https://new.example.com",
      monitorToken: `gizclaw_mk_${"y".repeat(32)}`,
    });
    rerender({ servers: [rotated] });
    await waitFor(() =>
      expect(
        loadNode.mock.calls.some(
          (call) =>
            (call[0] as ConsoleServer).url === "https://new.example.com",
        ),
      ).toBe(true),
    );
    const last = loadNode.mock.calls.at(-1)?.[0] as ConsoleServer;
    expect(last.monitorToken).toBe(rotated.monitorToken);
  });

  it("does not let an aborted old response replace the new node's rate baseline", async () => {
    let finishOld: (value: NodeSnapshot) => void = () => {};
    let finishNext: (value: NodeSnapshot) => void = () => {};
    const oldResponse = new Promise<NodeSnapshot>((resolve) => {
      finishOld = resolve;
    });
    const nextResponse = new Promise<NodeSnapshot>((resolve) => {
      finishNext = resolve;
    });
    const measured = (
      key: string,
      seconds: number,
      count: number,
    ): NodeSnapshot => ({
      ...snapshot,
      public_key: key,
      time: new Date(seconds * 1000).toISOString(),
      uptime_seconds: seconds,
      device_metrics: {
        online_devices: 1,
        transports: [
          {
            peer_kind: "device",
            rtp_tracks: { rx: 1, tx: 1 },
            data_channels: { packet: 1, service: 1, native: 0 },
            rtp_packets_total: { rx: count, tx: count },
            data_channel_messages_total: { rx: count, tx: count },
          },
        ],
      },
    });
    let newCalls = 0;
    loadNode.mockImplementation((target: ConsoleServer) => {
      if (target.url === "https://old.example.com") return oldResponse;
      newCalls++;
      if (newCalls === 1) return Promise.resolve(measured("new-node", 15, 10));
      if (newCalls === 2) return nextResponse;
      return new Promise<NodeSnapshot>(() => {});
    });
    const { result, rerender } = renderHook(
      ({ target }: { target: ConsoleServer }) => useFleet([target], 20),
      { initialProps: { target: server() } },
    );
    await waitFor(() => expect(loadNode).toHaveBeenCalled());
    rerender({ target: server({ url: "https://new.example.com" }) });
    await waitFor(() =>
      expect(result.current["server-1"]?.snapshot?.public_key).toBe("new-node"),
    );
    await act(async () => {
      finishOld(measured("old-node", 10, 500));
    });
    await waitFor(() => expect(newCalls).toBeGreaterThanOrEqual(2));
    await act(async () => {
      finishNext(measured("new-node", 20, 20));
    });
    expect(result.current["server-1"]?.deviceRates?.messagesRx).toBe(2);
  });
});
