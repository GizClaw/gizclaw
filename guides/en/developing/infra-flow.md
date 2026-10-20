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
module neither enumerates business records nor installs a real transport collector.

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
empty frames. Real activity still requires an explicitly owned aggregation job
writing configured infrastructure-route counters to shared KV.

After deployment, verify anonymous GET, cross-origin OPTIONS, invalid environment/cursor,
405, no-store and device/Monitor/Admin permission isolation at every AP. Check frame
environment, cursor, time and `simulated` to establish live coverage and freshness.

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
