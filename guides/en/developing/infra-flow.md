# Infrastructure Flow Feed

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/infraflow)

`pkgs/infraflow` stores fixed five-second infrastructure activity windows through
the existing `kv.Store`. `api/http/infra.json` owns the HTTP contract; regenerate
Go strict server/client/models with `go generate ./pkgs/infraflow/api` and the
control SDK's `generated/infra` with `npm --prefix sdk/js run gen:sdk`.

## Bounded aggregation

A producer registers at most 64 directed infrastructure routes at startup. It
merges counts, bytes, duration sums and duration sample counts into those fixed
cells; no user, device or session index exists. `count` means producer-defined
activities for that route's kind, not a universal HTTP request count. Drain takes
one completed five-second frame, with no KV I/O inside the aggregation lock.
Failed writes do not retain raw events or create an unbounded retry queue.

Frames are capped at 32 KiB and JavaScript-safe values, including frame totals.
Each environment uses 120 circular slots plus one head: at most 121 keys, or
484 across the four fixed environments. Active production covers approximately
ten minutes; idle retained slots remain bounded and never masquerade as fresh
activity. Conditional KV mutation commits the slot and head atomically.
Identical retries are allowed; conflicting publishers for the same window fail.
The aggregation job must have one explicit owner. Feed borrows the Store.

Route IDs must map to published topology IDs. Node-wide totals cannot recover
specific source/target traffic. A real producer reads configured route counters,
with one accounting owner per directed route to prevent double counting. This
module does not enumerate business records; Server installs the explicit activity capture below.

## Read API and wiring

`GET /gizclaw/v1/infra/flows?environment=prod&cursor=<previous cursor>` is public
and read-only. Browsers can read it anonymously without Authorization, a token or
an owner identity. Supplied legacy credentials are ignored, with no device API-key
or owner lookup. Only aggregate directed infrastructure activity is public; user,
device, Peer, session and raw event records are excluded. Environments are restricted
to `dev|e2e|prod|trial`; arbitrary queries, user filters and variable limits are rejected.

First connection returns only the latest frame. Continuous cursors can catch up
by at most three frames; future, expired or discontinuous cursors reset to latest.
Without a producer, frames and cursor are empty. A five-second per-environment
process cache shares at most four exact KV reads per fill; temporary errors are
also cached briefly. Caller cancellations and deadlines never enter that shared
cache, so healthy waiters can immediately reload. No HTTP write API exists.
GET/OPTIONS are allowed; other methods return 405 with `Allow: GET,OPTIONS`.
Server and Edge successes, errors and preflights are `no-store`. Cross-origin
responses echo Origin with `Vary: Origin`, or use `*` without an Origin.
`Access-Control-Allow-Methods` is fixed to `GET,OPTIONS`, allowed request headers
are `Authorization,Content-Type`, and browser credentials are not enabled.
OPTIONS needs neither query parameters nor an available KV Store and returns 204.
Invalid queries return 400; disabled/unavailable feeds return 503. An unreachable
Edge upstream retains the proxy error 502 `UPSTREAM_ERROR`. Reads have a two-second deadline.

Configure an explicit logical `kind: keyvalue` Store, preferably with its own prefix:

```yaml
infra_flow:
  store: infra-windows
```

An empty Store disables the feed and creates no implicit Store. Legacy
`read_token` is accepted and ignored: absent, empty, old or unset environment values
do not block startup or anonymous reads. A legacy token alone cannot enable the feed.
Server ordinary HTTP and trusted Edge HTTP mount this exact public route. Edge
forwards it to a reachable configured upstream without device API-key or owner
resolution. Adjacent paths, including a trailing slash, retain existing authorization.
Monitor, device, Peer and Admin boundaries remain independent. Servers must share
the environment's KV authority so regional AP ingress shares one cursor.

Environment APs use the same path and matching `environment` query:

| Environment | AP |
| --- | --- |
| `dev` | `https://ap.dev.gizclaw.com` |
| `e2e` | `https://ap.e2e.gizclaw.com` |
| `prod` | `https://ap.gizclaw.com` |
| `trial` | `https://ap.trial.gizclaw.com` |

For example, browsers can GET
`https://ap.dev.gizclaw.com/gizclaw/v1/infra/flows?environment=dev` without credentials.
Support depends on the deployed version and KV configuration; source and local tests
do not establish live acceptance.

## Release, deployment and acceptance

