import assert from "node:assert/strict";
import test from "node:test";
import {
  createGizClawPeerMonitorClient,
  createGizClawNodeMonitorClient,
  createGizClawDiscoveryClient,
  GizClawControlError,
} from "./index.ts";

test("monitor carries public-key Bearer, query boundaries across every peer read", async () => {
  const controller = new AbortController();
  const paths: string[] = [];
  const peer = createGizClawPeerMonitorClient({
    baseUrl: "https://monitor.example",
    publicKey: "test-peer",
    signal: controller.signal,
    fetch: async (input) => {
      const request = new Request(input);
      assert.equal(
        request.headers.get("Authorization"),
        "Bearer gizclaw_pk_test-peer",
      );
      assert.equal(request.signal.aborted, false);
      const url = new URL(request.url);
      paths.push(url.pathname);
      assert.equal(url.searchParams.has("public_key"), false);
      if (url.pathname.endsWith("audio.ogg"))
        return new Response(new Blob(["OggS"], { type: "audio/ogg" }));
      if (url.pathname.endsWith("/history")) {
        assert.equal(url.searchParams.get("query"), "hello & world");
        assert.equal(url.searchParams.get("cursor"), "boundary");
      }
      return Response.json({});
    },
  });
  await peer.listWorkspaces();
  await peer.listWorkspaceHistory("workspace", {
    query: "hello & world",
    cursor: "boundary",
    limit: 1,
  });
  await peer.searchLogs({
    start_time_ms: 1000,
    end_time_ms: 2000,
    level: "ERROR",
  });
  assert.equal(
    await (await peer.downloadHistoryAudio("workspace", "entry")).text(),
    "OggS",
  );
  assert.deepEqual(paths, [
    "/gizclaw/v1/device/workspaces",
    "/gizclaw/v1/device/workspaces/workspace/history",
    "/gizclaw/v1/device/logs/search",
    "/gizclaw/v1/device/workspaces/workspace/history/entry/audio.ogg",
  ]);
});

test("node token and unauthenticated discovery remain distinct", async () => {
  const node = createGizClawNodeMonitorClient({
    baseUrl: "https://monitor.example",
    token: "gizclaw_mk_test",
    fetch: async (input) => {
      const request = new Request(input);
      assert.equal(
        request.headers.get("Authorization"),
        "Bearer gizclaw_mk_test",
      );
      assert.equal(new URL(request.url).pathname, "/monitor/api/node");
      return Response.json({ error: "unauthorized" }, { status: 401 });
    },
  });
  await assert.rejects(
    node.get(),
    (err: unknown) => err instanceof GizClawControlError && err.status === 401,
  );
  const discovery = createGizClawDiscoveryClient({
    baseUrl: "https://monitor.example",
    fetch: async (input) => {
      assert.equal(new Request(input).headers.has("Authorization"), false);
      return Response.json({ public_keys: ["first", "second"] });
    },
  });
  assert.deepEqual(await discovery.findBySn("duplicate"), ["first", "second"]);
});

test("monitor requests follow client cancellation", async () => {
  for (const kind of ["peer", "node", "discovery"]) {
    const controller = new AbortController();
    let observed: AbortSignal | undefined;
    const options = {
      baseUrl: "https://monitor.example",
      signal: controller.signal,
      fetch: async (input: RequestInfo | URL) => {
        observed = new Request(input).signal;
        return Response.json({ public_keys: [] });
      },
    };
    if (kind === "peer") {
      await createGizClawPeerMonitorClient({
        ...options,
        publicKey: "peer",
      }).listWorkspaces();
    } else if (kind === "node") {
      await createGizClawNodeMonitorClient({ ...options, token: "node" }).get();
    } else {
      await createGizClawDiscoveryClient(options).findBySn("sn");
    }
    assert.ok(observed);
    assert.equal(observed.aborted, false);
    controller.abort();
    assert.equal(observed.aborted, true, `${kind} must propagate abort`);
  }
});
