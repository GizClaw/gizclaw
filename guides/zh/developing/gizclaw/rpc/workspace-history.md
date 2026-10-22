# Workspace History

源码：[rpc_workspace_history.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_workspace_history.go)

处理 Workspace history audio download RPC：读取指定 history entry 的音频 metadata 和内容，并通过 RPC stream 返回 binary frames。

History 数据和音频存储由 workspace/runtime service 拥有。

History entry 的 `audio_available` 表示 metadata 中存在保留的 Ogg/Opus 录音附件；
`replay_available` 也可以仅由文字内容提供。HTTP、Admin 和 Peer RPC 使用相同投影，
音频播放控件使用 `audio_available`，不把文字回放能力当作录音存在的证明。

Peer RPC 以 `name` 暴露每条 History 记录的稳定身份；play 和 audio 请求把同一个值作为 `history_name` 传回。History 没有独立的 Peer alias，因此该值是 canonical internal History ID 的原样投影。`actor_name` 只表示归属展示，永远不是身份或 selector。Admin History route 继续以 canonical `id` 暴露和选择同一条底层记录。

`server.workspace.history.list` 与设备 HTTP `GET /gizclaw/v1/device/workspaces/{workspaceId}/history` 使用同一套服务端查询：`order` 选择 `asc` 或 `desc`，可选的 `start_time_ms`（含）与 `end_time_ms`（不含）按 Unix 毫秒限定创建时间，续页请求同样生效。`cursor` 是不含自身的 entry ID 边界，可以是上一页的 `next_cursor`，也可以是任一条目的 `name`，用于从该条目按所请求方向继续读取。负数时间或 `start_time_ms` 不早于 `end_time_ms` 返回 `INVALID_ARGUMENT`。

History 是 Workflow Workspace 的能力。Friend 与 Friend Group 的 SFU Workspace 没有 History：`server.run.workspace.history` 返回空列表，`server.run.workspace.history.play` 返回 `not_found` 状态，`server.workspace.history.*` 与 Admin history route 返回空结果，都不是 not supported 错误；这类 Workspace 也不发送 `workspace_history_updated`。