Existing APs require a separate release of the GizClaw binaries and runtime image,
followed by Deploy configuration and a reviewed plan using a formal Release/Provider
snapshot. Bind each environment's Server `infra_flow.store` to an explicitly configured
shared KV Store and upgrade both Server and Edge. Publication, Deploy Apply and traffic
activation are independent operations. Without a producer, the enabled feed returns
empty frames. Enable the explicitly owned capture and producer below for real activity.

After deployment, verify anonymous GET, cross-origin OPTIONS, invalid environment/cursor,
405, no-store and device/Monitor/Admin permission isolation at every AP. Check frame
environment, cursor, time and `simulated` to establish live coverage and freshness.

## Real activity collection and ownership

This is a low-overhead, best-effort visual activity signal. Business work takes
priority over completeness: dropped samples, missing observations and missing
windows are acceptable. There is no replay, backfill or extrapolation into totals.
Do not use these values for billing, analytics, throughput accounting or latency SLIs.

Capture is limited to infrastructure Peer-to-Peer request surfaces. The installed
observers cover trusted Edge → Server HTTP and control RPC completions. Other
Server/Mesh peer paths have no capture here. Database/KV calls and other operations
inside a request are not instrumented; KV only stores and serves completed frames.

Each route reserves one sixty-fourth of the frame's safe numeric capacity for
sample bytes and elapsed time. Out-of-budget sources are omitted before merging,
so malformed values cannot suppress healthy sources' frames.

Each route retains at most 20 samples per five-second window. Background window
rotation grants 20 sampling credits per route. Once exhausted, the hot path
performs one atomic load, without clocks, allocations or locking.
Only selected requests are timed and counted for bytes; RPC byte wrappers are
installed only for selected calls. Completion tries to update fixed memory and
immediately drops on contention, missed windows or capacity. Business never waits.
Background reads own window rotation, quota refill and snapshot allocation. Without
a reader/collector, the initial quota exhausts and timing/byte work stops. Sampling
adds no database instrumentation or business KV operations.

Server processes own opt-in collection, without an extra daemon or Metrics Store.
Enable `producer` on exactly one designated Server per environment. Other Servers
configure only `capture` and the shared Feed Store. Clusters enable independently;
environments are fixed to `dev|e2e|trial|prod` and IDs must match the published topology.
Each source declares its complete route set. Configuration rejects duplicate directed
routes, source public keys, Edge owners, and more than 64 routes.

| Supported route | Unique owner | count | bytes | duration |
| --- | --- | --- | --- | --- |
| Configured Edge → authoritative Server | Receiving Server, matched by the trusted connection key in `edge-nodes` | Retained Edge HTTP/control RPC completion samples, including errors | HTTP response body bytes actually written by retained samples; retained RPC stream bytes actually read/written, including framing | Actual elapsed time of retained samples |

Peer request samples use `kind: request`. HTTP and control RPC are separate operations on the same
infrastructure route. HTTP bytes exclude request bodies and headers; RPC excludes
ICE/DTLS overhead. Database reads/writes, device/Server audio, models, assets,
replication and whole-host traffic are outside this collection coverage. Unattributed routes are absent. Feed polling is
excluded from Edge activity, preventing the visualization from driving itself.

Capture retains only the current and previous UTC-aligned five-second windows,
assigning retained samples by completion time. Monitor reads the completed window without
draining it, so multiple readers and scrape delay cannot steal counts or shift
counter differences between windows. Restart changes generation. The partial startup
window, partial windows after clock rollback and unavailable sources are omitted.
Oversized samples are dropped. Zero only means no samples were retained, never
proof that business was idle.
When every source is missing, no frame is written; consumers must check freshness.
Failed or missed windows are discarded without replaying history in the next frame.

Remote counters use the optional `infra_flows` field on the existing independently
authenticated `/monitor/api/node`. The producer checks configured public key, Server
role, environment, complete route set, snapshot time and window, rejecting stale or
unknown routes and invalid counters. Each window has at most 64 concurrent sources,
one request per source and 64 KiB per response. The publisher waits two seconds
after the boundary for source clocks to close their windows, matching the accepted
clock tolerance. There is still only one scrape, with one second for scraping and a
separate one-second Feed write budget. Remote endpoints require HTTPS; only loopback
tests allow HTTP. Redirects are disabled. Monitor tokens never enter public frames
or failure logs. Failures emit bounded window warnings. A conflicting same-window
publisher stops its worker without stopping business; operators must still configure
one publisher per environment. Shutdown cancels and joins collection before closing
Server and Stores, without publishing a partial shutdown window.

