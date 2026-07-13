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

Public key 是 Peer identity，不应和数据库 ID、connection ID 或 Edge assignment 混用。WebRTC connection lifecycle 属于 `giznet` 与根 `PeerManager`，不属于本 package。
