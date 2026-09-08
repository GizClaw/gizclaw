# Monitor

The monitoring console in `web/console/` (React, TypeScript, Vite, shadcn/ui,
Recharts) is built and embedded in the Go executable distributed with Server
and Edge. Open `/monitor/` on the node; `/monitor` redirects there. No external
static files are required. `web/console/DESIGN.md` defines its visual language
and data rules.

## Node snapshot API and authorization

Server and Edge mount `pkgs/monitor.Handler` on their existing HTTP/HTTPS
listeners. No dedicated monitor listener is introduced. The page is public;
node data still requires the independent Monitor Token.

GET `/monitor/api/node` reads this process only and requires
`Authorization: Bearer gizclaw_mk_...`. Configure each node independently:

```yaml
monitor:
  token: ${GIZCLAW_MONITOR_TOKEN}
```

Use at least 32 characters after the prefix; `openssl rand -hex 32` generates a
suitable random suffix. An empty configuration disables node data (503).
Invalid tokens return 401. Responses use no-store.

The endpoint answers CORS preflight and echoes the request Origin with
`Vary: Origin`, `Access-Control-Allow-Methods: GET,OPTIONS` and
`Access-Control-Allow-Headers: Authorization,Content-Type`, so a console hosted
on another origin can read snapshots. The Monitor Token travels in an explicit
Authorization header, so no browser credentials are shared.

The console keeps its configuration — nodes, tokens and the personal device
watch list — in origin-local IndexedDB encrypted with a nonextractable Web
Crypto key, and clears it on explicit logout. This is not an OS keychain:
same-origin scripts can use the key. Telemetry and logs are never persisted in
the browser.

Device monitoring uses the access point's Public APIs with the device's
`gizclaw_pk_` bearer. The Server's ordinary business HTTP ingress remains
disabled, so device APIs are served through an access point (Edge). Readonly
permits reads; fullcontrol permits supported volume and reboot operations, with
authorization rechecked by the owning Server.

## Data semantics

Node connection counts are process-local WebRTC associations, including upstream
connections. Service streams are counted separately. RX/TX are cumulative
WebRTC service payload bytes since process start, excluding ICE/DTLS overhead.
Peer counters come from the owning Server's Runtime. The console polls every
node in parallel every five seconds, derives rates from the cumulative counters
(a restart reads as zero, never a spike) and retains at most 600 samples per
node with 2/10/30 minute windows.

The node log buffer retains 500 structured process records, each with at most
4096 message bytes and up to 24 structured fields (keys ≤ 64 bytes, values ≤ 512
bytes) taken from the record's own attributes — `request_id`, `operation`,
`route`, `status`, `duration_ms`, stream identifiers and the rest — so one
request can be followed across records and nodes. Identity comes only from the
trusted logging context: a caller-supplied `peer_public_key` attribute stays an
ordinary field. Records do not survive restart. The console merges each node's
records by id across polls, so its window outlives the node's own ring.

`/gizclaw/v1/device/logs` returns only records whose structured peer_public_key
exactly matches the authorized device. These are server-side device-related
logs, not firmware serial logs.

Telemetry splits in two: metric fields (`battery.*`, `network.rssi_dbm`,
`network.signal_level`, `network.connected`, `system.*`, `gnss.*`) become stored
samples that can be queried over a time range, while status-only values
(firmware and software versions, cellular `rat`, `operator`, `imei`, `imsi`,
audio player state, OTA reports) only keep their latest value in the device
status. The console shows the first group under Telemetry with a trend chart
per field, and the second group under device status fields.

## Build and validation

```sh
npm ci
npm run build --workspace @gizclaw/console
npm test --workspace @gizclaw/console
go build ./cmd/gizclaw
```

