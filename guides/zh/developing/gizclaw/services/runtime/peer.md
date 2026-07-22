# Peer

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` 拥有 Server 侧持久化 Peer 资源，并实现 Admin HTTP 与 Peer HTTP 所需的 Peer CRUD、校验、索引和 connected-peer bootstrap。

## 核心结构与主函数

| 结构或函数 | 作用 |
| --- | --- |
| `Server` | 组合 Peer store、在线 `PeerManager` 与 HTTP service dependencies。 |
| `PeerManager` | 查询在线 Peer connection/runtime，不拥有持久化记录。 |
| `PeerAdminService` | 定义 Admin surface 需要的 Peer operations。 |
| `PeerHTTPService` | 定义 Peer-facing surface 需要的 Peer operations。 |
| `Server.EnsureConnectedPeer` | 为已认证 public key 创建默认 active Peer。 |
| `Server.LoadPeer` / `SavePeer` | 按 public key 读取或保存完整 Peer。 |
| `Server.BootstrapEdgeNodes` | 将配置中的 Edge Node identity 同步为 Peer 资源。 |
| `Server.DeleteSelf` | 原子删除 authenticated Peer，并写入 durable pending-deletion handoff。 |

Public key 是 Peer identity，不应和数据库 ID、connection ID 或 Edge assignment 混用。WebRTC connection lifecycle 属于 `giznet` 与根 `PeerManager`，不属于本 package。

Peer 删除会在同一个 KV transaction 中删除 active record 和全部 Peer indexes，并写入一条 `kind=peer` PendingDeletion；它不级联删除 Workspace、Pet、social、gameplay 或 RegistrationToken resource。Admin 删除不会强制关闭在线 connection。`server.peer.delete` 同时按 caller 与 connection generation 约束：已被替换的旧 connection 会在删除新 generation 前被拒绝。持久删除提交后，根 connection runtime 立即进入 retiring、摘除当前在线 connection 和 registration、拒绝新工作，然后尝试写入 acknowledgement 和 EOS；无论任一写入是否失败，都会关闭完整 Giznet connection。丢失 acknowledgement 后的 Client reconnect 可以创建新一代 active record，但不能覆盖旧 pending event；删除重试的 pending lookup 与 reconnect create 由同一把 per-record lock 串行化。configured Edge bootstrap 和 generic write 在 locator pending 期间保持 blocked，但 registration 拥有的 firmware binding 可以更新已 active 的重连 generation。
