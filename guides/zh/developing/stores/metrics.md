# pkgs/store/metrics

`pkgs/store/metrics` 提供 time-series sample 写入和查询 abstraction。GizClaw 使用它保存 Peer telemetry，并通过 Server/Admin surface 执行 instant query、range query 和 aggregation。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/metrics)

## 核心结构与实现

| 符号 | 作用 |
| --- | --- |
| `Store` | 定义 sample 写入、instant query 和 range query。 |
| `Sample` / `Point` / `Series` / `SeriesSet` | 表达输入 sample 与查询结果。 |
| `Selector` / `LabelMatcher` | 描述 metric name 与 label filtering。 |
| `Query` / `RangeQuery` | 描述查询时刻、时间区间、step 和 expression。 |
| `Aggregation` / `AggregateExpression` | 构造受支持的聚合 expression。 |
| `MemoryStore` | 提供进程内 time-series 实现。 |
| `PrometheusStore` | 通过 Prometheus-compatible API 写入和查询 metrics。 |
| `ValidateMetricName` / `ValidateLabelName` | 校验 metric 与 label contract。 |

## Ownership 边界

Metrics package 不拥有 GizClaw telemetry event schema。Telemetry packet 到 metric name、labels 和 sample value 的映射属于 `services/runtime/peertelemetry`。调用方负责控制 label cardinality、身份信息暴露和 query authorization。
