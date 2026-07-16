# Admin HTTP · ACL

`Implementation file: peer_service_serve_admin_acl.go`

Implement Admin CRUD/list endpoints for ACL role, view and policy binding, as well as pagination parameter conversion and ACL error mapping.

ACL resource semantics and authorization judgment belong to `services/system/acl`.

## Core structure and main function

| Function group | Function |
| --- | --- |
| `ListACLRoles` / `CreateACLRole` / `GetACLRole` / `PutACLRole` / `DeleteACLRole` | ACL Role management. |
| `ListACLViews` / `CreateACLView` / `GetACLView` / `PutACLView` / `DeleteACLView` | ACL View management. |
| `ListACLPolicyBindings` / `CreateACLPolicyBinding` / `GetACLPolicyBinding` / `PutACLPolicyBinding` / `DeleteACLPolicyBinding` | Policy Binding management. |
| `aclServer` | Get the configured ACL service. |
| `aclListParams` | Normalize cursor and limit. |
| `isBadACLRequest` | Determine whether the ACL error should be mapped to a bad request. |
