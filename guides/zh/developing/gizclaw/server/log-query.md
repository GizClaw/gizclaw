# Log Query

`实现文件：server_log_query.go`

定义 Server log 查询与流式读取的 service interface、请求参数、排序方式、结果类型和结构化错误，并将查询错误映射为 HTTP error response。

这里拥有查询 contract；具体日志 backend 由宿主注入。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `ServerLogStreamRequest` | 描述日志过滤、排序、cursor 与 follow 参数。 |
| `ServerLogQueryService` | 宿主日志 backend 需要实现的流式查询接口。 |
| `ServerLogQueryError` | 携带稳定 error code 与底层错误。 |
| `InvalidServerLogQuery` | 构造无效查询错误。 |
| `LogQueryNotConfigured` | 表示 Server 未配置日志查询 backend。 |
| `ServerLogBackendError` | 包装 backend 执行错误。 |
