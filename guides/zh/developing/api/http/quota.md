# Quota HTTP 服务

RuntimeProfile 可以为 Peer provider 调用选择 quota policy。当前支持 unlimited 和 custom；只有 custom 使用外部 HTTP 服务决定可用有效期，计费和额度折算由该服务拥有。

## 协议

Source contract 是 `api/http/quota.json`，Go client 由它生成至 `sdk/go/quota`。

`POST /v1/quota` 使用 `Authorization: Bearer <api_key>`。请求包含 `peer_public_key`、可选的 `identifiers` 和 `usage`。`identifiers` 引用完整的已有 `DeviceIdentifiers`，包括 SN、IMEI 列表和 labels；GizClaw 发送已知字段，不为 quota 额外采集设备信息。

`usage` 包含保留期内已落盘的小时累计快照，每条为 `model_id`、UTC 整点 `hour`、非负 `quantity`。model ID 使用 provider 计费模型/version/resource ID。接收方按 Peer/model/hour 保留最大的累计数量，重复报告不会重复扣量。首次没有用量时发送空数组。

```json
{
  "expires_at": "2030-01-01T00:00:00Z",
  "valid_until": "2029-12-01T00:00:00Z"
}
```

- `expires_at` 是可用有效期：省略或 null 表示不限；小于等于当前时间表示不允许使用。
- `valid_until` 是本次查询结果的有效期：必填、不可为 null，且必须晚于接收结果的当前时间。缺失、格式错误或已经失效的响应不能授权。

## RuntimeProfile

```yaml
spec:
  quota:
    type: custom
    endpoint: https://quota.example.com/v1/quota
    api_key: ${QUOTA_API_KEY}
```

`spec.quota` 可省略或设为 null，默认使用 unlimited；显式 `quota: {type: unlimited}` 具有相同调用行为。它们不发起 quota HTTP 请求，不要求 quota 服务或 SQL 用量存储。独立配置的 `services.peer_usage.store` 仍然记录 provider 实际报告的用量。空对象或未知 policy type 会被拒绝。

`type: custom` 必须提供 endpoint 和 api_key。endpoint 是完整的 HTTP(S) 地址，不接受 userinfo、query 或 fragment；API key 必须非空且不能包含换行。custom 还需要配置 `services.peer_usage.store` 的 SQL 用量存储。上报先 flush，再查询保留的小时记录；存储未配置或读取失败时拒绝 custom 调用，不提交虚假的空用量。SQL 将未配置 policy 保存为 JSON null，显式 policy 保存为带 type 的对象；既有未配置 Profile 继续使用 unlimited。

## 调用和生命周期

配置 custom 时，真实 Generator、Transformer 和 speech provider 调用先获取 quota 授权。connected Peer、Workspace owner 和独立 OpenAI HTTP 都使用相应 Peer 的配置。

custom policy 在 HTTP 查询结果有效期的中点刷新，保证在 `valid_until` 之前尝试重新查询；HTTP 响应省略/null 可用期限和拒绝的结果同样刷新。HTTP 尝试最长五秒，失败后按一秒间隔重试。失败不延长旧结果的任何时间；旧结果仍然有效时可以继续使用。

可用有效期到期、新结果拒绝使用，或查询结果过期而未取得替代结果时，会取消正在执行的 provider 调用。成功刷新可以延长活动调用的授权。空闲状态五分钟后清理。Server 关闭先取消并 join quota worker，再关闭用量存储。

独立 speech synthesis 在 metadata 已发送后撤权时，以提前 EOS 停止音频；已输出的音频可能只是部分内容，后续调用返回 permission denied。metadata 之前的拒绝直接返回 permission denied。

Quota 是时间授权，不保证逐 token 的预扣或数值硬上限。GenX provider 未报告的用量，以及进程异常退出时尚未落盘的用量，仍受 [Peer usage](/zh/developing/gizclaw/services/runtime/peerusage) 的持久化边界约束。

## Docker 验证

`bash tests/gizclaw-e2e/setup/run-quota.sh` 构建真实 Linux GizClaw 与 HTTP fixture，在隔离 Docker stack 中通过 giztest 的真实 RPC/HTTP transport 验证协议。fixture 同时提供确定性的 OpenAI 和 MiniMax provider 数据，捕获 identifiers 和用量报告。十二个场景包含原有十个 custom policy 的期限/上报场景，以及省略 policy 和显式 unlimited；两个 unlimited 场景都调用真实 provider adapter，并断言该 Peer 的 quota 请求为零。该验证不代表云端 provider 资格验证。
