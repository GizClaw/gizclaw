# Workspace History

`Implementation file: rpc_workspace_history.go`

Process Workspace history audio download RPC: Read the audio metadata and content of the specified history entry, and return binary frames through the RPC stream.

History data and audio storage are owned by the workspace/runtime service.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | The minimum service interface that the History audio handler depends on. |
| `handleWorkspaceHistoryAudioGet` | Verify the request, obtain the history audio, and write out the metadata and binary frames. |
