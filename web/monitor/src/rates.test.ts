import { GizClawControlError } from "@gizclaw/gizclaw-control";
import { describe, it, expect, vi } from "vitest";
vi.stubGlobal("window", { location: { origin: "http://localhost" } });
const { rates, nodeSchema, monitorErrorMessage } = await import("./api");
describe("monitor data boundaries", () => {
  it("calculates byte rates and resets on reconnect", () => {
    expect(
      rates({ time: 1000, rx: 100, tx: 200 }, { time: 3000, rx: 300, tx: 800 }),
    ).toMatchObject({ rx: 100, tx: 300 });
    expect(
      rates({ time: 1000, rx: 100, tx: 200 }, { time: 2000, rx: 1, tx: 2 }),
    ).toMatchObject({ rx: 0, tx: 0 });
  });
  it("rejects malformed snapshots instead of showing invented values", () => {
    expect(nodeSchema.safeParse({ role: "server" }).success).toBe(false);
  });
});

it("preserves HTTP status and server error codes in monitor failures", () => {
  const error = GizClawControlError.fromResult(
    "searchDeviceLogs",
    new Response(null, { status: 403 }),
    {
      error: { code: "API_KEY_OWNER_UNAVAILABLE", message: "Forbidden" },
    },
  );
  expect(monitorErrorMessage(error)).toBe(
    "403 · API_KEY_OWNER_UNAVAILABLE · searchDeviceLogs: Forbidden",
  );
  expect(monitorErrorMessage(new Error("offline"))).toBe("offline");
});
