# Quota HTTP 服务

Quota 用于在 provider 调用前取得时间授权。外部 quota 服务拥有计费规则与额度折算，GizClaw 负责报告已有用量并执行授权结果。

## 协议

[quota.json](https://github.com/GizClaw/gizclaw/blob/main/api/http/quota.json) 定义 HTTP 请求与响应，[Go quota client](https://github.com/GizClaw/gizclaw/tree/main/sdk/go/quota) 由它生成。服务接收已知设备标识与持久化用量快照；GizClaw 不为 quota 另外采集设备信息。

对接时应区分设备可使用期限与本次授权结果的缓存期限。重复报告累计快照不应重复扣量，具体字段与处理规则直接查阅 Schema。

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

授权覆盖连接 Peer、Workspace owner 和独立 OpenAI 调用。Server 刷新缓存结果并在授权失效时取消受影响的 provider 调用；刷新失败不能延长旧授权。

应用关闭时先结束 quota worker，再关闭其依赖的用量存储。刷新、取消和缓存清理实现见 [peerquota](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerquota)。

## 客户端错误

区分额度拒绝与无法取得有效授权，客户端据此提示用户或稍后重试。各 surface 的状态和消息由 adapter 映射，底层 endpoint、credential 与错误链不应返回给调用方。

流已经开始后发生撤权，调用方可能只收到部分输出；使用协议的终止结果判断调用是否完成。错误定义见 [errors.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerquota/errors.go)。

Quota 不提供逐 token 的预扣或精确计费保证，仍受 [Peer usage](../../gizclaw/services/runtime/peerusage) 的持久化边界约束。

## Docker 验证

```sh
bash tests/gizclaw-e2e/setup/run-quota.sh
```

该入口用隔离的 HTTP 与 provider fixture，经过真实 Server、Edge 和 SDK 验证授权、拒绝、过期与运行中撤权。场景与断言以 [测试入口](https://github.com/GizClaw/gizclaw/blob/main/tests/gizclaw-e2e/setup/run-quota.sh) 为准；它不代表云端 provider 验收。
