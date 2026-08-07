# Server

`实现文件：server.go`

定义可复用的 `Server` composition root：接收 identity、Peer listener、stores 与运行配置；初始化各领域 service；启动 HTTP 和 Peer listener；处理 Peer event；管理后台 cleanup、关闭顺序和 module store fallback。

它可以组合多个领域，但单一领域的 resource、validation、storage 和 lifecycle 应留在 `services/<domain>`。进程配置与启动属于 `cmd/internal/server`。

## AgentHost Store 绑定

Server Config 使用 `stores` 中的逻辑名称绑定 AgentHost 持久化能力：

```yaml
agent_host:
  runtime_store: agenthost
  flowcraft:
    state_store: flowcraft-state
    history_store: flowcraft-history
```

这些引用同时适用于分层的 `storage` 加 `stores` 布局和受支持的单层 `stores` 布局。Backend 配置仍属于被引用的 Store；`agent_host` 不接受目录、DSN、credential、prefix、scope 或 inline backend。

| 字段 | 必需 capability | 支持的 backend |
| --- | --- | --- |
| `agent_host.runtime_store` | `objectstore.ObjectStore` | filesystem ObjectStore |
| `agent_host.flowcraft.state_store` | `kv.Store` | Memory 或 Badger KV |
| `agent_host.flowcraft.history_store` | `logstore.MutableStore` | ClickHouse LogStore；不可变的 Volc LogStore 会被拒绝 |

`agent_host` 是这些绑定的唯一依据。省略整个 block 或某个内层引用会禁用对应可选能力；Store 名称本身不具有保留绑定语义。未知名称、错误 Store kind、不可变 History Store、未知字段或空引用都会让 Server 构造失败，不会 fallback。

`agent_host.flowcraft.memory_store`、`agent_host.eino.memory_store`、`memory_objects_store` 以及 `stores.kind: memory` 都不是合法 Server Config；严格 parser 会拒绝这些旧字段。Memory policy 由 Admin `MemoryLayout` 管理，实际连接由 RuntimeProfile `resources.memories` 管理。Server 只提供 MemoryLayout KV store 与 Server Workspace root；`flowcraft_bbh` 在后者下面构造 managed persistence。完整 contract 见 [Memory Store](/zh/developing/stores/memory)。

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
