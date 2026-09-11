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
logical ObjectStore that is not shared with Workspace assets or Agent Host
runtime data:

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
| `pkgs/gizlog` | Installs global `slog`, configures levels, and owns stderr and named LogStore sinks shared by Server and Edge. |
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

## Structured logs

Code continues to use the global `slog` logger and should prefer
`slog.LogAttrs(ctx, ...)` for scalar attributes. Configured loggers add Go
caller metadata to stderr and Store sinks; Store records expose it as
`source_file` and decimal `source_line`. When `system_log.node_id` is set,
every sink also receives that exact `node_id`. Deploy injects a stable logical
node name; GizClaw never derives one from an IP address, hostname, endpoint, or
public key.

AgentHost binds the authenticated Peer `peer_public_key` to the Agent execution
context. Provider records emitted through context-aware `slog` calls inherit
that field. Startup, storage, and other process-global records without a Peer
owner omit it instead of fabricating an identity. Admin `GET /logs/stream`
returns `node_id`, `source_file`, `source_line`, and applicable
`peer_public_key` values in the entry's scalar `fields` map without changing
the streaming schema.

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

HTTP completion also records `request_path` without query strings (up to 1024 bytes), `client_ip`, and `user_agent` (up to 512 bytes). Unmatched routes retain their actual path for 404 diagnosis. Public Edge derives the IP from its socket and overwrites the internal forwarding header; Server accepts that header only on the authenticated Edge service. Other `X-Forwarded-For` / `X-Real-IP` values are not trusted; an additional proxy in front of ingress appears as the socket peer. These values are log-only, never metric labels. Console uses the same HTTP method, actual path, IP, status, and duration summary for registered and unmatched routes. Business operation, route template, and User-Agent remain in record details. Historical records without an actual path fall back to the stored route.

HTTP ingress generates a random 128-bit lowercase hexadecimal `request_id`, overwrites client `X-Request-ID`, and returns it through the response header and CORS. Only the authenticated internal Edge HTTP service accepts an Edge-generated ID. Entropy failure rejects the request with HTTP 500. After API Key authentication, Context and completion logs carry the owner `peer_public_key` and resource `api_key_name`, never the bearer secret. Server returns authenticated identity through internal response headers for Edge completion; Edge strips those headers before responding to the client. Pre-authentication failures do not invent credential identity.

Public Peer RPC also generates a server-owned log ID; `RPCRequest.Id` remains unchanged for wire response matching. The authenticated internal Edge control RPC uses that field to carry an Edge-generated ingress ID, accepted only on that service, so route resolution before HTTP forwarding shares the trace. Device logs carry `session_id`: Edge logical sessions share their tunnel ID with Server, and direct connections generate an independent ID. Initial Edge handshake retries still create distinct logical session IDs. Persistent Agent runtimes retain connection identity, while a startup record links the reload request; later speech does not inherit that request ID or credential. Context propagates locally, not automatically across process boundaries or pooled physical connections.

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

## Edge upstream liveness

A healthy ICE pair does not prove a live upstream: ICE consent only shows that
the Server's UDP socket answers STUN, while Pion SCTP retransmits forever and
a DataChannel opens locally without a peer acknowledgement. The Edge therefore
probes every control and gateway upstream end to end with `GET /server-info`
over the Server's Edge HTTP service every 10 seconds (2 second timeout). The
control upstream skips a periodic probe after recent response headers and
probes immediately when a forwarded request waits 1 second for headers.

A probe that times out while the association received no other inbound data
logs `edge: upstream stalled` (`trigger=periodic|slow_request`, `probe_ms`,
`last_activity`) and `edge: upstream evicted` (`reason=liveness_probe_failed`).
Eviction closes the association, which fails its in-flight requests; `GET`,
`HEAD` and `OPTIONS` are retried on a fresh association. A timed-out probe
while other data still arrives is only `edge: upstream slow` at info level.
Replacement is logged as `edge: upstream redialing` followed by
`edge: upstream ICE selected` with a new epoch; failures log
`edge: upstream redial failed` with an exponential `retry_in` capped at 30
seconds. Every proxy failure logs `gizedge: upstream proxy error` with its
cause, at info level when the client had already gone away.

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

The shared `pkgs/genx/streamlog` observer emits `genx: stream` records with scope `peer_public_key → session_id → stream_id → role/boundary/segment_index`. It is installed at Peer boundaries and at the native ASR, TTS, Realtime, Eino, Flowcraft, and Audio Dock Transformer entries. Direct typed calls do not require Peer or Mux. Each invocation has independent `transformer_input` and `transformer_output` recorders, with a `transformer` implementation field. Output observation is at the stage pull boundary, not delivery acknowledgement; optional stream interfaces and interruption semantics remain intact. Normal producer closure preserves queued output logging; terminal reads and effective aborts flush unfinished text. Internal composition transfer queues do not create duplicate stage recorders. `agent_input`, `model_output`, and `peer_delivery` distinguish input, generation, and delivery. A native production callback observes output before enqueueing; streams without that protocol are observed when the consumer reads them. Delivery and audio drain do not prove device playback.

