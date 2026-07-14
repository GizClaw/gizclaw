# Admin HTTP · Telemetry

`实现文件：peer_service_serve_admin_telemetry.go`

实现最新 telemetry、历史查询和聚合 endpoints，解析 Peer public key 与字段过滤条件，并映射 telemetry service 错误。

Telemetry 解码、状态和指标聚合属于 `services/runtime/peertelemetry`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `GetPeerTelemetryLatest` | 返回指定 Peer 最新 telemetry sample。 |
| `QueryPeerTelemetry` | 按时间和字段查询 telemetry samples。 |
| `AggregatePeerTelemetry` | 在指定窗口聚合 telemetry。 |
| `parseAdminTelemetryPublicKey` | 解析并验证请求中的 Peer public key。 |
| `parsePeerTelemetryFields` | 解析字段过滤条件。 |
| `peerTelemetryAdminError` | 将 telemetry service error 映射为 Admin API error。 |
