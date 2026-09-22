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
  beforeEach(() => loadDeviceConfig.mockReset());

  it("shows manifest states and only installed procedures", async () => {
    loadDeviceConfig.mockResolvedValue({
      manifest: {
        state: "ok",
        data: {
          devices: [
            {
              id: "speaker",
              kind: "audio",
              states: [{ name: "volume", type: "int", access: "read_write" }],
            },
          ],
        },
      },
      tools: { state: "ok", data: ["device.find", "sound.play"] },
    });
    render(<PeerConfig peer={peer} />);
    await screen.findByText("device.find");
    expect(screen.getByText("volume: int · read_write")).toBeTruthy();
    expect(screen.getByText("sound.play")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /恢复出厂|调用|保存/ }),
    ).toBeNull();
  });

  it("shows each failed section on its own", async () => {
    loadDeviceConfig.mockResolvedValue({
      manifest: { state: "offline" },
      tools: { state: "error", message: "403 · forbidden" },
    });
    render(<PeerConfig peer={peer} />);
    await screen.findByText("设备离线");
    expect(screen.getByText("读取失败")).toBeTruthy();
    expect(screen.getByText("403 · forbidden")).toBeTruthy();
  });

  it("reads again on refresh", async () => {
    loadDeviceConfig.mockResolvedValue({
      manifest: { state: "ok", data: { devices: [] } },
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
