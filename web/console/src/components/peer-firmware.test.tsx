import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PeerFirmware } from "@/components/peer-firmware";
import type { PeerState } from "@/hooks/use-peers";
import {
  adminHTTPFixture,
  monitorFirmware,
  monitorHTTPFixture,
} from "@/test/monitor-http";

function state(
  status: Record<string, unknown> = {},
  runtime: Record<string, unknown> = {},
): PeerState {
  return {
    status: "online",
    samples: [],
    snapshot: {
      info: {},
      status,
      runtime: {
        online: true,
        debug_mode: "off",
        last_seen_at: "",
        ...runtime,
      },
    },
  };
}

async function confirmPush() {
  const push = await screen.findByRole("button", { name: "推送固件" });
  await waitFor(() => expect(push.hasAttribute("disabled")).toBe(false));
  fireEvent.click(push);
  fireEvent.click(screen.getByRole("button", { name: "确认推送" }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("single-device firmware push through the real control SDK", () => {
  it.each(["admin", "signed"])(
    "confirms and sends the selected channel and digest once with %s authorization",
    async (kind) => {
      const updates: unknown[] = [];
      let finish: ((response: Response) => void) | undefined;
      const respond = async (request: Request, path: string) => {
        if (path !== "/tool/v0/invoke") return;
        updates.push(await request.json());
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      };
      const fixture =
        kind === "admin"
          ? adminHTTPFixture(respond)
          : monitorHTTPFixture({ respond });
      vi.stubGlobal("fetch", fixture.fetch);
      const view = render(<PeerFirmware peer={fixture.peer} state={state()} />);
      const channel = await screen.findByRole("combobox", {
        name: "固件 channel",
      });
      expect(
        screen
          .getByRole("option", { name: "develop · 未配置固件" })
          .hasAttribute("disabled"),
      ).toBe(true);
      fireEvent.change(channel, { target: { value: "beta" } });
      fireEvent.click(screen.getByRole("button", { name: "推送固件" }));
      const dialog = screen.getByRole("dialog");
      expect(within(dialog).getByText(fixture.peer.publicKey)).toBeTruthy();
      expect(
        within(dialog).getByText(/channel：beta · 目标版本：1.1.0-beta.1/),
      ).toBeTruthy();
      fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
      expect(updates).toHaveLength(0);
      await confirmPush();
      await waitFor(() => expect(updates).toHaveLength(1));
      expect(updates[0]).toEqual({
        tool: "firmware.update",
        args: { channel: "beta", sha256: "b".repeat(64) },
      });
      const pushing = screen.getByRole("button", { name: "正在推送…" });
      expect(pushing.hasAttribute("disabled")).toBe(true);
      fireEvent.click(pushing);
      expect(updates).toHaveLength(1);
      finish?.(Response.json({ result: {} }));
      await screen.findByText(/设备已接受 beta/);
      expect(screen.queryByText("升级成功")).toBeNull();
      const ota = {
        state: "downloading",
        update_id: "attempt-1",
        observed_at: "2026-10-26T00:00:00Z",
        target_version: "1.1.0-beta.1",
        download_percent: 42,
      };
      view.rerender(
        <PeerFirmware peer={fixture.peer} state={state({ ota })} />,
      );
      expect(screen.getByText("下载中")).toBeTruthy();
      expect(screen.getByRole("progressbar").getAttribute("value")).toBe("42");
      view.rerender(
        <PeerFirmware
          peer={fixture.peer}
          state={state({
            ota: { ...ota, state: "succeeded", download_percent: 100 },
          })}
        />,
      );
      expect(screen.getByText("升级成功")).toBeTruthy();
      expect(fixture.requests).toHaveLength(2);
      expect(
        fixture.requests.every((request) =>
          request.authorization?.startsWith("Bearer gizclaw_sig_v1_"),
        ),
      ).toBe(true);
    },
  );

  it.each([
    [403, "DEBUG_ACCESS_FORBIDDEN"],
    [409, "DEVICE_OFFLINE"],
    [501, "DEVICE_UNSUPPORTED"],
    [400, "DEVICE_REJECTED"],
    [504, "DEVICE_TIMEOUT"],
  ])("shows %s %s without replaying the update", async (status, code) => {
    const fixture = monitorHTTPFixture({
      respond: (_request, path) =>
        path === "/tool/v0/invoke"
          ? Response.json(
              { error: { code, message: "update refused" } },
              { status },
            )
          : undefined,
    });
    vi.stubGlobal("fetch", fixture.fetch);
    render(<PeerFirmware peer={fixture.peer} state={state()} />);
    await confirmPush();
    await screen.findByText(new RegExp(`${status} · ${code}`));
    expect(screen.queryByText(/设备已接受/)).toBeNull();
    expect(
      fixture.requests.filter((request) => request.method === "POST"),
    ).toHaveLength(1);
  });

  it("reads firmware while offline and prevents writes from offline or stale state", async () => {
    const fixture = monitorHTTPFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const view = render(
      <PeerFirmware peer={fixture.peer} state={state({}, { online: false })} />,
    );
    const push = await screen.findByRole("button", { name: "推送固件" });
    expect(push.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("设备在线后才可推送固件。")).toBeTruthy();
    view.rerender(
      <PeerFirmware
        peer={fixture.peer}
        state={{ ...state(), status: "error" }}
      />,
    );
    expect(push.hasAttribute("disabled")).toBe(true);
    expect(
      screen.getByText("设备状态读取失败，恢复读取后可推送。"),
    ).toBeTruthy();
    expect(fixture.requests).toHaveLength(1);
  });

  it("requires fullcontrol only for public-key debug access", async () => {
    const fixture = monitorHTTPFixture({ authorize: () => true });
    const peer = { ...fixture.peer, signedBearer: undefined };
    vi.stubGlobal("fetch", fixture.fetch);
    const view = render(
      <PeerFirmware
        peer={peer}
        state={state({}, { debug_mode: "readonly" })}
      />,
    );
    const push = await screen.findByRole("button", { name: "推送固件" });
    expect(push.hasAttribute("disabled")).toBe(true);
    view.rerender(
      <PeerFirmware
        peer={peer}
        state={state({}, { debug_mode: "fullcontrol" })}
      />,
    );
    await confirmPush();
    await screen.findByText(/设备已接受 stable/);
    expect(
      fixture.requests.every(
        (request) =>
          request.authorization === `Bearer gizclaw_pk_${peer.publicKey}`,
      ),
    ).toBe(true);
  });

  it.each([
    [
      "unbound",
      () =>
        Response.json(
          { error: { code: "FIRMWARE_NOT_FOUND", message: "unbound" } },
          { status: 404 },
        ),
      "该设备未绑定固件配置，或绑定的配置已删除。",
    ],
    [
      "empty",
      () => Response.json({ slots: { stable: {}, beta: {}, develop: {} } }),
      "该设备绑定的固件配置中没有可推送的包。",
    ],
    [
      "malformed",
      () =>
        Response.json({
          slots: {
            ...monitorFirmware.slots,
            stable: {
              package: {
                ...monitorFirmware.slots.stable.package,
                sha256: "invalid",
              },
            },
          },
        }),
      "无法读取固件",
    ],
  ] as const)(
    "does not offer an update for %s firmware",
    async (_name, response, text) => {
      const fixture = monitorHTTPFixture({
        respond: (_request, path) =>
          path === "/firmware" ? response() : undefined,
      });
      vi.stubGlobal("fetch", fixture.fetch);
      render(<PeerFirmware peer={fixture.peer} state={state()} />);
      await screen.findByText(text);
      const push = screen.queryByRole("button", { name: "推送固件" });
      expect(push === null || push.hasAttribute("disabled")).toBe(true);
      expect(fixture.requests).toHaveLength(1);
    },
  );

  it("closes confirmation and reloads firmware when credentials change", async () => {
    const fixture = adminHTTPFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const view = render(<PeerFirmware peer={fixture.peer} state={state()} />);
    fireEvent.click(await screen.findByRole("button", { name: "推送固件" }));
    expect(screen.getByRole("dialog")).toBeTruthy();
    const next = fixture.rotateAdmin();
    view.rerender(<PeerFirmware peer={next} state={state()} />);
    await waitFor(() => expect(fixture.requests).toHaveLength(2));
    await screen.findByRole("combobox");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fixture.requests[0].authorization).not.toBe(
      fixture.requests[1].authorization,
    );
    expect(fixture.requests.every((request) => request.method === "GET")).toBe(
      true,
    );
  });

  it("cancels an in-flight push on unmount without reporting a result or replaying it", async () => {
    let signal: AbortSignal | undefined;
    let finish: ((response: Response) => void) | undefined;
    const fixture = monitorHTTPFixture({
      respond: (request, path) => {
        if (path !== "/tool/v0/invoke") return;
        signal = request.signal;
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      },
    });
    vi.stubGlobal("fetch", fixture.fetch);
    const view = render(<PeerFirmware peer={fixture.peer} state={state()} />);
    await confirmPush();
    await waitFor(() => expect(signal).toBeDefined());
    view.unmount();
    expect(signal?.aborted).toBe(true);
    finish?.(Response.json({ result: {} }));
    expect(fixture.requests).toHaveLength(2);
  });

  it("discards an old device read and confirmation when the target changes", async () => {
    let oldSignal: AbortSignal | undefined;
    let finishOld: ((response: Response) => void) | undefined;
    const first = monitorHTTPFixture({
      respond: (request, path) => {
        if (path !== "/firmware") return;
        oldSignal = request.signal;
        return new Promise<Response>((resolve) => {
          finishOld = resolve;
        });
      },
    });
    const second = monitorHTTPFixture({ publicKey: "second-device" });
    vi.stubGlobal("fetch", first.fetch);
    const view = render(<PeerFirmware peer={first.peer} state={state()} />);
    await waitFor(() => expect(oldSignal).toBeDefined());
    vi.stubGlobal("fetch", second.fetch);
    view.rerender(<PeerFirmware peer={second.peer} state={state()} />);
    await screen.findByRole("combobox");
    expect(oldSignal?.aborted).toBe(true);
    finishOld?.(
      Response.json({ ...monitorFirmware, description: "Old device firmware" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "推送固件" }));
    expect(
      within(screen.getByRole("dialog")).getByText("second-device"),
    ).toBeTruthy();
    expect(screen.queryByText("Old device firmware")).toBeNull();
    expect(first.requests).toHaveLength(1);
    expect(second.requests).toHaveLength(1);
  });

  it("preserves failed and unknown OTA reports and marks stale snapshots", () => {
    const fixture = monitorHTTPFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const ota = {
      state: "failed",
      update_id: "attempt-2",
      observed_at: "2026-10-26T00:00:00Z",
      error_code: "HASH_MISMATCH",
      error_message: "download changed",
    };
    const view = render(
      <PeerFirmware
        peer={fixture.peer}
        state={{ ...state({ ota }), status: "error" }}
      />,
    );
    expect(screen.getByText("升级失败")).toBeTruthy();
    expect(screen.getByText("HASH_MISMATCH · download changed")).toBeTruthy();
    expect(screen.getByText("上次快照")).toBeTruthy();
    view.rerender(
      <PeerFirmware
        peer={fixture.peer}
        state={state({ ota: { ...ota, state: "verifying" } })}
      />,
    );
    expect(screen.getByText("verifying")).toBeTruthy();
  });
});
