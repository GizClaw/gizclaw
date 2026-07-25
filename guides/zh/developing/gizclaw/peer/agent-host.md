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

Server 在启动时只解析一次 `agent_host.flowcraft.memory_store` 与
`agent_host.eino.memory_store`。Peer AgentHost 只借用这些 provider-neutral
`memory.Store` capability，不拥有或关闭它们。Flowcraft 与 Eino 只把
Workspace ID 绑定到 `Scope.AppID`；Peer identity 和 public key 永远不会被
替换成 `Scope.UserID`，调用方传入的 User、Agent、Run 维度保持不变。

新增 Workflow factory 都是相互独立的配置 adapter。它们不依赖 ToolCall，
也不会把 Workspace Toolkit policy 翻译成 provider-native tool。
