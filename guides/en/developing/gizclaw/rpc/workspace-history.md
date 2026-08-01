# Workspace History

`Implementation file: rpc_workspace_history.go`

Process Workspace history audio download RPC: Read the audio metadata and content of the specified history entry, and return binary frames through the RPC stream.

History data and audio storage are owned by the workspace/runtime service.

Friend Group message reads reuse this ownership without exposing a Workspace name. The Social adapter resolves the group's stored Workspace binding after current-membership authorization, obtains one internal History page for list projection, and shares the same audio reader/framing path for `server.friend_group.messages.audio.get`. The response is metadata, binary frames, then EOS; no Friend Group message store or nested RPC is involved.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcWorkspaceHistoryAudioService` | The minimum service interface that the History audio handler depends on. |
| `handleWorkspaceHistoryAudioGet` | Verify the request, obtain the history audio, and write out the metadata and binary frames. |
| `handleFriendGroupMessageAudioGet` | Authorize by Friend Group and stream the bound History audio. |
| `writeHistoryAudioResponse` | Shared metadata, binary-frame, and EOS writer. |
