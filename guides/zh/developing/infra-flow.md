# Infrastructure Flow Feed

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/infraflow)

`pkgs/infraflow` 为基础设施可视化保存固定五秒的链路聚合帧，通过已有 `kv.Store` 读写。
API source 是 `api/http/infra.json`，生成的 strict Go server/client/model 位于
`pkgs/infraflow/api`，JavaScript contract 位于 control SDK 的 `generated/infra`。

## 数据与资源边界

- 一个生产者在启动时注册最多 64 条基础设施有向链路，之后按索引合并计数，不能动态创建用户、设备或会话维度。
- 每五秒 Drain 一帧；`count`、`bytes`、`duration_count` 与 `duration_sum_ms` 是该链路在窗口内的汇总。
  `count` 是生产者定义的活动次数，不能把不同类型的计数解释成统一的 HTTP 请求数。时延均值使用总耗时除以有效样本数。
- 每帧最多 32 KiB，数值和整帧总量均不能超过 JavaScript 安全数值范围。活动量增加只改变有上限的数值，不增加条数。
- 每个环境使用 120 个循环槽位和一个 head，共最多 121 个键；四个固定环境总共最多 484 个键。
  活跃时覆盖约十分钟历史，停止写入后槽位保持有界，旧帧不会被当成新活动。
- 条件 mutation 原子提交槽位和 head。同窗口相同内容可重试，不同内容返回冲突；实际聚合任务需要明确单一 owner。
- KV 操作在聚合锁外执行，写入失败不会积累原始事件或无限重试。Feed 借用 Store，不接管连接关闭。

源与目标使用已发布基础设施拓扑的稳定节点 ID。仅凭节点总流量不能生成精确链路；真实生产者需要按
配置的链路读取计数，同一有向链路选择一个计数 owner，避免两端重复。此模块不枚举业务记录；Server 按显式配置安装下面的真实活动采集。

## AP 读取接口

`GET /gizclaw/v1/infra/flows?environment=prod&cursor=<上次游标>` 是公开只读接口，浏览器可直接匿名读取。
不需要 Authorization、token 或 owner 身份；即使携带旧凭据，也不会查询设备 Key 或归属。
公开内容只有基础设施有向链路的窗口汇总，不含用户、设备、Peer、会话或原始事件记录。
环境只能是 `dev|e2e|prod|trial`，不接受任意查询、用户过滤或可变 limit。

首次连接只返回最新帧；连续游标最多补读三帧。未来、过期或不连续的游标返回 `reset: true` 和最新帧，
不回放整个缓存。没有生产者时返回空 `frames` 和空游标。每个环境的最近帧共用五秒进程缓存，
每次填充最多四次精确 KV Get；可重试的读取错误也短暂缓存，避免故障时重复读取。
调用方的取消和 deadline 错误不进入共享缓存，健康的等待者可以立即重新加载。

Server 显式配置逻辑 KV Store 后启用读取：

```yaml
infra_flow:
  store: infra-windows
```

`infra-windows` 必须是 `stores` 中已配置的 `kind: keyvalue`，建议使用独立 prefix。空配置关闭接口，
不会建立隐式内存 Store。为兼容旧配置，`read_token` 字段仍可解析，但其值完全忽略；缺失、空值、
旧值或未设置的环境变量都不影响启用。仅配置旧 token 而不配置 Store 时仍关闭接口。
Server 普通 HTTP 与可信 Edge HTTP 都挂载这个公开的精确路径；Edge 将它路由到可达的配置 upstream，
不执行设备 API-key 或 owner 查询。相邻路径（包括结尾斜杠）继续走既有权限规则，
Monitor、设备、Peer 和 Admin 的授权保持独立。所有 Server 应读取同一环境的共享 KV；AP DNS 选择哪个区域不影响游标。

请求仅允许 GET/OPTIONS，其他方法返回 405 和 `Allow: GET,OPTIONS`。Server 与 Edge 的成功、错误和预检响应
都为 `no-store`，允许跨域读取，回显 Origin 并带 `Vary: Origin`；无 Origin 时为 `*`。
`Access-Control-Allow-Methods` 固定为 `GET,OPTIONS`，允许的请求头为 `Authorization,Content-Type`，不启用浏览器凭证。
OPTIONS 无需查询参数或可用 KV，返回 204。400 表示查询非法，503 表示未配置或 KV 暂不可用；
Edge 无法连接 upstream 时沿用代理的 502 `UPSTREAM_ERROR`。请求有两秒读取期限，没有公开 HTTP 写入接口。

各环境 AP 使用同一个路径和对应的 `environment` 查询：

| 环境 | AP |
| --- | --- |
| `dev` | `https://ap.dev.gizclaw.com` |
| `e2e` | `https://ap.e2e.gizclaw.com` |
| `prod` | `https://ap.gizclaw.com` |
| `trial` | `https://ap.trial.gizclaw.com` |

例如浏览器直接 GET `https://ap.dev.gizclaw.com/gizclaw/v1/infra/flows?environment=dev`，无需附加凭据。
AP 是否支持本契约取决于实际部署版本及 KV 配置，代码与本地测试不代表线上验收。

