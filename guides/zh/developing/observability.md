# Observability

Observability 用日志回答“某一次请求发生了什么”，用 metrics 回答“系统整体发生了多少次、耗时如何、当前状态如何”。两类信号共享产品语义，但不共享完整字段集：日志可以携带单次请求的关联信息，metrics label 必须控制 cardinality。

## 状态边界

当前代码已经支持：

- 进程级 `slog`，默认写入 stderr，并可选 fan-out 到 Volc TLS；
- GizClaw HTTP 与 Peer RPC 的单次结构化 completion log；
- 进程级 `gizmetrics` counter、gauge、histogram recorder，以及可复用的 `net/http` metrics wrapper；
- Admin HTTP `GET /logs/stream`，从已配置的日志 backend 流式查询日志；
- `pkgs/store/metrics.Store`，通过 Prometheus Remote Write 写入并通过 Prometheus HTTP API 查询；
- Peer telemetry 的 battery、GNSS、network 和 system metrics。

## 持久化 Go runtime profile

Process profiling 是 opt-in 能力，由 `cmd/internal/server` 拥有；它不会注册
`net/http/pprof`，也不会暴露 `/debug/pprof`。需要配置一个专用 logical
ObjectStore，不能与 Workspace assets、Gameplay assets 或 Agent Host runtime data
共享：

```yaml
storage:
  profile-files:
    kind: filesystem.dir
    dir: data/profiles
stores:
  runtime-profiles:
    kind: objectstore
    storage: profile-files
    prefix: pprof
profiling:
  enabled: true
  store: runtime-profiles
```

配置缺失、`{}` 和 `enabled: false` 不执行 Store operation，也不启动 worker；但只要
填写了 Store 名称，即使 disabled 也会验证引用。Enabled 时，command 在 listener
打开前发布一次 baseline；此后每次 attempt 完成后等待五分钟再开始下一次。Attempt
不会重叠，也不会 catch up。Shutdown 取消下一次 wait，让 active attempt 完成或清理，
join worker 后才关闭 logging 与 Stores；shutdown 不额外采集 snapshot。

完整证据的布局如下：

```text
runs/<UTC timestamp>-pid-<pid>/
  000000-baseline/{heap.pprof,allocs.pprof,goroutine.pprof,manifest.json}
  000001-<UTC timestamp>/{heap.pprof,allocs.pprof,goroutine.pprof,manifest.json}
```

每个 profile 直接从 `runtime/pprof` 流式写入；最后写入的 `manifest.json` 记录 run、
sequence、capture time、三个文件的 size 与 SHA-256。只有 valid manifest 才表示完整
set。失败 attempt 会尽力删除可识别的 partial objects；cleanup 失败时，下一次 attempt
会先重试该精确 prefix，成功前不会上传新 set。
Startup 会流式读取 manifest 引用的每个 profile 并验证实际 size 与 SHA-256，再保留旧
run 的 set，同时清理可识别的 manifest-less set；遇到 malformed manifest 或未知 name
时安全失败，不会删除未知数据。

Retention 跨所有 run 且 baseline 计数：最多 576 个 completed sets 和 1 GiB profile
bytes。单个 candidate 也共享一个 1 GiB streaming limit。Rotation 先删最旧 manifest，
再删对应 profiles，reader 不会把删除到一半的 set 误认为完整证据。Periodic failure
只输出一条 structured warning，并在下一个五分钟 wait 后重试；baseline failure 会终止
startup。

Profile 包含 package、function 与 source/build path metadata。必须使用 operator-only
bucket/container 与 prefix，禁止 public access 或作为 asset 提供，并通过 access-controlled
channel 传输。安全下载后的常见分析命令如下：

```sh
go tool pprof -top heap.pprof
go tool pprof -top -inuse_space heap.pprof
go tool pprof -top -alloc_space allocs.pprof
go tool pprof -top goroutine.pprof
go tool pprof -top -base baseline/heap.pprof later/heap.pprof
```

比较时应尽量使用同一个 build 的 profiles。保存的 profile 是诊断证据，本身不能证明
存在 leak，也不能直接归因 outage。

## 信号和 ownership

