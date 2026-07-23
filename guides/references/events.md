# Events

Peer Event Stream 是 Client / Device 与 Server 之间的连接级事件通道。它使用
service ID `0x20`（`EventStreamAgent`），可靠、有序、双向，并通常在整条 Peer
connection 生命周期内保持一条。实时 Opus bytes 仍通过 WebRTC audio RTP track
传输；Event Stream 只承载逻辑流边界、文本和资源失效通知。

每个 event 都是 `gizclaw.events.v1.PeerEvent` Protobuf message，通过
`FrameTypeBinary` 发送。当前 `version` 为 `1`。顶层 `type` 与 `oneof payload`
必须匹配，完整定义位于
[`api/proto/events/peer_event.proto`](https://github.com/GizClaw/gizclaw/blob/main/api/proto/events/peer_event.proto)。

## Event types

| `type` | 方向 | payload | 作用 |
| --- | --- | --- | --- |
| `BOS` | 双向 | `bos` | 开始一个由 `stream_id` 标识的逻辑 stream。 |
| `EOS` | 双向 | `eos` | 结束一个逻辑 stream；可携带 typed `error`。 |
| `TEXT_DELTA` | 双向 | `text_delta` | 发送增量文本片段。 |
| `TEXT_DONE` | 双向 | `text_done` | 发送最终文本片段并结束文本逻辑 stream。 |
| `WORKSPACE_HISTORY_UPDATED` | Server → Client / Device | `workspace_history_updated` | 指定 Workspace 的 history 已持久化更新。 |
| `FRIEND_RELATIONSHIP_UPDATED` | Server → Client / Device | `friend_relationship_updated` | 好友关系已创建或删除。 |
| `FRIEND_GROUP_UPDATED` | Server → Client / Device | `friend_group_updated` | 群组或成员关系已变化。 |

`kind` 的含义由选中的 payload 决定：

- `bos` / `eos` 使用 `StreamKind`：`TEXT`、`AUDIO`、`VIDEO` 或 `MIXED`。
- `workspace_history_updated.workspace_kind` 使用 `WorkspaceKind`：
  `WORKFLOW`、`DIRECT_CHATROOM` 或 `GROUP_CHATROOM`。
- 好友与群组事件分别使用自己的 `change` enum，不复用媒体 `kind`。

Direct Chatroom 与 Group Chatroom 共用 Chatroom Workflow driver，但它们的成员
拓扑和授权不同，因此通过 Workspace parameters 和 `WorkspaceKind` 明确区分，
不拆成两个 Agent driver。

## Connection ownership and delivery

Peer Event Stream 属于 Peer connection，不属于某个 Workspace、Agent 或页面。
一个 Client 应只维护一个 connection-scoped event session，再在本地按
`workspace_name`、`peer_public_key` 或 `friend_group_id` 分发。页面与 controller
不能各自打开新的 `0x20` channel。

资源事件是 best-effort invalidation hint，不是权威资源快照：

- Server 只在对应持久化操作成功后发送。
- Event 不要求 ACK、不离线排队、不 replay；投递失败不回滚业务状态。
- Client 收到 Event 后重新调用相关 RPC，不能直接把 payload 当作完整资源写入。
- 重复 Event 可以合并；同一资源刷新期间再次失效时，Client 最多追加一轮刷新。
- 页面打开、切换、前台恢复和重连仍必须主动拉取权威数据，以收敛错过的 Event。

好友关系事件发给关系双方。群组事件发给所有受影响的当前成员和变更前成员。
Workspace history 事件只发给当前有权访问该 Workspace 的在线 Peer。接收者由
Server 根据认证连接和权威 relationship 计算，不能由 Client payload 指定。

## Logical stream lifecycle

`stream_id` 关联同一轮的 BOS、内容与 EOS。`TEXT_DONE` 已包含文本结束语义，不需要
额外发送 EOS。音频的实时 packet 不在 Protobuf event 中：

```text
BOS(kind=AUDIO, stream_id=audio-42)
  + WebRTC RTP Opus packets
EOS(kind=AUDIO, stream_id=audio-42)
```

Server 只在对应的 `BOS(kind=AUDIO)` 通过本轮授权后接收该轮 Opus packets；
先到达或绕过 BOS 的 packets 会被丢弃。EOS 会关闭该轮输入 gate。

EOS 的 `error` 包含稳定 `code`、安全 fallback `message` 和 `retryable`。带 error
的 EOS 只结束对应逻辑 stream，不关闭 service `0x20`，其他事件和后续 turn 仍可
继续传输。

Chatroom 在每轮输入前检查权威访问权。已撤权的 active Chatroom 不自动切换
Workspace，也不生成拒绝文本或音频，而是返回同一 `stream_id` 的 EOS error：

| `code` | 含义 | `retryable` |
| --- | --- | ---: |
| `CHATROOM_FRIEND_REMOVED` | Direct Chat 好友关系已不存在。 | false |
| `CHATROOM_MEMBER_REMOVED` | Peer 已不是现有群组成员。 | false |
| `CHATROOM_GROUP_DELETED` | 群组已删除或正在删除。 | false |
| `CHATROOM_ACCESS_CHECK_FAILED` | 权威访问查询失败或状态不一致。 | true |

Client 应按 `code` 本地化显示，并结束对应的 loading/recording 状态；不能导航到
其他页面或自动选择默认 Workspace。未知 code 使用安全 fallback 或通用错误。

## Resource payloads

`workspace_history_updated`：

- `workspace_name`
- `workspace_kind`
- `last_updated_at_unix_ms`

`friend_relationship_updated`：

- `peer_public_key`：以当前接收者视角表示对端 Peer
- `workspace_name`
- `change`：`CREATED` 或 `DELETED`
- `revision_unix_ms`

`friend_group_updated`：

- `friend_group_id`
- `workspace_name`
- `change`：`CREATED`、`DELETED`、`MEMBER_ADDED`、`MEMBER_REMOVED` 或
  `MEMBER_ROLE_CHANGED`；群名称或描述变化使用 `METADATA_UPDATED`
- `revision_unix_ms`
- `affected_peer_public_key`：成员增删或角色变化时标识被修改的成员；
  群级变化时为空

## Four different end boundaries

| 边界 | 结束的对象 | 是否继续使用同一 Peer connection |
| --- | --- | --- |
| PeerEvent `EOS` / `TEXT_DONE` | 一个 `stream_id` 对应的逻辑 stream | 是 |
| RPC `FrameTypeEOS` | 当前方向的一段 RPC frame sequence | 是 |
| Service DataChannel EOF / close | 一条 Event、RPC 或 HTTP transport stream | 其他 channel 可以继续 |
| WebRTC connection close | 整条 Peer connection 及其 media、packet、service streams | 否 |

实现层的事件与 GenX chunk 映射见[开发指引：Stream Events](/zh/developing/gizclaw/peer/stream-event)。
