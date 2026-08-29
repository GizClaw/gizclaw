# Workspace History

`实现文件：rpc_workspace_history.go`

处理 Workspace history audio download RPC：读取指定 history entry 的音频 metadata 和内容，并通过 RPC stream 返回 binary frames。

History 数据和音频存储由 workspace/runtime service 拥有。

Peer RPC 以 `name` 暴露每条 History 记录的稳定身份；play 和 audio 请求把同一个值作为 `history_name` 传回。History 没有独立的 Peer alias，因此该值是 canonical internal History ID 的原样投影。`actor_name` 只表示归属展示，永远不是身份或 selector。Admin History route 继续以 canonical `id` 暴露和选择同一条底层记录。

Friend Group 消息读取复用这份 ownership，但不向客户端暴露 Workspace 名。Social adapter 校验当前成员身份后解析群组已保存的 Workspace binding；list 用一次内部 History page 完成投影，每条消息以 `name` 表示身份、以 `actor_name` 表示归属。get/audio 请求使用 `friend_group_name + history_name`，并复用同一套音频 reader/framing。响应依次为 metadata、binary frames 和 EOS，不使用 Friend Group 消息 store，也不发起嵌套 RPC。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | History audio handler 依赖的最小 service interface。 |
| `handleWorkspaceHistoryAudioDownload` | 验证请求，取得 history audio，并写出 metadata 与 binary frames。 |
| `handleFriendGroupMessageAudioDownload` | 按 Friend Group 鉴权并流式返回绑定的 History 音频。 |
| `writeHistoryAudioResponse` | 共享的 metadata、binary frame 与 EOS writer。 |
