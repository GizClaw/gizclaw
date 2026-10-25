import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import {
  createGizClawPeerMonitorClient,
  createGizClawNodeMonitorClient,
  createGizClawDiscoveryClient,
  GizClawControlError,
} from "./index.ts";

test("monitor renews target-bound admin credentials per request and preserves authorization failures", async (t) => {
  const [v] = JSON.parse(
    readFileSync(
      new URL("../gizclaw/testdata/signed-bearer.json", import.meta.url),
      "utf8",
    ),
  ) as { privateKey: string; publicKey: string; target: string; now: number }[];
  let now = v.now;
  let allowed = true;
  const seen: Request[] = [];
  t.mock.method(Date, "now", () => now * 1000);
  const peer = createGizClawPeerMonitorClient({
    baseUrl: "https://monitor.example",
    publicKey: v.target,
    identity: { privateKey: v.privateKey },
    fetch: async (input) => {
      const request = new Request(input);
      seen.push(request);
      const token = request.headers
        .get("Authorization")!
        .slice("Bearer ".length);
      const claims = JSON.parse(
        Buffer.from(
          token.split(".")[0].slice("gizclaw_sig_v1_".length),
          "base64url",
        ).toString(),
      );
      assert.deepEqual(claims, {
        iss: v.publicKey,
        aud: "gizclaw/peer-http/v1",
        target: v.target,
        iat: now,
        exp: now + 60,
      });
      assert.ok(!request.url.includes(v.privateKey));
      assert.ok(!JSON.stringify([...request.headers]).includes(v.privateKey));
      return allowed
        ? Response.json({})
        : Response.json(
            {
              error: {
                code: "API_KEY_OWNER_UNAVAILABLE",
                message: "forbidden",
              },
            },
            { status: 403 },
          );
    },
  });
  await peer.getStatus();
  now += 3600;
  await peer.getStatus();
  assert.notEqual(
    seen[0].headers.get("Authorization"),
    seen[1].headers.get("Authorization"),
  );
  allowed = false;
  await assert.rejects(
    peer.getStatus(),
    (error: unknown) =>
      error instanceof GizClawControlError && error.status === 403,
  );
  assert.equal(seen.length, 3); // No fallback to public-key debug access.
});

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

test("node monitor preserves device and infrastructure counters and accepts older nodes", async () => {
  const deviceMetrics = {
    online_devices: 3,
    transports: [
      {
        peer_kind: "device",
        rtp_tracks: { rx: 3, tx: 3 },
        data_channels: { packet: 3, service: 4, native: 0 },
        rtp_packets_total: { rx: 100, tx: 200 },
        data_channel_messages_total: { rx: 300, tx: 400 },
      },
    ],
  };
  const infraFlows = {
    environment: "e2e",
    generation: "process-generation",
    window_start: 1792195200000,
    window_end: 1792195205000,
    flows: [
      {
        source: "edge",
        target: "server",
        kind: "request",
        count: 5,
        bytes: 1169,
        duration_sum_ms: 12,
        duration_count: 5,
      },
    ],
  };
  for (const supported of [false, true]) {
    const node = createGizClawNodeMonitorClient({
      baseUrl: "https://monitor.example",
      token: "gizclaw_mk_test",
      fetch: async () =>
        Response.json({
          public_key: "node",
          role: "server",
          version: "dev",
          build_commit: "dev",
          time: "2026-10-17T00:00:00Z",
          uptime_seconds: 10,
          goroutines: 1,
          heap_bytes: 0,
          transport: {
            connections: 4,
            services: 0,
            inbound_service_channels: 0,
            rx_bytes: 0,
            tx_bytes: 0,
          },
          ...(supported
            ? { device_metrics: deviceMetrics, infra_flows: infraFlows }
            : {}),
        }),
    });
    const snapshot = await node.get();
    assert.deepEqual(snapshot.infra_flows, supported ? infraFlows : undefined);
    assert.equal(
      snapshot.device_metrics?.online_devices,
      supported ? 3 : undefined,
    );
    if (supported) assert.deepEqual(snapshot.device_metrics, deviceMetrics);
  }
});

test("explicit signed credentials override identity, bind the device and keep offline KV JSON", async () => {
  const publicKey = "signed-target";
  const token = `gizclaw_sig_v1_${btoa(JSON.stringify({ target: publicKey })).replace(/=/g, "")}.fixture`;
  const reported_kv = {
    nil: { value: null, observed_at: "2026-10-22T00:00:00Z" },
    nested: {
      value: { list: [false, 0] },
      observed_at: "2026-10-22T00:00:00Z",
    },
  };
  const peer = createGizClawPeerMonitorClient({
    baseUrl: "https://edge.example",
    publicKey,
    signedBearer: token,
    identity: { privateKey: "unused-identity" },
    fetch: async (input) => {
      const request = new Request(input);
      assert.equal(request.headers.get("Authorization"), `Bearer ${token}`);
      return Response.json({
        online: false,
        last_seen_at: "2026-10-22T00:00:00Z",
        debug_mode: "off",
        reported_kv,
      });
    },
  });
  assert.deepEqual((await peer.getRuntime()).reported_kv, reported_kv);
  assert.throws(() =>
    createGizClawPeerMonitorClient({
      baseUrl: "https://edge.example",
      publicKey: "other",
      signedBearer: token,
    }),
  );
});
