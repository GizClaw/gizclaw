# Realtime Source

`实现文件：peer_realtime_source.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_realtime_source.go` | 实现 Peer realtime input source；打开和关闭 GenX stream、推送 message chunk，并为连续音频 chunk 绑定稳定 stream ID。 |

这里负责将 connection-scoped input 转换为 Agent runtime 可消费的 realtime source，不拥有通用 GenX stream contract 或 Agent 实例生命周期。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerRealtimeSource` | 持有当前 GenX input stream 与音频 stream ID 状态。 |
| `newPeerRealtimeSource` | 创建 Peer realtime source。 |
| `OpenAgentInput` | 打开供 Agent Host 消费的 input stream。 |
| `Push` | 将 Peer message chunk 推入当前 input stream。 |
| `bindAudioStreamID` | 为连续音频 chunk 绑定稳定 stream ID。 |
| `Close` | 关闭 source 和底层 stream。 |
