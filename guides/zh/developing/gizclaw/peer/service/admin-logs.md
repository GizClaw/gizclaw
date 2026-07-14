# Admin HTTP · Logs

`实现文件：peer_service_serve_admin_logs.go`

实现 Server log 的 Admin SSE stream：解析过滤条件、处理首事件与流式错误，并编码 SSE events。

Log query contract 位于 Server Log Query；实际日志 backend 由宿主提供。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `StreamServerLogs` | 验证查询并建立 Admin SSE log stream。 |
| `serverLogStreamRequestFromParams` | 将 HTTP query params 转换为日志 backend 请求。 |
| `streamServerLogsResponse` | 持有首事件及后续 stream writer。 |
| `waitFirstServerLogEvent` | 在发送 HTTP headers 前等待首事件或首错误。 |
| `writeServerLogSSE` | 编码 SSE event 与 JSON data。 |
