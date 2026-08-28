# Realtime Source

`实现文件：peer_realtime_source.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_realtime_source.go` | 实现 Peer realtime input source；打开和关闭 GenX stream、推送 message chunk，并为连续音频 chunk 绑定稳定 stream ID。 |

这里负责将 connection-scoped input 转换为 Agent runtime 可消费的 realtime source，不拥有通用 GenX stream contract 或 Agent 实例生命周期。恢复逻辑由 `PeerConn` 与 AgentHost 拥有：workspace transition 中观察到 runtime revision 已变化的 chunk 必须丢弃，不能重试进入新的 source。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerRealtimeSource` | 持有当前 GenX input stream，以及已准入音频 route 的 stream ID 与 canonical MIME。 |
| `newPeerRealtimeSource` | 创建 Peer realtime source。 |
| `OpenAgentInput` | 打开供 Agent Host 消费的 input stream。 |
| `Push` | 将 Peer message chunk 推入当前 input stream。 |
| `bindAudioStreamIDLocked` | 在 source mutex 内为连续音频 chunk 绑定稳定 stream ID。 |
| `Close` | 关闭 source 和底层 stream。 |

`OpenAgentInput` 替换 runtime input 时，在 source mutex 内同时捕获并清除旧的 active
audio route，然后发布新的 GenX input。replacement callback 在释放 source mutex 后、
函数返回新 input 前执行；没有 active route 时不调用。route 一旦被捕获，后续 reopen
不能再次报告它。callback 失败会关闭这次新 input 并把错误返回 AgentHost，旧 route
不会恢复。

`bindAudioStreamIDLocked` 只在合法 audio BOS 上记录 route ID 和 canonical MIME，并把
route 绑定与当前 input generation 的选择保持在同一短临界区。runtime
替换清除绑定后，replacement BOS 到达前的 Opus packet 继续由现有 gate 丢弃；source
不会把旧 ID 自动绑定到新 runtime。
