import { renderHook, waitFor } from "@testing-library/react";
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
});
