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
| `Server.EnsureConnectedPeer` / `EnsureConnectedPeerGuarded` | 为已认证 public key 创建默认 active Peer；guarded 形式会在 per-record lock 内先重新校验 connection lifecycle state，再读取或创建记录。 |
| `Server.LoadPeer` / `SavePeer` | 按 public key 读取或保存完整 Peer。 |
| `Server.BootstrapEdgeNodes` | 将配置中的 Edge Node identity 同步为 Peer 资源。 |
| `Server.DeleteSelf` | 为 authenticated Peer 原子创建或复用 durable pending-deletion handoff。 |

Public key 是 Peer identity，不应和数据库 ID、connection ID 或 Edge assignment 混用。WebRTC connection lifecycle 属于 `giznet` 与根 `PeerManager`，不属于本 package。

Peer 删除会在 Peer KV 中创建或复用一条 `kind=peer` PendingDeletion，并立即把 public key 变成跨进程 identity fence。marker 存在期间，Admin Peer get/list 与同一 delete 仍可用于诊断和幂等重试；reconnect、Public login、existing session、WebRTC、RPC/stream、Edge bootstrap、业务读取与 mutation 都返回 `PEER_PENDING_DELETION`。在线 connection 会被 quiesce，terminal cleanup failure 也不会解除 fence。

Production handler 保存绑定 marker fingerprint 的 immutable retirement plan，并通过 Social、Workspace、Gameplay、Public Login 与 RuntimeProfile 的 narrow adapter 清除该 Peer 拥有或被计划选中的数据。全局 catalog/config、foreign resource、log 与 metrics 不参与删除。完成时同一 guarded KV mutation 删除 Peer payload、secondary index、plan、marker、locator 与 task，并在 `by-pubkey/<public-key>` 写入唯一的 `{"version":1,"state":"deleted"}` tombstone。Admin get/list 从该 sentinel 派生 `{public_key,status=deleted}`；其他入口返回 `PEER_DELETED`，同一 public key 永久不能重新注册。
