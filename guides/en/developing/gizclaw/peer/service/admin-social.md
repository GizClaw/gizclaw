# Admin HTTP · Social

`Implementation file: peer_service_serve_admin_social.go`

Implement the Admin endpoints of contact, friend, Peer friend, friend group, member and invite token; at the same time, read the workspace history list, details and audio through Peer RPC.

Social graph belongs to `services/social`; workspace history belongs to AI/runtime workspace service.

## Core structure and main function

| Function group | Function |
| --- | --- |
| `ListContacts` / `CreateContact` / `GetContact` / `PutContact` / `DeleteContact` | Contact Management. |
| `ListFriends` / `CreateFriend` / `GetFriend` / `DeleteFriend` | Friend request/relationship management. |
| `ListPeerFriends` / `CreatePeerFriend` / `GetPeerFriend` / `DeletePeerFriend` | Specify Peer's friend management. |
| `ListFriendGroups` / `CreateFriendGroup` / `GetFriendGroup` / `PutFriendGroup` / `DeleteFriendGroup` | Management by Friend Group. |
| `ListFriendGroupMembers` / `CreateFriendGroupMember` / `PutFriendGroupMember` / `DeleteFriendGroupMember` | Group member management. |
| `GetFriendGroupInviteToken` / `PutFriendGroupInviteToken` / `DeleteFriendGroupInviteToken` | Invite token management. |
| `ListWorkspaceHistory` / `GetWorkspaceHistory` / `DownloadWorkspaceHistoryAudio` | Read workspace history through Peer RPC. |
