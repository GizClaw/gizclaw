# pkgs/gizedge

`pkgs/gizedge` provides the GizClaw Edge Node ingress runtime. It accepts
public browser and device HTTP requests and forwards them to the configured
authoritative GizClaw Server over a `giznet` WebRTC connection.

When gateway mode is enabled, the Edge also terminates client WebRTC transport
and proxies logical connections over a bounded Server upstream pool. The Edge
is still not the owner of business data: client identity, final authorization,
domain services, and resource storage remain on the authoritative Server.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizedge)

## Directory structure

```text
pkgs/gizedge/
├── config.go     # Edge workspace configuration and boundary validation
├── edge.go       # public ingress, upstream connections, and request forwarding
├── gateway.go    # client termination, logical connections, and bounded upstream pool
├── upstream_relay.go # shared upstream TURN selection and health
└── turn.go       # optional TURN server runtime
```

`pkgs/gizedge` is currently a flat package. The code here together constitutes a single Edge Node runtime, and there are no internal modules that need to be broken into independent public sub-packages.

## Device connection lane

After startup, the Edge has a WebRTC giznet connection to the authoritative
Server. A device then performs Server discovery and gateway signaling through
the Edge:

```mermaid
sequenceDiagram
    participant Device as Device / Browser
    participant Edge as Edge Node
    participant Server as Authoritative Server

    Device->>Edge: GET /server-info
    Edge->>Server: GET /server-info over ServiceEdgeHTTP
    Server-->>Edge: Authoritative Server identity
    Edge-->>Device: Server identity and Edge transport metadata

    Note over Device: Offer is authenticated to the Edge transport identity
    Device->>Edge: POST /webrtc/v1/offer
    Edge-->>Device: Edge SDP answer
    Device->>Edge: WebRTC service, packet, and Opus lanes
    Edge->>Server: Delegated client identity over ServiceEdgeTunnel
    Edge->>Server: Multiplexed service frames
    Edge->>Server: Session-tagged packet and Opus frames
    Note over Server: Normal Peer lifecycle and authorization use the client identity
```

The ownership boundaries are:

- `/server-info.public_key` always identifies the authoritative Server.
  `transport.public_key`, `transport.endpoint`, and
  `transport.signaling_path` select only the Edge WebRTC transport.
- The Edge validates signaling and creates the client PeerConnection, then
  sends a short-lived, replay-protected delegated envelope containing the
  physical Edge identity, logical client public key, target Server identity,
  validity window, and remote address.
- The Server accepts `ServiceEdgeTunnel` only from active `edge-node` peers,
  validates the envelope, and attaches the logical client to the normal Peer
  lifecycle, service policy, and domain authorization.
- Reliable service streams use one tunnel control DataChannel per logical
  session. Direct packets and Opus use a separate unreliable, session-tagged
  packet lane.
- Gateway transport does not expose authoritative Server ICE/TURN servers to
  the client, so the normal gateway path creates no per-client Server TURN
  allocation.

The Edge does not execute GizClaw domain handlers locally or establish a second
business authorization model.

## Directory Responsibilities

### Edge Configuration

The Edge workspace configuration describes the basic information required to run the current node:

- The Edge Node's own giznet identity.
- Public HTTP listen address and external endpoint.
- The endpoint and public key of a single upstream Server, plus an optional
  relay-only TURN pool for Edge-to-Server PeerConnections.
- Selection of TLS certificate source.
- Optional TURN listener, public endpoint, relay address, credential and relay port range.
- Optional gateway ICE UDP listener, public endpoint, capacity, upstream pool,
  buffer, idle, and drain bounds.

The configuration belongs to the Edge runtime and does not reuse the storage, service or domain configuration of GizClaw Server. Server config should also not assume the public ingress and TURN parameters of the Edge process.

Currently only disabled paths work for the TLS certificate source; Edge RPC and file certificate sources are still not implemented. Development guidelines cannot write these configuration values ​​as supported capabilities.

### Public Ingress

Public ingress is responsible for:

- Listen to the public HTTP endpoint of the Edge Node.
- Forward allowed browser/device API requests to authoritative Server.
- Provides the CORS behavior required by ingress for browser requests.
- Publish Edge Node external endpoint in server-info response.
- Close the HTTP server, upstream connection and related listeners when the process stops.

Edge ingress does not have business implementations of Peer HTTP, OpenAI-compatible HTTP, or other product routes. The specific route is provided by `pkgs/gizclaw` Server, and Edge only forwards the public surface.

