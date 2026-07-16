# Log Query

`GET /logs/stream` 是 admin-only 的 system log adapter。Host 注入 `logstore.Querier`；adapter 固定添加 `Streams=[system]` 与 `Kinds=[log]`，所以同一 store 中的 chat、event 或 audit records 不会通过这个 endpoint 暴露。

首次请求必须提供毫秒 Unix 时间的 `start_time_ms` 和 `end_time_ms`，并可设置 limit、asc/desc order 和 filter。Filter 为 `*`，或最多 32 个 uppercase `AND` 连接的 clause：

```text
level:value
text:value
field:value
field!=value
field:*
-field:*
```

Value 是不含 whitespace、quote 或 backslash 的 token，或 JSON string literal；decoded value 不能包含 wildcard。Field 遵循 LogStore dotted attribute grammar。`message`、`stream`、`kind` 和 provider metadata/time field 保留；不接受 OR、regex、provider function 或 raw Volc expression。Filter 最长 4096 bytes，field 最长 128 bytes，decoded value 最长 1024 bytes。

Adapter 把完整 filter 解析为结构化 `logstore.Query`。返回 cursor 是 GizClaw-owned outer cursor，包含 normalized query 与 opaque inner Store cursor；客户端不能看到 provider Context。Continuation 可以只传 cursor，并可改变 limit。显式重复的 filter、time 或 order 必须与 cursor 一致，否则返回 `LOG_CURSOR_MISMATCH`。

查询未配置返回 HTTP 501 `LOG_QUERY_NOT_CONFIGURED`；无效 filter/cursor 返回 HTTP 400；store/provider failure 返回 HTTP 502。成功响应保持 SSE `log` 和 `end` events，stream 开始后的失败使用 `error` event。
