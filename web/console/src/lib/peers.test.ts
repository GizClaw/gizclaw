import { GizClawControlError } from "@gizclaw/gizclaw-control";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { loadDeviceConfig, loadPeer, peerGlance } from "@/lib/peers";

const device = {
  get: vi.fn(),
  getRuntime: vi.fn(),
  getStatus: vi.fn(),
  getSettings: vi.fn(),
  listRpcMethods: vi.fn(),
  listTools: vi.fn(),
};
vi.mock("@gizclaw/gizclaw-control", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@gizclaw/gizclaw-control")>()),
  createGizClawPeerMonitorClient: () => device,
}));

const failure = (kind: GizClawControlError["kind"], status: number) =>
  new GizClawControlError(kind, `${kind} failure`, { status });

const tool = {
  name: "usage_limit",
  control_access: "owner",
  i18n: { "zh-CN": { display_name: "使用时长" } },
  input_schema: { type: "object" },
};

describe("device config loader", () => {
  beforeEach(() => {
    for (const method of Object.values(device)) method.mockReset();
  });

  it("returns every section when the device answers", async () => {
    device.getSettings.mockResolvedValue({ nfc_enabled: true });
    device.listRpcMethods.mockResolvedValue({
      methods: ["client.device.settings.get", "client.device.actions.reboot"],
    });
    device.listTools.mockResolvedValue({ items: [tool] });
    const config = await loadDeviceConfig(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(config).toEqual({
      settings: { state: "ok", data: { nfc_enabled: true } },
      rpcMethods: {
        state: "ok",
        data: ["client.device.actions.reboot", "client.device.settings.get"],
      },
      tools: { state: "ok", data: [tool] },
    });
  });

  it("maps each failure independently", async () => {
    device.getSettings.mockRejectedValue(failure("deviceOffline", 409));
    device.listRpcMethods.mockRejectedValue(failure("deviceUnsupported", 501));
    device.listTools.mockResolvedValue({ items: [] });
    const config = await loadDeviceConfig(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(config.settings).toEqual({ state: "offline" });
    expect(config.rpcMethods).toEqual({ state: "unsupported" });
    expect(config.tools).toEqual({ state: "ok", data: [] });
  });

  it("keeps the message of other failures", async () => {
    device.getSettings.mockRejectedValue(failure("deviceTimeout", 504));
    device.listRpcMethods.mockRejectedValue(failure("deviceError", 502));
    device.listTools.mockRejectedValue(failure("forbidden", 403));
    const config = await loadDeviceConfig(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(config.settings).toEqual({
      state: "error",
      message: "设备未在超时时间内响应 · 504 · deviceTimeout failure",
    });
    expect(config.rpcMethods).toEqual({
      state: "error",
      message: "502 · deviceError failure",
    });
    expect(config.tools).toEqual({
      state: "error",
      message: "403 · forbidden failure",
    });
  });

  it("rejects instead of reporting a section once aborted", async () => {
    const controller = new AbortController();
    device.getSettings.mockImplementation(() => {
      controller.abort();
      return Promise.reject(new DOMException("aborted", "AbortError"));
    });
    device.listRpcMethods.mockResolvedValue({ methods: [] });
    device.listTools.mockResolvedValue({ items: [] });
    await expect(
      loadDeviceConfig("https://edge.example.com", "key", controller.signal),
    ).rejects.toThrow("aborted");
  });
});

describe("peer snapshot", () => {
  it("keeps the Workspace members of the runtime", async () => {
    device.get.mockResolvedValue({});
    device.getRuntime.mockResolvedValue({
      online: true,
      last_seen_at: "2026-09-18T00:00:00Z",
      active_workspace_name: "pet",
      pending_workspace_name: "story",
    });
    device.getStatus.mockResolvedValue({});
    const snapshot = await loadPeer(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(snapshot.runtime.active_workspace_name).toBe("pet");
    expect(snapshot.runtime.pending_workspace_name).toBe("story");
  });
});

describe("peer glance", () => {
  it("reads activity and firmware", () => {
    expect(
      peerGlance({
        activity: "chat",
        activity_detail: "与小猫聊天",
        firmware_version: "1.4.2",
        telemetry_observed_at: { activity: "2026-09-18T01:00:00Z" },
      }),
    ).toEqual({
      activity: "chat",
      activityDetail: "与小猫聊天",
      activityObservedAt: "2026-09-18T01:00:00Z",
      firmwareVersion: "1.4.2",
      firmwareSha256: undefined,
      signal: undefined,
    });
  });

  it("picks the more recently observed network route", () => {
    const status = {
      wifi_rssi_dbm: -55,
      cellular_rssi_dbm: -80,
      cellular_signal_level: 3,
      telemetry_observed_at: {
        wifi_rssi_dbm: "2026-09-18T01:00:00Z",
        cellular_rssi_dbm: "2026-09-18T02:00:00Z",
      },
    };
    expect(peerGlance(status).signal).toEqual({
      kind: "cellular",
      rssiDbm: -80,
      level: 3,
      observedAt: "2026-09-18T02:00:00Z",
    });
    status.telemetry_observed_at.wifi_rssi_dbm = "2026-09-18T03:00:00Z";
    expect(peerGlance(status).signal).toMatchObject({
      kind: "wifi",
      rssiDbm: -55,
    });
  });

  it("treats malformed members as absent", () => {
    expect(
      peerGlance({ wifi_rssi_dbm: "strong", firmware_version: 7 }),
    ).toMatchObject({ firmwareVersion: undefined, signal: undefined });
  });
});
