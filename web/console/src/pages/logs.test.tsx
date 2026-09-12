import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LogsPage } from "@/pages/logs";

const loadDeviceLogs = vi.fn();
vi.mock("@/lib/peers", () => ({
  loadDeviceLogs: (...args: unknown[]) => loadDeviceLogs(...args),
  peerId: (peer: { publicKey: string }) => peer.publicKey,
  peerLabel: (peer: { publicKey: string }) => peer.publicKey,
}));

afterEach(cleanup);

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

  it("opens the device named by the initial query", async () => {
    render(
      <LogsPage
        peers={[
          {
            publicKey: "first-key",
            label: "First",
            endpoint: "https://a.example.com",
            addedAt: 1,
          },
          {
            publicKey: "SecondKey",
            label: "Second",
            endpoint: "https://b.example.com",
            addedAt: 2,
          },
        ]}
        initialQuery="peer_public_key:SecondKey error_code:ASR_TIMEOUT"
      />,
    );
    await waitFor(() => expect(loadDeviceLogs).toHaveBeenCalled());
    expect(loadDeviceLogs.mock.calls[0].slice(0, 2)).toEqual([
      "https://b.example.com",
      "SecondKey",
    ]);
  });

  it("matches the named public key exactly before ignoring case", async () => {
    const peer = (publicKey: string, endpoint: string) => ({
      publicKey,
      label: publicKey,
      endpoint,
      addedAt: 1,
    });
    const peers = [
      peer("AbcKey", "https://upper.example.com"),
      peer("abckey", "https://lower.example.com"),
    ];
    const view = render(
      <LogsPage peers={peers} initialQuery="peer_public_key:abckey" />,
    );
    await waitFor(() => expect(loadDeviceLogs).toHaveBeenCalled());
    expect(loadDeviceLogs.mock.calls.at(-1)?.slice(0, 2)).toEqual([
      "https://lower.example.com",
      "abckey",
    ]);
    view.unmount();

    loadDeviceLogs.mockClear();
    render(
      <LogsPage
        peers={[peers[0]]}
        initialQuery="-peer_public_key:x peer:ABCKEY"
      />,
    );
    await waitFor(() => expect(loadDeviceLogs).toHaveBeenCalled());
    expect(loadDeviceLogs.mock.calls.at(-1)?.[1]).toBe("AbcKey");
  });

  it("opens the named device once the watch list catches up", async () => {
    const first = {
      publicKey: "first-key",
      label: "First",
      endpoint: "https://a.example.com",
      addedAt: 1,
    };
    const second = {
      publicKey: "SecondKey",
      label: "Second",
      endpoint: "https://b.example.com",
      addedAt: 2,
    };
    const query = "peer_public_key:SecondKey";
    const view = render(<LogsPage peers={[]} initialQuery={query} />);
    expect(loadDeviceLogs).not.toHaveBeenCalled();

    // The assistant added the device to the watch list after navigating.
    view.rerender(<LogsPage peers={[first, second]} initialQuery={query} />);
    await waitFor(() =>
      expect(loadDeviceLogs.mock.calls.at(-1)?.[1]).toBe("SecondKey"),
    );
    expect(
      loadDeviceLogs.mock.calls.every((call) => call[1] === "SecondKey"),
    ).toBe(true);

    // A device the user picks stays selected as the list changes again.
    fireEvent.change(screen.getByLabelText("数据源"), {
      target: { value: "first-key" },
    });
    await waitFor(() =>
      expect(loadDeviceLogs.mock.calls.at(-1)?.[1]).toBe("first-key"),
    );
    view.rerender(
      <LogsPage
        peers={[first, second, { ...second, publicKey: "third" }]}
        initialQuery={query}
      />,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(loadDeviceLogs.mock.calls.at(-1)?.[1]).toBe("first-key");
  });
});

it("excludes records older than 24 hours from the selected window", async () => {
  const now = Date.now();
  loadDeviceLogs.mockResolvedValue({
    items: [
      {
        time_ms: now - 2 * 3600000,
        level: "INFO",
        message: "recent record",
        fields: {},
      },
      {
        time_ms: now - 25 * 3600000,
        level: "INFO",
        message: "expired record",
        fields: {},
      },
    ],
    end: { has_next: false },
  });
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
      initialQuery=""
    />,
  );
  fireEvent.change(screen.getByLabelText("时间范围"), {
    target: { value: "86400" },
  });
  await waitFor(() => expect(screen.getByText("recent record")).toBeTruthy());
  expect(screen.queryByText("expired record")).toBeNull();
  const args = loadDeviceLogs.mock.calls.at(-1)!;
  expect(args[5] - args[4]).toBe(86400000);
});
