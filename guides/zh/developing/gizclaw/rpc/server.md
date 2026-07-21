# RPC Server

`实现文件：rpc_server.go`

定义 `rpcServer`、所需领域 service interfaces、connection handler、总 dispatch 和全部 Server RPC handlers。它根据 RPC method 分派普通或 streaming 请求，并在 RPC payload 与领域 service 类型之间转换。

Server methods 覆盖 Peer info、runtime status、run Agent、run workspace、history、memory recall、reload、stop 和 say。对于 contract 中已经规划但尚未实现的 methods，该文件返回统一的 not-implemented response。它拥有 RPC composition 与适配，不拥有 Peer、runtime、firmware 或 gameplay 的领域规则。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcServer` | 聚合 caller identity、Peer/runtime/resource services 与 streaming handlers。 |
| `rpcPeerService` / `rpcPeerRunService` / `rpcPeerRunRuntime` | RPC Server 依赖的最小领域 interfaces。 |
| `Handle` | 在 connection 上启动 RPC request loop。 |
| `dispatch` | 分发普通 request/response RPC methods。 |
| `dispatchStream` | 分发需要连续 frames 的 streaming RPC methods。 |
| `handleGetInfo` / `handlePutInfo` | 读取或更新当前 Peer device info。 |
| `handleGetRuntime` / `handleGetStatus` | 查询 Peer runtime 与 run status。 |
| `handleGetRunAgent` / `handleSetRunAgent` | 查询或选择当前运行 Agent。 |
| `handleGetRunWorkspace` / `handleSetRunWorkspace` / `handleReloadRunWorkspace` | 管理当前 run workspace。 |
| `handleListRunWorkspaceHistory` / `handlePlayRunWorkspaceHistory` | 列出或播放 workspace history。 |
| `handleGetRunWorkspaceMemoryStats` / `handleRunWorkspaceRecall` | 查询 memory stats 或执行 recall。 |
| `handleReloadRun` / `handleGetRunStatus` / `handleStopRun` | 控制完整 run lifecycle。 |
| `handleServerRunSay` | 向当前 run 提交 say input。 |
| `runWorkspaceState` | 聚合 Agent selection 与 run status 为 workspace state。 |
| `isPlannedServerMethod` / `rpcNotImplemented` | 识别已规划但尚未实现的 method，并生成统一响应。 |

`server.run.say` 只接收 `text` 与 RuntimeProfile `voice_alias`，不接受真实 Voice、Model 或 Credential 标识符。
