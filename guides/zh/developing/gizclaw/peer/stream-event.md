# Stream Events

`实现文件：peer_stream_event.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_stream_event.go` | 维护 Peer event subscriber/broadcast broker；在 `PeerStreamEvent` 与 GenX message chunk 之间双向转换；处理 text、control、blob/audio 事件；将 Agent output 编码为 event stream 或 stamped Opus direct packet；控制 Opus 发送节奏并把收到的事件推回 Agent input source。 |

这个前缀拥有 GizClaw Peer event stream 的产品映射。底层 stream transport 属于 `pkgs/giznet`；领域状态变化仍由产生事件的 service 拥有。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerStreamEventBroker` | 管理 event stream subscribers 并广播产品事件。 |
| `peerAgentOutput` | 消费 Agent output，将其转换为 events 或 stamped Opus packets。 |
| `peerOpusPacer` | 控制连续 Opus frame 的发送节奏。 |
| `readPeerStreamEvent` / `writePeerStreamEvent` | 解码和编码 Peer stream event。 |
| `peerStreamEventToChunk` | 将产品事件转换为 GenX message chunk。 |
| `peerStreamEventsFromChunk` | 将 GenX chunk 展开为一个或多个产品事件。 |
| `pushAgentChunk` | 将收到的事件 chunk 推入 Agent input source。 |
