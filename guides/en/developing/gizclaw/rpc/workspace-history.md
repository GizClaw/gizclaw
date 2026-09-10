# Workspace History

`Implementation file: rpc_workspace_history.go`

Process Workspace history audio download RPC: Read the audio metadata and content of the specified history entry, and return binary frames through the RPC stream.

History data and audio storage are owned by the workspace/runtime service.

Peer RPC exposes each History record's stable identity as `name`; play and audio requests pass that same value as `history_name`. The value is the canonical internal History ID projected verbatim because History has no separate Peer alias. `actor_name` is attribution text and is never an identity or selector. Admin History routes continue to expose and select the same underlying record through canonical `id`.

`server.workspace.history.list` shares the server-side query of device HTTP `GET /gizclaw/v1/device/workspaces/{workspaceId}/history`: `order` selects `asc` or `desc`, and optional `start_time_ms` (inclusive) and `end_time_ms` (exclusive) bound creation time in Unix milliseconds, including on continuation requests. `cursor` is an exclusive entry-ID boundary: the previous `next_cursor`, or any item `name` to continue from that item in the requested order. A negative time or a `start_time_ms` not before `end_time_ms` returns `INVALID_ARGUMENT`.

History is a Workflow Workspace capability. Friend and Friend Group SFU Workspaces have no History: `server.run.workspace.history` returns an empty list, `server.run.workspace.history.play` returns the `not_found` state, and `server.workspace.history.*` plus the Admin history routes return empty results, never a not-supported error; those Workspaces also never emit `workspace_history_updated`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | The minimum service interface that the History audio handler depends on. |
| `handleWorkspaceHistoryAudioDownload` | Verify the request, obtain the history audio, and write out the metadata and binary frames. |
| `writeHistoryAudioResponse` | Shared metadata, binary-frame, and EOS writer. |
