# Server

`实现文件：server.go`

定义可复用的 `Server` composition root：接收 identity、Peer listeners、显式 Store 能力与运行配置；初始化各领域 service；启动 HTTP 和 Peer listeners；处理 Peer event 并管理关闭流程。Server 不再包含 Store 名称或 prefix fallback。

它可以组合多个领域，但单一领域的 resource、validation、storage 和 lifecycle 应留在 `services/<domain>`。进程配置与启动属于 `cmd/internal/server`。

## Server info 与构建身份

`/server-info` 中的 `version` 和 `build_commit` 来自二进制编译期 metadata。正式 Release 的 `version` 是不带 `v` 前缀的 `MAJOR.MINOR.PATCH`，本地未注入的开发构建返回 `dev`。Release 构建必须同时注入版本与完整 source commit，不能从运行时配置覆盖软件版本。

`public_key` 仍是 authoritative Server 的唯一身份。Edge 只改写 transport endpoint，不取得 Server 业务身份，也不改变 Server 的 `public_key`、版本或构建 commit。

## Storage、Store 与 Service 组合

Server 配置明确分为三层：

| 层 | 结构 | Ownership |
| --- | --- | --- |
| `storage` | 动态 map | 物理连接、credential、pool/client 构造、readiness 与关闭 |
| `stores` | 动态 map | 逻辑接口 kind 与 prefix/table/topic scope |
| `services` | 固定类型结构 | 内置服务用于引用兼容 Store 的固定字段 |

Registry 名称精确匹配并区分大小写。Server 不会赋予 `peers`、`metrics` 等名称任何内置含义；每个内置消费者都通过固定的 `services` 字段绑定。核心 service block 必填，`services.agent_host`、`services.metrics` 与 `services.system_log` 可选；省略 SystemLog 时使用 info-level stderr。

全局与 per-sink `services.system_log` level 也决定新接受的 Edge-routed logical Peer 是否构造
完整 Server lifecycle observer。只有所有 sink 都拒绝 `INFO`，才会移除 connection、per-turn
与 Agent-runtime tracking；任意 Info-enabled sink 都会保持 observer 启用。该决定在 connection
生命周期内固定不变。这里没有 lifecycle-specific 配置字段，Server logging 配置也不控制独立
Edge 进程。

主要能力分组如下：

| Service 字段 | 所需能力 |
| --- | --- |
| `services.peer.store` | `keyvalue`；保存共享 Peer record 与固定 Server route |
| `services.peer_run.store` | 独立的 `keyvalue`；保存当前 Server 本地的 Peer 状态与 Agent selection |
| login、credential、firmware、RuntimeProfile、model、voice、MemoryLayout、provider tenants、workflow、toolkit、contact、friend 与 Friend Group | 各一个 `keyvalue`；内部 collection prefix 由代码拥有 |
| `services.workspace.history_assets_store`、`services.workspace.assets_store`、`services.gameplay.assets_store`、`services.agent_host.runtime_store` | `objectstore` |
| `services.gameplay.database_store` | `sql` |
| `services.workspace.history_store` | `log.mutable` |
| `services.agent_host.flowcraft.history_store` | `log.mutable` |
| `services.metrics.store` | `metrics` |
| `services.system_log.query_store` 与 Store sink | immutable Log 能力 |

Friend Group 的 groups、invite tokens、members 与 belongs 是同一 Service Store 上的代码内置 scope，因此天然共享一个原子 KV transaction boundary。共享 ObjectStore 必须使用非空、规范且互不重叠的 prefix。引用缺失、kind 不兼容、Flowcraft History 不可变或出现未知字段时，Server 会在打开 listener 前失败。

多 Server 拓扑中，各 Server 可以把 Peer、Friend 和 Friend Group 控制面 Store 绑定到同一个 Redis 7.0 connector 下互不重叠的 prefix。`services.peer_run.store` 仍然必填，必须与 `services.peer.store` 不同，但可以使用任意受支持的 `keyvalue` backend；Server 会在该 binding 下保留代码拥有的 `runs` namespace。Client activation 会在发布 connection 或启动 service work 之前原子 claim 未分配 Peer，或校验已有的固定 Server owner。RuntimeProfile、Workspace、History、Memory、asset 与其他 runtime Store 仍保持 Server 本地；这种组合不提供 Workspace routing 或 failover。

启动顺序依次为严格解析配置、打开物理 connector、构造逻辑 Store、解析 service 能力、由活跃 SQL 服务校验 schema，并安装日志与 metrics。取得 workspace PID ownership 后，可选 process profiling 解析其专用 ObjectStore 并发布 baseline；完成后才打开 public listener 并启动 `Server.Listen`。逻辑 Store 不关闭借用的 connector。Shutdown 会先 join profiling worker，再关闭 logging、逻辑 wrapper 与物理 connector。Process profiling 属于 `cmd/internal/server`；可复用的 `pkgs/gizclaw.Server` 不依赖 pprof。

