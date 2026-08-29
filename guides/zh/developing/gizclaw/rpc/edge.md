# Edge Routing

`实现文件：rpc_edge.go`

定义 `edgeRPCServer`，在 Edge Giznet service 上处理 Peer lookup、assignment 和 route resolve；统一编码 RPC result，并将 `peerroute`、Peer 与 KV 错误映射为 RPC error code。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `edgeRPCServer` | 持有 authoritative Peer route service。 |
| `Handle` | 在 Edge service connection 上处理一个 RPC lifecycle，随后关闭 connection。 |
| `dispatch` | 分发 Edge route RPC methods。 |
| `handleLookup` | 查询 Peer 当前 assignment。 |
| `handleAssign` | 创建或更新 Peer assignment。 |
| `handleResolve` | 解析目标 Peer 的有效 route。 |
| `edgeRequiredParams` | 解码并校验必需 params。 |
| `edgeRPCResult` / `edgeRPCError` | 编码 typed result 或映射领域错误。 |

`server.peer.lookup` 与 `server.route.resolve` 只读。`server.peer.assign` 会为当前 Server 原子 claim 缺少 assignment 的 Client Peer；owner 相同时返回现有记录，并且只能刷新这个 owner 的 endpoint/role metadata。不同 Server owner 返回 conflict，记录不会被覆盖；`expected_version` 只检测 stale update，不能授权 ownership transfer。
