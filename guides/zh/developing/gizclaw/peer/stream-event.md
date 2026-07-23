# Stream Events

`实现文件：peer_stream_event.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_stream_event.go` | 维护连接级 Peer event subscriber/broadcast broker；编码、校验 `PeerEvent` Protobuf；在 stream payload 与 GenX message chunk 之间双向转换；广播 Agent output 与资源失效事件，并把合法的上行 stream event 推回 Agent input source。 |

这个前缀拥有 GizClaw Peer event stream 与 GenX chunk 之间的产品映射。底层 stream transport 属于 `pkgs/giznet`；领域状态变化仍由产生事件的 service 拥有。

Event types、字段、方向和 BOS/EOS 边界见 [Events Reference](/references/events)；Event Stream 与 media、direct packet、RPC stream 的关系见 [Streams Reference](/references/streams)。本页只记录实现职责。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerStreamEventBroker` | 管理当前连接唯一的 event stream subscriber，并广播产品事件。 |
| `peerAgentOutput` | 消费 Agent output，广播 events，并把 audio 交给 `MixerOutput`。 |
| `readPeerStreamEvent` / `writePeerStreamEvent` | 只接受 `FrameTypeBinary`，解码和编码 `PeerEvent` Protobuf，并校验 `type` 与 `oneof payload`。 |
| `peerStreamEventToChunk` | 将产品事件转换为 GenX message chunk。 |
| `peerStreamEventsFromChunk` | 将 GenX chunk 展开为一个或多个产品事件。 |
| `pushAgentChunk` | 将收到的事件 chunk 推入 Agent input source。 |

下行 audio 不存在 raw Direct Opus 分支。`MixerOutput` 按 `(StreamID, canonical MIME)` 维护独立 decoder 与 PCM track；MIME EOS 只关闭对应 track，control-only EOS 关闭该 route 的全部 tracks。普通 EOS 使用 `CloseWrite` 排空缓存，error EOS 使用 `CloseWithError` 丢弃对应 track 的缓存。
