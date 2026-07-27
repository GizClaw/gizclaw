# Agent Host

`实现文件：peer_agent_host.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_agent_host.go` | 创建当前 Peer 专用的 Agent Host，注册持久化 Workflow driver，并注入从 Server 借用的 capability。 |

该文件只负责 Peer connection 上的 Host 接线。Agent instance、输入输出、history、toolkit 与运行生命周期属于 `services/runtime/agenthost`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `newPeerAgentHost` | 创建 Peer-scoped Agent Host，安装 Peer GenX provider，并注册 Flowcraft、DashScope Realtime、Doubao Realtime Duplex、Eino 等受支持的 Workflow factory。 |

Resolver 从 Workspace Workflow 顶层 `memory` 读取 alias，并在同一个 owner
RuntimeProfile snapshot 中解析 `MemoryLayout`、driver 与 typed connection。
Flowcraft 与 Eino factory 消费同一个 provider-neutral `memory.Store` contract；
Graph node 决定 Recall/Observe 映射。Workspace ID 是 `Scope.AppID`，Peer
identity 和 public key 不会替换成 `Scope.UserID`。

Runtime Registry 只以 Workspace 为 live Agent identity。同一 Workspace 的多个
stream 共用一个可并发 Agent；最后一个引用释放后关闭 generation，reload 后按新
Workflow 与 RuntimeProfile snapshot 构造。Caller-specific toolkit 和 model
resolution 发生在构造当前 snapshot 或每次 invocation 的既有上下文中，不增加
第二个 Workspace Agent identity。

新增 Workflow factory 都是相互独立的配置 adapter。它们不依赖 ToolCall，
也不会把 Workspace Toolkit policy 翻译成 provider-native tool。
