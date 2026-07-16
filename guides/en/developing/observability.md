# Observability

Observability uses logs to answer what happened to one request and metrics to describe counts, latency, and current state across the system. The signals share product semantics, but not every field: logs can carry request correlation fields, while metric labels must remain low-cardinality.

## Supported signals

The server currently provides:

- a process-wide `slog` logger that writes to stderr and can fan out to Volc TLS;
- one structured completion log for GizClaw HTTP requests and Peer RPC requests that reach the first frame;
- the process-wide `gizmetrics` counter, gauge, and histogram recorder, plus a reusable `net/http` metrics wrapper;
- Admin HTTP `GET /logs/stream` for querying the configured log backend;
- `pkgs/store/metrics.Store`, including Prometheus Remote Write and Prometheus HTTP API backends;
- Peer battery, GNSS, network, and system telemetry metrics.

## Ownership

| Layer | Responsibility |
| --- | --- |
| `cmd/internal/logging` | Installs global `slog`, configures levels, and owns stderr and Volc TLS sinks. |
| `pkgs/gizclaw/internal/observability` | Owns GizClaw request dimensions, safe annotations, mutable outcomes, and their `slog` projection. |
| `pkgs/gizmetrics` | Owns the process-wide no-op default, aggregation, bounded series map, batching, and shutdown flush. |
| `pkgs/gizmetrics/httpmetrics` | Owns reusable `net/http` request count, duration, in-flight, and response-byte measurement. |
| `pkgs/store/metrics` | Persists and queries numeric samples; it does not define product metric names or labels. |
| `services/runtime/peertelemetry` | Maps Peer telemetry packets to metric names, a `peer_id` label, and values. |

GenX stream and Transformer metrics belong in `pkgs/genx`. WebRTC connection, ICE, DataChannel, packet-loss, and RTT metrics belong in `pkgs/giznet/gizwebrtc`. The generic metrics runtime does not depend on those packages.

## Request dimensions

Logs and HTTP request metrics use the same bounded meanings where a dimension applies:

| Dimension | Values or source | Contract |
| --- | --- | --- |
| `transport` | `http`, `rpc` | WebRTC signaling is an HTTP operation, not a separate transport. |
| `surface` | `server-public`, `peer-http`, `admin-http`, `peer-openai`, `edge-http`, `peer-rpc` | Identifies the GizClaw ingress surface. |
| `operation` | OpenAPI operation ID, RPC method, or an explicitly registered constant | Unknown values become `unknown`; a raw path is never a fallback. |
| `method` | Standard HTTP method | Never includes a URL; every other value becomes `OTHER`. |
| `result` | `success`, `client_error`, `server_error`, `canceled`, `panic`, `transport_error` | Describes completion without replacing the HTTP or RPC code. |
| `status_class` | `2xx`, `3xx`, `4xx`, `5xx`, `unknown` | Supports aggregation while logs retain an exact status or response code when available. |

These values form one product taxonomy. Sinks, backends, and callers must not introduce synonyms or use a surface as a transport.

## Structured request logs

### Completion record

GizClaw emits scalar attributes through the global `slog` logger. The stable completion message is `gizclaw: request completed`. HTTP handlers emit once when they return. Peer RPC emits once after the first request frame has started; clean EOF before a new request's first frame emits no request record.

Every completion record includes `transport`, `surface`, `operation`, `result`, `status_class`, and `duration_ms`.

- HTTP also includes `method`, the normalized registered `route`, and numeric `status`.
- RPC includes numeric `rpc_code` only when the response contains a code.
- Either transport can include a safe `request_id`, authenticated `peer_public_key`, known `peer_role`, and bounded `error_code`.
- Domain code may add only `workspace_name`, `workflow_name`, `model_id`, `resource_kind`, and `resource_name` through the allowlisted annotation API.

Example:

```text
time=2026-07-16T10:00:00Z level=WARN msg="gizclaw: request completed" transport=rpc surface=peer-rpc operation=server.workspace.create result=client_error status_class=4xx rpc_code=400 error_code=INVALID_WORKSPACE request_id=req-01 duration_ms=12
```

The levels are deterministic:

| Level | Completion |
| --- | --- |
| `INFO` | Ordinary 2xx/3xx success. |
| `WARN` | HTTP 4xx, cancellation, application bad-request/forbidden/not-found/conflict responses, and JSON-RPC parse/invalid-request/invalid-params/method-not-found responses. |
| `ERROR` | HTTP 5xx, JSON-RPC internal error, panic, and transport or envelope failure. |

Streaming RPC emits one completion after the full stream handler returns. It never emits per-frame, audio, event-payload, or successful-chunk records.

### Request correlation

HTTP propagates an incoming `X-Request-ID` only when it contains 1-128 characters from `[A-Za-z0-9._-]`. Missing or invalid values are replaced with a random 128-bit lowercase hexadecimal ID, returned in the response header, and exposed through CORS. If entropy fails, the response is unchanged, the ID is omitted, and a rate-limited warning is emitted.

RPC reuses `RPCRequest.Id`; an invalid or unavailable ID is omitted from logs without changing the wire request.

### Filtering and safety

`GET /logs/stream` accepts a GizClaw-owned `filter`, not a backend-native query. A filter is `*` or at most 32 clauses joined by uppercase `AND`; supported clauses are `level:value`, `text:value`, `field:value`, `field!=value`, `field:*`, and `-field:*`. For example:

```text
level:ERROR
surface:peer-rpc
operation:"server.workspace.create"
error_code:INVALID_WORKSPACE
request_id:req-01
```

