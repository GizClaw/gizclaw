# Edge Routing

`实现文件：rpc_edge.go`

定义 `edgeRPCServer`，在 Edge Giznet service 上处理 Peer lookup、assignment、route resolve
与 API Key owner route resolve；统一编码 RPC result，并将 API Key、`peerroute`、Peer 与 KV
错误映射为 RPC error code。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `edgeRPCServer` | 持有 authoritative Peer route service。 |
| `Handle` | 在 Edge service connection 上处理一个 RPC lifecycle，随后关闭 connection。 |
| `dispatch` | 分发 Edge route RPC methods。 |
| `handleLookup` | 查询 Peer 当前 assignment。 |
| `handleAssign` | 创建或更新 Peer assignment。 |
| `handleResolve` | 解析目标 Peer 的有效 route。 |
| `handleAPIKeyResolve` | 认证 API Key，并解析 owner Peer 的有效 assignment。 |
| `edgeRequiredParams` | 解码并校验必需 params。 |
| `edgeRPCResult` / `edgeRPCError` | 编码 typed result 或映射领域错误。 |

`server.peer.lookup` 与 `server.route.resolve` 只读。`server.peer.assign` 会为当前 Server 原子 claim 缺少 assignment 的 Client Peer；owner 相同时返回现有记录，并且只能刷新这个 owner 的 endpoint/role metadata。不同 Server owner 返回 conflict，记录不会被覆盖；`expected_version` 只检测 stale update，不能授权 ownership transfer。

`server.api_key.resolve`（method ID 99）只通过已认证的 Edge RPC service 暴露。请求携带
Bearer credential，Server 使用自己的 `services.api_key.store` 认证并从 API Key record 取得
owner Peer，然后返回该 Peer 的现有 `PeerAssignment`。它不创建、移动或刷新 assignment。
无效/撤销 credential 返回 forbidden，missing/inactive owner route 返回 not found；错误不得回显 key。
