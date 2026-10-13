# Peer

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` 拥有 Server 侧持久化 Peer 资源，并实现 Admin HTTP 与 Peer HTTP 所需的 Peer CRUD、校验、索引和 connected-peer bootstrap。

Peer 的 KV mutation 使用未叠加 SQL 字段的持久化快照生成 CAS 条件和标识符索引。RegistrationToken 的 Firmware 绑定由 SQL 持有，读取时通过 `RegistrationFirmware` 投影；profile、设备 identifiers 刷新、approve/block 等局部更新不会把该投影当作 KV 旧值或写回 KV。Mutation 在提交前解析返回值的投影，因此 SQL 查询失败不会把已提交的写入报告为失败。真实的跨 Server 旧快照写入仍由原子 CAS 拒绝，删除围栏和索引更新保持同一事务。

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

每个 Peer 最多维护 10 个不同的 SN／IMEI 索引，非空 SN 计一个，IMEI 按完整的 `(TAC, serial)` 去重；重复或不完整 IMEI 不增加索引。每个 SN 或 IMEI 反查 Set 最多关联 10 个不同的 Peer 公钥。数量检查覆盖完整保存和设备字段刷新；集合容量在与主记录及索引变更相同的原子 mutation 中校验。超额拒绝不会更改原记录或任何旧、新索引，重复关联不消耗额外名额，移除关联后可再次添加。Admin refresh 分别返回 `409 DEVICE_IDENTIFIER_LIMIT_REACHED` 或 `409 DEVICE_IDENTIFIER_INDEX_FULL`。已有超额集合不自动删除成员，允许不增长的幂等关联和移除操作。

Peer 删除会在 Peer KV 中创建或复用一条 `kind=peer` PendingDeletion，并立即把 public key 变成跨进程 identity fence。marker 存在期间，Admin Peer get/list 与同一 delete 仍可用于诊断和幂等重试；reconnect、API Key HTTP、WebRTC、RPC/stream、Edge bootstrap、业务读取与 mutation 都返回 `PEER_PENDING_DELETION`。在线 connection 会被 quiesce，terminal cleanup failure 也不会解除 fence。

Production handler 在访问区域 SQL 或 Provider 前，从中央主库验证固定 home Server。共享任务由 home worker 认领；其他区域跳过认领，Admin retry 返回 `peer_home_mismatch`。缺失或无效 route 保留任务和身份围栏，不用本区空表推断删除完成。任务管理、扫描和整个 cleanup 调用链使用 [KV 主库读取](../../../stores/kv#redis-读写分离)，不依赖地区副本的复制进度。

retirement plan 绑定 marker fingerprint、home Server 和未叠加 SQL 字段的完整 KV 记录指纹。清理前验证 generation；最终 guarded mutation 再比较准确 KV 字节和当前 task lease，避免并发替换被旧计划删除。SQL `RegistrationFirmware` 仅用于展示，删除 owner binding 导致投影消失不会改变 KV generation。

本机 Contact 与用户 Workspace 由各自领域 adapter 清理。API Key cleanup 保留 owner retired marker，阻止并发创建 credential。Workspace 必须先完成 memory、History、runtime、icon、对象和 graph state 清理；随后清除 Peer scope 的当前 Memory binding，验证 absent，最后删除本区 RuntimeProfile owner binding。绑定指向的全局 Profile、Firmware、Workflow、RegistrationToken、log 和 metrics 保留。PeerRun 行保留为本机 Admin 目录与诊断状态，不是清理完成证据。

Social system Workspace 可以在多个区域落库，现有共享记录不包含所有副本的完成凭据。选择 Friend 或自己拥有的 Friend Group Workspace 的 Peer 计划返回 `social_replica_scope_unknown` 并保留围栏；不能把本区缺失的 Workspace 当成其他区域也已清理。独立的 Friend Group KV task 只确认其共享控制面退役，不证明所有区域 SQL 副本已经删除，见 [Social 多 Server 边界](../social#多-server-边界)。

支持读取 version 2 的无子资源计划：完整 Peer 字段必须一致，只允许计划含 SQL Firmware 投影而当前 KV 不含该字段的差异；在 home Server 重新检查 Contact、Social 与 Workspace 均为空后，条件写入有 home 与原始 generation 的计划。旧计划有子资源、home 发现计划遗漏的数据或 plan 版本不受支持时保留原计划，返回 `retirement_scope_ambiguous` 或 `retirement_plan_unsupported`。真实 KV Firmware 改变、其他字段改变和 Peer 缺失均不能绕过保护。

完成时同一 guarded KV mutation 删除 Peer payload、secondary index、plan、marker、locator、task 与任务索引，并在 `by-pubkey/<public-key>` 写入唯一的 `{"version":1,"state":"deleted"}` tombstone。Admin get/list 从 sentinel 派生 `{public_key,status=deleted}`；其他入口返回 `PEER_DELETED`，同一 public key 永久不能重新注册。固定 route 与 API Key retired marker 保留：前者保留 home 证据并防止重指派，后者是并发凭据创建围栏。恢复入口见 [CLI PendingDeletion](../../../../using/cli#pendingdeletion-恢复)。

Edge bootstrap 对已是 active Edge 的记录不重复写入。多个 Server 同时初始化同一 Edge 时，CAS 冲突最多重读重试 8 次；每次从最新记录合并角色和状态，保留并发更新的元数据与标识符索引，删除围栏仍然生效。

`blocked` 是可由 Admin approve 恢复为 active 的持久状态。`EnsureConnectedPeer` 与
`EnsureConnectedPeerGuarded` 对已有 blocked 记录（包括并发创建获胜的记录）返回
`ErrPeerBlocked`；不把它当作不存在，也不重建记录。Admin block 提交成功后先摘除
本地所有连接 generation，在 Manager 锁内设置旧 generation 的原子 retiring 标记，
再在锁外执行回调并关闭 transport。
block 不会给远端或离线 Peer 新增本地运行目录记录，也不依赖该目录可用。
持久化失败不踢下线。`EnsureAvailable` 保留删除围栏语义，
使 Admin 仍可读取、修改和解封 blocked 记录。连接与 service 执行边界见
[Management](../../peer/manager) 和 [Security Policy](../../server/security-policy)。

`Server.CheckActiveRole` 只读取当前 Peer KV 权限与删除围栏，不叠加 SQL Firmware 展示字段。它要求 active 状态与精确 role，KV 错误、pending deletion 和永久 tombstone 都拒绝；沿用调用方 context，不缓存判定。Admin/Edge transport 准入使用此入口，完整资源读取仍通过 `LoadPeer` 保留 Firmware 投影。

Peer 只退出别人拥有的群时，由共享 membership adapter 移除自身成员关系；其他成员和该群 Workspace 保留，不把它当作当前 Peer 的子资源清理。
