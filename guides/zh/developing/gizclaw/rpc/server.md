# RPC Server

源码：[rpc_server.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_server.go)

定义 `rpcServer`、所需领域 service interfaces、connection handler、总 dispatch 和全部 Server RPC handlers。它根据 RPC method 分派普通或 streaming 请求，并在 RPC payload 与领域 service 类型之间转换。

Server methods 覆盖 Peer info、runtime status、run Agent、run workspace、history、memory recall、reload、stop 和 say。对于 contract 中已经规划但尚未实现的 methods，该文件返回统一的 not-implemented response。它拥有 RPC composition 与适配，不拥有 Peer、runtime 或 firmware 的领域规则。

`server.run.say` 只接收 `text` 与 RuntimeProfile scoped `voice_name`，不接受真实 Voice、Model 或 Credential 标识符。

`handleReloadRunWorkspaceWithOptions` 可接收目标 `workspace_name` 和局部 `parameters`，复用 Peer resource 的参数校验与更新，再保存选择、执行一次 reload；省略目标时使用当前选择。完整请求与失败语义见 [Server Provided to Client](../../api/proto/rpc/server-provided-to-client)。
