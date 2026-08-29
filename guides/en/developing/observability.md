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

## Persistent Go runtime profiles

Process profiling is opt-in and owned by `cmd/internal/server`. It does not
register `net/http/pprof` or expose `/debug/pprof`. Configure a dedicated
logical ObjectStore that is not shared with Workspace assets, Gameplay assets,
or Agent Host runtime data:

```yaml
storage:
  profile-files:
    kind: filesystem.dir
    dir: data/profiles
stores:
  runtime-profiles:
    kind: objectstore
    storage: profile-files
    prefix: pprof
profiling:
  enabled: true
  store: runtime-profiles
```

Absent configuration, `{}`, and `enabled: false` perform no Store operation and
start no worker. A named Store is still validated while disabled. When enabled,
the command publishes one baseline before opening listeners, then waits five
minutes after each completed attempt before starting the next. Attempts never
overlap or catch up. Shutdown cancels the next wait, lets an active attempt
finish or clean up, joins the worker, and only then closes logging and Stores;
there is no shutdown snapshot.

Completed evidence has this layout:

```text
runs/<UTC timestamp>-pid-<pid>/
  000000-baseline/{heap.pprof,allocs.pprof,goroutine.pprof,manifest.json}
  000001-<UTC timestamp>/{heap.pprof,allocs.pprof,goroutine.pprof,manifest.json}
```

Each profile is streamed directly from `runtime/pprof`; `manifest.json` is
written last and records the run, sequence, capture time, size, and SHA-256 of
all three files. Only a valid manifest marks a completed set. Failed attempts
remove their recognizable partial objects best-effort. If cleanup fails, the
next attempt retries that exact prefix before uploading anything new. Startup
keeps sets from older runs only after streaming every referenced profile and
verifying its size and SHA-256, removes recognizable manifest-less sets, and
fails safely on malformed manifests or unrecognized names instead of deleting
unknown data.

Retention covers all runs and includes baselines: at most 576 completed sets
and 1 GiB of profile bytes. A candidate also has one shared 1 GiB streaming
limit. Rotation removes the oldest manifest first, then its profile objects, so
readers never mistake a partly deleted set for completed evidence. Periodic
failures produce one structured warning and retry after the next five-minute
wait; baseline failure aborts startup.

Profiles contain package, function, and source/build-path metadata. Use an
operator-only bucket/container and prefix, never make it public or serve it as
an asset, and transfer files through an access-controlled channel. After a safe
download, common analyses are:

```sh
go tool pprof -top heap.pprof
go tool pprof -top -inuse_space heap.pprof
go tool pprof -top -alloc_space allocs.pprof
go tool pprof -top goroutine.pprof
go tool pprof -top -base baseline/heap.pprof later/heap.pprof
```

Comparisons should use profiles from the same build where possible. A retained
profile is diagnostic evidence, not by itself proof of a leak or outage cause.

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

`server.speech.extract` uses the same completion record to expose only a closed stage/class code. Its stages are request, ASR, Extract Provider, result parsing, schema validation, and response encoding. Raw provider errors and request/result content remain excluded even when the wire response is a generic internal error.

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

Logs never contain authorization headers, cookies, signatures, nonces, private keys, credentials, access keys, SDP, raw URLs or queries, provider error text, validation text, or panic values. User-to-AI input, final ASR transcripts, and the AI response content actually delivered to the user are outside this prohibition. Completion records do not emit `error_message`. Only identities already used for authorization may be recorded as `peer_public_key`.

## Metrics

### Store and process recorder

[gizmetrics Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizmetrics) · [httpmetrics Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizmetrics/httpmetrics)

`pkgs/store/metrics.Store` accepts samples with a name, labels, timestamp, and value. The Prometheus backend writes through Remote Write and queries through `/api/v1/query` and `/api/v1/query_range`. GizClaw does not use Pushgateway and does not provide a `/metrics` scrape endpoint.

Callers record process values with `AddCounter`, `SetGauge`, and `ObserveHistogram`. Before `InstallStore` succeeds and after shutdown, those calls are concurrent-safe no-ops and start no worker. Only one live recorder can be installed.

The defaults are a 10-second flush interval, a 5-second append timeout, and 10,000 logical series. `WithFlushInterval`, `WithAppendTimeout`, and `WithMaxSeries` override them. Counters keep monotonic process-local totals, gauges keep the latest value, and histograms export cumulative `_bucket`, `_sum`, and `_count` samples including `le=+Inf`.

Metric names, label names, finite values, counter deltas, and histogram buckets are validated before aggregation. Invalid updates, changed series kinds or buckets, and updates beyond the series limit are dropped with rate-limited warnings that never include label values or raw invalid metric names. Business calls only take an in-process lock and never wait for `Store.Append`; failed or timed-out dirty batches remain available for retry.

