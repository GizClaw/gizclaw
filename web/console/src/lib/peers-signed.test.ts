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
} from "@/lib/peers";
import { monitorHTTPFixture } from "@/test/monitor-http";

afterEach(() => vi.unstubAllGlobals());

describe("Monitor signed reads with debug disabled", () => {
  const reads = {
    overview: (
      p: ReturnType<typeof monitorHTTPFixture>["peer"],
      s: AbortSignal,
    ) => loadPeer(p.endpoint, p.publicKey, s, p.signedBearer),
    telemetry: (
      p: ReturnType<typeof monitorHTTPFixture>["peer"],
      s: AbortSignal,
    ) => loadTelemetry(p.endpoint, p.publicKey, s, p.signedBearer),
    trends: (
      p: ReturnType<typeof monitorHTTPFixture>["peer"],
      s: AbortSignal,
    ) =>
      loadTelemetryRange(
        p.endpoint,
        p.publicKey,
        "battery.percent",
        1000,
        2000,
        s,
        p.signedBearer,
      ),
    workspaces: (
      p: ReturnType<typeof monitorHTTPFixture>["peer"],
      s: AbortSignal,
    ) => loadWorkspaces(p.endpoint, p.publicKey, s, p.signedBearer),
    history: (
      p: ReturnType<typeof monitorHTTPFixture>["peer"],
      s: AbortSignal,
    ) =>
      loadHistory(
        p.endpoint,
        p.publicKey,
        "signed-workspace",
        { query: "", order: "desc" },
        s,
        p.signedBearer,
      ),
    audio: (p: ReturnType<typeof monitorHTTPFixture>["peer"], s: AbortSignal) =>
      loadHistoryAudio(
        p.endpoint,
        p.publicKey,
        "signed-workspace",
        "signed-entry",
        s,
        p.signedBearer,
      ),
    logs: (p: ReturnType<typeof monitorHTTPFixture>["peer"], s: AbortSignal) =>
      loadDeviceLogs(
        p.endpoint,
        p.publicKey,
        "",
        undefined,
        1000,
        2000,
        undefined,
        s,
        p.signedBearer,
      ),
    wifi: (p: ReturnType<typeof monitorHTTPFixture>["peer"], s: AbortSignal) =>
      loadWifi(p.endpoint, p.publicKey, s, p.signedBearer),
    configuration: (
      p: ReturnType<typeof monitorHTTPFixture>["peer"],
      s: AbortSignal,
    ) => loadDeviceConfig(p.endpoint, p.publicKey, s, p.signedBearer),
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