| 层 | Ownership | 状态 |
| --- | --- | --- |
| `pkgs/gizlog` | 安装全局 `slog`、配置 level，并拥有 Server 与 Edge 共用的 stderr 和 named LogStore sink | 当前已有 |
| `pkgs/gizclaw/internal/observability` | GizClaw 的 transport、surface、operation、result、error 和安全字段 vocabulary，以及到 `slog` 的 projection | 当前已有 |
| `pkgs/gizmetrics` | 进程级 counter、gauge、histogram、聚合、批量 flush 和 no-op default | 当前已有 |
| `pkgs/gizmetrics/httpmetrics` | 通用 `net/http` request count、duration、in-flight 和 response bytes wrapper | 当前已有 |
| `pkgs/store/metrics` | 数值 sample 的持久化与查询 backend，不拥有业务 metric name 或 label | 当前已有 |
| `services/runtime/peertelemetry` | Peer telemetry packet 到 metric name、`peer_id` label 和数值的映射 | 当前已有 |

GenX stream、Transformer 的 EOS/cancel/backpressure 指标由 `pkgs/genx` 的 wrapper 拥有；WebRTC connection、ICE、DataChannel、packet loss 和 RTT 指标由 `pkgs/giznet/gizwebrtc` 的 observer 拥有。通用 metrics package 不反向依赖这些业务或 transport package。

## 统一请求维度

日志和进程请求 metrics 使用同一组有界语义：

| 维度 | 值或来源 | 说明 |
| --- | --- | --- |
| `transport` | `http`、`rpc` | WebRTC signaling 是 HTTP operation，不是独立 transport。 |
| `surface` | `server-public`、`peer-http`、`admin-http`、`peer-openai`、`edge-http`、`peer-rpc` | 表示请求从哪个 GizClaw ingress surface 进入。 |
| `operation` | OpenAPI operation ID、RPC method 或显式注册的常量 | 必须有界；无法识别时使用 `unknown`，不能回退到 raw path。 |
| `method` | HTTP method | 只使用标准 method，不包含 URL；其他值归一为 `OTHER`。 |
| `result` | `success`、`client_error`、`server_error`、`canceled`、`panic`、`transport_error` | 表示完成结果，不替代 HTTP/RPC code。 |
| `status_class` | `2xx`、`3xx`、`4xx`、`5xx`、`unknown` | 用于聚合；日志仍保留精确 `status` 或 `rpc_code`。 |

这些字段是产品 taxonomy。Sink、Prometheus backend 和调用方不能自行创造同义值，例如不能把 `peer-http` 同时作为 `transport` 和 `surface`。

## 结构化日志

### 输出格式

代码继续直接使用全局 `slog`，优先通过 `slog.LogAttrs(ctx, ...)` 输出 scalar attributes。配置化 logger 自动为 stderr 和 Store sink 增加调用源码；Store record 使用 `source_file` 和十进制 `source_line`。配置了 `system_log.node_id` 时，每个 sink 还包含完全相同的 `node_id`。该值由 Deploy 显式注入稳定的逻辑节点名，GizClaw 不根据 IP、hostname、endpoint 或 public key 推断节点身份。

AgentHost 把已认证 Peer 的 `peer_public_key` 放入 Agent 执行 context；使用 context-aware `slog` 的 provider 日志会继承该字段。启动、存储和其他没有 Peer owner 的进程级日志省略该字段，不能伪造身份。Volc TLS handler 将 `level`、`msg` 和每个 scalar attribute 保存为独立字段；`StreamServerLogs` 再规范化为：

| 返回字段 | 含义 |
| --- | --- |
| `time_ms` / `time_ns` | backend 提供的日志时间；`time_ns` 可选。 |
| `level` | 规范化后的日志 level。 |
| `message` | `slog.Record.Message`，来自 backend 的 `msg`。 |
| `source` | 当前 Volc sink 写入 `gizclaw`。 |
| `path` | 当前 Volc sink 写入 `slog`。 |
| `fields` | 除保留字段之外的结构化 scalar attributes，包括适用时的 `node_id`、`source_file`、`source_line` 和 `peer_public_key`。 |

请求 completion record 使用稳定 message `gizclaw: request completed`。HTTP handler 每次完成输出一次；Peer RPC 在第一帧开始后输出一次，连接在新请求首帧之前正常 EOF 时不输出：

