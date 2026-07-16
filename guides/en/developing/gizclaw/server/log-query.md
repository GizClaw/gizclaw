# Log Query

`GET /logs/stream` is the admin-only system-log adapter. The host injects a `logstore.Querier`; the adapter always adds `Streams=[system]` and `Kinds=[log]`, so chat, event, and audit records in the same store are not exposed by this endpoint.

The first request requires Unix-millisecond `start_time_ms` and `end_time_ms` and may set limit, asc/desc order, and a filter. A filter is `*` or at most 32 clauses joined by uppercase `AND`:

```text
level:value
text:value
field:value
field!=value
field:*
-field:*
```

A value is either a token without whitespace, quotes, or backslashes, or a JSON string literal; decoded values cannot contain wildcards. Standard `level` names are normalized to the uppercase form emitted by `slog`. Fields follow the LogStore dotted-attribute grammar. `message`, `stream`, `kind`, and provider metadata/time fields are reserved. OR, regex, provider functions, and raw Volc expressions are rejected. Filters are limited to 4096 bytes, fields to 128 bytes, and decoded values to 1024 bytes.

The adapter parses the complete filter into a structured `logstore.Query`. Returned cursors are encrypted with a process-local key and contain the normalized query and opaque inner Store cursor; provider Context is never exposed, and cursors do not survive a Server restart. A continuation may send only the cursor and may change limit. Explicit repeated filter, time, or order fields must match the cursor or return `LOG_CURSOR_MISMATCH`.

An absent query store returns HTTP 501 `LOG_QUERY_NOT_CONFIGURED`; invalid filters and cursors return HTTP 400; store/provider failures return HTTP 502. Successful responses keep the SSE `log` and `end` events, with an `error` event for failures after streaming begins.
