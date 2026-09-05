# Monitor

The embedded application in `web/monitor/` uses React, TypeScript, Vite, shadcn-style components and Recharts. `DESIGN.md` defines its Claude-inspired visual language. Application assets are local; the optional location map uses an external OpenStreetMap iframe.

## Routes and authorization

Server and Edge expose `/monitor/node` and `/monitor/peer` on their existing HTTP/HTTPS listeners. No dedicated monitor listener is introduced.

GET `/monitor/api/node` reads this process only and requires `Authorization: Bearer gizclaw_mk_...`. Configure each node independently:

```yaml
monitor:
  token: ${GIZCLAW_MONITOR_TOKEN}
```

Use at least 32 characters after the prefix; `openssl rand -hex 32` generates a suitable random suffix. An empty configuration disables node data (503). Invalid tokens return 401. Responses use no-store. Connections are encrypted with a nonextractable Web Crypto key and retained in origin-local IndexedDB across reloads and navigation. Explicit logout deletes the record. This is not an OS keychain: same-origin scripts can use the key. HTTPS or localhost is required for persistence; otherwise the connection can remain temporary. Telemetry and logs are not persisted in the browser. Use the existing TLS configuration for public access.

Peer monitoring uses Edge Public APIs and the selected device's public-key bearer. The Server's ordinary business HTTP ingress remains disabled, so open peer monitoring through an Edge. Readonly permits reads; fullcontrol permits supported volume and reboot operations, with authorization rechecked by the owning Server.

## Data semantics

Node connection counts are process-local WebRTC associations, including upstream connections. Service streams are counted separately. RX/TX are cumulative WebRTC service payload bytes since process start, excluding ICE/DTLS overhead. Peer counters come from the owning Server's Runtime. Polling resumes one second after each request finishes; charts retain at most 1800 samples with 2/10/30 minute windows and clamp counter resets to zero rates. Peer uplink is Server RX and downlink is Server TX. Resume starts a fresh sample baseline.

The node log buffer retains 500 structured process records, each with at most 4096 message bytes. `/gizclaw/v1/device/logs` returns only records whose structured peer_public_key exactly matches the authorized device. These are server-side device-related logs, not firmware serial logs. Records do not survive restart. The node viewer supports filtering, follow mode and virtual scrolling. The peer runtime-log page queries persistent storage instead of this buffer.

Peer configuration shows reported info, runtime and status. Telemetry shows the latest received samples; it does not claim to expose arbitrary firmware settings. Failed requests clear actionable data and display errors.

## Build and validation

```sh
npm ci
npm run build:monitor
npm test --workspace @gizclaw/monitor
go build ./cmd/gizclaw
```

Generated `dist/` assets are ignored. A tracked `.keep` keeps pure Go builds compilable before frontend generation; such builds return 503 for the UI until assets are built and the binary is recompiled. Linux Docker and macOS release builds generate assets before Go compilation.

For frontend development, run `npm run dev --workspace @gizclaw/monitor`; APIs proxy to the local Edge at port 9821. Peer calls use the generated HTTP client, JSON is validated at runtime, and credentials never enter URLs. Set `MONITOR_PROXY` to override the development proxy target.

## HTTP contract and validation

`api/http/monitor.json` owns the independent Monitor OpenAPI surface. Server and Edge mount `pkgs/monitor.Handler` on their existing HTTP listeners. Its token middleware runs before the generated standard-library router; `nodeServer` implements the generated strict interface. The console calls its generated JavaScript client. This surface is local to each process and does not use Peer assignment or Admin authentication.

| Status | Response |
| --- | --- |
| 200 | Generated `NodeSnapshot` with local counters and bounded logs |
| 401 | `{"error":"INVALID_MONITOR_TOKEN"}` |
| 503 | `{"error":"MONITOR_DISABLED"}` when no token is configured |
| 405 | Empty body, `Allow: GET` for unsupported methods |

Every node API response includes `Cache-Control: no-store`. The JSON success/error types and Go strict server/client are generated with `go generate ./pkgs/monitor/api`. JavaScript generation runs with `npm --prefix sdk/js run gen:sdk`; configuration and committed output paths are documented in [API generation](api/generation).

`npm test --workspace @gizclaw/monitor -- polling.test.tsx` mounts the real React application with a deterministic API stub and verifies successful sampling, failure clearing the chart/timestamp, and recovery with fresh samples. `go test ./pkgs/monitor -run TestGeneratedMonitorClientContract` verifies the generated client's 200/401/503 response handling and the 405 method boundary against the actual HTTP handler.

## Peer workspace

Telemetry is rendered as numeric cards with units, observation times and current-page sample trends. Missing fields are distinct from zero; unknown fields remain visible and raw JSON is collapsed. Location shows last-reported GNSS coordinates, altitude, accuracy and timestamps. Missing or invalid coordinates do not produce a map marker. Opening Location with valid coordinates automatically loads the map without another confirmation. CSP permits only `https://www.openstreetmap.org` in `frame-src` and retains `frame-ancestors 'none'`. The provider receives the viewport and coordinates. Browser-offline mode omits the iframe and retains reported values; a persistent hint explains that values remain available if the map service cannot load.

`GET /gizclaw/v1/device/workspaces` lists explicitly Peer-owned Workspaces, including system Workspaces, grouped by Workflow. Shared and ownerless Workspaces are excluded. `GET /gizclaw/v1/device/workspaces/{workspaceId}/history` searches persisted text with cursor pagination (up to 200 entries; UI uses 100). History cursors are entry-ID timestamp boundaries, not authorization tokens; malformed values return 400 `INVALID_HISTORY_CURSOR`. Browsing does not start an Agent. The nested `/{historyId}/audio.ogg` endpoint serves retained Ogg audio through authenticated requests.

`GET /gizclaw/v1/device/logs/search` queries `services.system_log.query_store` with time bounds, text, level and cursor pagination (up to 500 records; UI uses 200). The Server binds the Peer identity on every request and rejects another Peer's cursor. Levels are strictly DEBUG/INFO/WARN/ERROR; unsupported values return 400 `INVALID_REQUEST`. Malformed and cross-Peer log cursors return 400 `INVALID_LOG_CURSOR` and `LOG_CURSOR_MISMATCH`. An unconfigured store returns 500 `LOG_QUERY_NOT_CONFIGURED`. Logs use compact single-line time/level/module/message/error rows with horizontal scrolling.

These generated Peer APIs use existing Edge routing and authoritative runtime authorization. Readonly permits reads. Workflow specs and provider credentials are not exposed. The source contract is `api/http/peer.json`.
