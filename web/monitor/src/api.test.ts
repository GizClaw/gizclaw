// @vitest-environment jsdom
import { expect, test, vi } from "vitest";
import { loadPeer, monitorTelemetryFields } from "./api";

const fixture = vi.hoisted(() => ({
  active: 0,
  maximum: 0,
  fields: [] as string[],
}));
vi.mock("@gizclaw/gizclaw-control", () => ({
  GizClawControlError: class extends Error {},
  createGizClawDiscoveryClient: vi.fn(),
  createGizClawNodeMonitorClient: vi.fn(),
  createGizClawPeerMonitorClient: () => ({
    get: async () => ({}),
    getRuntime: async () => ({
      online: true,
      last_seen_at: "2026-09-06T00:00:00Z",
    }),
    getStatus: async () => ({ volume: 10 }),
    getTelemetryLatest: async (field: string) => {
      fixture.fields.push(field);
      fixture.active++;
      fixture.maximum = Math.max(fixture.maximum, fixture.active);
      await Promise.resolve();
      fixture.active--;
      return {
        peer_public_key: "pk",
        values: [{ field, value: 1, observed_at_unix_ms: 1000 }],
      };
    },
  }),
}));

test("monitor explicitly queries each displayed field with bounded concurrency", async () => {
  const peer = await loadPeer("pk", new AbortController().signal);
  expect(fixture.fields).toEqual(monitorTelemetryFields);
  expect(fixture.maximum).toBeLessThanOrEqual(4);
  expect(peer.telemetry.values).toHaveLength(monitorTelemetryFields.length);
  expect(peer.status.volume).toBe(10);
});
