# pkgs/store/logstore

`pkgs/store/logstore` provides reusable structured records, backend-neutral queries, and cursor pagination. Immutable drivers support append and query; mutable drivers additionally support replacing or deleting one record. Conversation, event, and audit producers retain ownership of authorization, retention, and canonical resources.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/logstore)

## Contract

`Appender.Append` returns a `RecordKey` for every accepted record. On a partial failure, the returned keys are the accepted prefix in input order. A key is the stable `Stream` and caller-generated `ID` pair. `ImmutableStore` combines append, query, and lifecycle capabilities. `MutableStore` extends it with `Replace` and `Delete`; callers that need mutation must resolve this capability explicitly.

`Replace` changes the record at one existing key and preserves that key. It is not an upsert and returns `ErrNotFound` when the key does not exist. `Delete` removes exactly one existing key and also returns `ErrNotFound` for a missing key.

A `Record` requires an `ID`, time, `Stream`, and `Kind`, and can carry severity, message, indexed scalar attributes, and an unindexed JSON payload. Attribute names are canonical dotted paths of at most 128 bytes; each segment matches `[A-Za-z_][A-Za-z0-9_-]*`, and scalar/object prefix conflicts are rejected.

`Query` is structured and never accepts a backend expression. Its time window is the millisecond-aligned half-open interval `[Start, End)`. Stream, kind, and severity are OR sets that are ANDed across fields; text is a case-sensitive literal phrase; attributes support `=`, `!=`, `exists`, and `not-exists`. Page limits are 1–1000. Opaque cursors bind selectors, text, time, and order while allowing a different continuation limit.

## Drivers

| Driver | Capability | Notes |
| --- | --- | --- |
| Volc TLS | `ImmutableStore` | Managed producer and SearchLogs query; mutations are unsupported |
| ClickHouse | `MutableStore` | Dedicated MergeTree table with synchronous replace and delete mutations |

Each named log store config selects exactly one driver.

### Volc TLS

```yaml
stores:
  logs:
    kind: log
    volc:
      endpoint: ${VOLC_TLS_ENDPOINT}
      region: ${VOLC_TLS_REGION}
      topic_id: ${VOLC_TLS_TOPIC_ID}
      access_key_id: ${VOLC_TLS_ACCESS_KEY_ID}
      access_key_secret: ${VOLC_TLS_ACCESS_KEY_SECRET}
```

The operator provisions the topic, logset, retention, and index. Store construction calls only `DescribeIndex`; it never calls `CreateIndex` or `ModifyIndex`. The required index disables full-text and automatic indexing and enables phrase indexing. `id`, `stream`, `kind`, and `level` are case-sensitive non-tokenized text; `msg` is case-sensitive text with an ASCII-whitespace delimiter and Chinese terms enabled; `attributes` is case-sensitive JSON with `IndexAll=true`; `payload` must remain unindexed. `DescribeIndex` may return the logical message delimiter as the literal escaped text ` \t\r\n`; the validator accepts that exact provider representation as equivalent without accepting other delimiter spellings. The operator decides whether to rebuild historical data after enabling phrase indexing on an existing topic.

See Volc TLS [CreateIndex](https://www.volcengine.com/docs/6470/112187), [query syntax](https://www.volcengine.com/docs/6470/1206705), and [phrase query](https://www.volcengine.com/docs/6470/1206697) references for the operator-owned schema and search behavior.

The provider layout uses `id`, `stream`, `kind`, `level`, and `msg`, expands dotted attributes into nested `attributes` JSON, and stores the optional payload. Before submission, the driver truncates oversized message, severity, attribute, and payload values to the producer limit while preserving `Stream`, `ID`, `Kind`, and time. JSON payloads are compacted and string values are shortened at value boundaries so the payload remains structurally valid and domain envelopes remain decodable. A record that cannot be reduced safely is rejected. `Append` returns the keys accepted by the producer, including the accepted prefix on a partial failure.

Generic records use provider source `gizclaw` and filename `logstore`; process-log `source=gizclaw` and `path=slog` remain logical attributes. Record timestamps retain nanoseconds when available, while SearchLogs ranges and ordering use milliseconds.

Queries use SearchLogs search expressions and provider Context, never SQL analysis. `Text` uses the key-value phrase form `msg:#"..."`; validated attribute names are emitted as JSON dotted paths such as `attributes.request_id`. Provider calls are capped at 30 seconds and honor shorter caller deadlines. Provider error bodies are not returned through the Store or Admin API. `Close` flushes the managed producer; the registry is its only owner.

For `Streams=[system]` and `Kinds=[log]`, the driver also includes old records whose provider source is `gizclaw` and filename is `slog`. They participate in the same provider-side ordering and cursor instead of being fetched and merged separately. This is record compatibility only; the removed Server `log` configuration remains unsupported.

### ClickHouse

```yaml
stores:
  flowcraft-history:
    kind: log
    clickhouse:
      dsn: ${CLICKHOUSE_DSN}
      database: gizclaw
      table: flowcraft_history
```

The driver creates and validates a dedicated `MergeTree` table, partitioned by month and ordered by `(timestamp, stream, id)`. `Append` serializes duplicate checks and synchronous batch insertion within one store instance, then returns keys only after commit. `Query` translates the structured contract directly to parameterized ClickHouse SQL and pages by `(timestamp, stream, id)` without a separate index. `Replace` uses a synchronous `ALTER UPDATE`, and `Delete` uses a synchronous `ALTER DELETE`; both target exactly one `(stream, id)` pair. The driver rejects duplicate keys instead of silently mutating multiple rows.

The `database` field is optional when the DSN already selects one. The ClickHouse driver does not impose an additional local payload-size limit; operators remain responsible for service limits, retention, and table policy. The named-store registry owns the connection lifecycle.

## Process logging

`system_log` controls the Server's `slog` pipeline and is not the product-record write API:

```yaml
system_log:
  level: info
  query_store: logs
  sinks:
    - kind: stderr
    - kind: store
      store: logs
    - kind: store
      store: audit-logs
      level: warn
```

Sinks run in order and may override the global level. Fanout attempts every enabled sink and joins errors. Store sinks write fixed `Stream=system` and `Kind=log` records but do not own named-store lifecycles. `query_store` must name a store sink in the same configuration; without it the Admin log endpoint returns `LOG_QUERY_NOT_CONFIGURED`. An absent `system_log` defaults to info-level stderr. The removed top-level `log` key is rejected and is not translated.
