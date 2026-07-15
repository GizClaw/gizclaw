# Workspace History

`实现文件：rpc_workspace_history.go`

处理 Workspace history audio download RPC：读取指定 history entry 的音频 metadata 和内容，并通过 RPC stream 返回 binary frames。

History 数据和音频存储由 workspace/runtime service 拥有。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | History audio handler 依赖的最小 service interface。 |
| `handleWorkspaceHistoryAudioGet` | 验证请求，取得 history audio，并写出 metadata 与 binary frames。 |
