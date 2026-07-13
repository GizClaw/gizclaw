# Utilities

`实现文件：rpc_utils.go`

提供 RPC runtime 共用的 dispatch loop、stream request 处理、Ping、client call、request/result 构造、payload validation、API error mapping 和类型转换 helper。

该文件是 RPC 内部辅助实现，不拥有独立领域能力。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `handleRPC` / `handleRPCWithStream` | 运行普通或 streaming RPC Server request loop。 |
| `handleRPCStreamRequest` | 选择 streaming dispatch 或普通 dispatch 并写回 response。 |
| `callRPC` | 在已有 connection 上执行一次 request/response 调用。 |
| `callRPCPing` / `handleRPCPing` | 通用 Ping client/server helper。 |
| `newRPCRequest` / `newRPCRequestParams` | 构造 typed RPC request 与 payload。 |
| `newRPCResultResponse` / `callRPCResult` | 编码或解码 typed result。 |
| `validateRPCParams` | 使用生成 payload decoder 校验 params。 |
| `rpcAPIError` / `rpcInvalidParams` / `rpcUnexpectedResponse` | 构造稳定 RPC errors。 |
| `convertRPCType` | 在结构兼容的 API types 之间执行转换。 |
