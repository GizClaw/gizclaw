# Management

`实现文件：peer_manager.go`

`peer_manager.go` 维护 Server 当前可见的在线 Peer，并提供面向其他 GizClaw 组件的 Peer 操作入口。

| 文件 | 包含的功能 |
| --- | --- |
| `peer_manager.go` | 维护在线 Peer 与连接替换；连接上线、下线和强制断开；查询连接及 Peer runtime；确保 Peer 资源存在；通过 Peer RPC 刷新设备硬件、SN、IMEI 与 labels；协调 telemetry status 的并发更新。 |

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

Connection activation 会先在 Manager 锁内为 public key 建立 reservation，再在不持有全局锁的情况下检查 durable Peer availability，并且只在 reservation 仍属于当前 connection 时发布它。pending marker 或 permanent tombstone 会使 activation 失败；等待 self-delete 的 reconnect 不会复用旧记录或创建新 generation。尚未发布 connection 的 reservation 处于离线状态。replacement 正在执行 durable ensure 时，原 generation 继续可用；强制下线会清除原 generation，但不会丢弃 replacement reservation；新 connection 只有发布完成后才会启动 transport service loop。connection-scoped self-delete 会先发布 deleting 状态，再提交 durable marker；提交后 Manager quiesce 该 identity，replacement activation、registration 与 Server 主动 Peer RPC 都持续被 durable fence 拒绝，其他 Peer 保持可用。

## 设备元数据归属

Peer 连接发布后，Server 会执行一次有界的设备信息刷新；失败不会中断连接，Admin 仍可主动调用 refresh 重试。`client.info.get` 只反向刷新 `HardwareInfo`（`hardware_revision`、`manufacturer`、`model`）。`client.identifiers.get` 只反向刷新 `DeviceIdentifiers`（`sn`、`imeis`、`labels`）。SN 是 Client 声明的可选弱标识，必须是有效 UTF-8 且不超过 256 bytes；Client 应让它对同一台物理设备保持稳定并尽量唯一，但 Server 不把它当作唯一身份。同一 SN 可以关联多个 Peer，Admin SN 查询返回全部匹配记录。由 Server 持有的个人资料字段 `name` 与 `emoji` 通过 `server.info.put` 修改，不会被反向刷新覆盖。`name` 必须是有效 UTF-8 且不超过 256 bytes，`emoji` 必须是有效 UTF-8 且不超过 64 bytes。

好友通过 `server.friend.info.get` 读取这些文本资料。该方法要求调用者作用域内已存在好友关系，并且不返回二进制头像数据。

设备自设调试权限与 SN/IMEI 多值查询见 [Public API](../../api/http/public)。
