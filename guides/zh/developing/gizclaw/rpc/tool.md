# Tool Invocation

`实现文件：rpc_tool.go`

实现 Server 调用 Peer tool 的链路：解析目标 Peer ID、确认在线状态、打开 RPC connection、发送 ToolInvoke request 并解码 response。

Tool resource、policy 和实际执行语义属于 `services/runtime/toolkit`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Manager.ToolPeerAvailable` | 判断目标 Peer 是否在线并可接受 tool invocation。 |
| `Manager.InvokePeerTool` | 解析 Peer ID、打开 RPC stream 并调用目标 Peer tool。 |
| `rpcClient.InvokeTool` | 构造 ToolInvoke request 并解码 typed response。 |
| `parseToolPeerID` | 将产品 Peer ID 转换为 Giznet public key。 |
