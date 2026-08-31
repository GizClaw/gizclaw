# pkgs/store/logstore

`pkgs/store/logstore` provides reusable structured records, backend-neutral queries, and cursor pagination. Immutable drivers support append and query; mutable drivers additionally support replacing or deleting one record. Conversation, event, and audit producers retain ownership of authorization and canonical resources; optional retention is configured on the logical Store and enforced by its driver without changing the LogStore interface.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/logstore)

## Contract

`Appender.Append` returns a `RecordKey` for every accepted record. On a partial failure, the returned keys are the accepted prefix in input order. A key is the stable `Stream` and caller-generated `ID` pair. `ImmutableStore` combines append, query, and lifecycle capabilities. `MutableStore` extends it with `Replace` and `Delete`; callers that need mutation must resolve this capability explicitly.

`Replace` changes the record at one existing key and preserves that key. It is not an upsert and returns `ErrNotFound` when the key does not exist. `Delete` removes exactly one existing key and also returns `ErrNotFound` for a missing key.

A `Record` requires an `ID`, time, `Stream`, and `Kind`, and can carry severity, message, indexed scalar attributes, and an unindexed JSON payload. Attribute names are canonical dotted paths of at most 128 bytes; each segment matches `[A-Za-z_][A-Za-z0-9_-]*`, and scalar/object prefix conflicts are rejected.

`Query` is structured and never accepts a backend expression. Its time window is the millisecond-aligned half-open interval `[Start, End)`. Stream, kind, and severity are OR sets that are ANDed across fields; text is a case-sensitive literal phrase; attributes support `=`, `!=`, `exists`, and `not-exists`. Page limits are 1–1000. Opaque cursors bind selectors, text, time, and order while allowing a different continuation limit.

## Drivers

| Driver | Capability | Notes |
| --- | --- | --- |
| Volc TLS | `ImmutableStore` | PutLogsV2 and SearchLogs over one TLS SDK client; mutations are unsupported |
| ClickHouse | `MutableStore` | Dedicated MergeTree table with synchronous replace and delete mutations |
| SQLite / PostgreSQL | `MutableStore` | Dedicated relational table, atomic append/replace/delete, and the shared SQL cursor |

Every logical Log Store declares `log.immutable` or `log.mutable`. `Stores.Log` accepts both declarations, while `Stores.MutableLog` accepts only `log.mutable`. Volc TLS cannot satisfy the mutable record capability required by Workspace History or Flowcraft History. Physical connection ownership remains under `storage`.

### Volc TLS

```yaml
storage:
  volc-logs:
    kind: volc-tls
    endpoint: ${VOLC_TLS_ENDPOINT}
    region: ${VOLC_TLS_REGION}
    access_key_id: ${VOLC_TLS_ACCESS_KEY_ID}
    access_key_secret: ${VOLC_TLS_ACCESS_KEY_SECRET}
stores:
  logs:
    kind: log.immutable
    storage: volc-logs
    topic_id: ${VOLC_TLS_TOPIC_ID}
```

The operator provisions the topic, logset, retention, and index. Store construction calls only `DescribeIndex`; it never calls `CreateIndex` or `ModifyIndex`. The required index disables full-text and automatic indexing and enables phrase indexing. `id`, `stream`, `kind`, and `level` are case-sensitive non-tokenized text; `msg` is case-sensitive text with an ASCII-whitespace delimiter and Chinese terms enabled; `attributes` is case-sensitive JSON with `IndexAll=true`; `payload` must remain unindexed. `DescribeIndex` may return the logical message delimiter as the literal escaped text ` \t\r\n`; the validator accepts that exact provider representation as equivalent without accepting other delimiter spellings. The operator decides whether to rebuild historical data after enabling phrase indexing on an existing topic.

