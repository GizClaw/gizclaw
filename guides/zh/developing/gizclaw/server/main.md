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
```

这些引用同时适用于分层的 `storage` 加 `stores` 布局和受支持的单层 `stores` 布局。Backend 配置仍属于被引用的 Store；`agent_host` 不接受目录、DSN、credential、prefix、scope 或 inline backend。

| 字段 | 必需 capability | 支持的 backend |
| --- | --- | --- |
| `agent_host.runtime_store` | `objectstore.ObjectStore` | filesystem ObjectStore |
| `agent_host.flowcraft.state_store` | `kv.Store` | Memory 或 Badger KV |
| `agent_host.flowcraft.history_store` | `logstore.MutableStore` | ClickHouse LogStore；不可变的 Volc LogStore 会被拒绝 |
| `agent_host.flowcraft.memory_objects_store` | `objectstore.ObjectStore` | filesystem ObjectStore |

`agent_host` 是这些绑定的唯一依据。省略整个 block 或某个内层引用会禁用对应可选能力；Store 名称本身不具有保留绑定语义。未知名称、错误 Store kind、不可变 History Store、未知字段或空引用都会让 Server 构造失败，不会 fallback。Flowcraft 或 Pet Workflow 启用长期 Memory 后，在构造 Agent 时必须能够取得 `memory_objects_store`；State 与 Flowcraft 内部 History 仍是可选能力。

修改引用后必须重启进程。GizClaw 不会在绑定变化时迁移、合并、复制或删除数据。Store Registry 拥有全部共享 backend，并在 Server shutdown 时各关闭一次；Workspace reload 和 Agent teardown 只关闭 per-Agent adapter。

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
