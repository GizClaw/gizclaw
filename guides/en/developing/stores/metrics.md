# pkgs/store/metrics

`pkgs/store/metrics` provides time-series sample writes, latest/range queries, and aggregation. GizClaw uses it for Peer telemetry and Server/Admin metrics surfaces.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/metrics)

## Contract and implementations

`Store` owns the backend-neutral sample and query contract. `MemoryStore` is in-process. `PrometheusStore` uses remote write and the HTTP query API. `ClickHouseStore` owns one validated MergeTree table. Telemetry mapping, label cardinality, identity exposure, and authorization remain owned by the calling service.

In Server Config, Prometheus connection URLs, bearer token, and reusable HTTP client belong to one physical `storage.kind: prometheus` connector. A logical Metrics Store only references it:

```yaml
storage:
  monitoring:
    kind: prometheus
    remote_write_url: ${PROMETHEUS_REMOTE_WRITE_URL}
    query_url: ${PROMETHEUS_QUERY_URL}
    bearer_token: ${PROMETHEUS_BEARER_TOKEN}
stores:
  telemetry:
    kind: metrics
    storage: monitoring
services:
  metrics:
    store: telemetry
```

ClickHouse is a physical SQL connector. The DSN and pool are configured once; each logical Metrics Store selects only a table. Several Metrics or Log Stores can share the pool, and closing one logical Store never closes it.

```yaml
storage:
  analytics:
    kind: clickhouse
    dsn: ${CLICKHOUSE_DSN}
stores:
  telemetry:
    kind: metrics
    storage: analytics
    table: gizclaw_metrics
```

For local tests, configure a property-free `storage.kind: memory` and reference it from `stores.kind: metrics`. Legacy backend fields and provider connection fields under `stores` are invalid.
