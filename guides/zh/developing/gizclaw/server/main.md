# Server

`实现文件：server.go`

定义可复用的 `Server` composition root：接收 identity、Peer listeners、显式 Store 能力与运行配置；初始化各领域 service；启动 HTTP 和 Peer listeners；处理 Peer event 并管理关闭流程。Server 不再包含 Store 名称或 prefix fallback。

它可以组合多个领域，但单一领域的 resource、validation、storage 和 lifecycle 应留在 `services/<domain>`。进程配置与启动属于 `cmd/internal/server`。

## Storage、Store 与 Service 组合

Server 配置明确分为三层：

| 层 | 结构 | Ownership |
| --- | --- | --- |
| `storage` | 动态 map | 物理连接、credential、pool/client 构造、readiness 与关闭 |
| `stores` | 动态 map | 逻辑接口 kind 与 prefix/table/topic scope |
| `services` | 固定类型结构 | 内置服务用于引用兼容 Store 的固定字段 |

Registry 名称精确匹配并区分大小写。Server 不会赋予 `peers`、`metrics` 等名称任何内置含义；每个内置消费者都通过固定的 `services` 字段绑定。核心 service block 必填，`services.agent_host`、`services.metrics` 与 `services.system_log` 可选；省略 SystemLog 时使用 info-level stderr。

主要能力分组如下：

| Service 字段 | 所需能力 |
| --- | --- |
| Peer、login、credential、firmware、RuntimeProfile、model、voice、MemoryLayout、provider tenants、workflow、toolkit、contact、friend 与 Friend Group | 各一个 `keyvalue`；内部 collection prefix 由代码拥有 |
| `services.workspace.assets_store`、`services.gameplay.assets_store`、`services.agent_host.runtime_store` | `objectstore` |
| `services.gameplay.database_store` | `sql` |
| `services.agent_host.flowcraft.history_store` | `log.mutable` |
| `services.metrics.store` | `metrics` |
| `services.system_log.query_store` 与 Store sink | immutable Log 能力 |

Friend Group 的 groups、invite tokens、members 与 belongs 是同一 Service Store 上的代码内置 scope，因此天然共享一个原子 KV transaction boundary。共享 ObjectStore 必须使用非空、规范且互不重叠的 prefix。引用缺失、kind 不兼容、Flowcraft History 不可变或出现未知字段时，Server 会在打开 listener 前失败。

启动顺序依次为严格解析配置、打开物理 connector、构造逻辑 Store、解析 service 能力、由活跃 SQL 服务校验 schema、安装日志与 metrics，最后才打开 listener。逻辑 Store 不关闭借用的 connector；关闭时先释放逻辑 wrapper，再关闭物理 connector。

旧的一层 Store 配置、顶层伪 service block、隐式 Store 名称、通用 `kind: log` 和 `gizclaw migrate` 命令均不受支持。开发环境应使用当前配置重新创建，不导入或转换旧数据。Gameplay 与 ClickHouse Store 初始化表仍属于活跃 schema lifecycle，不属于旧数据迁移。

### 完整配置

下面的完整生产配置让 prefix-scoped KV、table-scoped Metrics、immutable/mutable Log 和 Gameplay raw SQL Store 借用同一个 PostgreSQL pool，并让 ObjectStore 使用独立 filesystem root。SQL KV 的 prefix 由后端直接映射为物理表；配置没有 `table` 字段。所有逻辑 SQL Store 在 listener 启动前直接保证业务表与索引存在，再精确校验 schema；不会创建 version/history 表。任何一个失败都会终止启动而不会回退到其他 backend。SQLite 本地部署使用相同 `stores` 与 `services`，只把 `storage.database` 改为 `kind: sqlite` 和 `dir`/`dsn`。

<<< ../../../../snippets/server-storage-stores-services.yaml{yaml}

`memory.Store` 仍由 RuntimeProfile 与 MemoryLayout 选择；`stores.kind: memory` 无效。物理 `storage.kind: memory` 是无状态 marker；每个兼容的 keyvalue 或 metrics Store 都创建独立进程内 backend。完整契约见 [Memory Store](/zh/developing/stores/memory)。

每个 Workspace Agent generation 根据当前 RuntimeProfile snapshot 解析 memory alias。构造失败会使 Agent 初始化或 reload 显式失败。Server shutdown 关闭共享 Memory registry；Workspace reload 与最后一个 Agent 引用释放会关闭该 generation 的 lease，但不迁移、合并、复制或删除持久数据。

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
