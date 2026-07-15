# Admin HTTP · Social

`实现文件：peer_service_serve_admin_social.go`

实现 contact、friend、Peer friend、friend group、member 和 invite token 的 Admin endpoints；同时通过 Peer RPC 读取 workspace history 列表、详情和音频。

Social graph 属于 `services/social`；workspace history 属于 AI/runtime workspace service。

## 核心结构与主函数

| 函数组 | 作用 |
| --- | --- |
| `ListContacts` / `CreateContact` / `GetContact` / `PutContact` / `DeleteContact` | Contact 管理。 |
| `ListFriends` / `CreateFriend` / `GetFriend` / `DeleteFriend` | Friend request/relationship 管理。 |
| `ListPeerFriends` / `CreatePeerFriend` / `GetPeerFriend` / `DeletePeerFriend` | 指定 Peer 的 friend 管理。 |
| `ListFriendGroups` / `CreateFriendGroup` / `GetFriendGroup` / `PutFriendGroup` / `DeleteFriendGroup` | Friend Group 管理。 |
| `ListFriendGroupMembers` / `CreateFriendGroupMember` / `PutFriendGroupMember` / `DeleteFriendGroupMember` | Group member 管理。 |
| `GetFriendGroupInviteToken` / `PutFriendGroupInviteToken` / `DeleteFriendGroupInviteToken` | Invite token 管理。 |
| `ListWorkspaceHistory` / `GetWorkspaceHistory` / `DownloadWorkspaceHistoryAudio` | 通过 Peer RPC 读取 workspace history。 |
