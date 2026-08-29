# Workspace History

`Implementation file: rpc_workspace_history.go`

Process Workspace history audio download RPC: Read the audio metadata and content of the specified history entry, and return binary frames through the RPC stream.

History data and audio storage are owned by the workspace/runtime service.

Peer RPC exposes each History record's stable identity as `name`; play and audio requests pass that same value as `history_name`. The value is the canonical internal History ID projected verbatim because History has no separate Peer alias. `actor_name` is attribution text and is never an identity or selector. Admin History routes continue to expose and select the same underlying record through canonical `id`.

Friend Group message reads reuse this ownership without exposing a Workspace name. The Social adapter resolves the group's stored Workspace binding after current-membership authorization, obtains one internal History page for list projection, and exposes each message with `name` as identity and `actor_name` as attribution. Get and audio requests use `friend_group_name + history_name` and share the same audio reader/framing path. The response is metadata, binary frames, then EOS; no Friend Group message store or nested RPC is involved.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | The minimum service interface that the History audio handler depends on. |
| `handleWorkspaceHistoryAudioDownload` | Verify the request, obtain the history audio, and write out the metadata and binary frames. |
| `handleFriendGroupMessageAudioDownload` | Authorize by Friend Group and stream the bound History audio. |
| `writeHistoryAudioResponse` | Shared metadata, binary-frame, and EOS writer. |