### Upstream Connection

The Edge uses `pkgs/giznet/gizwebrtc` to connect to the configured authoritative
Server. `ServiceEdgeHTTP` carries public HTTP forwarding and
`ServiceEdgeTunnel` carries gateway logical sessions.

By default, omitting `upstream.ice-transport-policy` and
`upstream.ice-servers` preserves direct ICE. A relay deployment sets a pool of
at least two literal-IP TURN/UDP members:

```yaml
upstream:
  endpoint: https://server.example.invalid:9820
  public-key: <authoritative-server-key>
  ice-transport-policy: relay
  ice-servers:
    - urls: [turn:192.0.2.10:3478?transport=udp]
      username: <turn-rest-key-id>
      credential: <turn-rest-shared-secret>
      credential-mode: turn-rest
    - urls: [turn:192.0.2.11:3478?transport=udp]
      username: <turn-rest-key-id>
      credential: <turn-rest-shared-secret>
      credential-mode: turn-rest
```

Relay mode passes exactly one pool member and relay-only ICE to each new
upstream PeerConnection. HTTP forwarding and gateway upstreams share one
process-local round-robin health selector. A failed relay enters bounded
exponential backoff; another eligible member is tried within the existing
30-second connection budget, with at most five seconds per member. There is no
direct fallback. Successful reconnection clears that member's failure state,
while request cancellation, Edge shutdown, and individual logical-session
failure do not penalize it. Established gateway sessions remain pinned and may
fail with their physical upstream; a fresh client reconnect selects from the
current healthy pool.

Every pool member has exactly one lowercase `turn:` URL with a literal IPv4 or
bracketed IPv6 address, an explicit port, and only `transport=udp`. Static mode
(explicit or default) requires both `username` and `credential`. `turn-rest`
requires the shared-secret `credential`; its configured username/key ID is
optional. Invalid, duplicate, partial, hostname-based, TCP, or TLS relay
configuration fails before the Edge starts a listener.

Relay selection never changes `upstream.endpoint`, `upstream.public-key`, or
the Server identity used by signaling. The top-level `turn` block is separate:
it runs a downstream TURN server for device-to-Edge transport and is not a
member of the Edge-to-Server upstream pool. Relay usernames, credentials, SDP,
ICE candidate bodies, and business payloads must not be logged.

Each gateway upstream is one WebRTC PeerConnection and SCTP association. Every
logical session has its own `ServiceEdgeTunnel` DataChannel on its selected
upstream, but those DataChannels still share association-level congestion
control and scheduling. At startup the pool opens four upstreams, bounded by
`max-upstreams`, then assigns sessions by least-active selection. It opens
another upstream only after every healthy association reaches its configured
active-session capacity. This bounded warm pool avoids both single-association
head-of-line congestion and the cold-start cost of eagerly filling all 16
available slots.

By default, one upstream holds at most 2,048 active logical sessions and enters
draining after 8,192 cumulatively opened tunnel streams. The Edge fails startup
if it cannot establish the bounded warm pool. Later capacity growth fails only
the admission that required the unavailable association. Failure of one
upstream closes only its pinned sessions.

Pool eligibility has three states. A selectable association accepts new
admissions. A draining association accepts none, preserves already established
logical sessions, and closes after its final pinned session releases. A failed
association is terminal and closes immediately. A complete ten-second
`ServiceEdgeTunnel` open timeout, a pre-open DataChannel close or error, a new
service stream closing before delegated-session acceptance, or a complete
delegated-session handshake timeout drains a still-nonterminal association
without penalizing its TURN member; caller cancellation, Edge shutdown, an
explicit logical-session rejection, and other nonterminal protocol errors do
not change a healthy association's eligibility. Packet or parent-connection
failure marks the association failed and reports its relay attempt at most
once.

A fresh client has one private 30-second logical-session establishment budget
and may try at most two physical entries before Server acceptance. Each service
open receives at most ten seconds. An alternate creates a new service stream,
session ID, and delegated envelope; it does not replay an RPC or move an
accepted session. Selectable associations count toward warm capacity, while
`max-upstreams` continues to cap all live selectable and draining physical
associations. `X-GizClaw-Gateway-Upstream` remains the initially reserved entry
because signaling writes it before a possible alternate is known.

HTTP forwarding and gateway upstreams are long-lived runtime state. The Edge
package must not copy GizClaw handlers to bypass upstream unavailability.

