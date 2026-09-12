import {
  BUILTIN_KNOWLEDGE,
  createKnowledgeIndex,
  SourceError,
} from "@gizclaw/assistant";
import { GizClawControlError } from "@gizclaw/gizclaw-control";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  createConsoleRuntime,
  type ConsoleRuntimeDeps,
} from "./console-runtime";

const peers = vi.hoisted(() => ({
  findPeers: vi.fn(),
  loadPeer: vi.fn(),
  loadTelemetry: vi.fn(),
  loadTelemetryRange: vi.fn(),
  loadWifi: vi.fn(),
  loadWorkspaces: vi.fn(),
  loadHistory: vi.fn(),
  loadDeviceLogs: vi.fn(),
}));
vi.mock("@/lib/peers", () => ({
  ...peers,
  normalizePublicKey: (value: string) =>
    value.trim().replace(/^gizclaw_pk_/, ""),
  peerId: (peer: { endpoint: string; publicKey: string }) =>
    `${peer.endpoint}/${peer.publicKey}`,
  telemetryFields: ["network.rssi_dbm", "battery.percent"],
}));

const WATCHED = {
  publicKey: "watchedKey",
  label: "客厅",
  endpoint: "https://edge.example.com",
  addedAt: 1,
};

function setup(overrides: Partial<ConsoleRuntimeDeps> = {}) {
  const location = { hash: "#/peers", origin: "https://console.example.com" };
  const watch = vi.fn();
  const open = vi.fn();
  const confirm = vi.fn(() => true);
  const signal = new AbortController().signal;
  const deps: ConsoleRuntimeDeps = {
    config: () => ({
      name: "test",
      peers: [],
      servers: [
        {
          id: "edge-bj",
          name: "北京 Edge",
          url: "https://bj",
          role: "edge",
          monitorToken: "t",
        },
        {
          id: "server-sh",
          name: "上海",
          url: "https://sh",
          role: "server",
          monitorToken: "t",
        },
      ],
    }),
    fleet: () => ({
      "edge-bj": {
        status: "online",
        samples: [
          { timestamp: 1_000, time: "", rx: 1, tx: 2 },
          { timestamp: 9_000, time: "", rx: 30, tx: 40 },
        ],
        snapshot: undefined,
      },
    }),
    peers: () => [WATCHED],
    peerStates: () => ({}),
    watch,
    view: { current: undefined },
    signal: () => signal,
    window: {
      location,
      open,
      confirm,
    } as unknown as ConsoleRuntimeDeps["window"],
    now: () => 10_000,
    knowledge: () => createKnowledgeIndex(BUILTIN_KNOWLEDGE),
    ...overrides,
  };
  return {
    runtime: createConsoleRuntime(deps),
    location,
    watch,
    open,
    confirm,
    signal,
  };
}

beforeEach(() => {
  for (const mock of Object.values(peers)) mock.mockReset();
});

