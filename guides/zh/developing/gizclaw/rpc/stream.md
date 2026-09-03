# Streaming

`实现文件：rpc_stream.go`

定义 `rpcStream` 及 RPC request/response envelope 的读写：frame 序列、protobuf envelope continuation、EOS、typed method response 解码、iterator 和 connection I/O error normalization。

这是 RPC framing 层；底层 connection 和 service stream 属于 `pkgs/giznet`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcStream` | 包装 connection、context 与 RPC frame codec。 |
| `newRPCStream` | 创建 stream 并绑定 connection lifecycle context。 |
| `ReadFrame` / `WriteFrame` | 读写单个 typed RPC frame。 |
| `ReadRequest` / `WriteRequest` | 读写 RPC request。 |
| `ReadResponse` / `WriteResponse` | 读写 RPC response。 |
| `ReadRequestEnvelope` / `ReadResponseEnvelope` | 读取可能跨多个 frames 的 protobuf envelope。 |
| `WriteRequestEnvelope` / `WriteResponseEnvelope` | 写入 protobuf envelope 与 continuation frames。 |
| `Frames` / `WriteFrames` / `Responses` | 提供流式 iterator 读写。 |
| `ReadEOS` / `WriteEOS` | 处理 stream end marker。 |
| `normalizeIOError` | 将底层 I/O error 规范化为 RPC stream error。 |

## C request contract

C SDK 用 `gzc_rpc_request_start` / `gzc_rpc_request_result` /
`gzc_rpc_request_cancel` / `gzc_rpc_request_destroy` 表示可并发的 unary 请求。每个
request 独占一条 RPC service DataChannel 和有界的 frame、continuation、response
缓冲区；client 只保存非 owning 的 channel → request 关联。调用方串行调用
`gzc_client_poll` 推进全部请求，`result` 不 poll，pending 时返回
`GZC_ERR_WOULD_BLOCK`。

response envelope 和 EOS 都到达后结果为 `GZC_OK`；远端 RPC error 仍通过
`gzc_rpc_response_t.has_error` 表达。deadline 返回 `GZC_ERR_TIMEOUT`，取消、未完成
的远端关闭或 client close 返回 `GZC_ERR_CLOSED`。任一 terminal transition 只执行
一次 channel detach/close，request-owned response view 保持到 destroy。

两个 start 接口都接收 borrowed 的 `const gzc_rpc_request_options_t *`，其中
`on_complete` 与 `complete_userdata` 注册 `gzc_rpc_complete_cb`。request 第一次进入
terminal 时 SDK 同步且只回调一次，调用方无需遍历 pending request。全部终态都会通知：
response EOS、编解码或协议错误、DataChannel 关闭、传输错误和超时由 poll owner 交付；
`gzc_rpc_request_cancel`、`gzc_client_close`，以及对 pending request 调用
`gzc_rpc_request_destroy`，则在各自调用线程返回前交付。SDK 不为该 callback 创建线程。callback 只通知最终状态，不转移所有权；
callback 内外调用 `gzc_rpc_request_result` 得到相同结果。options 传 NULL 表示不注册
completion callback。

callback 只负责通知，不得用它所属的 request 或 client 重入 SDK。对自身 request 调用
`gzc_rpc_request_result` 是安全的；`gzc_rpc_request_cancel`、
`gzc_rpc_request_destroy`、`gzc_client_poll`、`gzc_client_close` 和
`gzc_client_destroy` 都不允许。后两者会摘链并释放 poll loop 正在遍历的 service
channel，在 callback 里调用会释放 SDK 仍在使用的状态。request 与 client 的 teardown
应放在交付该通知的 poll 调用返回之后。frame callback 受同样的限制。

流式调用复用同一个 request handle：调用方通过
`gzc_rpc_request_start_stream` 创建请求，用 `gzc_rpc_request_write` 排队 binary
request frame，再用 `gzc_rpc_request_finish_write` 排队 request EOS。Response
envelope、binary data 与 response EOS 由 frame callback 在 `gzc_client_poll` 中按序
交付，completion callback 在 response EOS 的 frame callback 返回之后触发。
上一帧仍 pending 时 write 返回 `GZC_ERR_WOULD_BLOCK`，调用方 poll 后重试，
不引入第二套 stream lifecycle。
