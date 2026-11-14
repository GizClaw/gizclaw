# Peer

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peer)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer)

`peer` 拥有 Server 侧持久化 Peer 资源，并实现 Admin HTTP 与 Peer HTTP 所需的 Peer CRUD、校验、索引和 connected-peer bootstrap。

Peer 的 KV mutation 使用未叠加 SQL 字段的持久化快照生成 CAS 条件和标识符索引。RegistrationToken 的 Firmware 绑定由 SQL 持有，读取时通过 `RegistrationFirmware` 投影；profile、设备 identifiers 刷新、approve/block 等局部更新不会把该投影当作 KV 旧值或写回 KV。Mutation 在提交前解析返回值的投影，因此 SQL 查询失败不会把已提交的写入报告为失败。真实的跨 Server 旧快照写入仍由原子 CAS 拒绝，删除围栏和索引更新保持同一事务。

Public key 是 Peer identity，不应和数据库 ID、connection ID 或 Edge assignment 混用。WebRTC connection lifecycle 属于 `giznet` 与根 `PeerManager`，不属于本 package。

每个 Peer 最多维护 10 个不同的 SN／IMEI 索引，非空 SN 计一个，IMEI 按完整的 `(TAC, serial)` 去重；重复或不完整 IMEI 不增加索引。每个 SN 或 IMEI 反查 Set 最多关联 10 个不同的 Peer 公钥。数量检查覆盖完整保存和设备字段刷新；集合容量在与主记录及索引变更相同的原子 mutation 中校验。超额拒绝不会更改原记录或任何旧、新索引，重复关联不消耗额外名额，移除关联后可再次添加。Admin refresh 分别返回 `409 DEVICE_IDENTIFIER_LIMIT_REACHED` 或 `409 DEVICE_IDENTIFIER_INDEX_FULL`。已有超额集合不自动删除成员，允许不增长的幂等关联和移除操作。

A Peer deletion request atomically publishes the permanent `deleted` tombstone and creates or reuses its durable cleanup request. The request retains the original unprojected Peer bytes for cleanup; repeating deletion does not replace the request or reset its progress. Admin get/list expose the tombstone immediately through the local write view, while other Server instances observe it according to the configured replica policy. Once visible, it rejects activation and business access with `PEER_DELETED`. Atomic mutation conditions prevent an old active observation from replacing the tombstone. Local connections are quiesced after the request commits.

The production handler validates the fixed home before accessing regional SQL or providers. Task discovery and domain reads start with the [configured reader](../../../stores/kv); a claim mismatch, failed lookup or inconsistent retirement proof permits a bounded primary refresh. Foreign workers leave shared work for the home Server. Unknown or invalid routes retain the task and identity fence, and local SQL absence never proves another region completed cleanup.

Cleanup starts when the task is discovered and repeats hourly for at least 32 completed executions. The KV task persists its execution count separately from failures; normal deferral does not consume the failure budget. Transient failures also retry hourly with a failure limit of at least 32. Each execution rebuilds the Social and Workspace inventories instead of treating a previous empty or partial snapshot as complete. Unfinished exact lifecycles remain in the durable plan across inventory refreshes. Successful handoffs are checkpointed individually, so later failures neither erase outstanding work nor replay completed lifecycles. Already dispatched child task identities remain tracked across passes, and their apparent absence is checked at the write authority before completion. A successful execution numbered 32 or later may finish only after its known cleanup and child tasks complete; otherwise the request remains. The RuntimeProfile owner binding stays available throughout these passes.

Contacts and ordinary Workspaces fence owner admission and capture their local SQL inventory transactionally. Friend and Group admission mutations check permanent Peer fences in their shared atomic roots. Reading back a locally committed fence does not prove that a replica has exposed every earlier relationship; later executions discover and clean late-visible records. Captured membership identities still protect each individual deletion from removing a different incarnation.

Admin pages skip local directory entries absent from the historical Peer reader and advance by the last scanned local key, without upgrading the query. SQL Contact and Workspace creation may refresh a missing Peer input once; ordinary Workspace reads use a separate availability callback. Read/write-separated KV backends make their own committed local writes visible through that callback; historical absence from other backend instances remains possible.

Retirement plans bind the marker fingerprint, home Server and exact original Peer bytes retained by the deletion request. Each refresh verifies the tombstone and compares the previous plan atomically before replacing its inventory. A genuine Peer replacement or invalid task remains fenced for investigation. SQL RegistrationFirmware display projections do not change the retained identity.

本机 Contact 与用户 Workspace 由各自领域 adapter 清理。API Key cleanup 保留 owner retired marker，阻止并发创建 credential。Workspace 必须先完成 memory、History、runtime、icon、对象和 graph state 清理；随后清除 Peer scope 的当前 Memory binding，验证 absent，最后删除本区 RuntimeProfile owner binding。绑定指向的全局 Profile、Firmware、Workflow、RegistrationToken、log 和 metrics 保留。PeerRun 行保留为本机 Admin 目录与诊断状态，不是清理完成证据。

Friend and Friend Group membership and room bindings are authoritative in shared KV. Peer cleanup retires the relationships it discovers and hands their known children to the owning domain adapters; it does not require a global SQL replica-completion ledger before processing them. Regional system Workspace rows are local runtime catalog replicas. The bounded repeated passes do not prove physical removal of every regional SQL replica or discovery of data that remains invisible throughout all executions. See [Social multi-Server boundaries](../social#多-server-边界).

支持读取 version 2 的无子资源计划：完整 Peer 字段必须一致，只允许计划含 SQL Firmware 投影而当前 KV 不含该字段的差异；在 home Server 重新检查 Contact、Social 与 Workspace 均为空后，条件写入有 home 与原始 generation 的计划。旧计划有子资源、home 发现计划遗漏的数据或 plan 版本不受支持时保留原计划，返回 `retirement_scope_ambiguous` 或 `retirement_plan_unsupported`。真实 KV Firmware 改变、其他字段改变和 Peer 缺失均不能绕过保护。

After the required executions and a successful final cleanup pass, guarded finalization atomically consumes the exact secondary indexes, plan, marker, locator, task and task indexes while preserving `{"version":1,"state":"deleted"}` at `by-pubkey/<public-key>`. Admin get/list 从 sentinel 派生 `{public_key,status=deleted}`；其他入口返回 `PEER_DELETED`，同一 public key 永久不能重新注册。固定 route 与 API Key retired marker 保留：前者保留 home 证据并防止重指派，后者是并发凭据创建围栏。home Server 会在启动和后台扫描中自动发现符合精确 Firmware 误判条件的旧 failed：要求 finalize 阶段、对应错误码、旧计划带 SQL Firmware 而 raw KV 不带、无子资源且本区快照为空。恢复和正常任务使用同一有界 worker，原任务状态经 CAS 才重新排队；升级后的计划不再自动重置重试次数。归属不明、真实替换、缺失和不明确区域依赖保持 failed。可选运维入口见 [CLI PendingDeletion](../../../../using/cli#pendingdeletion-恢复)。

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
