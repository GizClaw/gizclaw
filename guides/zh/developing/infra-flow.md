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

`GET /gizclaw/v1/infra/flows?environment=prod&cursor=<上次游标>` 使用独立的
`Authorization: Bearer gizclaw_mk_...` 读取凭据。设备 API Key 和设备公钥不授予读取权限。
环境只能是 `dev|e2e|prod|trial`，不接受任意查询、用户过滤或可变 limit。

首次连接只返回最新帧；连续游标最多补读三帧。未来、过期或不连续的游标返回 `reset: true` 和最新帧，
不回放整个缓存。没有生产者时返回空 `frames` 和空游标。每个环境的最近帧共用五秒进程缓存，
每次填充最多四次精确 KV Get；可重试的读取错误也短暂缓存，避免故障时重复读取。

Server 显式配置逻辑 KV Store 与读取凭据后安装此 handler：

```yaml
infra_flow:
  store: infra-windows
  read_token: ${GIZCLAW_INFRA_FLOW_READ_TOKEN}
```

`infra-windows` 必须是 `stores` 中已配置的 `kind: keyvalue`，建议使用独立 prefix。空配置关闭接口，
不会建立隐式内存 Store。Server 普通 HTTP 与可信 Edge HTTP 都挂载这个独立认证的精确路径；
其他客户端路径继续走既有权限规则。Edge 将它路由到可达的配置 upstream，由 Server 验证读取 token，
不执行设备 API-key owner 查询。所有 Server 应读取同一环境的共享 KV；AP DNS 选择哪个区域不影响游标。

请求仅允许 GET/OPTIONS，成功与失败都为 `no-store`。401 表示 token 无效，400 表示查询非法，
503 表示未配置或 KV 暂不可用。请求有两秒读取期限，没有公开 HTTP 写入接口。

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

打开 `http://127.0.0.1:5176/flow-feed.html`。该页通过本地代理读取真实 Go AP 接口；
token 仅用于当前进程的模拟数据。SDK 根据聚合帧计算窗口速率和加权平均耗时，每帧最多抽取 16 个
动画光点，活动光点总数不超过 24，绘制最多 30 FPS，空闲停止。暂停或隐藏页面取消读取与动画。
光点表示聚合活动，不对应单个请求，P50/P95/P99 也不从这些代表光点计算。

验证包含 KV 槽位与编码大小边界、百万倍计数、CAS 冲突、环境隔离、游标补读、缓存、
generated client、独立鉴权，以及 Edge 路由不会触发设备归属查询。

```sh
go test -race ./pkgs/infraflow/... -count=3
```