The static bundle in `web/console/dist/` (ignored by git) is embedded using
`go:embed`. Build the console before compiling Go or running tests that depend
on monitoring; missing assets fail compilation. Linux Docker builds and macOS
releases perform this step, and the executable needs no source directory at
runtime. The bundle can also be hosted separately. For development,
`npm run dev --workspace @gizclaw/console` serves it on port 5174 and proxies
`/gizclaw` to a local access point (override with `CONSOLE_DEVICE_PROXY`); node
snapshots are read directly from each configured node URL.

## HTTP contract and validation

`api/http/monitor.json` owns the independent Monitor OpenAPI surface. Its token
middleware runs before the generated standard-library router; `nodeServer`
implements the generated strict interface. The console calls its generated
JavaScript client. This surface is local to each process and does not use Peer
assignment or Admin authentication.

| Status | Response |
| --- | --- |
| 200 | Generated `NodeSnapshot` with local counters and bounded logs |
| 401 | `{"error":"INVALID_MONITOR_TOKEN"}` |
| 503 | `{"error":"MONITOR_DISABLED"}` when no token is configured |
| 405 | Empty body, `Allow: GET,OPTIONS` for unsupported methods |

Every node API response includes `Cache-Control: no-store`. The JSON
success/error types and Go strict server/client are generated with
`go generate ./pkgs/monitor/api`. JavaScript generation runs with
`npm --prefix sdk/js run gen:sdk`; configuration and committed output paths are
documented in [API generation](api/generation).

`go test ./pkgs/monitor` verifies token authorization, the CORS contract, the
405 method boundary, the generated client's 200/401/503 handling, and that
structured log fields reach the snapshot. `go test ./pkgs/gizlog` covers the
bounded record ring and its field limits.

## Device APIs used by the console

`GET /gizclaw/v1/device/workspaces` lists explicitly Peer-owned Workspaces,
including system Workspaces, grouped by Workflow. Shared and ownerless
Workspaces are excluded. `GET /gizclaw/v1/device/workspaces/{workspaceId}/history`
searches persisted text with cursor pagination (up to 200 entries; the console
uses 100). History cursors are entry-ID timestamp boundaries, not authorization
tokens; malformed values return 400 `INVALID_HISTORY_CURSOR`. Browsing does not
start an Agent. The nested `/{historyId}/audio.ogg` endpoint serves retained Ogg
audio through authenticated requests.

`GET /gizclaw/v1/device/telemetry/{field}/latest` returns the newest sample of
one field; `GET /gizclaw/v1/device/telemetry` returns a sampled range for a
field over an explicit time window.

`GET /gizclaw/v1/device/logs/search` queries the configured
`services.system_log.query_store` with positive Unix millisecond bounds, text of
at most 512 UTF-8 bytes, a strict `DEBUG|INFO|WARN|ERROR` level and a cursor; up
to 500 records per page. The Server binds the authorized Peer, so a cursor from
another device is rejected.

Acceptance for the real endpoints runs through
`bash tests/gizclaw-e2e/run_monitor_tests.sh`, covering device and node
authorization, chat history, runtime logs and audio download; see
[Monitor API giztest](testing#monitor-api-giztest).

## HTTP proxy channel lifecycle validation

`go test -race ./pkgs/giznet/gizhttp -run 'TestReverseProxyConcurrentStreamLifecycle|TestHTTPStreamTimeoutAndCancellationRelease' -count=3`
uses a real HTTP reverse proxy and production WebRTC configuration, covering
direct and local TURN/UDP relay paths and asserting the relay is actually
selected. Each concurrent round completes 4096 HTTP requests with 16-way
concurrency over one WebRTC connection; the timeout test covers header waits,
streaming body reads, caller cancellation and downstream TCP loss before and
during a response.

The test checks that inbound counts, bidirectional service stream totals and
HTTP server connections return to baseline while the parent WebRTC connection
stays open, then completes another request over the same connection. It does
not rely on closing the parent connection to reclaim resources, and does not
cover real public-network loss, a Hong Kong TURN deployment or long-running
conditions.
