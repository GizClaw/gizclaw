// @vitest-environment jsdom
import React from "react";
import {
  render,
  screen,
  cleanup,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { TelemetryPanel, LocationPanel, WorkflowsPanel } from "./device-panels";
import { loadWorkspaces, loadHistory, type PeerSnapshot } from "./api";
vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  loadWorkspaces: vi.fn(),
  loadHistory: vi.fn(),
}));
vi.mock("recharts", () => ({
  ResponsiveContainer: () => null,
  AreaChart: () => null,
  Area: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  CartesianGrid: () => null,
}));
afterEach(cleanup);
function peer(values: unknown[]): PeerSnapshot {
  return {
    info: {},
    runtime: { online: true, last_seen_at: "2026-09-05T00:00:00Z" },
    status: {},
    logs: [],
    telemetry: { values },
  };
}
test("telemetry renders zero as measured data and missing fields distinctly", () => {
  render(
    <TelemetryPanel
      peer={peer([
        { field: "battery.percent", value: 0, observed_at_unix_ms: Date.now() },
      ])}
    />,
  );
  expect(screen.getByRole("button", { name: /电量\s*0 %/ })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: /电池电压\s*未上报/ }),
  ).toBeTruthy();
  expect(
    screen.getByText("等待至少两次设备采样，未生成示例曲线。"),
  ).toBeTruthy();
});
test("location does not invent a marker for absent or invalid coordinates", () => {
  const result = render(<LocationPanel peer={peer([])} />);
  expect(screen.queryByTitle("设备位置地图")).toBeNull();
  result.rerender(
    <LocationPanel
      peer={peer([
        { field: "gnss.latitude", value: 91, observed_at_unix_ms: 1 },
        { field: "gnss.longitude", value: 0, observed_at_unix_ms: 1 },
      ])}
    />,
  );
  expect(screen.queryByTitle("设备位置地图")).toBeNull();
  result.rerender(
    <LocationPanel
      peer={peer([
        { field: "gnss.latitude", value: 0, observed_at_unix_ms: 1 },
        { field: "gnss.longitude", value: 0, observed_at_unix_ms: 1 },
      ])}
    />,
  );
  expect(screen.getByTitle("设备位置地图").getAttribute("src")).toContain(
    "marker=0,0",
  );
});

test("workflows select owned workspace history and search on the server", async () => {
  vi.mocked(loadWorkspaces).mockResolvedValue([
    {
      id: "a",
      name: "Conversation A",
      workflow_id: "flow-a",
      last_active_at: "2026-09-05T00:00:00Z",
    },
    {
      id: "b",
      name: "Conversation B",
      workflow_id: "flow-b",
      last_active_at: "2026-09-05T00:00:00Z",
    },
  ]);
  vi.mocked(loadHistory).mockImplementation(async (_key, id, query) => ({
    available: true,
    items: [
      {
        name: "entry",
        actor_name: "Device",
        type: "gear",
        text: `${id}:${query || "hello"}`,
        created_at: "2026-09-05T00:00:00Z",
        replay_available: false,
      },
    ],
    has_next: false,
  }));
  render(<WorkflowsPanel credential="peer-a" />);
  await screen.findByText("a:hello");
  fireEvent.change(screen.getByRole("combobox", { name: "Workflow" }), {
    target: { value: "flow-b" },
  });
  await screen.findByText("b:hello");
  expect(screen.queryByText("a:hello")).toBeNull();
  fireEvent.change(screen.getByRole("textbox", { name: "搜索历史聊天" }), {
    target: { value: "orange" },
  });
  fireEvent.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() => expect(screen.getByText("b:orange")).toBeTruthy());
  expect(vi.mocked(loadHistory).mock.calls.at(-1)?.slice(0, 4)).toEqual([
    "peer-a",
    "b",
    "orange",
    undefined,
  ]);
});
