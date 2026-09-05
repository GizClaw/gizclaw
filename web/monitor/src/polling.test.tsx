// @vitest-environment jsdom
import React, { type ReactNode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "./main";
import { loadNode, type NodeSnapshot } from "./api";

vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  loadNode: vi.fn(),
}));
vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  AreaChart: ({ data }: { data: unknown[] }) => (
    <div data-testid="traffic-chart">{data.length} samples</div>
  ),
  Area: () => null,
  CartesianGrid: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
}));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

test("poll failure clears live samples and timestamp and recovers with fresh samples", async () => {
  vi.useFakeTimers();
  const snapshot: NodeSnapshot = {
    public_key: "test-node",
    role: "server",
    time: "2026-09-05T00:00:00Z",
    uptime_seconds: 100,
    goroutines: 12,
    heap_bytes: 1024,
    transport: { connections: 2, services: 3, rx_bytes: 2048, tx_bytes: 1024 },
    logs: [],
  };
  vi.mocked(loadNode).mockResolvedValue(snapshot);
  render(<App />);
  await act(async () => {
    fireEvent.change(screen.getByLabelText("Monitor Token"), {
      target: { value: "gizclaw_mk_test_only_monitor_token_0000000000" },
    });
    fireEvent.submit(
      screen.getByRole("button", { name: "连接节点" }).closest("form")!,
    );
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(screen.getByTestId("traffic-chart").textContent).toContain(
    "2 samples",
  );
  expect(screen.getByText(/^更新于 /)).toBeTruthy();

  vi.mocked(loadNode).mockRejectedValue(new Error("backend disconnected"));
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(screen.queryByTestId("traffic-chart")).toBeNull();
  expect(screen.queryByText(/^更新于 /)).toBeNull();
  expect(screen.getByRole("alert").textContent).toContain(
    "backend disconnected",
  );
  expect(screen.getByText("等待数据")).toBeTruthy();

  vi.mocked(loadNode).mockResolvedValue(snapshot);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000);
  });
  expect(screen.getByTestId("traffic-chart").textContent).toContain(
    "2 samples",
  );
  expect(screen.queryByRole("alert")).toBeNull();
});