This example captures and publishes on one Server. Store names refer to the normal
`stores` inventory; replace example IDs, public key and Store name with explicit
cluster mappings. Edge needs no new capture configuration; its key must already be
in `edge-nodes`.

```yaml
infra_flow:
  store: infra-windows
  environment: e2e
  capture:
    node: server-shanghai
    routes:
      - source: edge-shanghai
        target: server-shanghai
        edge_public_key: ${EDGE_PUBLIC_KEY}
  producer: {}
```

For multiple Servers, add remote sources on the designated publisher. Remote nodes
keep `capture`, omit `producer` and configure an independent `monitor.token`. The
publisher's `routes` must exactly match each remote node's complete directed routes:

```yaml
infra_flow:
  store: infra-windows
  environment: e2e
  producer:
    sources:
      - endpoint: https://server.example.invalid/monitor/api/node
        public_key: ${SOURCE_SERVER_PUBLIC_KEY}
        token: ${SOURCE_MONITOR_TOKEN}
        routes:
          - source: edge-shanghai
            target: server-shanghai
```

Local `capture` routes are included automatically; do not configure them again as a
remote source. The Feed Store is a frame persistence backend. Servers must contain
`infra_flow.capture`, `producer` and Monitor `infra_flows` support; read-only-feed
versions such as v0.33.8 and earlier cannot install this producer. Formal release
and per-environment deployment require separate acceptance. Edge continues to use a
version supporting the public feed. Older SDK consumers may ignore the optional
Monitor field.

Local acceptance uses independent real Server/Edge processes through the normal
release startup path: two Servers share temporary SQLite KV, one exports capture
through authenticated Monitor, and the other publishes Feed windows. Edge forwards
HTTP and control RPC over real WebRTC. A normal business API key allows four HTTP
calls and one control RPC to trigger five peer-surface activities. Internal
database work creates no additional Flow.
Anonymous reads must return fresh `simulated: false` frames with matching bytes,
sample durations, window, cursor, the next window with no new samples and environment
isolation. Separate deterministic tests cover failures, source validation, quota,
drops, clock skew, positive numeric overflow, nonblocking business progress, restart, cancellation
and the 121-key bound.

```sh
go test ./cmd/internal/server -run '^TestInfraFlowProductionServerEdgeCollection$' -count=1 -v
go test -race ./pkgs/infraflow/... -count=1
go test ./pkgs/infraflow -run '^$' -bench '^BenchmarkExhaustedSamplingGate$' -benchmem -cpu 1,4
go test ./pkgs/gizclaw -run '^$' -bench '^BenchmarkHTTPActivitySampling$' -benchmem -cpu 1,4
```

Comparison benchmarks use identical business/logging work to check added allocations
and request handling cost. Completeness never justifies waiting for collection. A
blocked-collector test holds the aggregation lock while 80,000 concurrent peer
completion observations must return without waiting; contended samples are dropped.

This is loopback/SQLite protocol acceptance, not physical-device, TiKV, production
VMP or deployed-AP acceptance.

## Local synthetic preview

In a GizClaw checkout run `go run ./examples/infra-flow-demo`. This loopback-only
example explicitly selects `kv.NewMemory`, uses the real read handler, and writes
one frame every five seconds for eight synthetic nodes and twelve routes. Its
`/demo/config` and `/demo/scale` are example-only routes. Switching simulated
activity from 1 to 1,000,000 changes values, never route count or KV slot count.

In a devtools checkout run:

```sh
make infra-globe-build
npm run dev --workspace @gizclaw/infra-globe -- --port 5176 --strictPort
```

Open `http://127.0.0.1:5176/flow-feed.html`. The local Go example also accepts a
direct anonymous read:

```sh
curl 'http://127.0.0.1:9819/gizclaw/v1/infra/flows?environment=prod'
```

`/demo/config` returns topology, endpoint and an explicit simulated flag, with no token.
The SDK derives rates and weighted mean latency from aggregates, with
at most sixteen representative particles per frame, twenty-four active particles,
and thirty drawn frames per second. Idle rendering stops; pausing or hiding the
document cancels reads and animation. Particles are activity illustrations, not
individual requests or percentile observations.

`go test -race ./pkgs/infraflow/... -count=3` covers fixed slots/bytes, million-fold
count scaling, CAS conflicts, environment isolation, cursor recovery, caching,
anonymous generated-client GET/OPTIONS, legacy configuration and read deadlines.
Server/Edge integration checks verify permission isolation and routing without
device-owner resolution.
