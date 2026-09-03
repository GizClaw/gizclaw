# Workspace History

`实现文件：rpc_workspace_history.go`

处理 Workspace history audio download RPC：读取指定 history entry 的音频 metadata 和内容，并通过 RPC stream 返回 binary frames。

History 数据和音频存储由 workspace/runtime service 拥有。

Peer RPC 以 `name` 暴露每条 History 记录的稳定身份；play 和 audio 请求把同一个值作为 `history_name` 传回。History 没有独立的 Peer alias，因此该值是 canonical internal History ID 的原样投影。`actor_name` 只表示归属展示，永远不是身份或 selector。Admin History route 继续以 canonical `id` 暴露和选择同一条底层记录。

History 是 Workflow Workspace 的能力。Friend 与 Friend Group 的 SFU Workspace 没有 History：`server.run.workspace.history` 返回空列表，`server.run.workspace.history.play` 返回 `not_found` 状态，`server.workspace.history.*` 与 Admin history route 返回空结果，都不是 not supported 错误；这类 Workspace 也不发送 `workspace_history_updated`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | History audio handler 依赖的最小 service interface。 |
| `handleWorkspaceHistoryAudioDownload` | 验证请求，取得 history audio，并写出 metadata 与 binary frames。 |
| `writeHistoryAudioResponse` | 共享的 metadata、binary frame 与 EOS writer。 |