Events are `stream_start`, nonempty `first_text` / `first_audio`, accumulated `text`, and `stream_end`. First text includes the first nonempty fragment; first audio includes MIME and frame size, not binary content or inferred syllables. Text flushes at sentence punctuation/newlines, EOS, error, cancellation, and runtime termination. Retention is bounded to 64 MIME routes and 4096 unfinished text bytes per route; long text flushes on UTF-8 boundaries, and eviction records its terminal reason. Reload flushes old routes without disabling observation of the replacement runtime.

`started_at`, `observed_at`, and `ended_at` are absolute timestamps. `duration_ms` starts at the first observed output-route chunk. When input provenance is known, `input_started_at` and `input_elapsed_ms` provide response latency; after input EOS, `input_ended_at` and `after_input_end_ms` measure post-submission delay. Use `input_elapsed_ms` for first text/audio latency, not output duration, which can be zero. For PTT final transcripts, post-input delay measures recognition waiting, not provider-internal compute. Eino/Flowcraft explicitly link input and output Stream IDs when creating a response; this is logging metadata and does not change ResponseEpoch or interruption ownership. Unowned output never inherits the latest input timing. The `genx_input_to_first_output_seconds` histogram uses the installed metrics store via `gizmetrics`, with only `boundary`, `event`, and `role` labels.

The bounded per-turn stages are `turn_started`, `input_first_event`, `input_terminal`, `interrupt_observed`, `agent_input_first_push`, `agent_transform_started`, `agent_output_produced`, `output_first_event`, `agent_output_delivered`, `agent_terminal`, `output_terminal`, and `turn_terminal`. Turn boundaries use `component=peer_turn`, transport input and output stages keep `component=peer_input|agent_output`, and the four Agent boundary stages use `component=agent_runtime`. Every applicable stage is emitted at most once per turn. The first produced and delivered records contain one closed `output_modality` value: `transcript_text`, `assistant_text`, `assistant_audio`, `assistant_eos`, `interrupt`, `control`, or `other`; later chunks only update the bounded terminal snapshot.

`agent_transform_started` means the selected transformer consumed the input, not merely that the Peer queue accepted it. `agent_output_produced` classifies GenX source chunks before independent consumer scheduling. `agent_output_delivered` classifies actual successfully broadcast Peer events; audio additionally waits for mixer drain. A failed broadcast, failed or abandoned drain, or suppressed aggregate boundary does not add a delivered modality. Empty-label text and blob events use the same assistant fallback as the Peer client, while an empty-label control-only EOS remains `other`.

`agent_terminal.terminal_class` is one of `completed`, `interrupted`, `provider_error`, `transform_error`, `stream_error`, `caller_canceled`, or `deadline_exceeded`. `turn_terminal` adds `agent_transform_started`, `agent_terminal_observed`, `produced_modalities`, `delivered_modalities`, and five sorted unique class sets: `source_part_classes` (`text`, `audio`, `control`, `other`), `source_label_classes` and `peer_event_label_classes` (`assistant`, `transcript`, `history`, `empty`, `other`), `peer_event_types` (`bos`, `eos`, `text_delta`, `text_done`), and `peer_event_kinds` (`text`, `audio`, `video`, `mixed`, `unspecified`). This distinguishes zero output, transcript-only, audio-only, EOS/interruption-only, Agent failure, and downstream delivery failure without logging raw labels or payloads. Closed `result` values are `success`, `replaced`, `interrupted`, `canceled`, `timeout`, `closed`, `runtime_error`, and `incomplete`; closed terminal or interruption `reason` values are `completed`, `input_replaced`, `control_interrupt`, `expected_interruption`, `caller_canceled`, `deadline_exceeded`, `stream_closed`, `internal_error`, and `state_limit`. Raw errors are never copied.

Lifecycle-stage volume is bounded by turns times this fixed stage set, not by packets, audio frames, text deltas, or control fragments. Conversation-content records grow with sentences and fixed boundary events; unfinished text has a fixed memory bound. Active and recently replaced state is capped, completed state is released, and connection teardown emits one terminal summary for every retained incomplete turn before clearing the correlation maps. Observation does not retry or reorder stream data or own Peer, AgentHost, provider, interruption, timeout, or cleanup behavior. Log sinks retain their normal synchronous handling semantics.

The Server constructs the connection, turn, and Agent-runtime observer for every direct or Edge logical Peer. Conversation audit content is not optional and has no independent disable setting. Log sinks still own persistence and level policy, but the runtime no longer skips content-correlation state because `INFO` was filtered when the connection started.