旧的一层 Store 配置、顶层伪 service block、隐式 Store 名称、通用 `kind: log` 和 `gizclaw migrate` 命令均不受支持。开发环境应使用当前配置重新创建，不导入或转换旧数据。Gameplay 与 ClickHouse Store 初始化表仍属于活跃 schema lifecycle，不属于旧数据迁移。

### 完整配置

下面的完整生产配置让 prefix-scoped KV、table-scoped Metrics、immutable/mutable Log 和 Gameplay raw SQL Store 借用同一个 PostgreSQL pool，并让 ObjectStore 使用独立 filesystem root。SQL KV 的 prefix 由后端直接映射为物理表；配置没有 `table` 字段。所有逻辑 SQL Store 在 listener 启动前直接保证业务表与索引存在，再精确校验 schema；不会创建 version/history 表。任何一个失败都会终止启动而不会回退到其他 backend。SQLite 本地部署使用相同 `stores` 与 `services`，只把 `storage.database` 改为 `kind: sqlite` 和 `dir`/`dsn`。

<<< ../../../../snippets/server-storage-stores-services.yaml{yaml}

`memory.Store` 仍由 RuntimeProfile 与 MemoryLayout 选择；`stores.kind: memory` 无效。物理 `storage.kind: memory` 是无状态 marker；每个兼容的 keyvalue 或 metrics Store 都创建独立进程内 backend。完整契约见 [Memory Store](/zh/developing/stores/memory)。

每个 Workspace Agent generation 根据当前 RuntimeProfile snapshot 解析 memory alias。构造失败会使 Agent 初始化或 reload 显式失败。Server shutdown 关闭共享 Memory registry；Workspace reload 与最后一个 Agent 引用释放会关闭该 generation 的 lease，但不迁移、合并、复制或删除持久数据。

## Workspace reward 生命周期

启动 Workspace reward dispatcher 时只验证 Gameplay SQL schema 与单例 activation
boundary，然后启动有界 due-work poller。Server 启动期间不会枚举或读取任何持久化
Workspace record 或 History，也没有周期性的 Workspace catalog scan。AgentHost runtime
成功发布后只为该精确 Workspace 安排懒对账；History append 后的通知只是可选的低延迟
提示，允许丢弃。已有 pending、retry 与过期 claim window 在重启后直接从 Gameplay SQL 恢复。

## Pending-deletion processor

Server 初始化会验证 pending-deletion source 与 handler registry，但不会启动后台任务。`Server.Listen` 启动一次立即扫描和有界 worker pool；`Server.Close` 取消 scan 与 active attempt，等待全部 processor goroutine 退出，然后 command layer 才能关闭 store。领域持久化 store 是 queue 的唯一事实来源；producer wake signal 与内存 dispatch channel 只用于降低延迟，因此重启和周期扫描仍能恢复已提交但 signal 丢失的 work。

首个 production registration 是 `source=gameplay`，只声明 `kind=pet`，并且仅在配置 `GameplayDB` 时存在。Peer、Workspace 与 Friend Group 在对应领域 handler 实现前不会注册。非法、重复、不完整或 backend capability 不满足要求的 registration 会让初始化失败。

可选顶层 `pending_deletion` 配置的默认值为：`scan_interval: 30s`、`page_size: 100`、`dispatch_capacity: 256`、`workers: 4`、`lease_duration: 2m`、`attempt_timeout: 90s`、`retry_initial: 5s`、`retry_max: 30m`、`max_attempts: 10`。Duration 与 count 必须为正且有界；attempt timeout 必须短于 lease；retry initial 不能大于 retry max；严格配置解析会拒绝未知字段。系统没有 completion retention 配置。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Server`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server) | 可复用 GizClaw Server 的 composition root。 |
| [`PeerListenerOptions`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerListenerOptions) / [`PeerListenerFactory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerListenerFactory) | 描述并创建 Peer listener。 |
| [`Server.ServeHTTP`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.ServeHTTP) | 服务 Server HTTP surface。 |
| [`Server.Listen`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Listen) / [`Serve`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Serve) | 创建 listeners 并接受 Peer connections。 |
| [`Server.PublicKey`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.PublicKey) | 返回 Server identity public key。 |
| [`Server.PeerService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.PeerService) / [`Manager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Manager) | 返回已组装的 Peer service 或在线 Peer Manager。 |
| `init` | 初始化 stores、领域 services、HTTP mux 和 Peer Runtime。 |
| `servePeerListener` | 接受单个 listener 上的 Peer connections。 |
| `startCleanup` | 启动后台资源清理。 |
| [`Server.Close`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Close) | 停止 listeners、后台任务并关闭 Server 资源。 |
