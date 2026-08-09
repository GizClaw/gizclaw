# pkgs/store/metrics

`pkgs/store/metrics` 提供时序 sample 写入、latest/range query 与 aggregation。GizClaw 用它保存 Peer telemetry，并支持 Server/Admin metrics surface。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/metrics)

## Contract 与实现

`Store` 拥有 backend-neutral sample 与 query contract。`MemoryStore` 是进程内实现；`PrometheusStore` 使用 remote write 和 HTTP query API；`ClickHouseStore` 拥有一张经过校验的 MergeTree table；`SQLStore` 借用 SQLite/PostgreSQL pool 和一张独立 table。Telemetry mapping、label cardinality、identity exposure 与 authorization 仍由调用 service 拥有。

在 Server Config 中，Prometheus URL、bearer token 与可复用的官方 `api.Client` 属于一个物理 `storage.kind: prometheus` connector。查询和 remote write 都通过这个 client 的 `Do`，逻辑 Metrics Store 只借用它：

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

ClickHouse 是物理 SQL connector。DSN 与 pool 只配置一次；每个逻辑 Metrics Store 只选择 table。多个 Metrics 或 Log Store 可以共享 pool，关闭其中一个逻辑 Store 不会关闭 pool。

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

本机测试可配置一个无属性的 `storage.kind: memory`，再让 `stores.kind: metrics` 引用它。每个逻辑 Store 会得到独立的 `MemoryStore`；memory marker 本身不缓存实例。旧 backend 字段和 `stores` 下的 provider connection 字段无效。

SQLite/PostgreSQL table 以 signed nanoseconds 保存 UTC time，以 IEEE-754 bits 保存所有 `float64` 值，并用自增 sequence 明确相同 timestamp 的最后写入顺序。Label 使用 collision-safe series key 和 canonical flat JSON；nil/empty labels 属于同一 series。`Append` 是完整 batch transaction，Latest、Range 与 Aggregate 保持相同 selector、boundary、window 和 ordering contract；Regexp 与 aggregation 可以在 Go 内执行以保证两种方言一致。逻辑 Store 不拥有 pool。
