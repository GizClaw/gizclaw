# Events

Peer Event Stream 用于在 Client / Device 与 Server 之间传递 Agent 的生命周期、文本和产品通知。Client 使用 service ID `0x20`（`EventStreamAgent`）打开一条可靠、有序、双向的 service stream，并通常在整个 Peer connection 生命周期内保持它。

每条消息都是一个 `PeerStreamEvent` JSON object。当前协议版本为 `1`；发送端使用 RPC `Text` frame，接收端同时接受 `Text` 和 `JSON` frame。完整 Schema 位于 `api/http/shared/peer_stream_event.json`。

实时 Opus bytes 不放在 Event Stream 中，而是通过 WebRTC audio RTP track 传输。Event Stream 只携带音频等逻辑内容的 BOS、EOS、MIME 和错误信息。各传输通道的关系见 [Streams](./streams)。

## Event types

| `type` | 方向 | 作用 |
| --- | --- | --- |
| `bos` | 双向 | 开始一个由 `stream_id` 标识的逻辑 stream。可以用 `kind` 与 `mime_type` 声明内容类型。 |
| `eos` | 双向 | 结束一个逻辑 stream。`error` 非空时表示该 stream 异常结束；不会关闭 Event Stream service。 |
| `text.delta` | 双向 | 发送一个增量文本片段；同一逻辑 stream 的片段使用相同 `stream_id`。 |
| `text.done` | 双向 | 发送最后一个文本片段并同时结束该逻辑 stream。 |
| `workspace.history.updated` | Server → Client / Device | 通知当前 Workspace history 已更新；优先使用 `last_updated_at` 表示更新时间。 |

上行事件进入 Agent input；下行事件来自 Agent output，并广播给当前 Event Stream subscribers。

## Fields

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `v` | integer | 是 | Event 协议版本，当前为 `1`。 |
| `type` | string | 是 | 上表中的事件类型。未知类型会被拒绝。 |
| `stream_id` | string | 否 | 关联同一逻辑 stream 的稳定标识。BOS、内容和 EOS 应使用相同值。 |
| `seq` | int64 | 否 | 发送端提供的非负事件序号；它不是 RPC frame 序号。 |
| `timestamp` | int64 | 否 | Unix 时间戳，单位为毫秒。 |
| `last_updated_at` | date-time | 否 | RFC 3339 时间，供 `workspace.history.updated` 表示最后更新时间。 |
| `kind` | string | 否 | 逻辑内容类型：`text`、`audio`、`video` 或 `mixed`。 |
| `label` | string | 否 | 产品或控制标签；history 更新使用 `workspace.history.updated`。 |
| `mime_type` | string | 否 | 逻辑内容的 MIME type。`kind=audio` 且未提供时按 `audio/opus` 处理。 |
| `text` | string | 否 | `text.delta` 或 `text.done` 的文本内容。 |
| `error` | string | 否 | 逻辑 stream 异常结束时的错误消息。 |

Schema 禁止未声明字段。除 `v` 和 `type` 外，字段是否出现取决于事件类型；接收端不能假设可选字段一定存在。

## Text stream example

以下三条事件表示一个完整的文本 stream：

```json
{"v":1,"type":"bos","stream_id":"answer-42","kind":"text"}
{"v":1,"type":"text.delta","stream_id":"answer-42","text":"你好，"}
{"v":1,"type":"text.done","stream_id":"answer-42","text":"世界。"}
```

`text.done` 已经包含该逻辑 stream 的结束语义，不需要再发送一条 `type=eos`。

## Audio lifecycle example

```json
{"v":1,"type":"bos","stream_id":"audio-42","kind":"audio","mime_type":"audio/opus"}
{"v":1,"type":"eos","stream_id":"audio-42","kind":"audio","mime_type":"audio/opus"}
```

这两条事件只描述逻辑边界；`audio-42` 的实时 Opus packets 仍由 RTP track 承载。

## Workspace history notification

```json
{
  "v": 1,
  "type": "workspace.history.updated",
  "last_updated_at": "2026-07-21T08:30:00Z"
}
```

## Four different end boundaries

| 边界 | 结束的对象 | 是否继续使用同一 Peer connection |
| --- | --- | --- |
| Event `type=eos` / `text.done` | 一个 `stream_id` 对应的业务 stream | 是 |
| RPC `FrameTypeEOS` | 当前方向的一段 RPC frame sequence | 是 |
| Service DataChannel EOF / close | 一条 Event、RPC 或 HTTP transport stream | 其他 channel 可以继续 |
| WebRTC connection close | 整条 Peer connection 及其 media、packet、service streams | 否 |

实现层的事件与 GenX chunk 映射见[开发指引：Stream Events](/zh/developing/gizclaw/peer/stream-event)。
