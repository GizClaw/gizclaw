# Monitor

监控控制台位于 `web/console/`（React、TypeScript、Vite、shadcn/ui、Recharts），构建后
嵌入 Go 二进制，随 Server 和 Edge 一起分发。访问节点的 `/monitor/` 即可打开，无需另行部署
静态文件；`/monitor` 自动重定向到该入口。设计与数据约束见 `web/console/DESIGN.md`。

## 节点快照接口与权限

Server 与 Edge 在现有 HTTP/HTTPS listener 上挂载 `pkgs/monitor.Handler`，不新增仅监控
listener。页面可公开加载，节点数据仍由独立 Monitor Token 保护。

GET `/monitor/api/node` 只读取当前进程，使用独立的
`Authorization: Bearer gizclaw_mk_...`。在节点配置文件中设置：

```yaml
monitor:
  token: ${GIZCLAW_MONITOR_TOKEN}
```

Token 前缀为 `gizclaw_mk_`，其后至少 32 个字符，建议用 `openssl rand -hex 32` 生成随机
部分。空配置关闭节点数据接口（503），非法 Token 返回 401，响应均为 no-store。

该接口处理 CORS 预检并回显请求 Origin，附带 `Vary: Origin`、
`Access-Control-Allow-Methods: GET,OPTIONS` 和
`Access-Control-Allow-Headers: Authorization,Content-Type`，因此部署在其他 origin 的
控制台也能读取快照。Monitor Token 通过显式 Authorization 头传递，不共享浏览器凭证。

控制台把配置（节点、Token 和本人的设备关注列表）用不可导出的 Web Crypto 密钥加密后存入
同源 IndexedDB，退出时清除。这不是操作系统钥匙串，同源脚本仍能使用该密钥；Telemetry 与
日志不写入浏览器存储。

设备监控走接入点的 Public API，使用设备的 `gizclaw_pk_` Bearer。Server 的普通业务 HTTP
入口仍返回 PRIVATE_INGRESS_DENIED，因此设备接口由接入点（Edge）提供。readonly 允许读取，
fullcontrol 才允许音量和重启等操作，Server 每次重新校验权限。

## 数据语义

节点连接数是进程内的 WebRTC 关联数，包含上游连接；服务流单独计数。RX/TX 是进程启动以来
的 WebRTC 服务负载字节，不含 ICE/DTLS 开销。设备计数来自所属 Server 的 Runtime。控制台
每 5 秒并行轮询所有节点，用累计计数换算速率（重启读作 0，而不是尖峰），每个节点最多保留
600 个采样，提供 2/10/30 分钟窗口。

日志通过设备的持久化 LogStore 搜索接口查询，支持时间范围、文本、级别与分页。节点快照只返回运行状态和传输计数。

Telemetry 分两类：指标字段（`battery.*`、`network.rssi_dbm`、`network.signal_level`、
`network.connected`、`system.*`、`gnss.*`）会写入采样存储，可按时间区间查询历史；只反映
状态的值（固件与软件版本、蜂窝 `rat`、`operator`、`imei`、`imsi`、音频播放器状态、OTA 上报）
只保留最新值，进入设备状态。控制台把前者放在 Telemetry 页并逐字段提供趋势图，后者放在状态
字段页。

## 构建与验证

```sh
npm ci
npm run build --workspace @gizclaw/console
npm test --workspace @gizclaw/console
go build ./cmd/gizclaw
```

控制台构建产物是 `web/console/dist/` 静态文件（git 忽略），构建步骤会生成 `assets_generated.go`，通过 `go:embed` 逐个列出产物文件并嵌入二进制。
清单同样由 git 忽略；缺失任一列出的文件都会使 Go 编译失败。
编译 Go 或运行依赖监控模块的 Go 测试前必须先构建控制台；缺少产物会使编译失败。Linux Docker
构建和 macOS 发布流程都会执行此步骤，运行时不依赖源目录。产物也可单独部署为静态站点。开发时
`npm run dev --workspace @gizclaw/console` 监听 5174，并把 `/gizclaw` 代理到本地接入点
（可用 `CONSOLE_DEVICE_PROXY` 覆盖）；节点快照直接从配置中的节点地址读取。

## HTTP 契约与验证

