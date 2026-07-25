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
    memory_objects_store: flowcraft-memory-objects
    memory_store: agent-memory
  eino:
    memory_store: agent-memory
```

这些引用同时适用于分层的 `storage` 加 `stores` 布局和受支持的单层 `stores` 布局。Backend 配置仍属于被引用的 Store；`agent_host` 不接受目录、DSN、credential、prefix、scope 或 inline backend。

| 字段 | 必需 capability | 支持的 backend |
| --- | --- | --- |
| `agent_host.runtime_store` | `objectstore.ObjectStore` | filesystem ObjectStore |
| `agent_host.flowcraft.state_store` | `kv.Store` | Memory 或 Badger KV |
| `agent_host.flowcraft.history_store` | `logstore.MutableStore` | ClickHouse LogStore；不可变的 Volc LogStore 会被拒绝 |
| `agent_host.flowcraft.memory_objects_store` | `objectstore.ObjectStore` | filesystem ObjectStore |
| `agent_host.flowcraft.memory_store` | `memory.Store` | Flowcraft、Mem0 或 Volc Memory |
| `agent_host.eino.memory_store` | `memory.Store` | Flowcraft、Mem0 或 Volc Memory |

`agent_host` 是这些绑定的唯一依据。省略整个 block 或某个内层引用会禁用对应可选能力；Store 名称本身不具有保留绑定语义。未知名称、错误 Store kind、不可变 History Store、未知字段或空引用都会让 Server 构造失败，不会 fallback。

对于 Flowcraft 和 Pet，已配置的 `memory_store` 优先于由 `memory_objects_store` 支撑的内嵌 Flowcraft provider。选择外部 Store 时，专属于内嵌 provider 的 extraction、embedding、rerank、graph、layout 与 tier 配置会被拒绝。Eino Workflow 未声明 Memory policy 时不要求 Store；声明后必须配置 `agent_host.eino.memory_store`。

同一个逻辑 Memory Store 可以同时绑定给两个 factory。每个 Workspace Agent 借用一个以 Workspace 名称作为 `AppID` 的 App-scoped view。该 view 不会从 owner 或 Peer public key 推导 `UserID`，也不会改写 Agent 自己提供的 User、Agent 或 Run 维度。Memory runtime status 使用命令层记录的 provider kind，而不是检查 Store 的具体 Go 类型。

修改引用后必须重启进程。GizClaw 不会在绑定变化时迁移、合并、复制或删除数据；Scope 或 locator 发生不兼容变化后会重新创建开发期 Memory 数据。Store Registry 拥有全部共享 backend，并在 Server shutdown 时各关闭一次；Workspace reload 和 Agent teardown 只关闭 per-Agent adapter。

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