| Attribute | HTTP | RPC | 可用于定位 | Metrics label |
| --- | --- | --- | --- | --- |
| `transport` | 是 | 是 | 协议 | 低基数，可用 |
| `surface` | 是 | 是 | ingress | 低基数，可用 |
| `operation` | 是 | 是 | handler / RPC method | 低基数，可用 |
| `result` | 是 | 是 | 完成分类 | 低基数，可用 |
| `status_class` | 是 | 是 | 聚合状态 | 低基数，可用 |
| `duration_ms` | 是 | 是 | 单次耗时 | 不作为 label；使用 histogram value |
| `method` / `route` / `status` | 是 | 否 | HTTP 请求与精确状态 | 仅 `method`、`status_class` 可作为通用 HTTP labels |
| `rpc_code` | 否 | 响应包含 code 时 | JSON-RPC 或应用 code | 不直接作为通用 label |
| `error_code` | 失败时 | 失败时 | 稳定领域错误 | 只有封闭且有界的 code 集合才可作为产品 metric label |
| `request_id` | 是 | 是 | 单次请求关联 | 禁止作为 label |
| `peer_public_key` / `peer_role` | 已认证且已知时 | 已认证且已知时 | 调用方身份 | 禁止作为进程请求 metric label |
| `workspace_name`、`workflow_name`、`model_id`、`resource_kind`、`resource_name` | 已安全解析且需要时 | 已安全解析且需要时 | 领域上下文 | 禁止作为进程请求 metric label |

格式示例：

```text
time=2026-07-16T10:00:00Z level=WARN msg="gizclaw: request completed" transport=rpc surface=peer-rpc operation=server.workspace.create result=client_error status_class=4xx rpc_code=400 error_code=INVALID_WORKSPACE request_id=req-01 duration_ms=12
```

### Level

| Level | 请求结果 |
| --- | --- |
| `INFO` | 普通 2xx/3xx completion。 |
| `WARN` | HTTP 4xx、RPC bad request/forbidden/not found/conflict、JSON-RPC parse/invalid request/invalid params/method not found，以及取消。 |
| `ERROR` | HTTP 5xx、JSON-RPC internal error、panic 和 transport failure。 |

Streaming RPC 只在完整 stream handler 返回时输出一次 completion record；不输出 per-frame、audio、event payload 或成功 chunk 日志。

`server.speech.extract` 只通过同一条 completion record 暴露封闭的阶段/类别 code。阶段包括 request、ASR、Extract Provider、结果解析、Schema 校验和 response 编码。即使 wire response 是通用 internal error，也不会记录 Provider 原始错误或 request/result 内容。

### 筛选

`GET /logs/stream` 的 `filter` 使用 GizClaw-owned grammar，不接受 backend-native query。Filter 为 `*`，或最多 32 个 uppercase `AND` 连接的 clause；支持 `level:value`、`text:value`、`field:value`、`field!=value`、`field:*` 和 `-field:*`。例如：

```text
level:ERROR
surface:peer-rpc
operation:"server.workspace.create"
error_code:INVALID_WORKSPACE
request_id:req-01
```

Value 是不含 whitespace、quote、backslash 或 wildcard 的 token，或不含 wildcard 的 JSON string literal。标准 level 名称会归一化为 uppercase。Field 使用 LogStore dotted-attribute grammar；`message`、`stream`、`kind` 和 provider metadata/time field 保留。不接受 OR、regex、provider function 或 raw provider expression。Filter 最长 4096 bytes，field 最长 128 bytes，decoded value 最长 1024 bytes。请求 completion fields 落地并建立索引后，Grafana 与 Admin log query 都应直接按 scalar field 筛选，不解析 `message`。

首次查询必须提供 inclusive `start_time_ms` 和 exclusive `end_time_ms`。`limit` 默认 100、最大 1000，`order` 是 `asc` 或 `desc`。下一页使用 `end` event 返回的 opaque cursor；带 cursor 继续查询时不能改变 filter、时间范围或 order。

### 敏感信息

日志不得包含 Authorization、cookie、signature、nonce、private key、credential、access key、SDP、raw URL/query、provider error text 或任意 panic value。用户与 AI 之间的输入、最终 ASR transcript 和实际投递的 AI 回复内容不在本节的禁止范围内。