describe("console runtime", () => {
  it("searches the knowledge index the panel holds", async () => {
    const index = createKnowledgeIndex([
      {
        id: "import/a",
        title: "阳台排障",
        source: "a.md",
        text: "# 信号\n阳台音箱信号弱时切换到 2.4G。",
      },
    ]);
    const { runtime } = setup({ knowledge: () => index });
    const [passage] = await runtime.knowledge.search("阳台音箱信号", 3);
    expect(passage).toMatchObject({ title: "阳台排障", source: "a.md" });
  });

  it("navigates by hash and watches devices the target page needs", () => {
    const { runtime, location, watch } = setup();
    runtime.page.navigate({ page: "server", id: "edge-bj" });
    expect(location.hash).toBe("#/server/edge-bj");
    expect(runtime.page.current()).toEqual({ page: "server", id: "edge-bj" });

    runtime.page.navigate({
      page: "logs",
      query: "peer_public_key:newKey error_code:X",
    });
    expect(location.hash).toBe(
      `#/logs?q=${encodeURIComponent("peer_public_key:newKey error_code:X")}`,
    );
    expect(watch).toHaveBeenCalledWith({
      publicKey: "newKey",
      label: "",
      endpoint: "https://console.example.com",
      addedAt: 10_000,
    });

    watch.mockClear();
    runtime.page.navigate({ page: "peer", publicKey: "watchedKey" });
    expect(watch).not.toHaveBeenCalled();
  });

  it("opens links only after the user confirms", async () => {
    const declined = setup({});
    declined.confirm.mockReturnValue(false);
    await expect(
      declined.runtime.page.openLink("https://x.example"),
    ).resolves.toBe(false);
    expect(declined.open).not.toHaveBeenCalled();

    const accepted = setup();
    await expect(
      accepted.runtime.page.openLink("https://x.example"),
    ).resolves.toBe(true);
    expect(accepted.open).toHaveBeenCalledWith(
      "https://x.example",
      "_blank",
      "noopener,noreferrer",
    );
  });

  it("reads the published page view, or the route when nothing is published", () => {
    const view = { current: { page: "设备列表" } as unknown };
    const { runtime } = setup({ view });
    expect(runtime.view.snapshot()).toEqual({ page: "设备列表" });
    view.current = undefined;
    expect(runtime.view.snapshot()).toEqual({ route: { page: "peers" } });
  });

  it("maps fleet state to nodes and traffic windows", async () => {
    const { runtime } = setup();
    const nodes = await runtime.fleet.nodes();
    expect(nodes.map((node) => [node.id, node.status, node.rates])).toEqual([
      [
        "edge-bj",
        "online",
        { rx_bytes_per_second: 30, tx_bytes_per_second: 40 },
      ],
      ["server-sh", "loading", undefined],
    ]);
    expect(await runtime.fleet.traffic("edge-bj", 5)).toEqual([
      {
        time: new Date(9_000).toISOString(),
        rx_bytes_per_second: 30,
        tx_bytes_per_second: 40,
      },
    ]);
    await expect(runtime.fleet.traffic("missing", 60)).rejects.toMatchObject({
      code: "NOT_FOUND",
    });
  });

  it("calls device sources through the device's own endpoint", async () => {
    const { runtime, signal } = setup();
    peers.loadPeer.mockResolvedValue({
      info: {},
      runtime: { online: true },
      status: {},
    });
    await runtime.devices.status("gizclaw_pk_watchedKey");
    expect(peers.loadPeer).toHaveBeenCalledWith(
      "https://edge.example.com",
      "watchedKey",
      signal,
    );

    peers.loadTelemetry.mockResolvedValue([
      { field: "network.rssi_dbm", value: -80, observed_at_unix_ms: 1 },
      { field: "battery.percent", value: 50, observed_at_unix_ms: 1 },
    ]);
    expect(
      await runtime.devices.telemetry("other", ["battery.percent"]),
    ).toEqual([
      { field: "battery.percent", value: 50, observed_at_unix_ms: 1 },
    ]);
    expect(peers.loadTelemetry).toHaveBeenCalledWith(
      "https://console.example.com",
      "other",
      signal,
    );

    await expect(
      runtime.devices.telemetryRange({
        publicKey: "k",
        field: "gnss.x",
        startMs: 0,
        endMs: 1,
      }),
    ).rejects.toMatchObject({ code: "UNSUPPORTED_FIELD" });

    peers.loadHistory.mockResolvedValue({
      available: true,
      items: [],
      has_next: false,
      next_cursor: "c",
    });
    expect(
      await runtime.devices.history({
        publicKey: "watchedKey",
        workspaceId: "ws",
        order: "desc",
        limit: 50,
      }),
    ).toEqual({ items: [], nextCursor: "c" });
    expect(peers.loadHistory.mock.calls[0][3]).toMatchObject({
      query: "",
      order: "desc",
      limit: 50,
    });
  });

  it("searches logs with the console level names", async () => {
    const { runtime, signal } = setup();
    peers.loadDeviceLogs.mockResolvedValue({
      items: [],
      end: { has_next: true, next_cursor: "n" },
    });
    expect(
      await runtime.logs.search({
        publicKey: "watchedKey",
        level: "error",
        sinceMs: 5,
        untilMs: 9,
      }),
    ).toEqual({ items: [], nextCursor: "n" });
    expect(peers.loadDeviceLogs).toHaveBeenCalledWith(
      "https://edge.example.com",
      "watchedKey",
      "",
      "ERROR",
      5,
      9,
      undefined,
      signal,
    );
  });

  it("turns client failures into source errors", async () => {
    const { runtime } = setup();
    peers.loadPeer.mockRejectedValue(
      new GizClawControlError("forbidden", "debug off", {
        status: 403,
        code: "DEBUG_ACCESS_FORBIDDEN",
      }),
    );
    await expect(runtime.devices.status("k")).rejects.toMatchObject({
      code: "DEBUG_ACCESS_FORBIDDEN",
      httpStatus: 403,
    });
    peers.loadWifi.mockRejectedValue(new TypeError("Failed to fetch"));
    const failure = await runtime.devices
      .wifi("k")
      .catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(SourceError);
    expect(failure).toMatchObject({ code: "NETWORK_ERROR" });
  });
});
