# Management

`实现文件：peer_manager.go`

`peer_manager.go` 维护 Server 当前可见的在线 Peer，并提供面向其他 GizClaw 组件的 Peer 操作入口。

| 文件 | 包含的功能 |
| --- | --- |
| `peer_manager.go` | 维护在线 Peer 与连接替换；连接上线、下线和强制断开；查询连接及 Peer runtime；确保 Peer 资源存在；通过 Peer RPC 刷新设备、硬件、IMEI 与 labels；协调 telemetry status 的并发更新。 |

这个前缀拥有 Server 视角的在线连接索引和跨连接操作，不拥有 Peer 持久化模型。Peer 资源本身属于 `services/runtime/peer`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Manager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager) | 聚合领域 services，并维护 public key 到在线 connection 的索引。 |
| [`NewManager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#NewManager) | 创建 Manager，并设置 Peer service。 |
| `activePeer` | 保存单个 Peer 当前生效的 connection。 |
| [`Manager.SetPeerUp`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.SetPeerUp) / [`SetPeerDown`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.SetPeerDown) / [`ForcePeerDown`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.ForcePeerDown) | 管理 connection 上线、条件下线和强制下线。 |
| `allowService` / `allowActivePeerRole` | 根据 Peer role 判断 Giznet service 准入。 |
| [`Manager.Peer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.Peer) / [`PeerRuntime`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.PeerRuntime) | 查询在线 connection 或 runtime 快照。 |
| [`Manager.EnsurePeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.EnsurePeer) | 确保持久化 Peer resource 存在。 |
| [`Manager.RefreshPeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Manager.RefreshPeer) / `refreshPeer` | 通过 Peer RPC 拉取设备信息，并将变化写回 Peer resource。 |
| `peerRPCConn` / `callPeerRPC` | 打开 Peer RPC stream 并执行 typed RPC call。 |
| `retainTelemetryStatusLock` / `releaseTelemetryStatusLock` | 按 public key 管理 telemetry status 更新锁的生命周期。 |
| `applyPeerRefreshInfo` / `applyPeerRefreshIdentifiers` | 将 RPC refresh response 合并到持久化 Peer model。 |

Connection registration 更新只接受在线索引中已存在且完全相同的 connection，绝不会为缺失条目重新创建在线 Peer。registration 发布与 `PeerConn` retiring 共用同一个 Manager 临界区：要么 registration 先完成、随后被 retiring 摘除，要么 retiring 先完成、迟到的 registration 被拒绝。

## 设备元数据归属

`client.info.get` 只反向刷新 `HardwareInfo`（`hardware_revision`、`manufacturer`、`model`）。`client.identifiers.get` 只反向刷新 `DeviceIdentifiers`（`sn`、`imeis`、`labels`）。由 Server 持有的个人资料字段 `name` 与 `emoji` 通过 `server.info.put` 修改，不会被反向刷新覆盖。`name` 必须是有效 UTF-8 且不超过 256 bytes，`emoji` 必须是有效 UTF-8 且不超过 64 bytes。

好友通过 `server.friend.info.get` 读取这些文本资料。该方法要求调用者作用域内已存在好友关系，并且不返回二进制头像数据。