Completion record 不输出 `error_message`。响应 message、`err.Error()`、validation/provider text 和 panic value 均不会投影到结构化字段；`peer_public_key` 只记录已经用于 authorization 的认证身份。

## Metrics

### 写入与查询路径

`pkgs/store/metrics.Store` 接收带 name、labels、timestamp 和 value 的 sample。Prometheus backend 使用 Remote Write 写入，通过 `/api/v1/query` 和 `/api/v1/query_range` 查询；项目不使用 Pushgateway，也不提供 `/metrics` scrape endpoint。

当前 Peer telemetry 直接在带 timeout 的上下文中调用 `Store.Append`。进程 metrics runtime 则先在内存中聚合 counter、gauge 和 histogram，再按 batch flush，避免在 HTTP 业务路径执行 Remote Write。未配置名为 `metrics` 的 store 时不安装 recorder，埋点调用保持 no-op，不创建隐式 memory store。

### 进程级 recorder

[gizmetrics Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizmetrics) · [httpmetrics Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizmetrics/httpmetrics)

调用方通过 `AddCounter`、`SetGauge` 和 `ObserveHistogram` 记录数据。`InstallStore` 一次只允许安装一个 live recorder；安装前和 shutdown 后调用均为 no-op。默认 flush interval 是 10 秒，单次 append timeout 是 5 秒，逻辑 series 上限是 10,000，也可通过 `WithFlushInterval`、`WithAppendTimeout` 和 `WithMaxSeries` 调整。

Counter 保存进程内单调累计值，gauge 保存最新值，histogram 导出累计的 `_bucket`、`_sum` 和 `_count` samples，并总是包含 `le=+Inf`。Metric name、label name、数值和 buckets 在进入聚合 map 前校验；非法 update、同一 series 改变类型/buckets 或超过 series 上限时丢弃并输出限频且不包含 label value 的 warning。业务调用不等待 `Store.Append`，失败或超时的 dirty samples 留待下一次 flush。

`cmd/internal/server` 只在配置了名为 `metrics` 的 store 时安装 recorder。关闭顺序固定为 `gizclaw.Server`、recorder final flush、store registry；recorder 不关闭 store。Standalone Edge 使用顶层 `metrics` 配置直接连接 Prometheus Remote Write/query backend，并在关闭 HTTP、gateway 与 upstream 后 final flush，再关闭 backend connector。

### WebRTC 与 Edge metrics

WebRTC transport 记录 signaling、dial、连接和 service DataChannel 的请求量、结果、时延与 active gauge。通用 labels 只使用有界的 `node_role=application|edge`、`role=client|server`、`direction=inbound|outbound` 和稳定 `result`；PubKey、session ID、service ID、URL、SDP 与内容都不进入 labels。

Edge gateway 额外记录所有入口 signaling（包括在 WebRTC listener 之前被容量控制拒绝的请求）、pending admission、active logical session、burst SCTP、upstream 状态/占用、tunnel channel、logical-session establishment 和 bridge terminal。主要 metric families 为 `giz_webrtc_*` 与 `giz_edge_webrtc_*`。`giz_edge_webrtc_capacity_limit{resource=...}` 暴露与 active gauge 对应的配置上限。

### 当前 Peer telemetry metrics

当前所有 Peer telemetry series 只有 `peer_id` label。它是设备查询的显式 identity 维度，不应复制到通用 HTTP/RPC 请求 metrics。

| Metric | 含义 | 单位或取值 |
| --- | --- | --- |
| `gizclaw_peer_battery_percent` | 电池电量 | 0-100 percent |
| `gizclaw_peer_battery_charging` | 是否充电 | 0 或 1 |
| `gizclaw_peer_battery_voltage_mv` | 电池电压 | millivolt |
| `gizclaw_peer_gnss_latitude` | 纬度 | degree |
| `gizclaw_peer_gnss_longitude` | 经度 | degree |
| `gizclaw_peer_gnss_altitude_m` | 海拔 | meter |
| `gizclaw_peer_gnss_accuracy_m` | 定位精度 | meter |
| `gizclaw_peer_network_rssi_dbm` | 网络 RSSI | dBm |
| `gizclaw_peer_network_signal_level` | 设备报告的信号等级 | 原始数值 |
| `gizclaw_peer_network_connected` | 是否联网 | 0 或 1 |
| `gizclaw_peer_system_uptime_seconds` | 系统运行时间 | second |
| `gizclaw_peer_system_free_memory_bytes` | 可用内存 | byte |
| `gizclaw_peer_system_temperature_c` | 系统温度 | Celsius |

