import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { URL as NodeURL } from "node:url";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PeerChat } from "@/components/peer-chat";
import { PeerConfig } from "@/components/peer-config";
import { PeerLocation } from "@/components/peer-location";
import { PeerTelemetry } from "@/components/peer-telemetry";
import { LogsPage } from "@/pages/logs";
import { adminHTTPFixture, monitorHTTPFixture } from "@/test/monitor-http";

beforeEach(() => {
  vi.stubGlobal("URL", NodeURL);
  vi.spyOn(NodeURL, "createObjectURL").mockReturnValue(
    "blob:monitor-test-audio",
  );
  vi.spyOn(NodeURL, "revokeObjectURL").mockImplementation(() => {});
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "Audio",
    class extends EventTarget {
      readonly src: string;
      constructor(src: string) {
        super();
        this.src = src;
      }
      play() {
        return Promise.resolve();
      }
      pause() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Monitor pages with a signed credential and debug off", () => {
  it.each([
    { name: "telemetry", View: PeerTelemetry, ready: "50" },
    { name: "location", View: PeerLocation, ready: "51.000000, 12.000000" },
    { name: "history", View: PeerChat, ready: "Signed history" },
    { name: "hardware", View: PeerConfig, ready: "NFC · nfc.left" },
  ])(
    "$name reloads when the configured admin identity changes",
    async ({ View, ready }) => {
      vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
      const f = adminHTTPFixture();
      vi.stubGlobal("fetch", f.fetch);
      const view = render(<View peer={f.peer} />);
      await screen.findAllByText(ready);
      const count = f.requests.length;
      view.rerender(<View peer={f.rotateAdmin()} />);
      await waitFor(() => expect(f.requests.length).toBeGreaterThan(count));
      await screen.findAllByText(ready);
      expect(screen.queryByText(/读取失败/)).toBeNull();
    },
  );

  it("reloads logs with the new admin and exposes denial after removing the identity", async () => {
    const f = adminHTTPFixture();
    vi.stubGlobal("fetch", f.fetch);
    const view = render(<LogsPage peers={[f.peer]} initialQuery="" />);
    await screen.findByText("Signed log");
    const next = f.rotateAdmin();
    view.rerender(<LogsPage peers={[next]} initialQuery="" />);
    await waitFor(() => expect(f.requests).toHaveLength(2));
    await screen.findByText("Signed log");
    view.rerender(
      <LogsPage peers={[{ ...next, identity: undefined }]} initialQuery="" />,
    );
    await screen.findByText(/DEBUG_ACCESS_FORBIDDEN/);
    expect(f.requests.at(-1)?.authorization).toBe(
      `Bearer gizclaw_pk_${next.publicKey}`,
    );
  });

  it("cancels pending reads when the signed credential is replaced", async () => {
    const f = monitorHTTPFixture();
    const previous: AbortSignal[] = [];
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = new Request(input, init);
        if (
          request.headers.get("Authorization") !==
          `Bearer ${f.peer.signedBearer}`
        )
          return f.fetch(request);
        previous.push(request.signal);
        return new Promise<Response>((_resolve, reject) => {
          request.signal.addEventListener(
            "abort",
            () => reject(new DOMException("replaced", "AbortError")),
            { once: true },
          );
        });
      },
    );
    const view = render(<PeerTelemetry peer={f.peer} />);
    await waitFor(() => expect(previous.length).toBeGreaterThan(0));
    view.rerender(<PeerTelemetry peer={f.rotate()} />);
    await waitFor(() =>
      expect(screen.getAllByText("50").length).toBeGreaterThan(0),
    );
    expect(previous.every((signal) => signal.aborted)).toBe(true);
    expect(screen.queryByText("Telemetry 读取失败")).toBeNull();
  });

  it("reads telemetry and trends again when the credential changes", async () => {
    const f = monitorHTTPFixture();
    vi.stubGlobal("fetch", f.fetch);
    const view = render(<PeerTelemetry peer={f.peer} />);
    await waitFor(() =>
      expect(screen.getAllByText("50").length).toBeGreaterThan(0),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "battery.percent 趋势" }),
    );
    await waitFor(() =>
      expect(f.requests.some((r) => r.path === "/telemetry")).toBe(true),
    );
    expect(screen.queryByText("Telemetry 读取失败")).toBeNull();
    const next = f.rotate();
    view.rerender(<PeerTelemetry peer={next} />);
    await waitFor(() =>
      expect(
        f.requests.some(
          (r) =>
            r.path.endsWith("/latest") &&
            r.authorization === `Bearer ${next.signedBearer}`,
        ),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(
        f.requests.some(
          (r) =>
            r.path === "/telemetry" &&
            r.authorization === `Bearer ${next.signedBearer}`,
        ),
      ).toBe(true),
    );
  });

  it("loads GNSS coordinates and renews their signed reads", async () => {
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    const f = monitorHTTPFixture();
    vi.stubGlobal("fetch", f.fetch);
    const view = render(<PeerLocation peer={f.peer} />);
    await screen.findByText("51.000000, 12.000000");
    const next = f.rotate();
    view.rerender(<PeerLocation peer={next} />);
    await waitFor(() =>
      expect(
        f.requests.filter(
          (r) => r.authorization === `Bearer ${next.signedBearer}`,
        ),
      ).toHaveLength(2),
    );
    expect(screen.queryByText("读取定位失败")).toBeNull();
  });

  it("reads Workspaces, retained history and audio with the signed credential", async () => {
    const f = monitorHTTPFixture();
    vi.stubGlobal("fetch", f.fetch);
    const view = render(<PeerChat peer={f.peer} />);
    await screen.findByText("Signed history");
    fireEvent.click(screen.getByRole("button", { name: "播放录音" }));
    await screen.findByRole("button", { name: "暂停录音" });
    expect(
      f.requests.find((r) => r.path.endsWith("/audio.ogg"))?.authorization,
    ).toBe(`Bearer ${f.peer.signedBearer}`);
    const next = f.rotate();
    view.rerender(<PeerChat peer={next} />);
    await waitFor(() =>
      expect(
        f.requests.some(
          (r) =>
            r.path.endsWith("/history") &&
            r.authorization === `Bearer ${next.signedBearer}`,
        ),
      ).toBe(true),
    );
    await screen.findByText("Signed history");
  });

  it("reloads device logs after replacing the signed credential", async () => {
    const f = monitorHTTPFixture();
    vi.stubGlobal("fetch", f.fetch);
    const view = render(
      <LogsPage peers={[f.peer]} initialQuery="peer:signed-device" />,
    );
    await screen.findByText("Signed log");
    const next = f.rotate();
    view.rerender(
      <LogsPage peers={[next]} initialQuery="peer:signed-device" />,
    );
    await waitFor(() =>
      expect(
        f.requests.some(
          (r) =>
            r.path === "/logs/search" &&
            r.authorization === `Bearer ${next.signedBearer}`,
        ),
      ).toBe(true),
    );
    await screen.findByText("Signed log");
  });

  it("keeps hardware and management reads signed after credential replacement", async () => {
    const f = monitorHTTPFixture();
    vi.stubGlobal("fetch", f.fetch);
    const view = render(<PeerConfig peer={f.peer} />);
    await screen.findByText("NFC · nfc.left");
    expect(screen.queryByText("读取失败")).toBeNull();
    const next = f.rotate();
    view.rerender(<PeerConfig peer={next} />);
    await waitFor(() =>
      expect(
        f.requests.filter(
          (r) => r.authorization === `Bearer ${next.signedBearer}`,
        ).length,
      ).toBe(6),
    );
    expect(screen.queryByText("读取失败")).toBeNull();
  });
});
