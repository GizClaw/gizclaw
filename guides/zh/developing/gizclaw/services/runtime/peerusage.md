# Peer 模型用量

`pkgs/gizclaw/services/runtime/peerusage` 保存 Peer 的小时用量，并拥有非阻塞 recorder、后台写入与 90 天保留策略。

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

## 表结构

固定父表 `peer_model_usage_hourly`：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `peer_public_key` | TEXT | Peer 身份；删除 Peer 不级联删除用量 |
| `model_id` | TEXT | provider 计费模型/version/resource ID |
| `hour_unix_nano` | BIGINT | UTC 整点小时的 Unix nanoseconds |
| `writer_id` | TEXT | 进程/桶写入 epoch，用于重试幂等 |
| `quantity` | BIGINT | 此 writer 对此小时的累计非负数量 |

主键是 `(peer_public_key, model_id, hour_unix_nano, writer_id)`。同一 writer 的累计快照只会把 `quantity` 提升为更大值；数据库已经 commit 而响应丢失时，重复写相同快照不会再次累加。多个 writer 独立写入，查询通过 `SUM(quantity)` 和 `GROUP BY model_id, hour_unix_nano` 返回 Peer/model/hour 汇总。这不是独立的总量表或逐请求日志。

## 小时、日分区与保留

小时是统计粒度，UTC 日是 PostgreSQL 分区粒度。PG 父表按 `hour_unix_nano` range partition，子表命名为 `peer_model_usage_hourly_pYYYYMMDD`；只读写父表，PG 自动路由子表。初始化和批次写入准备所需日及下一日分区，删除完全早于保留边界的日分区。共享 `storage.SQLDailyPartitions` 也被 LogStore 使用；History 的到期时间分区和 `_keys` 清理语义保持不变。

保留边界是当前 UTC 小时减 90 天。查询立即隐藏更早小时；PG 物理删除按整日完成，SQLite 删除过期 row。后台每小时执行一次空闲维护，因此没有新用量时也会回收旧数据。不存在单独总量表、分区登记表或每 Peer 一张表。

## 并发、持久化与关闭

Provider callback 不执行数据库或网络 I/O，只归并内存中的小时计数。后台每秒提交累计快照；写失败保留待确认快照，下一次重试。SQL pool 关闭前，Server 取消并 join worker，然后在 30 秒 context 内 flush；未成功持久化会返回错误。

只有成功 flush 的记录是持久化数据。非正常进程退出可能丢失一秒写入窗口或数据库故障期间尚未确认的内存数量；这套 recorder 不保证 crash-proof billing。没有外部授权、quota 兑换或 expiry 执行逻辑。