查询示例：

```text
gizclaw_peer_battery_percent{peer_id="<public-key>"}
last_over_time(gizclaw_peer_system_temperature_c{peer_id="<public-key>"}[5m])
```

### 统一 HTTP server metrics

通用 HTTP wrapper 定义以下 metric families：

| Metric | 类型 | Labels |
| --- | --- | --- |
| `giz_http_server_requests_total` | Counter | `surface`, `operation`, `method`, `status_class`, `result` |
| `giz_http_server_request_duration_seconds` | Histogram | `surface`, `operation`, `method`, `status_class`, `result`, exporter 增加的 `le` |
| `giz_http_server_requests_in_flight` | Gauge | `surface`, `operation`, `method` |
| `giz_http_server_response_bytes_total` | Counter | `surface`, `operation`, `method`, `status_class`, `result` |

Duration buckets 是 `0.005`、`0.01`、`0.025`、`0.05`、`0.1`、`0.25`、`0.5`、`1`、`2.5`、`5` 和 `10` seconds。

`method` 只保留 `GET`、`HEAD`、`POST`、`PUT`、`PATCH`、`DELETE`、`OPTIONS`、`CONNECT` 和 `TRACE`，其他值归一为 `OTHER`。In-flight gauge 在同一进程的多个 wrapper 实例之间按相同 label set 聚合。Wrapper 保留底层 writer 已支持的 `http.Flusher`、`http.Hijacker`、`io.ReaderFrom` 和 `http.Pusher`，记录 panic 后继续抛出，不改变 recovery policy。

`httpmetrics.Wrap` 是可复用测量能力，并不会自动给所有 GizClaw surface 增加 request metrics。具体产品 operation 的接入需要由 owner package 显式提供稳定 resolver。WebRTC transport 使用独立的 `giz_webrtc_*` families；Peer RPC 与 GenX 仍不由这个 HTTP wrapper 采集。

聚合示例：

```text
sum by (surface, operation, status_class) (
  rate(giz_http_server_requests_total[5m])
)

histogram_quantile(
  0.95,
  sum by (le, surface, operation) (
    rate(giz_http_server_request_duration_seconds_bucket[5m])
  )
)
```

### Label cardinality

进程请求 metrics 只接受有限枚举或注册表中的值。禁止使用 raw URL/path/query、request ID、peer public key、workspace/workflow/model/resource identifier、credential/provider message、error message、prompt 或其他用户内容作为 label。

`operation` 必须来自 generated operation ID、RPC method 或显式注册常量；未识别时使用 `unknown`。如果某个 `error_code` 来自开放文本或 provider，不能成为 label；只有 server-owned、封闭且有界的 code 集合才能进入产品 metric。

## Edge upstream ICE observation

Edge 成功持有 live upstream 后，`edge: upstream ICE selected` 使用
`upstream_kind=control|gateway`、有界 upstream ID 和 connection epoch 关联 selected pair。
Candidate 字段只允许 type、UDP/TCP protocol、IPv4/IPv6 family、component、state、
nomination、支持的 counter 和可选的零起始 relay-member ordinal，这些字段均为有界基数。

日志及派生 capacity artifact 禁止包含 IP 地址、端口、TURN URL、SDP、candidate ID/body、
foundation、priority、username、credential 或 mutable Pion value。没有 selected pair 时输出
warning；仅凭配置不能宣称实际使用了 relay。

2026-08-04 的本机验收把这些 selected-pair 记录与精确 Coturn allocation/traffic counter
组合使用。全部 12 轮产品测试都证明了指定 path；同一 clean head 的纯 Giznet lane 随后
复现 direct 818/798 Mbps 与 REST Coturn 488/526 Mbps，Coturn counter 同时增长约
220/219 MB。该诊断不包含产品 Edge 和 Server，因此 material 产品差异归属于本机 Coturn
relay path，而不是 Edge/Server resource owner。这里的 counter 支撑有界因果结论；仅凭
配置不能得出该结论。

