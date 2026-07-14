# Admin HTTP · ACL

`实现文件：peer_service_serve_admin_acl.go`

实现 ACL role、view 和 policy binding 的 Admin CRUD/list endpoints，以及分页参数转换和 ACL 错误映射。

ACL 资源语义和授权判断属于 `services/system/acl`。

## 核心结构与主函数

| 函数组 | 作用 |
| --- | --- |
| `ListACLRoles` / `CreateACLRole` / `GetACLRole` / `PutACLRole` / `DeleteACLRole` | ACL Role 管理。 |
| `ListACLViews` / `CreateACLView` / `GetACLView` / `PutACLView` / `DeleteACLView` | ACL View 管理。 |
| `ListACLPolicyBindings` / `CreateACLPolicyBinding` / `GetACLPolicyBinding` / `PutACLPolicyBinding` / `DeleteACLPolicyBinding` | Policy Binding 管理。 |
| `aclServer` | 取得已配置的 ACL service。 |
| `aclListParams` | 规范化 cursor 与 limit。 |
| `isBadACLRequest` | 判断 ACL error 是否应映射为 bad request。 |
