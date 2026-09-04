# Workspace History

`Implementation file: rpc_workspace_history.go`

Process Workspace history audio download RPC: Read the audio metadata and content of the specified history entry, and return binary frames through the RPC stream.

History data and audio storage are owned by the workspace/runtime service.

Peer RPC exposes each History record's stable identity as `name`; play and audio requests pass that same value as `history_name`. The value is the canonical internal History ID projected verbatim because History has no separate Peer alias. `actor_name` is attribution text and is never an identity or selector. Admin History routes continue to expose and select the same underlying record through canonical `id`.

History is a Workflow Workspace capability. Friend and Friend Group SFU Workspaces have no History: `server.run.workspace.history` returns an empty list, `server.run.workspace.history.play` returns the `not_found` state, and `server.workspace.history.*` plus the Admin history routes return empty results, never a not-supported error; those Workspaces also never emit `workspace_history_updated`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | The minimum service interface that the History audio handler depends on. |
| `handleWorkspaceHistoryAudioDownload` | Verify the request, obtain the history audio, and write out the metadata and binary frames. |
| `writeHistoryAudioResponse` | Shared metadata, binary-frame, and EOS writer. |
