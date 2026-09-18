import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PeerConfig } from "@/components/peer-config";

const loadDeviceConfig = vi.fn();
vi.mock("@/lib/peers", () => ({
  loadDeviceConfig: (...args: unknown[]) => loadDeviceConfig(...args),
}));

const peer = {
  publicKey: "device-key",
  label: "Device",
  endpoint: "https://edge.example.com",
  addedAt: 1,
};

afterEach(cleanup);

describe("device config tab", () => {
  beforeEach(() => {
    loadDeviceConfig.mockReset();
  });

  it("labels settings and marks absent members unsupported", async () => {
    loadDeviceConfig.mockResolvedValue({
      settings: {
        state: "ok",
        data: {
          screen_brightness: 60,
          screen_off_timeout_ms: 0,
          key_feedback: "sound_and_vibrate",
          nfc_enabled: false,
        },
      },
      rpcMethods: { state: "ok", data: ["client.device.settings.get"] },
      tools: {
        state: "ok",
        data: [
          {
            name: "usage_limit",
            control_access: "owner",
            i18n: {
              en: { display_name: "Usage limit" },
              "zh-CN": { display_name: "使用时长", description: "每日上限" },
            },
            input_schema: {
              type: "object",
              required: ["minutes"],
              properties: {
                minutes: { type: "integer" },
                enabled: { type: "boolean" },
              },
            },
          },
          {
            name: "night_mode",
            control_access: "owner",
            i18n: { en: { display_name: "Night mode" } },
            input_schema: {},
          },
        ],
      },
    });
    render(<PeerConfig peer={peer} />);
    await screen.findByText("60%");
    expect(screen.getByText("常亮")).toBeTruthy();
    expect(screen.getByText("声音和振动")).toBeTruthy();
    expect(screen.getByText("关闭")).toBeTruthy();
    // Six of the ten contract members are absent.
    expect(screen.getAllByText("不支持")).toHaveLength(6);
    expect(screen.getByText("使用时长")).toBeTruthy();
    expect(screen.getByText("每日上限")).toBeTruthy();
    expect(
      screen.getByText("参数 minutes: integer, enabled?: boolean"),
    ).toBeTruthy();
    expect(screen.getByText("Night mode")).toBeTruthy();
    expect(screen.getByText("参数 无参数")).toBeTruthy();
    expect(screen.getAllByText("权限 owner")).toHaveLength(2);
    expect(screen.getByText("client.device.settings.get")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /恢复出厂|调用|保存/ }),
    ).toBeNull();
  });

  it("shows each failed section on its own", async () => {
    loadDeviceConfig.mockResolvedValue({
      settings: { state: "offline" },
      rpcMethods: { state: "unsupported" },
      tools: { state: "error", message: "403 · forbidden" },
    });
    render(<PeerConfig peer={peer} />);
    await screen.findByText("设备离线");
    expect(screen.getByText("设备不支持")).toBeTruthy();
    expect(screen.getByText("读取失败")).toBeTruthy();
    expect(screen.getByText("403 · forbidden")).toBeTruthy();
  });

  it("reads again on refresh", async () => {
    loadDeviceConfig.mockResolvedValue({
      settings: { state: "ok", data: {} },
      rpcMethods: { state: "ok", data: [] },
      tools: { state: "ok", data: [] },
    });
    render(<PeerConfig peer={peer} />);
    const refresh = await screen.findByRole("button", { name: /刷新/ });
    expect(loadDeviceConfig).toHaveBeenCalledTimes(1);
    expect(loadDeviceConfig.mock.calls[0].slice(0, 2)).toEqual([
      "https://edge.example.com",
      "device-key",
    ]);
    fireEvent.click(refresh);
    await waitFor(() => expect(loadDeviceConfig).toHaveBeenCalledTimes(2));
  });
});
