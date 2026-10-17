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

`GET /gizclaw/v1/infra/flows?environment=prod&cursor=<previous cursor>` requires
an independent `gizclaw_mk_` Bearer read credential. Device API keys/public keys
grant no access. Environments are restricted to `dev|e2e|prod|trial`; arbitrary
queries, user filters and variable limits are rejected.

First connection returns only the latest frame. Continuous cursors can catch up
by at most three frames; future, expired or discontinuous cursors reset to latest.
Without a producer, frames and cursor are empty. A five-second per-environment
process cache shares at most four exact KV reads per fill; temporary errors are
also cached briefly. No HTTP write API exists. GET/OPTIONS are allowed; every
response is `no-store`. Invalid credentials return 401, invalid queries 400,
and disabled/unavailable feeds 503. Reads have a two-second deadline.

Configure an explicit logical `kind: keyvalue` Store, preferably with its own prefix:

```yaml
infra_flow:
  store: infra-windows
  read_token: ${GIZCLAW_INFRA_FLOW_READ_TOKEN}
```

Empty configuration disables the feed and creates no implicit Store. Server
ordinary HTTP and trusted Edge HTTP mount this exact independently authenticated
route; other public API authorization is unchanged. Edge forwards it to a
reachable configured upstream without device-owner resolution. Servers must
share the environment's KV authority so regional AP ingress shares one cursor.

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

Open `http://127.0.0.1:5176/flow-feed.html`. The local proxy reads the actual Go
AP API. The SDK derives rates and weighted mean latency from aggregates, with
at most sixteen representative particles per frame, twenty-four active particles,
and thirty drawn frames per second. Idle rendering stops; pausing or hiding the
document cancels reads and animation. Particles are activity illustrations, not
individual requests or percentile observations.

`go test -race ./pkgs/infraflow/... -count=3` covers fixed slots/bytes, million-fold
count scaling, CAS conflicts, environment isolation, cursor recovery, caching,
the generated client and authorization. Edge integration checks ensure this
read-only route does not resolve device ownership.
