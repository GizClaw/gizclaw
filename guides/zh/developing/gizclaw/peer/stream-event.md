# Stream Events

`实现文件：peer_stream_event.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_stream_event.go` | 维护 Peer event subscriber/broadcast broker；在 `PeerStreamEvent` 与 GenX message chunk 之间双向转换；处理 text、control、blob/audio 事件；广播 Agent output event，并把收到的事件推回 Agent input source。 |

这个前缀拥有 GizClaw Peer event stream 的产品映射。底层 stream transport 属于 `pkgs/giznet`；领域状态变化仍由产生事件的 service 拥有。完整 Peer connection 中的 media、packet、Event Stream 和动态 service stream 关系见 [Connection](./conn#一条-peer-connection-内的传输拓扑)。

`EventStreamAgent 0x20` 是 Client 主动打开、Server 接受的可靠双向 service stream：

- 上行事件由 Client 发给 Server，转换为 GenX chunk 后推入 Agent input source。
- 下行事件来自 Agent output chunk，Server 将其转换为 `PeerStreamEvent` 并广播给当前 subscribers。
- BOS/EOS 是某个 `stream_id` 的业务边界，不表示 Event Stream DataChannel 本身结束。实时 Opus payload 通过 WebRTC media track 传输，Event Stream 只传输对应的 control 和产品事件。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerStreamEventBroker` | 管理 event stream subscribers 并广播产品事件。 |
| `peerAgentOutput` | 消费 Agent output，广播 events，并把 audio 交给 `MixerOutput`。 |
| `readPeerStreamEvent` / `writePeerStreamEvent` | 解码和编码 Peer stream event。 |
| `peerStreamEventToChunk` | 将产品事件转换为 GenX message chunk。 |
| `peerStreamEventsFromChunk` | 将 GenX chunk 展开为一个或多个产品事件。 |
| `pushAgentChunk` | 将收到的事件 chunk 推入 Agent input source。 |

下行 audio 不存在 raw Direct Opus 分支。`MixerOutput` 按 `(StreamID, canonical MIME)` 维护独立 decoder 与 PCM track；MIME EOS 只关闭对应 track，control-only EOS 关闭该 route 的全部 tracks。普通 EOS 使用 `CloseWrite` 排空缓存，error EOS 使用 `CloseWithError` 丢弃对应 track 的缓存。
