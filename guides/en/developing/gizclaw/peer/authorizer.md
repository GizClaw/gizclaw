# Authorization

`Implementation file: peer_authorizer.go`

`peer_authorizer.go` Connect the current Peer identity to the GizClaw ACL system.

| Documentation | Features included |
| --- | --- |
| `peer_authorizer.go` | Perform authorization based on the current Peer public key, Peer config and ACL service; parse the ACL view applicable to the Peer; list the policy bindings corresponding to the view. |

Here is the adaptation layer between connection identity and ACL realm. The resource semantics of role, view, policy and binding still belong to `services/system/acl`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerAuthorizer` | Bind the current Peer public key, ACL service and Peer config service. |
| `Authorize` | Perform ACL authorization using the current Peer identity. |
| `ListPolicyBindings` | Returns the policy bindings corresponding to the current Peer view. |
| `peerView` | Parse the ACL view required for authorization from the Peer config. |
