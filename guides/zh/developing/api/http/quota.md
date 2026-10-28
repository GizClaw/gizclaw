# Quota HTTP 服务

Quota 使用后台查询得到的时间授权约束 provider 调用。外部 quota 服务拥有计费规则与额度折算，GizClaw 负责报告已有用量并执行已知的使用期限。

## 协议

[quota.json](https://github.com/GizClaw/gizclaw/blob/main/api/http/quota.json) 定义 HTTP 请求与响应，[Go quota client](https://github.com/GizClaw/gizclaw/tree/main/sdk/go/quota) 由它生成。服务接收已知设备标识与持久化用量快照；GizClaw 不为 quota 另外采集设备信息。

对接时应区分设备可使用期限与本次查询结果的新鲜度期限。重复报告累计快照不应重复扣量，具体字段与处理规则直接查阅 Schema。

## RuntimeProfile

外部授权的最小配置示例：

```yaml
spec:
  quota:
    type: custom
    endpoint: https://quota.example.com/v1/quota
    api_key: ${QUOTA_API_KEY}
```

custom policy 需要配置 Peer usage SQL 存储，以便报告真实已保存的用量。无需外部授权时使用 unlimited policy；它不调用外部 quota 服务。配置解析见 [RuntimeProfile quota](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/system/runtimeprofile/quota.go)。

## 调用和生命周期

授权覆盖连接 Peer、Workspace owner 和独立 OpenAI 调用。Provider 调用只使用本地最近一次结果，不等待 hook 的网络请求或用量上报；Server 在后台查询和刷新。首次没有结果时放行。缓存仅保留在进程内，重启或空闲状态回收后重新以未知结果开始。

`valid_until` 只用于安排后台刷新；结果变旧、hook 超时、HTTP 错误、响应无效或用量上报失败都不会撤销授权，也不会中断正在进行的调用。失败保留最近一次结果，后台继续重试，并输出按 Peer/policy 限频的 WARN；日志只包含 Peer 身份和安全错误原因，不包含 endpoint、credential、响应正文或底层 provider 错误。

只有明确的 `expires_at` 使用期限可以拒绝新调用并取消受影响的 provider 调用，包括先前返回的未来期限随后到期。已知到期不会因结果变旧或刷新失败恢复使用；后续成功返回延期或无限使用结果后才恢复。没有明确使用期限时，即使一直无法刷新也继续放行。

应用关闭时先结束 quota worker，再关闭其依赖的用量存储。刷新、取消和缓存清理实现见 [peerquota](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerquota)。

所有 Peer 与 policy 的 hook 请求复用同一个 service-owned HTTP client 与 keep-alive 连接池；每个 worker 保留自己的 SDK client，认证头仍按对应 policy 设置。成功、HTTP 错误和 JSON 解析失败均读取并关闭响应正文，让可复用的连接归还池；响应大小和请求期限继续受限，不跟随重定向。Service 关闭时先取消并等待 worker，再释放空闲连接。

## 客户端错误

明确到期继续返回额度耗尽错误；hook 故障只输出 WARN，不向客户端返回额度不可用错误。配置错误或 service lifecycle 错误仍由 adapter 映射到对应状态和消息，底层 endpoint、credential 与错误链不应返回给调用方。

流已经开始后发生撤权，调用方可能只收到部分输出；使用协议的终止结果判断调用是否完成。错误定义见 [errors.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerquota/errors.go)。

Quota 不提供逐 token 的预扣或精确计费保证，仍受 [Peer usage](../../gizclaw/services/runtime/peerusage) 的持久化边界约束。

## Docker 验证

```sh
bash tests/gizclaw-e2e/setup/run-quota.sh
```

该入口用隔离的 HTTP 与 provider fixture，经过真实 Server、Edge 和 SDK 验证授权、拒绝、过期与运行中撤权。Giztest 用可控的阻塞 hook 证明首次和后续调用不等待授权响应，观察真实 HTTP 超时后继续调用，并验证旧结果在刷新失败时继续生效，包括保留明确到期的拒绝。Docker runner 将故障记录与 Server 的 Peer WARN 日志关联，检查错误分类与限频，并用 fixture 实际观察的连接地址确认不同 Peer 复用 HTTP 连接。场景与断言以 [测试入口](https://github.com/GizClaw/gizclaw/blob/main/tests/gizclaw-e2e/setup/run-quota.sh) 为准；它不代表云端 provider 验收。
