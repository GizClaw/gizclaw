# RPC Server

`Implementation file: rpc_server.go`

Define `rpcServer`, required domain service interfaces, connection handler, total dispatch and all Server RPC handlers. It dispatches normal or streaming requests based on the RPC method and converts between RPC payloads and domain service types.

Server methods cover Peer info, runtime status, run Agent, run workspace, history, memory recall, reload, stop and say. For methods that have been planned but not yet implemented in the contract, this file returns a unified not-implemented response. It has RPC composition and adaptation, but no domain rules for peers, runtime, firmware or gameplay.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcServer` | Aggregate caller identity, Peer/runtime/resource services and streaming handlers. |
| `rpcPeerService` / `rpcPeerRunService` / `rpcPeerRunRuntime` | Minimal domain interfaces that RPC Server depends on. |
| `Handle` | Start RPC request loop on connection. |
| `dispatch` | Distributes normal request/response RPC methods. |
| `dispatchStream` | Distribute streaming RPC methods that require consecutive frames. |
| `handleGetInfo` / `handlePutInfo` | Read or update the current Peer device info. |
| `handleGetRuntime` / `handleGetStatus` | Query Peer runtime and run status. |
| `handleGetRunAgent` / `handleSetRunAgent` | Query or select the currently running Agent. |
| `handleGetRunWorkspace` / `handleSetRunWorkspace` / `handleReloadRunWorkspace` | Manage the current run workspace. |
| `handleListRunWorkspaceHistory` / `handlePlayRunWorkspaceHistory` | List or play workspace history. |
| `handleGetRunWorkspaceMemoryStats` / `handleRunWorkspaceRecall` | Query memory stats or execute recall. |
| `handleReloadRun` / `handleGetRunStatus` / `handleStopRun` | Control the complete run lifecycle. |
| `handleServerRunSay` | Submit say input to the current run. |
| `runWorkspaceState` | Aggregate Agent selection and run status into workspace state. |
| `isPlannedServerMethod` / `rpcNotImplemented` | Identify methods that have been planned but not yet implemented, and generate a unified response. |

`server.run.say` accepts `text` and a RuntimeProfile `voice_alias`. It does not accept canonical Voice, Model, or Credential identifiers.