### Gateway Capacity and Lifecycle

The default gateway capacity is 30,000 sessions across at most 16 upstreams.
Signaling reserves handshake, total-session, and upstream-stream capacity
before creating Server state. Exhaustion returns stable `503`
`gateway_over_capacity` JSON with `Retry-After: 1`.

Each session has a default 1 MiB bounded tunnel buffer. Temporary reader
slowdown applies backpressure instead of truncating a large reliable stream;
exceeding the session or frame bound closes that session. Idle sessions expire
after five minutes. Shutdown stops admission, drains for 30 seconds, and then
closes remaining sessions.

The composed nonterminal recovery regression uses two digest-pinned Coturn
members, relay-only upstream ICE, a test-only silent UDP fault boundary, and no
direct Edge-to-Server UDP path:

```bash
bash tests/gizclaw-e2e/run_gateway_relay_recovery_tests.sh
```

It proves that the initial service stream can open locally and then reach its
complete delegated-session handshake timeout, after which the same client
completes Register and Ping through the alternate before logical-session
acceptance. Live relay host failure, drain, capacity, and soak remain deployment
acceptance rather than package E2E claims.

`max-upstreams` is a capacity ceiling, not an eager throughput target. A single
association can serialize a large concurrent burst, while opening every slot
at once pays multiple independent SCTP cold-start and congestion-recovery
costs. The bounded four-association warm pool is the measured default for the
100-session local burst; higher-session tests must measure before changing that
tradeoff.

At the same defaults, 30,000 mostly idle sessions average 1,875 sessions per
upstream, below the 2,048 hard limit. This is a capacity-model target, not a
real 30,000-session proof. The local baseline starts one Server and two
independently identified Edges, establishes 100 real client PeerConnections,
and verifies a bounded ping on every session. All sessions then cross one start
barrier to upload 4 MiB each concurrently, followed by a concurrent 4 MiB
download each. The test keeps the connections alive for another minute and
runs multiple bounded ping rounds:

```bash
bash tests/gizclaw-e2e/run_gateway_capacity_tests.sh
```

The default artifact is the ignored
`tests/gizclaw-e2e/testdata/gateway-capacity-100.json`;
`GIZCLAW_E2E_GATEWAY_CAPACITY_ARTIFACT` selects another output path. The run
requires 100/100 establishments, successful pings, no unexpected disconnect or
identity crossover, and an upstream assignment for every session on both
Edges. Each throughput direction requires 100/100 complete transfers, at least
200 Mbps aggregate, and at least 0.8 times the same run's sustained
single-session rate measured with 32 MiB. The larger baseline payload prevents
a short burst from inflating the baseline and making the ratio flaky. The
absolute floor rejects the former low-tens-of-Mbps association ceiling. The
retention ratio avoids demanding linear scaling when one session already
saturates the current path while still rejecting material concurrency
regression. This is evidence for the tested local Docker topology, not a
bandwidth promise for another network.

The artifact records the load-driver host and configuration, establishment and
ping failures, RTT, bytes, Edge/upstream distribution, RSS, Go/runtime active
CPU estimates, file descriptors, heap, and goroutines. Upload and download
each include the single-session baseline, 100-session aggregate Mbps computed
over one shared wall-clock interval, per-session rate percentiles, exact byte
completion, failures, and per-Edge/upstream aggregates. Client durations are
not summed or substituted for the shared start-to-finish interval. Ping
evidence also includes per-round and per-Edge/upstream RTT and failure
summaries so a transient path is not hidden by one aggregate percentile.
Memory and CPU points include their measurement source. Without Linux
`/proc/self/statm`, `rss_bytes` is marked as the `go_memstats_sys` fallback and
is not complete process RSS. Unsupported file-descriptor sampling is reported
as `-1`. A throughput failure, absolute-Mbps violation, or retention-ratio
violation also makes the entrypoint exit nonzero.

The fixed 500-session qualification is:

```bash
bash tests/gizclaw-e2e/run_gateway_capacity_500_tests.sh
```

It repeats three fresh one-Server/two-Edge stacks with zero ramp and 500
simultaneous Dials. Each Edge owns 250 sessions and exactly four upstream
associations. Every run requires 500/500 usable sessions, no failures,
disconnects, restarts, or identity crossover, at least 20 sessions/s, Dial p95
at most 1 second and p99 at most 5 seconds, and exactly 500 MiB transferred in
each direction at least 200 Mbps aggregate. The 32 MiB single-session
baseline and aggregate ratios are diagnostic only. Publishable artifacts must
report the clean final PR head.

