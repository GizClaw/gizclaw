# Tool Invocation

`Implementation file: rpc_tool.go`

Implement the link for Server to call Peer tool: parse target Peer ID, confirm online status, open RPC connection, send ToolInvoke request and decode response.

Tool resource, policy and actual execution semantics belong to `services/runtime/toolkit`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Manager.ToolPeerAvailable` | Determine whether the target Peer is online and can accept tool invocation. |
| `Manager.InvokePeerTool` | Parse the Peer ID, open the RPC stream and call the target Peer tool. |
| `rpcClient.InvokeTool` | Construct ToolInvoke request and decode typed response. |
| `parseToolPeerID` | Convert product peer ID to Giznet public key. |
