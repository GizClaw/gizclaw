import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LogsPage } from "@/pages/logs";

const loadDeviceLogs = vi.fn();
vi.mock("@/lib/peers", () => ({
  loadDeviceLogs: (...args: unknown[]) => loadDeviceLogs(...args),
  peerId: (peer: { publicKey: string }) => peer.publicKey,
  peerLabel: (peer: { publicKey: string }) => peer.publicKey,
}));

describe("persistent log search", () => {
  beforeEach(() => {
    loadDeviceLogs.mockReset();
    loadDeviceLogs.mockResolvedValue({
      items: [],
      end: { has_next: false },
    });
  });

  it("does not offer a node cache when no device is selected", () => {
    render(<LogsPage peers={[]} initialQuery="" />);
    expect(screen.queryByText("节点进程日志")).toBeNull();
    expect(screen.getByText(/请选择设备查询持久化日志/)).toBeTruthy();
    expect(loadDeviceLogs).not.toHaveBeenCalled();
  });
  it("queries the persistent store for the selected device", async () => {
    render(
      <LogsPage
        peers={[
          {
            publicKey: "device-key",
            label: "Device",
            endpoint: "https://edge.example.com",
            addedAt: 1,
          },
        ]}
        initialQuery="timeout"
      />,
    );
    await waitFor(() => expect(loadDeviceLogs).toHaveBeenCalled());
    expect(loadDeviceLogs.mock.calls[0].slice(0, 4)).toEqual([
      "https://edge.example.com",
      "device-key",
      "timeout",
      undefined,
    ]);
    expect(screen.queryByText("节点进程日志")).toBeNull();
  });
});
