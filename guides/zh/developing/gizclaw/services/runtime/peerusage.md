# Peer 模型用量

`pkgs/gizclaw/services/runtime/peerusage` 保存 Peer 的小时用量，并负责用量的异步写入与保留。

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerusage)

## 配置与调用边界

```yaml
stores:
  peer-usage:
    kind: sql
    storage: database
services:
  peer_usage:
    store: peer-usage
```

`database` 必须是 SQLite 或 PostgreSQL connector。省略 `services.peer_usage` 时不启用持久化计量；配置后，schema 校验在 listener 启动前完成。逻辑 Store 借用 pool，不关闭物理连接。

Peer connection、独立 OpenAI HTTP 调用和 Workspace owner 的 `peergenx.Service` 都为实际 provider 调用安装 recorder。记录归属调用 Peer 或 Workspace owner public key。`model_id` 使用 provider 实际计费模型/version/resource ID；TTS 使用计费模型或 resource ID，不使用 Voice ID。GenX 的 `Input`、`CachedInput`、`Output` 是不重叠数量，三者相加成为 `quantity`；不保存 modality、定价、转换单位或详细调用日志。

## 存储与查询

用量按 Peer、provider 模型和时间窗口聚合，查询读取聚合结果，不另建逐请求日志或第二套总量。重试幂等、SQL schema、分区和保留策略见 [store.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerusage/store.go)。

## 持久化与关闭

Provider callback 将数量交给内存 recorder，后台负责写入；应用关闭时应先完成 recorder 的清理，再关闭借用的 SQL pool。实现见 [recorder.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerusage/recorder.go)。

只有成功写入的记录是持久化数据。异常退出或数据库故障可能丢失尚未确认的数量；这套 recorder 用于运行统计，不代替计费系统。
