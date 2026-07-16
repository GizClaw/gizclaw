# pkgs/store/logstore

`pkgs/store/logstore` 提供跨业务领域复用的 append-only record、结构化查询和分页能力。它不是可编辑 message/resource database；conversation、event、audit 等生产者仍拥有自己的 authorization、retention 和 canonical resource。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/logstore)

## Contract

`Appender`、`Querier` 和 `Store` 分别表达写入、查询和完整生命周期。`Record` 必须提供调用方生成的 `ID`、时间、`Stream` 与 `Kind`，并可附带 severity、message、indexed scalar attributes 和不索引的 JSON payload。Attribute 使用最长 128 bytes 的 canonical dotted path；每段匹配 `[A-Za-z_][A-Za-z0-9_-]*`，scalar/object prefix conflict 会被拒绝。

`Query` 使用结构化 selector，不接受 backend expression。时间窗口为毫秒对齐的 `[Start, End)`；stream、kind 和 severity 各自是 OR set，set 之间为 AND；text 是 case-sensitive phrase；attribute 支持 `=`、`!=`、`exists` 和 `not-exists`。Page limit 为 1–1000。Opaque cursor 绑定 selector、text、time 和 order，但允许 continuation 改变 limit。

## Volc TLS

当前仅提供 Volc TLS driver：

```yaml
stores:
  logs:
    kind: log
    volc:
      endpoint: ${VOLC_TLS_ENDPOINT}
      region: ${VOLC_TLS_REGION}
      topic_id: ${VOLC_TLS_TOPIC_ID}
      access_key_id: ${VOLC_TLS_ACCESS_KEY_ID}
      access_key_secret: ${VOLC_TLS_ACCESS_KEY_SECRET}
```

Topic、logset、retention 和 index 都由 operator 预先创建。构造 store 时只调用 `DescribeIndex`，不会调用 `CreateIndex` 或 `ModifyIndex`。必需配置为：关闭 full-text 和 auto-index，启用 phrase index；`id`、`stream`、`kind`、`level` 是 case-sensitive non-tokenized text；`msg` 是 case-sensitive、ASCII whitespace delimiter、包含中文的 text；`attributes` 是 case-sensitive、`IndexAll=true` 的 JSON；`payload` 不得建立 index。已有 topic 后续启用 phrase index 时，历史数据是否 rebuild 由 operator 决定。

Operator-owned schema 和 search behavior 可参考 Volc TLS 的 [CreateIndex](https://www.volcengine.com/docs/6470/112187)、[query syntax](https://www.volcengine.com/docs/6470/1206705) 和 [phrase query](https://www.volcengine.com/docs/6470/1206697)。

Provider layout 固定使用 `id`、`stream`、`kind`、`level`、`msg`，把 dotted attributes 展开为 nested `attributes` JSON，并原样保存可选 payload。Generic record 的 provider source 为 `gizclaw`、filename 为 `logstore`；process log 的 `source=gizclaw`、`path=slog` 仍是 logical attribute。Record timestamp 会保留可用的 nanoseconds，而 SearchLogs range 和 ordering 使用 milliseconds。

查询使用 SearchLogs search expression 和 provider Context，不使用 SQL analysis。`Text` 使用 key-value phrase 形式 `msg:#"..."`，dynamic attribute field name 会在翻译时加引号。Provider call 最长 30 秒，并服从更短的 caller deadline；Store 和 Admin API 不返回 provider error body。`Close` 会 flush managed producer，且只有 registry 拥有它的生命周期。

当查询固定为 `Streams=[system]`、`Kinds=[log]` 时，driver 也会匹配 provider source 为 `gizclaw`、filename 为 `slog` 的旧记录。新旧记录共用 provider-side ordering 和 cursor，不会分别查询后再合并。这只是 record compatibility；已移除的 Server `log` 配置仍不兼容。

## Process logging

`system_log` 是 Server 自身的 `slog` pipeline，不是产品 record 写入 API：

```yaml
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

Sink 按顺序执行，每个 sink 可覆盖 level；fanout 会尝试所有 enabled sink 并汇总 error。Store sink 固定写入 `Stream=system`、`Kind=log`，但不拥有 named store 的生命周期。`query_store` 必须指向同一配置中的一个 store sink；未设置时 Admin log endpoint 返回 `LOG_QUERY_NOT_CONFIGURED`。缺少整个 `system_log` 时默认是 info-level stderr。旧的 top-level `log` 配置会直接报错，不自动转换。
