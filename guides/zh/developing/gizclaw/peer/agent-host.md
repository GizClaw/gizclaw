# Agent Host

`实现文件：peer_agent_host.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_agent_host.go` | 基于通用 `agenthost.Host` 创建当前 Peer 专用的 Agent Host，并接入 Peer-backed GenX provider。 |

该文件只负责 Peer connection 上的 Host 接线。Agent instance、输入输出、history、toolkit 与运行生命周期属于 `services/runtime/agenthost`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `newPeerAgentHost` | 基于通用 Host 创建 Peer-scoped Agent Host，并安装 Peer GenX provider。 |
