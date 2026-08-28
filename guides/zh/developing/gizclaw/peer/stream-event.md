# Stream Events

`实现文件：peer_stream_event.go`

| 文件 | 包含的功能 |
| --- | --- |
| `peer_stream_event.go` | 维护连接级 Peer event subscriber/broadcast broker；编码、校验 `PeerEvent` Protobuf；在 stream payload 与 GenX message chunk 之间双向转换；广播 Agent output 与资源失效事件，并把合法的上行 stream event 推回 Agent input source。 |

这个前缀拥有 GizClaw Peer event stream 与 GenX chunk 之间的产品映射。底层 stream transport 属于 `pkgs/giznet`；领域状态变化仍由产生事件的 service 拥有。

Event types、字段、方向和 BOS/EOS 边界见 [Events Reference](/references/events)；Event Stream 与 media、direct packet、RPC stream 的关系见 [Streams Reference](/references/streams)。本页只记录实现职责。

`peerStreamEventBroker` 的唯一 writer subscriber 是当前 Peer connection 在 ready
前绑定的 physical `0x20`。它不是可选页面 service；physical stream 关闭会结束
整条 Peer connection。SDK 可在该 connection owner 之上建立多个本地 subscriber，
conversation、Workspace reload/set 和 controller disposal 只管理本地订阅，
不能调用 transport open/close。

runtime input replacement 产生的 `INPUT_ROUTE_RELOADED` EOS 也由这个 broker 在唯一
writer 上有序发送。route state owner 在发起 `Broadcast` 前已经清除旧输入授权；
`Broadcast` 返回成功后 AgentHost 才能完成 reload。写入失败不会改走另一条 Event
Stream、不会恢复旧 route，也不会把 mandatory signal 降级为 best-effort notification。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerStreamEventBroker` | 管理当前连接唯一、必需的 physical event writer，并广播产品事件。 |
| `peerAgentOutput` | 消费 Agent output，把 source audio 交给 `MixerOutput`，并广播包括混音总生命周期在内的 events。 |
| `readPeerStreamEvent` / `writePeerStreamEvent` | 只接受 `FrameTypeBinary`，解码和编码 `PeerEvent` Protobuf，并校验 `type` 与 `oneof payload`。 |
| `peerStreamEventToChunk` | 将产品事件转换为 GenX message chunk。 |
| `peerStreamEventsFromChunk` | 将 GenX chunk 展开为一个或多个产品事件。 |
| `pushAgentChunk` | 将收到的事件 chunk 推入 Agent input source。 |

下行 audio 不存在 raw Direct Opus 分支。`MixerOutput` 按 `(StreamID, canonical MIME)` 维护独立 decoder 与 PCM track；MIME EOS 只关闭对应 track，control-only EOS 关闭该 route 的全部 tracks。普通 EOS 使用 `CloseWrite` 排空缓存，error EOS 使用 `CloseWithError` 丢弃对应 track 的缓存。

`peerAgentOutput` 只在 `MixerOutput` 排空对应 track 后观察这些 source boundary，并把它们聚合成一条 mixed-audio epoch：第一条 active source 触发 `BOS(kind=AUDIO)`，重叠 source 的 boundary 不下发，只有最后一条 active source 被移除时才触发 `EOS(kind=AUDIO)`。route-level interruption 只移除对应 route。总 boundary 沿用第一条 source 的 stream ID 与 label，sequence 归零，MIME 留空；原因是连接只有一条固定 Opus 下行 channel。source MIME 只供 decoder 使用，不描述混音后的 wire channel。

对经 Edge 路由的 Peer，`peerAgentOutput` 只通过 producer response epoch 的不可变 input-route provenance 把 output 绑定到 logical turn。该关联跨越 input replacement 存活，因此旧 response 即使第一次被迟到观察也仍归属旧 `turn_index`；self-start 或第三方无 provenance response 绝不 fallback 到 current turn。记录有界 `output_terminal` 并完成 owning turn 的是 response-complete boundary，而不是第一个 MIME EOS。

Produced accounting 分类 GenX source chunk；delivered accounting 只分类 `Broadcast` 成功后的实际 `PeerEvent`，aggregate audio 还必须已经完成 mixer drain。Broadcast 失败、drain 失败或 abandon，以及被聚合器抑制的 audio boundary 都不会增加 delivered modality。空 label text/blob event 沿用 Peer client 的 assistant fallback；空 label control-only EOS 仍为 `other`。Terminal snapshot 只包含排序后的封闭 source part/label 与 Peer event type/kind/label class；payload 和 raw label 始终排除。
