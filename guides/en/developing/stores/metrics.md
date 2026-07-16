# pkgs/store/metrics

`pkgs/store/metrics` Provides time-series sample writing and query abstractions. GizClaw uses it to save Peer telemetry and perform instant query, range query and aggregation through Server/Admin surface.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/metrics)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Store` | Define sample writing, instant query and range query. |
| `Sample` / `Point` / `Series` / `SeriesSet` | Express input sample and query results. |
| `Selector` / `LabelMatcher` | Describe metric name and label filtering. |
| `Query` / `RangeQuery` | Describe the query time, time interval, step and expression. |
| `Aggregation` / `AggregateExpression` | Constructs a supported aggregate expression. |
| `MemoryStore` | Provides in-process time-series implementation. |
| `PrometheusStore` | Write and query metrics through Prometheus-compatible API. |
| `ValidateMetricName` / `ValidateLabelName` | Verify metric and label contract. |

## Ownership Boundary

The Metrics package does not own the GizClaw telemetry event schema. The mapping of Telemetry packet to metric name, labels and sample value belongs to `services/runtime/peertelemetry`. The caller is responsible for controlling label cardinality, identity exposure, and query authorization.