`cmd/internal/server` installs the recorder only when the `metrics` store exists. Shutdown order is `gizclaw.Server`, final recorder flush, then the store registry. The recorder never closes the store and no implicit memory store is created. Standalone Edge connects its top-level `metrics` configuration directly to a Prometheus Remote Write/query backend; shutdown final-flushes after HTTP, gateway, and upstream teardown, then closes the backend connector.

### WebRTC and Edge metrics

The WebRTC transport records signaling, dials, connections, and service DataChannel request counts, outcomes, latency, and active gauges. Its bounded labels are `node_role=application|edge`, `role=client|server`, `direction=inbound|outbound`, and stable `result` values. Public keys, session IDs, service IDs, URLs, SDP, and content never become metric labels.

The Edge gateway additionally records every ingress signaling request, including capacity rejection before the WebRTC listener, pending admissions, active logical sessions, burst SCTP use, upstream state/load, tunnel channels, logical-session establishment, and bridge terminals. The families use `giz_webrtc_*` and `giz_edge_webrtc_*`; `giz_edge_webrtc_capacity_limit{resource=...}` publishes configured ceilings corresponding to the active gauges.

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

The wrapper is reusable infrastructure and does not automatically instrument every GizClaw surface. Product owners must opt in with a stable resolver. WebRTC transport uses the separate `giz_webrtc_*` families; Peer RPC and GenX remain outside this HTTP wrapper.

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

## Edge upstream ICE observation

After a live Edge upstream is owned, `edge: upstream ICE selected` correlates
the selected pair with `upstream_kind=control|gateway`, a bounded upstream ID,
and connection epoch. Candidate fields are limited to type, UDP/TCP protocol,
IPv4/IPv6 family, component, state, nomination, supported counters, and an
optional zero-based relay-member ordinal. These fields have bounded
cardinality.

The log and derived capacity artifact must never include IP addresses, ports,
TURN URLs, SDP, candidate IDs or bodies, foundations, priorities, usernames,
credentials, or mutable Pion values. Absence of a selected pair is a warning;
configuration alone must not be reported as proof that relay was used.

The 2026-08-04 local qualification combined these selected-pair records with
exact Coturn allocation and traffic counters. All 12 product runs proved the
requested path; a same-head pure-Giznet lane then reproduced direct 818/798
Mbps versus REST Coturn 488/526 Mbps while Coturn counters grew by about
220/219 MB. Because that diagnostic excludes the product Edge and Server, the
material product delta is assigned to the local Coturn relay path rather than
an Edge/Server resource owner. The counters support this bounded causal claim;
configuration alone would not.

## Adding instrumentation

1. Decide whether the question needs one-request evidence, an aggregate trend, or both.
2. Reuse the shared `transport`, `surface`, `operation`, and `result` taxonomy instead of creating synonyms.
3. Keep safe correlation data in logs and only low-cardinality dimensions in metrics.
4. Put generic HTTP measurement in `pkgs/gizmetrics/httpmetrics`, GizClaw product fields in `pkgs/gizclaw/internal/observability`, and GenX or WebRTC measurements in their owner packages.
5. Test success, client/server errors, cancellation, panic, streaming, backend failure, redaction, and the no-store path without changing response or lifecycle behavior.

## Peer stream lifecycle and conversation content

`gizclaw: peer stream lifecycle` correlates a direct or Edge-routed logical Peer from Server input through Agent output; the Edge path also covers gateway admission. The authenticated logical identity is `peer_public_key`, and Edge plus Server share one `tunnel_session_id`. Connection-level `component`, `stage`, `result`, `reason`, `last_stage`, and `duration_ms` records remain available. A connection terminal also contains `input_event_observed`, `agent_input_opened`, `agent_input_pushed`, and `output_event_observed` so a zero-event connection failure remains distinguishable.

The Edge `bridge_started` terminal keeps the first connection-level bridge path,
direction, phase, and closed error class. Destination-open failures are folded
into one count with first/last direction and class. Exact established-session or
association capacity ownership adds `bridge_capacity_scope`,
`bridge_active_channels`, and `bridge_channel_limit`; absent exact ownership,
those fields are omitted rather than inferred. All bridge dimensions are
top-level scalar log fields, never metric labels, and no per-service record or raw
error is emitted.

Each authorized input BOS allocates a positive, monotonically increasing `turn_index` within the Peer connection. Edge queries use `(tunnel_session_id, turn_index)` and direct queries use `(peer_public_key, turn_index)`; these fields are never placed on the wire or used as metric labels. Input and assistant output identifiers are independent: their safe correlation fields are `input_stream_id_hash` and `output_stream_id_hash`. Output binds only through the producer response epoch's immutable owning input route; there is no current-turn, timing, Workspace, or output-ID fallback. Replaced turns remain boundedly retained so an old epoch's first late chunk and terminal stay on the original turn. Output without provenance remains unowned by per-turn records.