## 发布、部署与验收

现有 AP 生效需要独立发布包含此契约的 GizClaw 二进制与 runtime image，再由 Deploy 使用正式
Release/Provider snapshot 准备配置和部署计划。确认各环境 Server 的 `infra_flow.store` 指向显式配置的共享 KV，
并分别升级 Server 与 Edge；发布、Deploy Apply 和流量切换属于独立操作。
没有生产者时，启用的 feed 返回空帧；真实活动需要按下节启用唯一归属的采集与发布。

部署后逐个 AP 匿名验证 GET、跨域 OPTIONS、非法环境/游标、405、no-store 与设备/Monitor/Admin 权限隔离，
并检查返回帧的 `environment`、游标、时间和 `simulated`，才能确认线上数据覆盖和新鲜度。

## 真实活动采集与唯一归属

这是展示用的低开销、尽力而为活动信号。优先保证业务路径，不追求完整记录：丢样、漏记和窗口缺失均允许，
不补录、不重放、不按采样率推算总量，不用于计费、分析、吞吐统计或业务延迟 SLI。

采集边界仅限基础设施 Peer 之间的数据请求 Surface。目前接入可信 Edge → Server 的 HTTP 与控制 RPC 完成事件；
其他 Server/Mesh Peer 路径没有安装采集器。数据库/KV 调用和请求内部操作均不插桩；KV 在这里仅用于保存和读取最终窗口帧。

每条链路为样本字节和耗时各保留整帧安全数值容量的六十四分之一；超出预算的来源在合并前省略，
避免异常大数值导致其他健康来源也无法发布。

每条链路每五秒窗口最多保留 20 个样本，后台窗口轮转时补充 20 次采样额度。
额度耗尽时热路径只读取一次原子值，不读时钟、不分配、不加锁；
只有选中的请求才计时、计字节，RPC 也仅为选中请求安装字节观察层。选中样本完成后尝试更新固定内存单元，
锁正被快照占用、窗口错过或达到容量时立即丢弃，业务不等待。窗口轮转、额度补充和快照分配都由后台读取完成。
没有读者/采集任务时，初始额度耗尽后不会继续做计时和计字节工作。采样不增加数据库插桩或业务 KV 操作。

采集由 Server 进程拥有，按配置启用，不需要额外守护进程或 Metrics Store。每个环境只在一台指定 Server
配置 `producer`；其他 Server 只配置 `capture` 和共享 Feed Store。各集群可独立启用，环境只能是
`dev|e2e|trial|prod`，节点 ID 必须对应已发布的拓扑。一个来源声明其完整链路集合，启动时拒绝重复链路、
重复来源公钥、重复 Edge 归属及超过 64 条链路的配置。

| 支持的链路 | 唯一计数 owner | count | bytes | duration |
| --- | --- | --- | --- | --- |
| 已配置 Edge → authoritative Server | 接收该 Edge 的 Server；以可信连接公钥匹配 `edge-nodes` | 保留下来的 Edge HTTP/控制 RPC 完成样本，包含错误结果 | 保留样本实际写出的 HTTP 响应正文；RPC 样本实际读写的逻辑流字节（含 framing） | 保留样本的实际经过时间 |

Peer 请求样本使用 `kind: request`；同一 Edge 的 HTTP 与控制 RPC 是不同操作，可合并成一条基础设施链路。
HTTP 字节不含请求体/headers；RPC 不含 ICE/DTLS 等外层开销。
数据库读写，以及设备/Server 音频、模型、资产、复制和整机网络流量
均不属于当前采集覆盖。没有精确归属的链路不创建记录。只读 Feed 的轮询不计入 Edge 活动，避免可视化自激。

采集单元只保留当前和上一个五秒 UTC 窗口，保留的样本按完成时间归入窗口。Monitor 快照读取已完成窗口，
不会 Drain 或偷走计数，所以多读者和抓取延迟不会导致双计或跨窗口差分。进程重启有新的 generation；
启动和时钟回退后的不完整窗口及不可用来源省略，过大的单个样本直接丢弃。
零值只表示本窗口没有保留下来的样本，不能推断没有业务活动。全部来源不可用时不写帧，读取端依据时间检查新鲜度。
暂停或写入失败不补写历史、不把丢失窗口累加到下一帧。

远端计数通过已有且独立认证的 `/monitor/api/node` 可选字段 `infra_flows` 读取。生产者验证配置公钥、
Server 角色、环境、完整链路集合、快照时间与目标窗口；拒绝旧窗口、未知链路及非法计数。
每窗口至多 64 个并行来源、每来源一次请求、最多 64 KiB 响应。发布者在窗口边界后等待两秒再抓取，
让来源在允许的时钟误差内关闭窗口；仍只抓取一次，共用一秒预算，写 Feed 另有一秒预算。
远端要求 HTTPS，只有 loopback 本地测试允许 HTTP；禁止跟随重定向，Monitor Token 不进入公开 Feed 或故障日志。
故障只输出有界的窗口警告。独立 publisher 的同窗口冲突会停止该 worker，业务继续运行；运营方仍须保证每环境
仅一个启用的 publisher。停止时先取消并等待 worker，再关闭 Server 和 Store，不发布部分关闭窗口。

