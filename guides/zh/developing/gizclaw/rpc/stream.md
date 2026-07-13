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