On 2026-08-02, three clean-head runs on one Darwin/arm64 host with 16 logical
CPUs, Go 1.26.4, and an OrbStack Linux/aarch64 Docker one-Server/two-Edge
topology with direct container signaling endpoints all passed. Every run
established 500/500 usable sessions, reported zero transfer failure or
unexpected disconnect, and transferred exactly 500 MiB in each direction
above 200 Mbps. These measurements qualify only that host and topology; they
do not qualify 1,000 sessions, a soak, or a deployment network.

The fixed relay-only 1,000-session burst and soak entrypoints are:

```bash
bash tests/gizclaw-e2e/run_gateway_capacity_1000_tests.sh
bash tests/gizclaw-e2e/run_gateway_capacity_1000_soak_tests.sh
```

The burst entrypoint requires a clean head and repeats three fresh
one-Server/two-Edge/two-Coturn stacks. Each run releases 1,000 Dials through
one barrier with concurrency 1,000 and no ramp, holds the 1,000 live sessions
for 30 seconds, and performs final liveness before bounded teardown. Each Edge
must own exactly 500 sessions across four gateway upstreams. The establishment,
exact 1 GiB-per-direction application transfer, 200 Mbps aggregate, timing,
resource, relay-selection, ten-allocation, restart, and cleanup gates are the
same fixed contract as the smaller tiers. The load driver fixes and records
`GOGC=200` because the 1,000-way client heap otherwise adds measured GC tail
latency; this setting does not alter Edge, Server, or Coturn runtime behavior.

The soak entrypoint first reruns all three burst repetitions on the same clean
head, then starts one new no-ramp 1,000-session stack and holds it for 60
minutes. Complete liveness rounds start every 30 seconds. Distinct initial and
final 1 MiB-per-session upload/download checkpoints must each exceed 200 Mbps,
and each final direction must retain at least 80% of its initial aggregate and
of its per-session p50, p95, and p99 throughput.

Artifact version 13 records the actual hold boundaries and compares the first
and last ten minutes. The median per-round RTT p99, process RSS, open FDs, and
available Go heap and goroutine medians must not grow by more than 20%. CPU and
network-rate changes use the same relative bound with absolute noise floors of
0.10 core and 1,024 bytes/s; UDP/UDP6 socket medians use the 20% bound. These
checks cover the load driver, both Edges, both Coturn members, and Server, reject
process-counter resets, and require resource gaps no larger than 2.1 seconds.
Unavailable external Go runtime metrics and load-driver socket/network metrics
are named explicitly rather than represented by fabricated values.

Logical-session cleanup has a 30-second bound; the ten physical TURN allocations
are checked from source-qualified Coturn counters once per second while the
Edges are alive and must return to zero within 15 seconds after Edge shutdown.
These commands qualify only their recorded host, Docker engine, clean commit,
and topology; they are not a 30,000-session or WAN guarantee.

This fixed qualification establishes the 1,000-session burst and soak boundary
only. It does not infer a higher-session capacity projection.
Repeatable transport and full Edge benchmarks are:

```bash
go test -tags giznet_e2e ./tests/giznet-e2e/webrtc \
  -run '^$' -bench BenchmarkWebRTCServiceThroughput -benchtime=1x -count=5
go test ./pkgs/gizedge \
  -run '^$' -bench BenchmarkGatewayServiceThroughput -benchtime=1x -count=5
```

The deployed Docker suite records separate upload-only and download-only
one-client versus three-client observations. It is informational by default.
Controlled runners may set positive minimum client/aggregate ratios when the
path has enough headroom, or direction-specific aggregate Mbps floors derived
from independently measured usable bandwidth:

```bash
GIZCLAW_E2E_SPEED_MIN_UPLOAD_AGGREGATE_MBPS='<upload floor>' \
GIZCLAW_E2E_SPEED_MIN_DOWNLOAD_AGGREGATE_MBPS='<download floor>' \
go test -tags gizclaw_e2e ./tests/gizclaw-e2e/go/edge \
  -run '^TestGatewaySpeedOneVersusThreeClients$' -count=1 -v
```

`GIZCLAW_E2E_SPEED_MIN_CLIENT_RATIO` and
`GIZCLAW_E2E_SPEED_MIN_AGGREGATE_SCALE` remain available for an unsaturated
runner. Do not use a scale-only threshold when one client already approaches
the usable path limit.