下面是同一 Server 同时采集并发布的配置。Store 名引用正常的 `stores` 清单；替换示例节点 ID、公钥与
Store 名为所在集群的真实映射。Edge 无需新增采集配置，公钥仍须在 `edge-nodes` 中。

```yaml
infra_flow:
  store: infra-windows
  environment: e2e
  capture:
    node: server-shanghai
    routes:
      - source: edge-shanghai
        target: server-shanghai
        edge_public_key: ${EDGE_PUBLIC_KEY}
  producer: {}
```

多 Server 环境在指定 publisher 上增加远端来源；远端只保留上述 `capture`，移除 `producer`，
并配置独立 `monitor.token`。发布端的 `routes` 必须完整匹配远端声明的节点和方向：

```yaml
infra_flow:
  store: infra-windows
  environment: e2e
  producer:
    sources:
      - endpoint: https://server.example.invalid/monitor/api/node
        public_key: ${SOURCE_SERVER_PUBLIC_KEY}
        token: ${SOURCE_MONITOR_TOKEN}
        routes:
          - source: edge-shanghai
            target: server-shanghai
```

同一进程有本地 `capture` 时自动加入其链路，无需重复配置为远端来源。Feed Store 是窗口帧存储后端。
Server 必须包含 `infra_flow.capture`、`producer` 和 Monitor `infra_flows` 的实现；
仅支持公开 Feed 读取的 v0.33.8 及更早版本不能安装此生产者。正式发布和各环境部署后仍需独立验收。
Edge 继续使用支持公开只读 Feed 的版本，SDK 可兼容忽略新的可选 Monitor 字段。

本地验收使用真正的 Server/Edge 独立进程和正常发布启动路径：两个 Server 共用临时 SQLite KV，
一个 Server 采集并通过受认证 Monitor 提供窗口，另一个负责写 Feed；Edge 经真实 WebRTC 转发 HTTP 与控制 RPC。
测试创建正常业务 API Key，以四次 HTTP 与一次控制 RPC 触发五次 Peer Surface 活动，内部数据库操作不产生额外 Flow，
再匿名读取新鲜 `simulated: false` 帧，并核对样本字节、时延、窗口、游标、无新增样本的下一窗口及环境隔离。
故障隔离、来源校验、时钟误差、正数溢出隔离、采样硬上限、丢样、不阻塞业务、重启、取消及 121 个 KV 键上限另由确定性测试验证。

```sh
go test ./cmd/internal/server -run '^TestInfraFlowProductionServerEdgeCollection$' -count=1 -v
go test -race ./pkgs/infraflow/... -count=1
go test ./pkgs/infraflow -run '^$' -bench '^BenchmarkExhaustedSamplingGate$' -benchmem -cpu 1,4
go test ./pkgs/gizclaw -run '^$' -bench '^BenchmarkHTTPActivitySampling$' -benchmem -cpu 1,4
```

对照基准使用相同业务与日志工作，检查开启采样后是否新增分配及请求处理开销；它们不把样本数量作为完整流量，
也不以保全数据为由等待采集。阻塞测试在聚合锁被持有时并发提交 80,000 次 Peer 完成观察，要求观察立即返回并丢弃争用样本。

这些是 loopback/SQLite 协议验收，不表示物理设备、TiKV、生产 VMP 或线上 AP 已通过。

## 本地模拟与消费者

GizClaw checkout 中运行：

```sh
go run ./examples/infra-flow-demo
```

这个 loopback-only 示例明确使用 `kv.NewMemory` 和同一个生产 handler，每五秒生成一帧，
包含 8 个模拟节点和 12 条预配置链路。`/demo/config` 和 `/demo/scale` 仅属于这个示例，
不会挂载到 Server。模拟活动量可在 1 与 100 万倍间切换，聚合调用次数、槽位数量和链路条数保持不变。

devtools checkout 中运行：

```sh
make infra-globe-build
npm run dev --workspace @gizclaw/infra-globe -- --port 5176 --strictPort
```

打开 `http://127.0.0.1:5176/flow-feed.html`。本地 Go 示例也可直接匿名读取：

```sh
curl 'http://127.0.0.1:9819/gizclaw/v1/infra/flows?environment=prod'
```

`/demo/config` 返回拓扑、路径与显式模拟标记，不包含 token。SDK 根据聚合帧计算窗口速率和加权平均耗时，每帧最多抽取 16 个
动画光点，活动光点总数不超过 24，绘制最多 30 FPS，空闲停止。暂停或隐藏页面取消读取与动画。
光点表示聚合活动，不对应单个请求，P50/P95/P99 也不从这些代表光点计算。

验证包含 KV 槽位与编码大小边界、百万倍计数、CAS 冲突、环境隔离、游标补读、缓存、
generated client 匿名 GET/OPTIONS、配置兼容、读取期限，以及 Server/Edge 权限隔离和无需设备归属查询的路由。

```sh
go test -race ./pkgs/infraflow/... -count=3
```