The existing `gizclaw: assistant route failed` Error record keeps bounded route and Workspace fields plus the terminal failure itself: `error_code` (`STREAM_ERROR` when the producer set none), `retryable`, the producer's raw `error` message when it is non-empty, and `failure_class` (`provider` or `transform`) when the chunk carries one. Copying the raw message here is deliberate: without it an operator sees only that a turn failed, never why. Producers must not put credentials or secrets in terminal error text, exactly as they must not put them in stream IDs. It remains an operational failure record, not a conversation-content record, and cannot replace the per-turn actual-delivery reply.

Lifecycle diagnostic stream identifiers retain their stable 128-bit hash; GenX content records use actual `stream_id` values to match device events. The hash contract trims leading and trailing Unicode whitespace, UTF-8 encodes the result, applies unkeyed SHA-256, keeps the first 16 digest bytes, and emits 32 lowercase hexadecimal characters. Empty normalized IDs are omitted. It performs no case folding or Unicode normalization and uses no salt or HMAC key. For example, `stream-42` maps to `0f3a788cbbee0b932cfcac7d71645f31`. This is a stable correlation token that avoids accidental raw-value disclosure, not an anonymization boundary: low-entropy IDs are dictionary-testable, so producers must not put credentials or secrets in stream IDs. Session, turn, Peer, Workspace, and stream identifiers remain log-only dimensions and must never become metric labels. Lifecycle records must not contain remote addresses, SDP, ICE candidate bodies, credentials, raw provider errors, or panic values; the `gizclaw: assistant route failed` record is not a lifecycle record and does carry the raw terminal error; this restriction does not prohibit recording user-to-AI input, final ASR transcripts, or the AI response content actually delivered to the user.

### Logging audit scope

Runtime logs use the calling context to retain authenticated request or connection identity. SFU participant and talk events use the attachment context. Physical upstream ICE, channel capacity, and startup logs use node, upstream ID, and connection epoch dimensions rather than one request sharing the connection. StreamBuilder no longer warns per fragment about unbound tools; actual invocation still returns an explicit missing-tool error. Provider text-send fragments are covered by the shared stage aggregation instead of separate per-fragment logs.

Internal Stream ID remapping retains process-local `source_stream_id` provenance. It contributes input latency only when it exactly matches an observed input; it neither changes ResponseEpoch ownership nor enters the device wire protocol.

### Speech and model latency metrics

These histograms use seconds and the configured `gizmetrics` store. Query `_count`, `_sum`, and cumulative `_bucket` series for counts, means, and quantiles. First-output observations require nonempty content and occur once per actual request or output stream. Empty results, unknown input ownership, and missing first output never produce synthetic zero samples.

| Metric | Start → end |
| --- | --- |
| `genx_asr_first_text_seconds` | First nonempty input audio → first nonempty transcript |
| `genx_asr_final_result_seconds` | Input EOS → successful nonempty transcript stream EOS |
| `genx_model_first_text_seconds` | Actual streaming model request → first nonempty reply text |
| `genx_model_request_duration_seconds` | Actual streaming model request → completion, including errors and cancellation |
| `genx_tts_first_audio_seconds` | First nonempty input text → first nonempty audio chunk |
| `genx_realtime_first_text_seconds` | First nonempty input → first realtime reply text |
| `genx_realtime_first_audio_seconds` | First nonempty input → first realtime reply audio |
| `memory_recall_duration_seconds` | Recall call → returned result, including retrieval, filtering, and hydration |
| `genx_input_end_to_first_output_seconds` | Known owning input EOS → first text or audio output |

Streaming OpenAI-compatible and Gemini generators measure actual provider calls, including those made by Eino and Flowcraft, excluding preceding recall, tools, and workflow preparation. Native speech observation covers Doubao ASR, AST, TTS, realtime/duplex, DashScope realtime, and MiniMax TTS. ASR first text means the first transcript exposed by the Transformer; final-only modes do not measure the provider's first interim hypothesis. Realtime latency includes input duration and is distinct from text-model TTFT. First audio means server-observed data, not device playback.

Model labels are `provider` and `model`; speech also uses `mode` (`asr`, `ast`, `tts`, `realtime`). Models are configured provider model, version, or speech resource IDs, never workflow names, voice IDs, or request-supplied values. Recall uses `backend`. Flowcraft adds `embedding_model_ref` and `rerank_model_ref`: configured resource references, not resolved upstream model IDs. Mem0 adds `flavor`; unknown remote model identities are omitted. Request duration and Recall use `result`: `success`, `error`, `timeout`, or `canceled`.

End-to-end series use only `boundary`, `event`, and `role`, separating model production from Peer delivery. They remain distinct from input-start-based `genx_input_to_first_output_seconds`. Output preceding input EOS has no EOS-based sample. Public keys, API key names, session/request/stream IDs, and text remain log-only dimensions.

`tests/gizclaw-e2e/run_observability_tests.sh` runs Eino text, Doubao realtime PTT, and Flowcraft voice Giztests, then queries collected metrics to validate required series, model labels, counts, seconds, and cumulative buckets.