### TURN

The Edge Node can run an optional TURN UDP relay at the same time, providing relay capabilities for connections that cannot directly establish a WebRTC path.

TURN runtime is only responsible for relay listener, authentication and relay port range. It is not responsible for GizClaw user logins, peer ACLs, route assignments, or business authorization. TURN credential and GizClaw resource credential are not the same type of data.

### Upstream relay qualification

An Edge can connect its control association and bounded gateway upstream pool
directly to the authoritative Server or force those associations through the
configured Coturn pool with `ice-transport-policy: relay`. Relay mode selects
one pool member per Dial and does not fall back to a direct candidate. This is
an Edge-to-Server product topology; the transport-only Giznet Coturn suite does
not exercise it.

Successful upstream ownership emits one sanitized selected-ICE observation for
the control epoch and one for each live gateway entry. The observation is used
by the gateway capacity qualification described in the Testing Guide. It is
diagnostic evidence, not a public API or a production performance guarantee.

In the 2026-08-04 local 12-run qualification, all 100- and 500-session direct
and relay runs passed. Direct/Coturn median throughput was 654/578 versus
416/568 Mbps at 100 sessions and 476/612 versus 417/606 Mbps at 500 sessions.
The material upload delta was also reproduced by the same-head transport-only
Giznet diagnostic after removing the product Edge and Server, so this result
does not identify an Edge pool, tunnel, or Server capacity limit. The measured
owner boundary is the local Coturn relay path; see the Testing Guide for the
full latency table and non-production boundary.

## Dependencies

```mermaid
flowchart TB
    Command["cmd/internal/commands/edge<br/>Process entry"] --> GizEdge["pkgs/gizedge<br/>Edge runtime"]
    GizEdge --> GizClaw["pkgs/gizclaw<br/>Edge HTTP / Tunnel contracts"]
    GizEdge --> Giznet["pkgs/giznet<br/>Connection contract"]
    GizEdge --> GizHTTP["pkgs/giznet/gizhttp<br/>HTTP adapter"]
    GizEdge --> GizWebRTC["pkgs/giznet/gizwebrtc<br/>WebRTC transport"]
    GizEdge --> TURN["Pion TURN"]
```

The dependency direction is:

- CLI command select workspace and start `pkgs/gizedge`.
- `pkgs/gizedge` consumes the Edge service contract defined by GizClaw, but does not depend on the specific domain service.
- Edge uses `giznet`, `gizhttp` and `gizwebrtc` to establish the upstream data path.
- `pkgs/gizclaw` and `pkgs/giznet` are not dependent on `pkgs/gizedge`.

## Ownership Boundary

Should be placed at `pkgs/gizedge`:

- Edge workspace configuration and Edge-specific validation.
- Public ingress listener, proxy and Edge response rewrite.
- Edge to authoritative Server connection, login, reconnection and forwarding life cycle.
- Client WebRTC termination, logical-session admission, upstream pool, and
  gateway shutdown.
- TURN relay run by Edge Node itself.
- Shutdown and cleanup behaviors only belong to the Edge process.

Should not be placed in `pkgs/gizedge`:

- Peer, workspace, firmware, gameplay, social or Agent domain services.
- Authoritative resource storage and final resource-access decisions.
- Transport-independent connection contract or generic WebRTC implementation.
- HTTP/RPC handler for GizClaw Server.
- Server storage backend, migration and workspace runtime assembly.
- Global peer directory, mesh membership, cross-server data synchronization or route replication.

These contents belong to `pkgs/gizclaw`, `pkgs/giznet`, `cmd/internal/server` respectively, or are still the subsequent design scope of server mesh.

## Current boundary

Currently `pkgs/gizedge` connects to one authoritative Server and supports Edge
HTTP ingress, optional gateway termination, and optional TURN relay.

It is not a complete server mesh:

- The Edge is configured for one upstream Server.
- `ServiceEdgeHTTP` carries public request forwarding.
- `ServiceEdgeTunnel` carries logical client sessions over a bounded upstream
  pool.
- Edge control-plane RPC, certificate distribution, and non-disabled TLS
  certificate sources are not complete.
- The Edge does not maintain mesh membership or a global peer/resource route
  registry.
- This package does not replicate data or events between Servers.

Therefore, when adding a capability, you must first determine whether it is the responsibility of the current Edge ingress or the future work of the server mesh control plane; you cannot directly write `pkgs/gizedge` just because the capability is related to the public network entry point.
