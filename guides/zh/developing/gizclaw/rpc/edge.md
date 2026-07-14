# Edge Routing

`实现文件：rpc_edge.go`

定义 `edgeRPCServer`，在 Edge Giznet service 上处理 Peer lookup、assignment 和 route resolve；统一编码 RPC result，并将 `peerroute`、Peer 与 KV 错误映射为 RPC error code。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `edgeRPCServer` | 持有 authoritative Peer route service。 |
| `Handle` | 在 Edge service connection 上运行 RPC loop。 |
| `dispatch` | 分发 Edge route RPC methods。 |
| `handleLookup` | 查询 Peer 当前 assignment。 |
| `handleAssign` | 创建或更新 Peer assignment。 |
| `handleResolve` | 解析目标 Peer 的有效 route。 |
| `edgeRequiredParams` | 解码并校验必需 params。 |
| `edgeRPCResult` / `edgeRPCError` | 编码 typed result 或映射领域错误。 |
