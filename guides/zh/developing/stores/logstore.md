# pkgs/store/logstore

`pkgs/store/logstore` 提供跨业务领域复用的结构化 record、backend-neutral 查询与 cursor 分页。Immutable driver 支持追加与查询；mutable driver 还支持替换或删除单条 record。Conversation、event、audit 等生产者仍拥有自己的 authorization、retention 和 canonical resource。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/logstore)

## Contract

`Appender.Append` 会为每条已接受的 record 返回一个 `RecordKey`。发生部分失败时，返回值是按输入顺序排列的已接受前缀。Key 是稳定的 `Stream` 与调用方生成的 `ID` 组合。`ImmutableStore` 组合追加、查询和生命周期能力；`MutableStore` 在此基础上增加 `Replace` 与 `Delete`，需要修改能力的调用方必须显式解析这个 capability。

`Replace` 修改一个已存在 key 对应的 record，并保持该 key 不变；它不是 upsert，key 不存在时返回 `ErrNotFound`。`Delete` 只删除一个已存在 key，key 不存在时也返回 `ErrNotFound`。

`Record` 必须提供 `ID`、时间、`Stream` 与 `Kind`，并可附带 severity、message、indexed scalar attributes 和不索引的 JSON payload。Attribute 使用最长 128 bytes 的 canonical dotted path；每段匹配 `[A-Za-z_][A-Za-z0-9_-]*`，scalar/object prefix conflict 会被拒绝。

`Query` 使用结构化 selector，不接受 backend expression。时间窗口为毫秒对齐的 `[Start, End)`；stream、kind 和 severity 各自是 OR set，set 之间为 AND；text 是 case-sensitive literal phrase；attribute 支持 `=`、`!=`、`exists` 和 `not-exists`。Page limit 为 1–1000。Opaque cursor 绑定 selector、text、time 和 order，但允许 continuation 改变 limit。

## Drivers

| Driver | Capability | 说明 |
| --- | --- | --- |
| Volc TLS | `ImmutableStore` | 同一 TLS SDK client 上的 PutLogsV2 与 SearchLogs；不支持 mutation |
| ClickHouse | `MutableStore` | 独立 MergeTree 表与同步 replace/delete mutation |
| SQLite / PostgreSQL | `MutableStore` | 独立关系表、原子 append/replace/delete 与共享 SQL cursor |

每个逻辑 Log Store 都必须声明 `log.immutable` 或 `log.mutable`。`Stores.Log` 接受两种声明，`Stores.MutableLog` 只接受 `log.mutable`。Volc TLS 不能满足可变 Flowcraft History。物理连接 ownership 始终属于 `storage`。

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

### ClickHouse

```yaml
storage:
  analytics:
    kind: clickhouse
    dsn: ${CLICKHOUSE_DSN}
stores:
  flowcraft-history:
    kind: log.mutable
    storage: analytics
    database: gizclaw
    table: flowcraft_history
```

Driver 会创建并校验独立 `MergeTree` 表，按月分区，并按 `(timestamp, stream, id)` 排序。`Append` 会在同一 store instance 内串行执行查重与同步 batch insert，只在 commit 后返回 key；`Query` 把结构化 contract 直接转换为参数化 ClickHouse SQL，通过 `(timestamp, stream, id)` 分页，不建立额外分页索引。`Replace` 使用同步 `ALTER UPDATE`，`Delete` 使用同步 `ALTER DELETE`；二者都只针对一个 `(stream, id)`。发现重复 key 时会报错，不会静默修改多行。

物理 DSN 已选择 database 时可以省略逻辑 `database` 字段。ClickHouse driver 不额外施加本地 payload 大小限制；service limit、retention 和 table policy 仍由 operator 负责。逻辑 Metrics 与 Log Store 可以共享一个物理 pool，但不拥有它。

### SQLite / PostgreSQL

每个逻辑 Store 必须声明独立 `table`。关系表用 `(stream, id)` 唯一标识 record，以 UTC nanoseconds 保存 time，并按 `(time, stream, id)` 稳定分页；flat attributes 使用 canonical JSON，payload 保留原始 JSON bytes。Append 在一个 transaction 中查重并写入完整 batch；Replace 不是 upsert，不能改变 time；Delete 对缺失 key 返回 `ErrNotFound`。Immutable 与 mutable 声明使用同一实现，但 Registry 只对 `log.mutable` 暴露 mutation capability。

ClickHouse、SQLite 和 PostgreSQL 共用 version-1 opaque cursor：它绑定 normalized selector、text、millisecond-aligned `[Start, End)` 和 order，允许 continuation 修改 limit，并保持 16 KiB bound。相同 records 可以跨三种 SQL driver continuation。SQLite/PostgreSQL 的 text matching 保持 case-sensitive literal，attribute matcher 在 validated flat map 上执行；逻辑 Store 不关闭共享 pool。

## Process logging

`services.system_log` 是 Server 自身的 `slog` pipeline，不是产品 record 写入 API：

```yaml
services:
  system_log:
    level: info
    query_store: logs
    sinks:
      - kind: stderr
      - kind: store
        store: logs
      - kind: store
        store: audit-logs
        level: warn
```

Sink 按顺序执行，每个 sink 可覆盖 level；fanout 会尝试所有 enabled sink 并汇总 error。Store sink 固定写入 `Stream=system`、`Kind=log`，但不拥有 named store 的生命周期。`query_store` 必须指向同一配置中的一个 Store sink；未设置时 Admin log endpoint 返回 `LOG_QUERY_NOT_CONFIGURED`。缺少 `services.system_log` 时默认是 info-level stderr。旧的 top-level `log` 与 `system_log` 配置会直接报错，不自动转换。