Values are unquoted tokens without whitespace, quotes, backslashes, or wildcards, or JSON string literals without wildcards. Standard level names are normalized to uppercase. Fields use the LogStore dotted-attribute grammar; `message`, `stream`, `kind`, and provider metadata/time fields are reserved. OR, regular expressions, provider functions, and raw provider expressions are rejected. Filters are limited to 4096 bytes, fields to 128 bytes, and decoded values to 1024 bytes. Completion fields stay independent scalar values, so callers do not parse `message`.

Completion logs never contain authorization headers, cookies, signatures, nonces, private keys, credentials, access keys, request or response bodies, SDP, audio, images, files, prompts, conversation text, workflow events, raw URLs or queries, provider error text, validation text, or panic values. They do not emit `error_message`. Only identities already used for authorization may be recorded as `peer_public_key`.

## Metrics

### Store and process recorder

[gizmetrics Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizmetrics) · [httpmetrics Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizmetrics/httpmetrics)

`pkgs/store/metrics.Store` accepts samples with a name, labels, timestamp, and value. The Prometheus backend writes through Remote Write and queries through `/api/v1/query` and `/api/v1/query_range`. GizClaw does not use Pushgateway and does not provide a `/metrics` scrape endpoint.

Callers record process values with `AddCounter`, `SetGauge`, and `ObserveHistogram`. Before `InstallStore` succeeds and after shutdown, those calls are concurrent-safe no-ops and start no worker. Only one live recorder can be installed.

The defaults are a 10-second flush interval, a 5-second append timeout, and 10,000 logical series. `WithFlushInterval`, `WithAppendTimeout`, and `WithMaxSeries` override them. Counters keep monotonic process-local totals, gauges keep the latest value, and histograms export cumulative `_bucket`, `_sum`, and `_count` samples including `le=+Inf`.

Metric names, label names, finite values, counter deltas, and histogram buckets are validated before aggregation. Invalid updates, changed series kinds or buckets, and updates beyond the series limit are dropped with rate-limited warnings that never include label values or raw invalid metric names. Business calls only take an in-process lock and never wait for `Store.Append`; failed or timed-out dirty batches remain available for retry.

`cmd/internal/server` installs the recorder only when the `metrics` store exists. Shutdown order is `gizclaw.Server`, final recorder flush, then the store registry. The recorder never closes the store and no implicit memory store is created.

### Peer telemetry

Peer telemetry series use only the explicit `peer_id` identity label:

| Metric | Meaning |
| --- | --- |
| `gizclaw_peer_battery_percent` | Battery percentage. |
| `gizclaw_peer_battery_charging` | Charging state as 0 or 1. |
| `gizclaw_peer_battery_voltage_mv` | Battery voltage in millivolts. |
| `gizclaw_peer_gnss_latitude`, `gizclaw_peer_gnss_longitude` | GNSS coordinates in degrees. |
| `gizclaw_peer_gnss_altitude_m`, `gizclaw_peer_gnss_accuracy_m` | Altitude and accuracy in metres. |
| `gizclaw_peer_network_rssi_dbm` | Network RSSI in dBm. |
| `gizclaw_peer_network_signal_level` | Device-reported signal level. |
| `gizclaw_peer_network_connected` | Connectivity as 0 or 1. |
| `gizclaw_peer_system_uptime_seconds` | System uptime. |
| `gizclaw_peer_system_free_memory_bytes` | Free memory. |
| `gizclaw_peer_system_temperature_c` | System temperature in Celsius. |

### Reusable HTTP metrics

`httpmetrics.Wrap` records:

| Metric | Type | Labels |
| --- | --- | --- |
| `giz_http_server_requests_total` | Counter | `surface`, `operation`, `method`, `status_class`, `result` |
| `giz_http_server_request_duration_seconds` | Histogram | `surface`, `operation`, `method`, `status_class`, `result`, plus exported `le` |
| `giz_http_server_requests_in_flight` | Gauge | `surface`, `operation`, `method` |
| `giz_http_server_response_bytes_total` | Counter | `surface`, `operation`, `method`, `status_class`, `result` |

Duration buckets are `0.005`, `0.01`, `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, `5`, and `10` seconds. Methods are limited to `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`, `CONNECT`, and `TRACE`; every other value becomes `OTHER`. In-flight values aggregate across wrapper instances in the same process.

The wrapper preserves `http.Flusher`, `http.Hijacker`, `io.ReaderFrom`, and `http.Pusher` when the underlying writer supports them. It records a panic and re-panics, leaving recovery policy unchanged. The operation resolver must return a stable registered name; raw paths, queries, request IDs, peer keys, and product identifiers never become labels.

The wrapper is reusable infrastructure and does not automatically instrument every GizClaw surface. Product owners must opt in with a stable resolver. Peer RPC, GenX, and WebRTC metrics remain separate work owned by their packages.

PromQL examples:

```text
sum by (surface, operation, status_class) (
  rate(giz_http_server_requests_total[5m])
)

histogram_quantile(
  0.95,
  sum by (le, surface, operation) (
    rate(giz_http_server_request_duration_seconds_bucket[5m])
  )
)
```

## Adding instrumentation

1. Decide whether the question needs one-request evidence, an aggregate trend, or both.
2. Reuse the shared `transport`, `surface`, `operation`, and `result` taxonomy instead of creating synonyms.
3. Keep safe correlation data in logs and only low-cardinality dimensions in metrics.
4. Put generic HTTP measurement in `pkgs/gizmetrics/httpmetrics`, GizClaw product fields in `pkgs/gizclaw/internal/observability`, and GenX or WebRTC measurements in their owner packages.
5. Test success, client/server errors, cancellation, panic, streaming, backend failure, redaction, and the no-store path without changing response or lifecycle behavior.