The stable `gizclaw: AI conversation content` message records actual dialogue content for the turn. Every record contains `peer_public_key`, `tunnel_session_id` when available, `turn_index`, `content_role=user|assistant`, a zero-based per-role `content_index`, `content_source`, `event_type`, and the original `content`. Direct text is recorded with `content_source=agent_input` only after authorized input enters the Agent queue. Final ASR transcript text is recorded with `content_source=final_transcript` when the Agent produces it. AI text is recorded with `content_source=peer_delivery` only after the corresponding Peer event broadcasts successfully; generated but undelivered text is not recorded as a reply. Within one `content_source`, sorting and concatenating `content` by `(peer_public_key, tunnel_session_id?, turn_index, content_role, content_index)` reconstructs that direct utterance, final transcript, or delivered response without conflating the direct-input and ASR representations.

The bounded per-turn stages are `turn_started`, `input_first_event`, `input_terminal`, `interrupt_observed`, `agent_input_first_push`, `agent_transform_started`, `agent_output_produced`, `output_first_event`, `agent_output_delivered`, `agent_terminal`, `output_terminal`, and `turn_terminal`. Turn boundaries use `component=peer_turn`, transport input and output stages keep `component=peer_input|agent_output`, and the four Agent boundary stages use `component=agent_runtime`. Every applicable stage is emitted at most once per turn. The first produced and delivered records contain one closed `output_modality` value: `transcript_text`, `assistant_text`, `assistant_audio`, `assistant_eos`, `interrupt`, `control`, or `other`; later chunks only update the bounded terminal snapshot.

`agent_transform_started` means the selected transformer consumed the input, not merely that the Peer queue accepted it. `agent_output_produced` classifies GenX source chunks before independent consumer scheduling. `agent_output_delivered` classifies actual successfully broadcast Peer events; audio additionally waits for mixer drain. A failed broadcast, failed or abandoned drain, or suppressed aggregate boundary does not add a delivered modality. Empty-label text and blob events use the same assistant fallback as the Peer client, while an empty-label control-only EOS remains `other`.

`agent_terminal.terminal_class` is one of `completed`, `interrupted`, `provider_error`, `transform_error`, `stream_error`, `caller_canceled`, or `deadline_exceeded`. `turn_terminal` adds `agent_transform_started`, `agent_terminal_observed`, `produced_modalities`, `delivered_modalities`, and five sorted unique class sets: `source_part_classes` (`text`, `audio`, `control`, `other`), `source_label_classes` and `peer_event_label_classes` (`assistant`, `transcript`, `history`, `empty`, `other`), `peer_event_types` (`bos`, `eos`, `text_delta`, `text_done`), and `peer_event_kinds` (`text`, `audio`, `video`, `mixed`, `unspecified`). This distinguishes zero output, transcript-only, audio-only, EOS/interruption-only, Agent failure, and downstream delivery failure without logging raw labels or payloads. Closed `result` values are `success`, `replaced`, `interrupted`, `canceled`, `timeout`, `closed`, `runtime_error`, and `incomplete`; closed terminal or interruption `reason` values are `completed`, `input_replaced`, `control_interrupt`, `expected_interruption`, `caller_canceled`, `deadline_exceeded`, `stream_closed`, `internal_error`, and `state_limit`. Raw errors are never copied.

Lifecycle-stage volume is bounded by turns times this fixed stage set, not by packets, audio frames, text deltas, or control fragments. Conversation-content records grow with text chunks so lifecycle state never caches unbounded content. Active and recently replaced state is capped, completed state is released, and connection teardown emits one terminal summary for every retained incomplete turn before clearing the correlation maps. Instrumentation does not block, retry, reorder, or alter Peer, AgentHost, provider, interruption, timeout, or cleanup behavior.

The Server constructs the connection, turn, and Agent-runtime observer for every direct or Edge logical Peer. Conversation audit content is not optional and has no independent disable setting. Log sinks still own persistence and level policy, but the runtime no longer skips content-correlation state because `INFO` was filtered when the connection started.

The existing `gizclaw: assistant route failed` Error record continues to keep bounded route, Workspace, and error fields. It is not a conversation-content record and cannot replace the per-turn actual-delivery reply.

Untrusted stream identifiers use a stable 128-bit hash; raw `stream_id` values are never logged. The hash contract trims leading and trailing Unicode whitespace, UTF-8 encodes the result, applies unkeyed SHA-256, keeps the first 16 digest bytes, and emits 32 lowercase hexadecimal characters. Empty normalized IDs are omitted. It performs no case folding or Unicode normalization and uses no salt or HMAC key. For example, `stream-42` maps to `0f3a788cbbee0b932cfcac7d71645f31`. This is a stable correlation token that avoids accidental raw-value disclosure, not an anonymization boundary: low-entropy IDs are dictionary-testable, so producers must not put credentials or secrets in stream IDs. Session, turn, Peer, Workspace, and stream identifiers remain log-only dimensions and must never become metric labels. Lifecycle records must not contain remote addresses, SDP, ICE candidate bodies, credentials, raw provider errors, or panic values; this restriction does not prohibit recording user-to-AI input, final ASR transcripts, or the AI response content actually delivered to the user.
