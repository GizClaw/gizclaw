# Admin HTTP · Telemetry

源码：[peer_service_serve_admin_telemetry.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_admin_telemetry.go)

实现最新 telemetry、历史查询和聚合 endpoints，解析 Peer public key 与字段过滤条件，并映射 telemetry service 错误。

Telemetry 解码、状态和指标聚合属于 `services/runtime/peertelemetry`。
