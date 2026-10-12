# pkgs/store/logstore

`pkgs/store/logstore` 提供日志服务上的结构化 record、backend-neutral 查询与 cursor 分页。LogStore 支持追加与查询，event、audit 等生产者拥有 authorization 与 canonical resource；retention 由日志服务管理。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/logstore)

## Contract

`Appender.Append` 会为每条已接受的 record 返回一个 `RecordKey`。发生部分失败时，返回值是按输入顺序排列的已接受前缀。Key 是稳定的 `Stream` 与调用方生成的 `ID` 组合。`ImmutableStore` 组合追加、查询和生命周期能力。

`Record` 必须提供 `ID`、时间、`Stream` 与 `Kind`，并可附带 severity、message、indexed scalar attributes 和不索引的 JSON payload。Attribute 使用最长 128 bytes 的 canonical dotted path；每段匹配 `[A-Za-z_][A-Za-z0-9_-]*`，scalar/object prefix conflict 会被拒绝。

`Query` 使用结构化 selector，不接受 backend expression。时间窗口为毫秒对齐的 `[Start, End)`；stream、kind 和 severity 各自是 OR set，set 之间为 AND；text 是 case-sensitive literal phrase；attribute 支持 `=`、`!=`、`exists` 和 `not-exists`。Page limit 为 1–1000。Opaque cursor 绑定 selector、text、time 和 order，但允许 continuation 改变 limit。

## Drivers

| Driver | Capability | 说明 |
| --- | --- | --- |
| Volc TLS | `ImmutableStore` | 同一 TLS SDK client 上的 PutLogsV2 与 SearchLogs；不支持 mutation |

每个逻辑 Log Store 声明 `log.immutable`，通过 `Stores.Log` 获取。当前实现为 Volc TLS；不支持 SQLite、PostgreSQL 或 ClickHouse 数据库 backend，也不注册 `log.mutable`。Workspace 与 Eino 的对话历史使用 [Memory History Store](./memory#conversation-history)，不经过 LogStore。物理连接 ownership 属于 `storage`。

### Volc TLS

```yaml
storage:
  volc-logs:
    kind: volc-tls
    endpoint: ${VOLC_TLS_ENDPOINT}
    region: ${VOLC_TLS_REGION}
    access_key_id: ${VOLC_TLS_ACCESS_KEY_ID}
    access_key_secret: ${VOLC_TLS_ACCESS_KEY_SECRET}
stores:
  logs:
    kind: log.immutable
    storage: volc-logs
    topic_id: ${VOLC_TLS_TOPIC_ID}
```

Topic、logset、retention 和 index 都由 operator 预先创建。构造 store 时只调用 `DescribeIndex`，不会调用 `CreateIndex` 或 `ModifyIndex`。必需配置为：关闭 full-text 和 auto-index，启用 phrase index；`id`、`stream`、`kind`、`level` 是 case-sensitive non-tokenized text；`msg` 是 case-sensitive、ASCII whitespace delimiter、包含中文的 text；`attributes` 是 case-sensitive、`IndexAll=true` 的 JSON；`payload` 不得建立 index。`DescribeIndex` 可能把这个逻辑 delimiter 返回成字面转义文本 ` \t\r\n`；validator 仅把这个精确的 provider 表示视为等价形式，不接受其他 delimiter 写法。已有 topic 后续启用 phrase index 时，历史数据是否 rebuild 由 operator 决定。

Operator-owned schema 和 search behavior 可参考 Volc TLS 的 [CreateIndex](https://www.volcengine.com/docs/6470/112187)、[query syntax](https://www.volcengine.com/docs/6470/1206705) 和 [phrase query](https://www.volcengine.com/docs/6470/1206697)。

Provider layout 固定使用 `id`、`stream`、`kind`、`level`、`msg`，把 dotted attributes 展开为 nested `attributes` JSON，并保存可选 payload。提交前，driver 会把超出单批限制的 message、severity、attribute 与 payload value 自动截断，同时保留 `Stream`、`ID`、`Kind` 与时间。JSON payload 会先压缩，再按 value 边界缩短 string。同步 `PutLogsV2` 按最多 4096 条、512 KiB 分批顺序提交；只有整批成功才返回该批 key，失败时返回此前成功批次构成的输入前缀。

Generic record 的 provider source 为 `gizclaw`、filename 为 `logstore`；process log 的 `source=gizclaw`、`path=slog` 仍是 logical attribute。Record timestamp 会保留可用的 nanoseconds，而 SearchLogs range 和 ordering 使用 milliseconds。

查询使用 SearchLogs search expression 和 provider Context，不使用 SQL analysis。`Text` 使用 key-value phrase 形式 `msg:#"..."`，已验证的 attribute name 以 `attributes.request_id` 这类 JSON dotted path 输出。Provider call 最长 30 秒，并服从更短的 caller deadline；Store 和 Admin API 不返回 provider error body。物理 Storage 只持有一个 TLS SDK client，topic-scoped Store 用同一个 client 读写，不创建 producer 或第二个 client。

当查询固定为 `Streams=[system]`、`Kinds=[log]` 时，driver 也会匹配 provider source 为 `gizclaw`、filename 为 `slog` 的旧记录。新旧记录共用 provider-side ordering 和 cursor，不会分别查询后再合并。这只是 record compatibility；已移除的 Server `log` 配置仍不兼容。

## Process logging

`services.system_log` 是 Server 自身的 `slog` pipeline，不是产品 record 写入 API：

```yaml
services:
  system_log:
    level: info
    node_id: ${GIZCLAW_NODE_ID}
    query_store: logs
    sinks:
      - kind: stderr
      - kind: store
        store: logs
      - kind: store
        store: audit-logs
        level: warn
```

Sink 按顺序执行，每个 sink 可覆盖 level；fanout 会尝试所有 enabled sink 并汇总 error。非空 `node_id` 会经过环境变量展开并附加到每个 sink；Deploy 必须提供稳定逻辑节点名，GizClaw 不从网络或主机信息推断。stderr 启用 Go caller 输出，Store sink 还把调用位置保存为 `source_file` 与十进制 `source_line`。Store sink 固定写入 `Stream=system`、`Kind=log`，但不拥有 named store 的生命周期。`query_store` 必须指向同一配置中的一个 Store sink；未设置时 Admin log endpoint 返回 `LOG_QUERY_NOT_CONFIGURED`。缺少 `services.system_log` 时默认是 info-level stderr。旧的 top-level `log` 与 `system_log` 配置会直接报错，不自动转换。
