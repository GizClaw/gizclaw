# Events

Peer Event Stream 是 Client / Device 与 Server 之间的连接级事件通道，用于传输逻辑流边界、文本和资源变化通知。实时音频通过独立的 WebRTC media track 传输。

事件类型、payload 和字段见 [peer_event.proto](https://github.com/GizClaw/gizclaw/blob/main/api/proto/events/peer_event.proto)；接收与发送实现见 [Peer connection](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_conn.go) 和 [Stream events](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_stream_event.go)。

## Event types

事件用于表达媒体的开始与结束、文本内容及资源失效。应用使用生成的事件类型处理 payload；具体枚举与消息结构直接查阅上述 Schema。

## Connection ownership and delivery

Event session 由 Peer connection 拥有。页面、Workspace 和本地 consumer 订阅同一个 session，关闭一个订阅不应关闭整个连接。重连由 connection owner 统一处理。

资源变化通知用于提示重新读取权威资源。它不是资源快照，也不是持久化队列；前台恢复、页面打开和重连时仍需主动刷新数据，不能只依赖通知。

## Logical stream lifecycle

逻辑 stream 用于关联同一轮输入或输出。开始、数据和结束通过相应 SDK 组合；音频输入需要先确认 Server 已准备接收，再发送媒体。

Event 和 RTP 属于独立通道，接收端不能假定它们跨通道按发送顺序到达。优先使用 SDK 的逻辑流接口，避免在 App 中重复实现音频边界、缓冲与收尾。

Server 将多个 Agent 的音频输出混音为连接的下行媒体；source 的生命周期与用户接收到的播放生命周期因此不同。输出聚合的说明见 [Stream Events](/zh/developing/gizclaw/peer/stream-event)。

## Resource payloads

资源事件携带当前接收者可见的资源引用，用于发起后续查询。Server 根据认证连接和资源权限选择接收者，Client 不能通过事件指定其他用户的访问范围。资源字段定义仍由事件 Schema 与对应 RPC Schema 拥有。

## Four different end boundaries

逻辑输出结束、一次 RPC 结束、本地订阅释放和 Peer connection 关闭是不同操作。用户界面退出通常释放自己的订阅；是否关闭连接由 connection owner 决定。一次调用或一轮对话的结束不应误伤其他 consumer。

可靠 service stream 与媒体的关系见 [Streams](./streams)；C 调用方的资源生命周期见 [gzc_event.h](https://github.com/GizClaw/gizclaw/blob/main/sdk/c/gizclaw/include/gzc_event.h)。