`api/http/monitor.json` 拥有独立的 Monitor OpenAPI surface。Token 中间件在生成的标准库
路由之前执行，`nodeServer` 实现生成的 strict 接口，控制台调用生成的 JavaScript client。
该 surface 属于每个进程本地，不使用 Peer 指派或 Admin 认证。

| 状态 | 响应 |
| --- | --- |
| 200 | 生成的 `NodeSnapshot`，包含本地运行状态与计数 |
| 401 | `{"error":"INVALID_MONITOR_TOKEN"}` |
| 503 | 未配置 Token 时返回 `{"error":"MONITOR_DISABLED"}` |
| 405 | 空响应体，`Allow: GET,OPTIONS` |

节点接口响应均带 `Cache-Control: no-store`。JSON 成功/错误类型与 Go strict server/client 由
`go generate ./pkgs/monitor/api` 生成，JavaScript 由 `npm --prefix sdk/js run gen:sdk`
生成，配置与产物路径见 [API 生成](api/generation)。

`go test ./pkgs/monitor` 覆盖 Token 授权、CORS 契约、405 方法边界、生成 client 的
200/401/503 处理，以及节点快照不包含日志；`go test ./pkgs/gizlog` 验证配置的日志输出。

## 控制台使用的设备接口

`GET /gizclaw/v1/device/workspaces` 只列出该 Peer 明确拥有的 Workspace（含系统 Workspace），
按 Workflow 分组，不返回共享和无 owner 的空间。
`GET /gizclaw/v1/device/workspaces/{workspaceId}/history` 从持久化 History 查询文本并游标
分页，每页最多 200 条（控制台使用 100 条）。`order` 默认 `desc`（最新在前），也可为 `asc`；
可选的 `start_time_ms`（含）与 `end_time_ms`（不含）按 Unix 毫秒限定创建时间，续页请求同样生效。
游标是不含自身的 entry ID 时间边界：传上一页的 `next_cursor` 按同一方向继续，或传任一条目的
`name` 按所请求方向离开该条目。控制台用 `end_time_ms` 跳到所选日期当天结束之前的记录，再用
`desc` 续页读更早的记录、用最上面条目的 `name` 加 `asc` 读更新的记录。游标不是授权凭证，格式非法
返回 400 `INVALID_HISTORY_CURSOR`；`start_time_ms` 不早于 `end_time_ms` 等非法参数返回 400
`INVALID_REQUEST`。浏览历史不启动 Agent。音频通过同路径下的
`/{historyId}/audio.ogg` 认证读取。

`GET /gizclaw/v1/device/telemetry/{field}/latest` 返回单个字段的最新采样；
`GET /gizclaw/v1/device/telemetry` 按明确的时间区间返回该字段的采样序列。

`GET /gizclaw/v1/device/logs/search` 查询配置的 `services.system_log.query_store`，支持正数
Unix 毫秒区间、最长 512 UTF-8 字节的文本、严格 `DEBUG|INFO|WARN|ERROR` 级别和游标，每页最多
500 条。Server 强制绑定授权 Peer，续页不能改用其他 Peer 的游标。

真实接口验收运行 `bash tests/gizclaw-e2e/run_monitor_tests.sh`，覆盖设备与节点权限、聊天历史、
运行日志和音频下载，详见 [Monitor API giztest](testing#monitor-api-giztest)。

## HTTP 代理通道生命周期验证

`go test -race ./pkgs/giznet/gizhttp -run 'TestReverseProxyConcurrentStreamLifecycle|TestHTTPStreamTimeoutAndCancellationRelease' -count=3`
使用真实 HTTP reverse proxy 和生产 WebRTC 配置，分别覆盖直连与本地 TURN/UDP 中继，并断言中继
实际被选中。每轮并发测试在同一条 WebRTC 连接上以 16 路并发完成 4096 次 HTTP 请求；超时测试覆盖
响应头等待、流式响应体读取、调用方取消，以及下游 TCP 在响应前和响应中断开。

测试在父 WebRTC 连接仍保持打开时检查入站计数、双向服务流总数和 HTTP 服务端连接数回到基线，并在
异常请求后通过原连接再次完成请求。它不依靠关闭父连接回收资源，也不覆盖真实公网丢包、香港 TURN
部署或长时间运行条件。
