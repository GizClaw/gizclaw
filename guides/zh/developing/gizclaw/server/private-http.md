# Private HTTP

`实现文件：server_private_http.go`

验证 private HTTP ingress 的 session headers，根据调用方 public key 执行 ingress authorization，并为 public login 构造 session authorizer。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `ErrPrivateHTTPIngressDenied` | Private ingress authorization 拒绝错误。 |
| `Server.AuthenticateHTTPSessionHeaders` | 从 Authorization 与 public-key headers 验证 session identity。 |
| `Server.AuthorizePrivateHTTPIngress` | 判断指定 Peer 是否允许访问 private HTTP ingress。 |
| `PrivateHTTPIngressLoginAuthorizer` | 将 Server ingress authorization 适配为 public-login authorizer。 |
