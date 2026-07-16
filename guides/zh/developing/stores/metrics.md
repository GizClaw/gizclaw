# pkgs/store/metrics

`pkgs/store/metrics` 提供 backend-neutral 的 time-series sample 写入和查询 abstraction。GizClaw 使用它保存 Peer telemetry，并通过 Server/Admin surface 执行 latest、range 和 aggregation 查询。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/metrics)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `Store` | 定义 sample 写入以及 latest、range、aggregate 查询。 |
| `Sample` / `Point` / `Series` / `SeriesSet` | 表达输入 sample 与查询结果。 |
| `Selector` / `LabelMatcher` | 描述 metric name 与 label filtering。 |
| `LatestQuery` / `RangeQuery` / `AggregateQuery` | 使用 `Selector` 描述查询时刻、时间区间、step、bucket 和聚合操作。 |
| `Aggregation` | 描述 `avg`、`min`、`max`、`sum`、`count` 和 `last` 聚合。 |
| `MemoryStore` | 提供进程内 time-series 实现。 |
| `PrometheusStore` | 通过 Prometheus-compatible API 写入和查询 metrics。 |
| `ClickHouseStore` | 通过官方 ClickHouse Go driver 的 `database/sql` 接口批量写入和查询 metrics。 |
| `ValidateMetricName` / `ValidateLabelName` | 校验 metric 与 label contract。 |

## Ownership 边界

Metrics package 不拥有 GizClaw telemetry event schema。Telemetry packet 到 metric name、labels 和 sample value 的映射属于 `services/runtime/peertelemetry`。调用方负责控制 label cardinality、身份信息暴露和 query authorization。

PromQL 是 `PrometheusStore` 的私有实现细节，service 不应构造或解析 PromQL。`MemoryStore`、`PrometheusStore` 和 `ClickHouseStore` 对外实现相同的时间边界、空窗口和 label matcher 语义。

## 配置

每个 `kind: metrics` store 必须且只能选择一个 backend：

```yaml
stores:
  telemetry:
    kind: metrics
    clickhouse:
      dsn: clickhouse://clickhouse:9000/gizclaw
      table: metrics
```

进程内 store 可使用 `memory: {}`；旧配置 `backend: memory` 仍受支持。Prometheus 配置保持 `prometheus.remote_write_url`、`prometheus.query_url` 和可选 `prometheus.bearer_token`。

ClickHouse backend 会创建单机 `MergeTree` 表并检查列类型。表按月分区，排序键为 metric、确定性 series identity 和 timestamp。写入使用一个 driver batch，并启用 `async_insert=1` 与 `wait_for_async_insert=1`，使服务端 batching 的 flush 错误同步返回。当前 contract 不包含 schema migration、TTL 或 cluster topology 管理。