## 新增埋点时

1. 先判断问题需要单次请求证据、聚合趋势，还是两者都需要。
2. 从统一 taxonomy 选择 `transport`、`surface`、`operation` 和 `result`，不要创建同义字段。
3. 日志保留诊断所需的安全关联信息；metrics 只保留低 cardinality labels。
4. HTTP 通用测量放在 `pkgs/gizmetrics/httpmetrics`，GizClaw 产品字段放在 `pkgs/gizclaw/internal/observability`，GenX 与 WebRTC 指标留在各自 owner package。
5. 测试成功、4xx/5xx、取消、panic、streaming、backend failure、redaction 和 no-store 路径，并证明 instrumentation 不改变业务 response 或 lifecycle。

## Peer stream 生命周期与对话内容

`gizclaw: peer stream lifecycle` 把 direct Peer 或经 Edge 路由的 logical Peer 从 Server input 一直关联到 Agent output；Edge 路径还覆盖 gateway admission。认证后的 logical identity 记录为 `peer_public_key`，Edge 与 Server 使用相同的 `tunnel_session_id`。Connection-level `component`、`stage`、`result`、`reason`、`last_stage` 与 `duration_ms` record 保持兼容；connection terminal 继续包含 `input_event_observed`、`agent_input_opened`、`agent_input_pushed` 和 `output_event_observed`，用于区分 zero-event connection failure。

Edge 的 `bridge_started` terminal 会保留首个 connection-level bridge path、direction、phase
与封闭 error class。Destination-open failure 聚合为一个 count 以及 first/last direction 和 class。
只有能够精确归属 established session 或 association capacity 时，才增加
`bridge_capacity_scope`、`bridge_active_channels` 和 `bridge_channel_limit`；否则省略这些字段，
不做推断。所有 bridge dimension 都是顶层 scalar log field，绝不成为 metric label，也不产生
per-service record 或复制 raw error。

每个授权通过的 input BOS 都在当前 Peer connection 内分配一个单调递增的正数 `turn_index`。Edge 路径使用 `(tunnel_session_id, turn_index)`，direct 路径使用 `(peer_public_key, turn_index)` 查询单个 logical turn；这些字段不进入 wire contract，也不能成为 metric label。Input 与 assistant output 的 stream identifier 可以不同，分别记录为安全的 `input_stream_id_hash` 和 `output_stream_id_hash`。Output 只通过 producer response epoch 的不可变 owning input route 绑定；不存在 current-turn、timing、Workspace 或 output-ID fallback。被替换 turn 会有界保留，使旧 epoch 第一次迟到的 chunk 与 terminal 仍归属原 turn；无 provenance output 不归属 per-turn record。

稳定 message `gizclaw: AI conversation content` 记录该 turn 的实际对话内容。每条 record 包含 `peer_public_key`、可用时的 `tunnel_session_id`、`turn_index`、`content_role=user|assistant`、从 0 开始且按 role 独立递增的 `content_index`、`content_source`、`event_type` 和原始 `content`。Direct text 只在授权输入成功进入 Agent queue 后以 `content_source=agent_input` 记录；音频输入的最终 ASR 文本在 Agent 产出 transcript 时以 `content_source=final_transcript` 记录；AI 文本只在对应 Peer event 成功 broadcast 后以 `content_source=peer_delivery` 记录。生成但未投递的 AI 文本不记为回复。在同一 `content_source` 内按 `(peer_public_key, tunnel_session_id?, turn_index, content_role, content_index)` 排序并拼接 `content`，可分别重组直接问话、最终 transcript 或实际投递回复，避免混淆 direct input 与 ASR 两种表示。

有界的 per-turn stage 包括 `turn_started`、`input_first_event`、`input_terminal`、`interrupt_observed`、`agent_input_first_push`、`agent_transform_started`、`agent_output_produced`、`output_first_event`、`agent_output_delivered`、`agent_terminal`、`output_terminal` 和 `turn_terminal`。Turn boundary 使用 `component=peer_turn`，transport input/output stage 保持 `component=peer_input|agent_output`，四个 Agent boundary stage 使用 `component=agent_runtime`。每个适用 stage 在一个 turn 中最多输出一次。首次 produced/delivered record 包含一个封闭的 `output_modality` 值：`transcript_text`、`assistant_text`、`assistant_audio`、`assistant_eos`、`interrupt`、`control` 或 `other`；后续 chunk 只更新有界 terminal snapshot。

