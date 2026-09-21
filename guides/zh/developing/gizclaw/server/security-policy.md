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

## 所有准入模式下的 blocked 强制执行

`open` 只省略握手凭证预检，不解除 Peer 封禁。Peer activation 对已有 `blocked`
记录返回 `ErrPeerBlocked`，关闭连接，不创建或替换 Peer 记录，也不写入新的
PeerRoutes assignment 或 LocalRuns。该规则同样适用于 registration-token 模式以及
Edge 转发到本 Server 的 logical Peer；Edge 自身的握手仍不受 Server 的准入开关约束。

Admin block 会持久化状态，在同一 public key 的记录协调内摘除本 Server 当前持有的
connection、Edge transport 和正在激活的 reservation，再在锁外将旧 generation
标记为 retiring 并关闭 transport。已有 stream 随连接关闭；新的 connection 即使
已经打开 Event transport，也必须通过 activation 才能开始服务。迟到的 activation
不能发布已摘除的 reservation。Admin approve 恢复 active 后可重新连接。

普通 `ServicePeerRPC`、`ServicePeerHTTP`、`ServicePeerOpenAI` 和
`EventStreamAgent` 的 service 授权不读取存储，不增加 deadline。Event transport
先于 activation 建立，所以 service label 被允许不代表 Peer 已激活或可以执行业务。
blocked 强制由 activation 与 block 时的连接撤销共同完成，旧连接的 retiring 标记
仅使用内存。共享存储变慢不会通过每次普通 service 打开传播成 transport 拒绝。

Admin/Edge role service 保留原有的 `allowActivePeerRole` 查询及宿主 policy 回退。
角色查询沿用调用方 context，DataChannel 回调沿用 `context.Background()`，不添加
固定超时；Manager 角色授权仍要求 active 状态及匹配 role。宿主 Admin grant 不会
让连接绕过 activation 的 blocked 检查。

## 内置 registration-token policy

- 空凭证要求已存在 Peer 且 `Status != blocked`，并通过 `EnsureAvailable` 排除 pending
  deletion 和永久 tombstone。存储错误拒绝。
- 非空结构化凭证必须满足 `version == 1 && type == "gizclaw.com/registration_token"`；未知 version
  或 type 直接拒绝，不查询 Peer 或 token 存储，也不占查询预算。只有 `value` 去除首尾空白后
  作为 RegistrationToken 交给 `PreflightRegistration`，要求 token 启用、未过期且激活数低于上限，
  Profile/Firmware 解析成功才放行。已知
  blocked、pending deletion 或 tombstone 不会因为携带 token 而绕过检查。已知公钥
  携带无效凭证同样拒绝，不回退空凭证路径。

谓词不创建 Peer，不绑定 RuntimeProfile owner 或 firmware，也不激活 runtime。
`server.register` 在同一 SQL 事务中再次判定限制、记录激活，并绑定 owner 与 firmware。
两个新设备争最后一个名额时只有一个注册成功，另一方得到 `PermissionDenied`，不留下半绑定。握手通过不会赋予 AI
资源权限；未注册连接仍没有 RuntimeProfile。返回错误和日志不包含 token。

非空凭证查询前使用每个 Server policy 实例共享的失败预算：每 60 秒窗口最多 64 次
失败，最多 8 个同时在途查询，在途请求预占失败名额。这个限制跨公钥生效，随机换 key
或 token 不能绕过。相同 token 的同时查询直接拒绝；失败 token 按与解析器一致的
`type + "\0" + strings.TrimSpace(value)` 的 SHA-256 摘要做 1 秒负缓存，上限 1024 项，不存原始 token，不缓存成功。
缓存满时有界淘汰，失败预算不受淘汰影响；过期项在下一次查询时清理。

查询继承请求取消/deadline，并额外限制为 2 秒；锁内只有内存记账。所有判定都是只读
存储查询，缓存和计数只存在内存中。预算或并发额度耗尽时，合法新 token 也会被暂时
拒绝；调用方应退避重试，窗口到期后恢复。空凭证的已知 Peer 重连不占 token 查询预算。
多进程部署的预算各自独立。默认 `open` 不执行这些握手 token 查询与限制，但仍执行上面的 activation 检查与 block 时的连接撤销。

管理员重新启用、延期或调高上限后，先前失败最多在负缓存中保留 1 秒；全局失败预算仍按原有 60 秒窗口恢复。已知 Peer 不带凭证重连不受 token 禁用、过期或降低限额影响。这些修改只阻止新的激活，不吊销已有设备；已有设备主动携带受限 token 时仍会被握手预检拒绝。激活定义与 admin 编辑见 [RuntimeProfile 与设备注册](../services/runtime-profile#registrationtoken)。

内置 credential type 使用 `gizclaw.com/` 域名前缀；自定义 policy 应使用自己的域名前缀，避免类型冲突。裸 `registration_token` 不属于内置类型，会被拒绝。`value` 最多 512 个 UTF-8 字节，客户端构造 helper 与编码器都会检查，服务端解码后也检查；GZOF 的 4096 字节总编码上限保持独立。
