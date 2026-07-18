# Security Policy

`实现文件：server_security_policy.go`

实现 Giznet Server 的 transport security policy：判断 public key 是否允许建立 Peer connection，以及该 Peer 是否允许打开指定 Giznet service。

它负责 connection/service 准入；产品资源访问由 RuntimeProfile、owner 和领域关系决定。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`ServerSecurityPolicy`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy) | 将完整 Server 配置适配为 Giznet security policy。 |
| [`ServerSecurityPolicy.AllowPeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy.AllowPeer) | 判断 public key 是否允许建立 connection。 |
| [`ServerSecurityPolicy.AllowService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy.AllowService) | 根据 Peer identity 与 service ID 判断 service 准入。 |
