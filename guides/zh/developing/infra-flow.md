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
配置的链路读取计数，同一有向链路选择一个计数 owner，避免两端重复。此模块不枚举业务记录，也不自动
安装真实网络采集器。

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
没有生产者时，启用的 feed 返回空帧；真实基础设施活动仍需由明确拥有配置链路计数的聚合任务写入共享 KV。

部署后逐个 AP 匿名验证 GET、跨域 OPTIONS、非法环境/游标、405、no-store 与设备/Monitor/Admin 权限隔离，
并检查返回帧的 `environment`、游标、时间和 `simulated`，才能确认线上数据覆盖和新鲜度。

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
