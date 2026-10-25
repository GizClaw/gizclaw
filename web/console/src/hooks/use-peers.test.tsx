import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useWatchedPeers } from "@/hooks/use-peers";
import { peerId } from "@/lib/peers";
import type { GizClawPeerMonitorIdentity } from "@gizclaw/gizclaw-control";
import { adminHTTPFixture } from "@/test/monitor-http";

const storage = vi.hoisted(() => ({ readPeers: vi.fn(), savePeers: vi.fn() }));
vi.mock("@/lib/store", () => storage);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

it("cancels old admin reads on replacement/removal and keeps the private key out of the watch list", async () => {
  const f = adminHTTPFixture();
  const raw = { ...f.peer, identity: undefined };
  storage.readPeers.mockResolvedValue(JSON.stringify([raw]));
  storage.savePeers.mockResolvedValue(undefined);
  const old: AbortSignal[] = [];
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      if (f.requests.length === 0 && old.length < 3) {
        old.push(request.signal);
        return new Promise<Response>((_resolve, reject) => {
          request.signal.addEventListener(
            "abort",
            () => reject(new DOMException("replaced", "AbortError")),
            { once: true },
          );
        });
      }
      return f.fetch(request);
    },
  );
  const { result, rerender } = renderHook(
    ({ identity }: { identity: GizClawPeerMonitorIdentity | undefined }) =>
      useWatchedPeers(60_000, false, identity),
    {
      initialProps: {
        identity: f.peer.identity as GizClawPeerMonitorIdentity | undefined,
      },
    },
  );
  await waitFor(() => expect(old).toHaveLength(3));
  const next = f.rotateAdmin();
  rerender({ identity: next.identity });
  await waitFor(() =>
    expect(result.current.states[peerId(raw)]?.status).toBe("online"),
  );
  expect(old.every((signal) => signal.aborted)).toBe(true);
  expect(result.current.peers[0].identity).toBeUndefined();
  act(() => result.current.add({ ...raw, label: "renamed" }));
  await waitFor(() => expect(storage.savePeers).toHaveBeenCalled());
  expect(storage.savePeers.mock.calls.at(-1)?.[0]).not.toContain("privateKey");
  rerender({ identity: undefined });
  await waitFor(() =>
    expect(result.current.states[peerId(raw)]?.status).toBe("error"),
  );
  expect(f.requests.at(-1)?.authorization).toBe(
    `Bearer gizclaw_pk_${raw.publicKey}`,
  );
});
