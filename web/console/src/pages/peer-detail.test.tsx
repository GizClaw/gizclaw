import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PeerDetailPage } from "@/pages/peer-detail";
import type { PeerState } from "@/hooks/use-peers";

vi.mock("@/components/peer-chat", () => ({ PeerChat: () => null }));
vi.mock("@/components/peer-telemetry", () => ({ PeerTelemetry: () => null }));
vi.mock("@/components/peer-location", () => ({ PeerLocation: () => null }));
vi.mock("@/components/traffic-chart", () => ({
  TrafficChart: () => null,
  TrafficLegend: () => null,
}));
vi.mock("@/components/peer-config", () => ({
  PeerConfig: () => <p>config tab</p>,
}));

const peer = {
  publicKey: "device-key",
  label: "Device",
  endpoint: "https://edge.example.com",
  addedAt: 1,
};

const state = (
  status: Record<string, unknown>,
  runtime: Record<string, unknown> = {},
): PeerState => ({
  status: "online",
  samples: [],
  snapshot: {
    info: {},
    runtime: {
      online: true,
      last_seen_at: "2026-09-18T00:00:00Z",
      ...runtime,
    },
    status,
  },
});

afterEach(cleanup);

describe("peer detail at a glance", () => {
  it("shows activity, firmware, signal and Workspace", () => {
    render(
      <PeerDetailPage
        peer={peer}
        windowSeconds={120}
        state={state(
          {
            activity: "chat",
            activity_detail: "与小猫聊天",
            firmware_version: "1.4.2",
            firmware_sha256: "a".repeat(64),
            wifi_rssi_dbm: -55,
          },
          { active_workspace_name: "pet", pending_workspace_name: "story" },
        )}
      />,
    );
    expect(screen.getByText("对话")).toBeTruthy();
    expect(screen.getByText("与小猫聊天")).toBeTruthy();
    expect(screen.getByText("1.4.2")).toBeTruthy();
    expect(screen.getByText(`SHA-256 ${"a".repeat(12)}…`)).toBeTruthy();
    expect(screen.getByText("-55 dBm")).toBeTruthy();
    expect(screen.getByText(/^Wi-Fi · /)).toBeTruthy();
    expect(screen.getByText("pet")).toBeTruthy();
    expect(screen.getByText("切换中 → story")).toBeTruthy();
  });

  it("falls back to cellular and keeps unknown activities verbatim", () => {
    render(
      <PeerDetailPage
        peer={peer}
        windowSeconds={120}
        state={state({
          activity: "camera",
          cellular_rssi_dbm: -85,
          cellular_signal_level: 2,
        })}
      />,
    );
    expect(screen.getByText("camera")).toBeTruthy();
    expect(screen.getByText("-85 dBm")).toBeTruthy();
    expect(screen.getByText(/^蜂窝 · 等级 2 · /)).toBeTruthy();
    expect(screen.getByText("尚未提交过 Workspace")).toBeTruthy();
  });

  it("opens the device config tab lazily", () => {
    render(
      <PeerDetailPage peer={peer} windowSeconds={120} state={state({})} />,
    );
    expect(screen.queryByText("config tab")).toBeNull();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "设备配置" }), {
      button: 0,
    });
    expect(screen.getByText("config tab")).toBeTruthy();
  });
});
