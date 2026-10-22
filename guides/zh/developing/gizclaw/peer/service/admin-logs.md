# Admin HTTP · Logs

源码：[peer_service_serve_admin_logs.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_admin_logs.go)

实现 Server log 的 Admin SSE stream：解析过滤条件、处理首事件与流式错误，并编码 SSE events。

Log query contract 位于 Server Log Query；实际日志 backend 由宿主提供。
