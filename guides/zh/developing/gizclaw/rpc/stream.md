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

## C unary request contract

C API v5 用 `gzc_rpc_request_start` / `gzc_rpc_request_result` /
`gzc_rpc_request_cancel` / `gzc_rpc_request_destroy` 表示可并发的 unary 请求。每个
request 独占一条 RPC service DataChannel 和有界的 frame、continuation、response
缓冲区；client 只保存非 owning 的 channel → request 关联。调用方串行调用
`gzc_client_poll` 推进全部请求，`result` 不 poll，pending 时返回
`GZC_ERR_WOULD_BLOCK`。

response envelope 和 EOS 都到达后结果为 `GZC_OK`；远端 RPC error 仍通过
`gzc_rpc_response_t.has_error` 表达。deadline 返回 `GZC_ERR_TIMEOUT`，取消、未完成
的远端关闭或 client close 返回 `GZC_ERR_CLOSED`。任一 terminal transition 只执行
一次 channel detach/close，request-owned response view 保持到 destroy。同步
`gzc_rpc_call_service` 和 `gzc_rpc_call` 复用同一状态机，但把最终 response 复制回
原有 client-owned view 以保持既有签名和生命周期。
