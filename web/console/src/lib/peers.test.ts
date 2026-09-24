import { GizClawControlError } from "@gizclaw/gizclaw-control";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { loadDeviceConfig, loadPeer, loadWifi, peerGlance } from "@/lib/peers";

const device = {
  get: vi.fn(),
  getRuntime: vi.fn(),
  getStatus: vi.fn(),
  getMhsManifest: vi.fn(),
  readMhsStates: vi.fn(),
  listSavedWifi: vi.fn(),
  listTools: vi.fn(),
};
vi.mock("@gizclaw/gizclaw-control", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@gizclaw/gizclaw-control")>()),
  createGizClawPeerMonitorClient: () => device,
}));

const failure = (kind: GizClawControlError["kind"], status: number) =>
  new GizClawControlError(kind, `${kind} failure`, { status });

const manifest = {
  devices: [
    {
      id: "speaker",
      kind: "audio",
      states: [{ name: "volume", type: "int", access: "read_write" }],
    },
  ],
};

describe("device config loader", () => {
  beforeEach(() => {
    for (const method of Object.values(device)) method.mockReset();
  });

  it("returns every section when the device answers", async () => {
    device.getMhsManifest.mockResolvedValue(manifest);
    device.listTools.mockResolvedValue({
      tools: ["sound.play", "device.find"],
    });
    const config = await loadDeviceConfig(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(config).toEqual({
      manifest: { state: "ok", data: manifest },
      tools: { state: "ok", data: ["device.find", "sound.play"] },
    });
  });

  it("maps each failure independently", async () => {
    device.getMhsManifest.mockRejectedValue(failure("deviceOffline", 409));
    device.listTools.mockResolvedValue({ tools: [] });
    const config = await loadDeviceConfig(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(config.manifest).toEqual({ state: "offline" });
    expect(config.tools).toEqual({ state: "ok", data: [] });
  });

  it("keeps the message of other failures", async () => {
    device.getMhsManifest.mockRejectedValue(failure("deviceTimeout", 504));
    device.listTools.mockRejectedValue(failure("forbidden", 403));
    const config = await loadDeviceConfig(
      "https://edge.example.com",
      "key",
      new AbortController().signal,
    );
    expect(config.manifest).toEqual({
      state: "error",
      message: "设备未在超时时间内响应 · 504 · deviceTimeout failure",
    });
    expect(config.tools).toEqual({
      state: "error",
      message: "403 · forbidden failure",
    });
  });

  it("rejects instead of reporting a section once aborted", async () => {
    const controller = new AbortController();
    device.getMhsManifest.mockImplementation(() => {
      controller.abort();
      return Promise.reject(new DOMException("aborted", "AbortError"));
    });
    device.listTools.mockResolvedValue({ tools: [] });
    await expect(
      loadDeviceConfig("https://edge.example.com", "key", controller.signal),
    ).rejects.toThrow("aborted");
  });
});

describe("Wi-Fi diagnostics", () => {
  it("reads the profile's Wi-Fi states and saved networks", async () => {
    device.getMhsManifest.mockResolvedValue({
      devices: [
        {
          id: "wifi.main",
          kind: "wifi",
          states: [
            { name: "connected", type: "bool", access: "read" },
            { name: "ssid", type: "string", access: "read" },
            { name: "rssi-dbm", type: "int", access: "read" },
            { name: "secret", type: "string", access: "write" },
          ],
        },
      ],
    });
    device.readMhsStates.mockResolvedValue({
      states: [
        { device_id: "wifi.main", state: "connected", value: true },
        { device_id: "wifi.main", state: "ssid", value: "Home" },
        { device_id: "wifi.main", state: "rssi-dbm", value: -61 },
      ],
    });
    device.listSavedWifi.mockResolvedValue({ networks: [{ ssid: "Home" }] });
    expect(
      await loadWifi(
        "https://edge.example.com",
        "key",
        new AbortController().signal,
      ),
    ).toEqual({
      status: { connected: true, ssid: "Home", rssi_dbm: -61 },
      saved: ["Home"],
    });
    expect(device.readMhsStates).toHaveBeenCalledWith({
      states: [
        { device_id: "wifi.main", state: "connected" },
        { device_id: "wifi.main", state: "ssid" },
        { device_id: "wifi.main", state: "rssi-dbm" },
      ],
    });
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
