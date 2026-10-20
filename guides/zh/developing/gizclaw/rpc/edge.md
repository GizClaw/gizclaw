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
Bearer credential。随机 API Key 从 `services.api_key.store` 取得 owner；签名 Bearer 通过与
HTTP 共用的 `resolveSignedPeerAccess` 验签并检查当前 Admin/Client 权限，取得签名目标。
两者返回现有 `PeerAssignment`，不创建、移动或刷新 assignment；原有 HTTP 请求仍会再次
认证与授权。
无效/撤销 credential 返回 forbidden，missing/inactive owner route 返回 not found；错误不得回显 key。

## 跨 Server 通知

`edge.social.ping.relay`（149）由 Edge 提供给已验证身份的配置上游 Server；
`server.social.ping.deliver`（150）由 Server 提供给 active Edge。两者只在 physical
`ServiceEdgeRPC`（0x31）上传输 `SocialPingDeliveryRequest`：目标 PK、
`ClientSocialPingRequest` 与 `deadline_unix_ms`。响应 `SocialPingDeliveryResponse`
为空，且只能在设备 ACK 后返回成功。设备端继续使用原有 `client.social.ping`。

发起 Manager 先使用本地 Peer；没有本地连接时按 Edge PK 顺序选择一个当前可信连接，
不在发送后切换中继。Edge 在控制与 gateway pool 两类 outbound Server connection 上
注册反向 handler，device ingress 不注册。Edge 查询现有 assignment 后，只向配置匹配的
Server 发出一次 deliver；目标 handler 重新校验 active Edge role，只查本地 Peer，不能递归。
目标离线返回 NOT_FOUND，超时返回 DEADLINE_EXCEEDED，中继/传输不可用返回 UNAVAILABLE。
Social adapter 将未收到设备 ACK 的尝试计为未送达，保留原有对外结果与窗口释放语义。

所有 hop 保留相同 deadline 与请求关联 ID，接收方额外把本地工作限制在 3 秒内。
取消、断连、超时与成功均关闭请求 stream；parent connection 仍由原 transport lifecycle
持有。上游重连共享一次建立过程，等待和建立均可取消，网络 I/O 不持有 transport mutex。
