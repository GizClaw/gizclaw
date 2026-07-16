# pkgs/store/logstore

`pkgs/store/logstore` provides reusable append-only records, structured queries, and pagination. It is not an editable message/resource database; conversation, event, and audit producers retain ownership of authorization, retention, and canonical resources.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/logstore)

## Contract

`Appender`, `Querier`, and `Store` expose write, read, and complete lifecycle capabilities. A `Record` requires a caller-generated `ID`, time, `Stream`, and `Kind`, and can carry severity, message, indexed scalar attributes, and an unindexed JSON payload. Attribute names are canonical dotted paths of at most 128 bytes; each segment matches `[A-Za-z_][A-Za-z0-9_-]*`, and scalar/object prefix conflicts are rejected.

`Query` is structured and never accepts a backend expression. Its time window is the millisecond-aligned half-open interval `[Start, End)`. Stream, kind, and severity are OR sets that are ANDed across fields; text is a case-sensitive phrase; attributes support `=`, `!=`, `exists`, and `not-exists`. Page limits are 1–1000. Opaque cursors bind selectors, text, time, and order while allowing a different continuation limit.

## Volc TLS

Volc TLS is the only current driver:

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

The operator provisions the topic, logset, retention, and index. Store construction calls only `DescribeIndex`; it never calls `CreateIndex` or `ModifyIndex`. The required index disables full-text and automatic indexing and enables phrase indexing. `id`, `stream`, `kind`, and `level` are case-sensitive non-tokenized text; `msg` is case-sensitive text with an ASCII-whitespace delimiter and Chinese terms enabled; `attributes` is case-sensitive JSON with `IndexAll=true`; `payload` must remain unindexed. The operator decides whether to rebuild historical data after enabling phrase indexing on an existing topic.

See Volc TLS [CreateIndex](https://www.volcengine.com/docs/6470/112187), [query syntax](https://www.volcengine.com/docs/6470/1206705), and [phrase query](https://www.volcengine.com/docs/6470/1206697) references for the operator-owned schema and search behavior.

The provider layout uses `id`, `stream`, `kind`, `level`, and `msg`, expands dotted attributes into nested `attributes` JSON, and stores the optional payload unchanged. Generic records use provider source `gizclaw` and filename `logstore`; process-log `source=gizclaw` and `path=slog` remain logical attributes. Record timestamps retain nanoseconds when available, while SearchLogs ranges and ordering use milliseconds.

Queries use SearchLogs search expressions and provider Context, never SQL analysis. `Text` uses the key-value phrase form `msg:#"..."`; dynamic attribute field names are quoted before translation. Provider calls are capped at 30 seconds and honor shorter caller deadlines. Provider error bodies are not returned through the Store or Admin API. `Close` flushes the managed producer; the registry is its only owner.

For `Streams=[system]` and `Kinds=[log]`, the driver also includes old records whose provider source is `gizclaw` and filename is `slog`. They participate in the same provider-side ordering and cursor instead of being fetched and merged separately. This is record compatibility only; the removed Server `log` configuration remains unsupported.

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