`agent_transform_started` 表示所选 transformer 已读取 input，而不只是 Peer queue 已接受。`agent_output_produced` 在独立 consumer 调度前分类 GenX source chunk；`agent_output_delivered` 只分类实际成功 broadcast 的 Peer event，audio 还要求 mixer drain 完成。Broadcast 失败、drain 失败或 abandon，以及被聚合器抑制的 boundary 都不会增加 delivered modality。空 label text/blob event 使用与 Peer client 相同的 assistant fallback；空 label control-only EOS 仍为 `other`。

`agent_terminal.terminal_class` 只能是 `completed`、`interrupted`、`provider_error`、`transform_error`、`stream_error`、`caller_canceled` 或 `deadline_exceeded`。`turn_terminal` 在既有有界布尔值之外增加 `agent_transform_started`、`agent_terminal_observed`、`produced_modalities`、`delivered_modalities` 与五组排序去重的 class：`source_part_classes`（`text`、`audio`、`control`、`other`），`source_label_classes` 和 `peer_event_label_classes`（`assistant`、`transcript`、`history`、`empty`、`other`），`peer_event_types`（`bos`、`eos`、`text_delta`、`text_done`），以及 `peer_event_kinds`（`text`、`audio`、`video`、`mixed`、`unspecified`）。这些字段在不记录 raw label 或 payload 的前提下区分 zero output、transcript-only、audio-only、仅 EOS/interruption、Agent failure 与 downstream delivery failure。封闭的 `result` 取值为 `success`、`replaced`、`interrupted`、`canceled`、`timeout`、`closed`、`runtime_error` 和 `incomplete`；terminal 或 interruption 的封闭 `reason` 取值为 `completed`、`input_replaced`、`control_interrupt`、`expected_interruption`、`caller_canceled`、`deadline_exceeded`、`stream_closed`、`internal_error` 和 `state_limit`。Raw error 绝不被复制。

Lifecycle stage 日志量只随 turn 数乘固定 stage 集合增长，不随 packet、audio frame、text delta 或 control fragment 增长；对话内容日志则按文本 chunk 增长，以避免在 lifecycle state 中缓存无界内容。Active 与 recently replaced state 有固定上限，completed state 会被释放；connection teardown 会为每个仍保留的 incomplete turn 输出一次 terminal summary，再清空 correlation map。Instrumentation 不阻塞、不重试、不重排，也不改变 Peer、AgentHost、provider、interruption、timeout 或 cleanup 行为。

Server 为每个 direct 或 Edge logical Peer 构造 connection、turn 与 Agent-runtime observer；对话审计内容不是可选字段，也没有独立关闭配置。日志 sink 仍负责持久化与级别策略，但 runtime 不再因为启动时 `INFO` 被过滤而跳过内容关联状态。

既有 `gizclaw: assistant route failed` Error record 继续保留有界 route、Workspace 与 error 字段；它不是对话内容 record，也不能替代按 turn 关联的实际投递回复。

不可信 stream identifier 使用稳定的 128-bit hash，绝不记录 raw `stream_id`。哈希契约固定为：去掉首尾 Unicode 空白字符，将结果按 UTF-8 编码，使用无密钥 SHA-256，保留摘要前 16 字节并输出 32 位小写十六进制；规范化后为空时省略该字段。不做大小写折叠或 Unicode 规范化，也不使用 salt 或 HMAC key。例如 `stream-42` 固定得到 `0f3a788cbbee0b932cfcac7d71645f31`。它只是避免意外暴露原值的稳定关联 token，不是匿名化边界：低熵 ID 仍可被字典枚举，因此上游不得把凭据或秘密放进 stream ID。Session、turn、Peer、Workspace 和 stream identifier 只能用于日志查询，不能成为 metric label。Lifecycle record 禁止包含 remote address、SDP、ICE candidate body、credential、provider raw error 或 panic value；这条限制不禁止记录用户与 AI 之间的输入、最终 ASR transcript 和实际投递的 AI 回复内容。
