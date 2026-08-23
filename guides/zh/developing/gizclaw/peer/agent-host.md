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
Workflow 与 RuntimeProfile snapshot 构造。

Peer connection 还会为每次 run 附加一套独立的 current-Peer Tool execution
scope，其中只包含该 Peer 的 RuntimeProfile Tool binding snapshot，以及执行
`client_rpc` 的准确 accepted connection。Workspace-owner Resource access 不能覆盖
它。共享 Agent 只接收一个 `genx.ToolInvoker`；每次 Transform 都从自己的 context
解析 Tool，因此多个并发 Peer 即使共享 Agent，也不会共享 Tool definition、
handler、argument、result 或 connection。

Flowcraft、Eino、DashScope Realtime 与豆包 Realtime Duplex factory 都把同一个
接口注入已有 Transformer config。Provider ToolCall ID 与 continuation 始终留在
Transformer 内部；AgentHost 只按 canonical Resource name 分发到 `http_request`
或当前 connection 的 `client.tool.invoke`，不会把 Tool control traffic 投影到
public assistant stream。

OpenAI Responses 通过相同 canonical Resolver 与共享 Runtime Registry 建立 request-scoped direct Workspace attachment，不读取或修改 PeerRun selection。Server-side tool 继续遵守 Workflow policy；由于 OpenAI request 没有稳定的 client-tool transport contract，connection-scoped client tool 会 fail closed。受限 History observer 返回 Response projection 使用的准确已持久化 assistant entry。

当 assistant route 以 provider 或 runtime error EOS 结束时，Peer output adapter 会原样转发该 EOS，并输出一条带 Peer、当前 Workspace、stream、error code 与 retryable 关联字段的结构化 Server error 日志。预期的 `interrupted` turn replacement EOS 是控制事件，不作为故障记录。日志副作用不会让长期 output consumer 失败、追加第二个 EOS，也不会阻止后续 turn 或 Workspace reload。