See Volc TLS [CreateIndex](https://www.volcengine.com/docs/6470/112187), [query syntax](https://www.volcengine.com/docs/6470/1206705), and [phrase query](https://www.volcengine.com/docs/6470/1206697) references for the operator-owned schema and search behavior.

The provider layout uses `id`, `stream`, `kind`, `level`, and `msg`, expands dotted attributes into nested `attributes` JSON, and stores the optional payload. Before submission, the driver truncates oversized message, severity, attribute, and payload values while preserving `Stream`, `ID`, `Kind`, and time. Synchronous `PutLogsV2` calls submit sequential batches of at most 4096 records and 512 KiB. Keys are accepted only after a complete batch succeeds, so a failure returns the input prefix covered by earlier successful batches.

Generic records use provider source `gizclaw` and filename `logstore`; process-log `source=gizclaw` and `path=slog` remain logical attributes. Record timestamps retain nanoseconds when available, while SearchLogs ranges and ordering use milliseconds.

Queries use SearchLogs search expressions and provider Context, never SQL analysis. `Text` uses the key-value phrase form `msg:#"..."`; validated attribute names are emitted as JSON dotted paths such as `attributes.request_id`. Provider calls are capped at 30 seconds and honor shorter caller deadlines. Provider error bodies are not returned through the Store or Admin API. Physical Storage owns one TLS SDK client; topic-scoped Stores use that same client for reads and writes without creating a producer or second client.

For `Streams=[system]` and `Kinds=[log]`, the driver also includes old records whose provider source is `gizclaw` and filename is `slog`. They participate in the same provider-side ordering and cursor instead of being fetched and merged separately. This is record compatibility only; the removed Server `log` configuration remains unsupported.

### ClickHouse

```yaml
storage:
  analytics:
    kind: clickhouse
    dsn: ${CLICKHOUSE_DSN}
stores:
  flowcraft-history:
    kind: log.mutable
    storage: analytics
    database: gizclaw
    table: flowcraft_history
```

The driver creates and validates a dedicated `MergeTree` table, partitioned by month and ordered by `(timestamp, stream, id)`. `Append` serializes duplicate checks and synchronous batch insertion within one store instance, then returns keys only after commit. `Query` translates the structured contract directly to parameterized ClickHouse SQL and pages by `(timestamp, stream, id)` without a separate index. `Replace` uses a synchronous `ALTER UPDATE`, and `Delete` uses a synchronous `ALTER DELETE`; both target exactly one `(stream, id)` pair. The driver rejects duplicate keys instead of silently mutating multiple rows.

The logical `database` field is optional when the physical DSN already selects one. With `ttl`, the driver writes an `expires_at` value and creates or validates a table-level `TTL expires_at` deletion expression; Get and Query hide expired rows before asynchronous merges physically remove them. The ClickHouse driver does not impose an additional local payload-size limit. Logical Metrics and Log Stores may share one physical pool without owning it.

### SQLite / PostgreSQL

Each logical Store requires a dedicated `table`. The relational Store uniquely keys records by `(stream, id)`, stores time as UTC nanoseconds, and pages in stable `(time, stream, id)` order. Flat attributes use canonical JSON, while payload JSON retains its original bytes. Append rejects duplicates and writes a complete batch in one transaction. Get reads one exact `(stream, id)` key. Replace is not an upsert and cannot change time. Get and Delete return `ErrNotFound` for a missing key. Replace does not extend retention. Immutable and mutable declarations use the same implementation, but the registry exposes mutation only for `log.mutable`. Workspace History requires `MutableRecordStore`: text and structured metadata remain in LogStore, while ObjectStore contains only binary assets referenced by a record.

With `ttl`, both drivers assign `expires_at_unix_nano` from the backend write time and filter expired records from Get and Query. SQLite removes expired physical rows during subsequent appends. PostgreSQL creates the configured `table` as a range-partitioned parent over `expires_at_unix_nano`; callers always query that parent. The driver creates the required UTC daily child partition and the following day's partition during initialization and append, and drops fully expired daily partitions during the same maintenance points. A companion `<table>_keys` table preserves `(stream, id)` uniqueness across all daily partitions. Partition and key-table lifecycle is automatic and requires no caller-selected child table.

ClickHouse, SQLite, and PostgreSQL share the version-1 opaque cursor. It binds normalized selectors, text, the millisecond-aligned `[Start, End)` interval, and order while allowing a continuation to change its limit, with the same 16 KiB bound. Identical records can continue across all three SQL drivers. SQLite/PostgreSQL text matching remains case-sensitive and literal, attribute matchers run over the validated flat map, and the logical Store never closes its shared pool.

## Process logging

`system_log` controls the Server's `slog` pipeline and is not the product-record write API:

```yaml
services:
  system_log:
    level: info
    node_id: ${GIZCLAW_NODE_ID}
    query_store: logs
    sinks:
      - kind: stderr
      - kind: store
        store: logs
      - kind: store
        store: audit-logs
        level: warn
```

Sinks run in order and may override the global level. Fanout attempts every enabled sink and joins errors. A non-empty `node_id` is expanded from the environment and attached to every sink; Deploy supplies a stable logical node name and GizClaw does not infer one from network or host metadata. Stderr enables Go caller output, while Store sinks also persist the call site as `source_file` and decimal `source_line`. Store sinks write fixed `Stream=system` and `Kind=log` records but do not own named-store lifecycles. `query_store` must name a Store sink in the same configuration; without it the Admin log endpoint returns `LOG_QUERY_NOT_CONFIGURED`. An absent `services.system_log` defaults to info-level stderr. Removed top-level `log` and `system_log` keys are rejected and are not translated.
