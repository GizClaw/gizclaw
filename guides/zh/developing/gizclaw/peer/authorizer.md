# Authorization

`实现文件：peer_authorizer.go`

`peer_authorizer.go` 将当前 Peer identity 接到 GizClaw ACL 系统。

| 文件 | 包含的功能 |
| --- | --- |
| `peer_authorizer.go` | 根据当前 Peer public key、Peer config 和 ACL service 执行授权；解析适用于该 Peer 的 ACL view；列出 view 对应的 policy bindings。 |

这里是 connection identity 与 ACL 领域之间的适配层。Role、view、policy 和 binding 的资源语义仍属于 `services/system/acl`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerAuthorizer` | 绑定当前 Peer public key、ACL service 与 Peer config service。 |
| `Authorize` | 使用当前 Peer identity 执行 ACL 授权。 |
| `ListPolicyBindings` | 返回当前 Peer view 对应的 policy bindings。 |
| `peerView` | 从 Peer config 解析授权所需的 ACL view。 |
