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

Logs are queried through the device persistent LogStore search API, with time ranges, text, level and pagination. Node snapshots contain the binary version and build commit, runtime status and transport counters; the console shows the version in the node list and snapshot tab.

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
a generated, git-ignored `assets_generated.go` manifest that names every output
file in `go:embed`. Removing any listed file fails Go compilation. Build the console before compiling Go or running tests that depend
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
| 200 | Generated `NodeSnapshot` with build identity, local runtime status and counters |
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
node snapshots exclude logs. `go test ./pkgs/gizlog` covers configured log sinks.

## Device APIs used by the console

`GET /gizclaw/v1/device/workspaces` lists explicitly Peer-owned Workspaces,
including system Workspaces. Shared, ownerless, and pending-deletion
Workspaces are excluded. Each item identifies its Workflow by `collection` and
`workflow_name`, which the console shows instead of an Admin Workflow ID. `GET /gizclaw/v1/device/workspaces/{workspaceId}/history`
searches persisted text with cursor pagination (up to 200 entries; the console
uses 100). `order` defaults to `desc` (newest first) and also accepts `asc`;
optional `start_time_ms` (inclusive) and `end_time_ms` (exclusive) bound
creation time in Unix milliseconds and still apply to continuation requests.
History cursors are exclusive entry-ID timestamp boundaries: pass the previous
`next_cursor` to continue in the same order, or any item `name` to page away
from that item in the requested order. Console search lists only the entries
matching `query`; selecting one reads its surroundings with that entry's `name`
as the cursor in both `asc` and `desc`, locates and highlights it in the full
timeline, then continues with `desc` for older entries and reads newer entries
with `asc` from the topmost item `name`.
Cursors are not authorization tokens; malformed values return 400
`INVALID_HISTORY_CURSOR`, and invalid parameters such as a `start_time_ms` not
before `end_time_ms` return 400 `INVALID_REQUEST`. Browsing does not start an
Agent. The nested `/{historyId}/audio.ogg` endpoint serves retained Ogg
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

## Diagnostic assistant

The diagnostic assistant lives in `web/assistant/` (private workspace
`@gizclaw/assistant`), an agent package independent of React; see
`web/assistant/DESIGN.md` for the design. It reaches a node's `/openai/v1`
through the Chat Completions model of `@openai/agents-core` and
`@openai/agents-openai`. Every tool runs in the browser; GizClaw only forwards
tool declarations, calls, and results.

Every way out of the assistant is injected through `AssistantRuntime`: `page`
(navigation, opening links, the current route), `view` (a structured snapshot
of what the current page shows), `fleet` (node status and in-memory traffic
samples), `devices` (device lookup, status, latest and ranged telemetry,
Wi-Fi, conversation Workspaces and history), and `logs` (log search).
`src/apis.ts` is the tool catalog: each tool declares the runtime methods it
calls, its description, zod parameters and implementation; tools are generated
from it, and a test ensures every runtime method is used by exactly one tool.
The tools are read-only; device writes such as reboot, volume, Wi-Fi scans and
changes, and deletion have no runtime method. Log search aggregates by level,
operation, error code, RPC status code, and HTTP status, and navigation to the
log page builds the console query from a device key, error code, level and
text. Source failures reach the model as error codes; for
`DEBUG_ACCESS_FORBIDDEN` the assistant explains that read-only debug mode must
be enabled on the device instead of claiming to have done it. Tool results,
conversation history included, are treated as untrusted data.

Validation has two layers. `npm test --workspace @gizclaw/assistant` runs the
scenario set through the real Agents SDK runner with a `FakeRuntime` covering
every source and a scripted model, proving tool wiring, argument validation,
result handoff, and navigation. `TestAssistantScenariosWithLiveModel` in
`tests/gizclaw-e2e/go/openai` runs the same scenarios against the Docker stack's
RuntimeProfile `llm` (Volc Ark `doubao-mini-chat`), asserting only tool calls,
the final route, and key facts in the reply, giving each scenario three
attempts to separate a small model's variance from behavior the assistant
cannot reach.

### Console chat entry

The chat button in the bottom-right corner of the Monitor console opens the
diagnostic assistant panel. The panel code (the agent, the model client, and
`@assistant-ui/react`) loads on first open and stays out of the initial bundle.

The assistant uses an existing device's API key. Add to the console
configuration:

```json
"assistant": {
  "apiKey": "gizclaw_sk_v1_...",
  "model": "llm",
  "endpoint": "https://edge.example.com"
}
```

`apiKey` must be a device API key starting with `gizclaw_sk_v1_`; the assistant
uses it over `/openai/v1` to call models from that device's RuntimeProfile.
`model` is the RuntimeProfile model alias and defaults to `llm`. `endpoint` is
optional and defaults to the device API endpoint: `deviceEndpoint` or the
console's own origin. The key can also read and control the device it belongs
to, so like Monitor tokens it is stored encrypted with the configuration in the
browser, cleared on logout, and included in exports. Without `assistant` the
panel only explains how to configure it and sends no request.

Each turn runs `@gizclaw/assistant` with that key. The console implements
`AssistantRuntime` with its hash router, `useFleet`, the watch list, and
`lib/peers`: navigation changes `location.hash`, external links go through
`window.confirm`, and navigating to a device page or device logs adds an
unwatched device to the watch list. Each page publishes a structured snapshot
with `usePageView` for the assistant to read; the DOM is never read. Control
errors reach the assistant as codes such as `DEBUG_ACCESS_FORBIDDEN` or the
network failure `NETWORK_ERROR`. The panel shows every reply with each tool
action, can stop a running turn, and offers no editing or regeneration. When the
log page's initial query names `peer_public_key:`, that device is the source.
