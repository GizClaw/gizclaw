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

## 配置与组合

官方 Server YAML 使用 `peer-admission` 选择握手准入，与 `admin-public-key` 的 Admin
service 授权独立：

```yaml
peer-admission: registration-token
```

省略或设置 `open` 时保持开放准入；其他值启动前报错。`ServerSecurityPolicy.AllowPeer`
只检查 Server 是否存在并回落转发宿主 policy，未注入时放行；判定逻辑由
`RegistrationTokenSecurityPolicy` 拥有。官方宿主即使未配置 admin key 也会安装所选
准入 policy。Admin key 不绕过准入，其 Admin service 权限仍独立保留；已有 role
授权继续由 Manager 判断。

启用后，已有可用且非 blocked 的 Peer 无凭证仍可重连，包括已有 `auto_registered`
记录。未知旧设备需先登记 Peer 或升级 SDK，在握手中携带有效凭证。全新 bootstrap
admin 也需要先登记或携带有效 token；仅配置 `admin-public-key` 不代表已有 Peer。
`edge-nodes` 在 Server 启动时已登记，因此 Edge 上游仍可无凭证连接。

该开关只约束本 Server 的 WebRTC signaling；Edge 自己终止客户端握手，随后创建的
logical tunnel 不再进入这里，不受这个 Server 开关保护。部署边界见 [Gizedge](../../gizedge)。

## 内置 registration-token policy

- 空凭证要求已存在 Peer 且 `Status != blocked`，并通过 `EnsureAvailable` 排除 pending
  deletion 和永久 tombstone。存储错误拒绝。
- 非空凭证按 RegistrationToken 交给 `ResolveRegistration`；解析成功才放行。已知
  blocked、pending deletion 或 tombstone 不会因为携带 token 而绕过检查。已知公钥
  携带无效凭证同样拒绝，不回退空凭证路径。

谓词不创建 Peer，不绑定 RuntimeProfile owner 或 firmware，也不激活 runtime。
`server.register` 仍负责再次解析可重复使用的 token 并完成绑定。握手通过不会赋予 AI
资源权限；未注册连接仍没有 RuntimeProfile。返回错误和日志不包含 token。

非空凭证查询前使用每个 Server policy 实例共享的失败预算：每 60 秒窗口最多 64 次
失败，最多 8 个同时在途查询，在途请求预占失败名额。这个限制跨公钥生效，随机换 key
或 token 不能绕过。相同 token 的同时查询直接拒绝；失败 token 按与解析器一致的
trim 后 SHA-256 摘要做 30 秒负缓存，上限 1024 项，不存原始 token，不缓存成功。
缓存满时有界淘汰，失败预算不受淘汰影响；过期项在下一次查询时清理。

查询继承请求取消/deadline，并额外限制为 2 秒；锁内只有内存记账。所有判定都是只读
存储查询，缓存和计数只存在内存中。预算或并发额度耗尽时，合法新 token 也会被暂时
拒绝；调用方应退避重试，窗口到期后恢复。空凭证的已知 Peer 重连不占 token 查询预算。
多进程部署的预算各自独立。默认 `open` 不执行这些查询与限制。
