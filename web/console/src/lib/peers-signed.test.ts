import { peerAuth } from "@/lib/peer-auth";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  loadDeviceConfig,
  loadDeviceLogs,
  loadHistory,
  loadHistoryAudio,
  loadPeer,
  loadTelemetry,
  loadTelemetryRange,
  loadWifi,
  loadWorkspaces,
  type WatchedPeer,
} from "@/lib/peers";
import { adminHTTPFixture, monitorHTTPFixture } from "@/test/monitor-http";

afterEach(() => vi.unstubAllGlobals());

describe("Monitor signed reads with debug disabled", () => {
  const reads = {
    overview: (p: WatchedPeer, s: AbortSignal) =>
      loadPeer(p.endpoint, p.publicKey, s, peerAuth(p)),
    telemetry: (p: WatchedPeer, s: AbortSignal) =>
      loadTelemetry(p.endpoint, p.publicKey, s, peerAuth(p)),
    trends: (p: WatchedPeer, s: AbortSignal) =>
      loadTelemetryRange(
        p.endpoint,
        p.publicKey,
        "battery.percent",
        1000,
        2000,
        s,
        peerAuth(p),
      ),
    workspaces: (p: WatchedPeer, s: AbortSignal) =>
      loadWorkspaces(p.endpoint, p.publicKey, s, peerAuth(p)),
    history: (p: WatchedPeer, s: AbortSignal) =>
      loadHistory(
        p.endpoint,
        p.publicKey,
        "signed-workspace",
        { query: "", order: "desc" },
        s,
        peerAuth(p),
      ),
    audio: (p: WatchedPeer, s: AbortSignal) =>
      loadHistoryAudio(
        p.endpoint,
        p.publicKey,
        "signed-workspace",
        "signed-entry",
        s,
        peerAuth(p),
      ),
    logs: (p: WatchedPeer, s: AbortSignal) =>
      loadDeviceLogs(
        p.endpoint,
        p.publicKey,
        "",
        undefined,
        1000,
        2000,
        undefined,
        s,
        peerAuth(p),
      ),
    wifi: (p: WatchedPeer, s: AbortSignal) =>
      loadWifi(p.endpoint, p.publicKey, s, peerAuth(p)),
    configuration: (p: WatchedPeer, s: AbortSignal) =>
      loadDeviceConfig(p.endpoint, p.publicKey, s, peerAuth(p)),
  };
  it.each(Object.entries(reads))(
    "%s uses the signed credential on every HTTP request",
    async (_name, read) => {
      const fixture = monitorHTTPFixture();
      vi.stubGlobal("fetch", fixture.fetch);
      const result = await read(fixture.peer, new AbortController().signal);
      expect(result).toBeDefined();
      expect(fixture.requests.length).toBeGreaterThan(0);
      expect(
        fixture.requests.every(
          (r) => r.authorization === `Bearer ${fixture.peer.signedBearer}`,
        ),
      ).toBe(true);
      if (_name === "configuration")
        expect(result).toMatchObject({
          manifest: { state: "ok" },
          tools: { state: "ok" },
          queries: expect.arrayContaining([
            expect.objectContaining({
              result: expect.objectContaining({ state: "ok" }),
            }),
          ]),
        });
    },
  );
  it.each(Object.entries(reads))(
    "%s signs every request with the shared admin identity",
    async (_name, read) => {
      const fixture = adminHTTPFixture();
      vi.stubGlobal("fetch", fixture.fetch);
      const result = await read(fixture.peer, new AbortController().signal);
      expect(result).toBeDefined();
      expect(fixture.requests.length).toBeGreaterThan(0);
      expect(
        fixture.requests.every(
          (r) =>
            r.authorization?.startsWith("Bearer gizclaw_sig_v1_") &&
            !r.authorization.includes(fixture.peer.identity.privateKey),
        ),
      ).toBe(true);
      if (_name === "configuration")
        expect(result).toMatchObject({
          manifest: { state: "ok" },
          tools: { state: "ok" },
          queries: expect.arrayContaining([
            expect.objectContaining({
              result: expect.objectContaining({ state: "ok" }),
            }),
          ]),
        });
    },
  );

  it("rejects public-key access while debug is off", async () => {
    const fixture = monitorHTTPFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    await expect(
      loadTelemetry(
        fixture.peer.endpoint,
        fixture.peer.publicKey,
        new AbortController().signal,
      ),
    ).rejects.toMatchObject({ status: 403, code: "DEBUG_ACCESS_FORBIDDEN" });
  });
  it("uses a replacement credential and propagates cancellation", async () => {
    const fixture = monitorHTTPFixture();
    vi.stubGlobal("fetch", fixture.fetch);
    const signal = new AbortController().signal;
    await reads.telemetry(fixture.peer, signal);
    const next = fixture.rotate();
    await reads.telemetry(next, signal);
    expect(fixture.requests.at(-1)?.authorization).toBe(
      `Bearer ${next.signedBearer}`,
    );
    const aborted = new AbortController();
    aborted.abort();
    await expect(reads.telemetry(next, aborted.signal)).rejects.toThrow();
  });
});
