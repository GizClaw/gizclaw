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

日志通过设备的持久化 LogStore 搜索接口查询，支持时间范围、文本、级别与分页。节点快照只返回二进制版本与构建提交、运行状态和传输计数；控制台在节点列表和运行快照中显示版本。

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
| 200 | 生成的 `NodeSnapshot`，包含构建信息、本地运行状态与计数 |
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
不返回共享、无 owner 和删除中的空间；每项以 `collection` 与 `workflow_name` 标识 Workflow，
控制台按这两个名字标注，不显示 Admin Workflow ID。
`GET /gizclaw/v1/device/workspaces/{workspaceId}/history` 从持久化 History 查询文本并游标
分页，每页最多 200 条（控制台使用 100 条）。`order` 默认 `desc`（最新在前），也可为 `asc`；
可选的 `start_time_ms`（含）与 `end_time_ms`（不含）按 Unix 毫秒限定创建时间，续页请求同样生效。
游标是不含自身的 entry ID 时间边界：传上一页的 `next_cursor` 按同一方向继续，或传任一条目的
`name` 按所请求方向离开该条目。控制台的搜索只列出 `query` 命中的记录；点击一条后，以该条目的
`name` 为游标分别用 `asc` 和 `desc` 读取它前后的记录，在完整时间线中定位并高亮这一条，此后继续用
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

## 诊断助手

诊断助手位于 `web/assistant/`（private workspace `@gizclaw/assistant`），是与 React 无关的
agent 包，设计细节见 `web/assistant/DESIGN.md`。它通过 `@openai/agents-core` 与
`@openai/agents-openai` 的 Chat Completions 模型访问节点的 `/openai/v1`，工具全部在浏览器中
执行，GizClaw 只转发工具声明、调用与结果。

助手的所有出口都经 `AssistantRuntime` 注入：`page`（跳转、打开链接、当前路由）、`view`（当前
页面显示内容的结构化快照）、`fleet`（节点状态与内存中的流量采样）、`devices`（设备查找、状态、
最新与区间 telemetry、Wi-Fi、对话 Workspace 与对话历史）和 `logs`（日志查询）。`src/apis.ts`
是工具声明表：每个工具声明它调用的 runtime 方法、说明、zod 参数和实现，工具由该表生成，测试保证
每个 runtime 方法恰好被一个工具使用。工具只读，设备的重启、音量、Wi-Fi 扫描与修改、删除等写操作
没有对应的 runtime 方法。日志查询按级别、操作、错误码、RPC 状态码与 HTTP 状态聚合，跳转日志页时
由工具根据设备公钥、错误码、级别和关键词拼出控制台查询。出口失败以错误码交给模型，例如
`DEBUG_ACCESS_FORBIDDEN` 时助手说明需要在设备端开启只读调试模式，而不声称已替用户完成。工具结果
（包括对话历史）按不可信数据处理。

验证分两层。`npm test --workspace @gizclaw/assistant` 用覆盖全部出口的 `FakeRuntime` 与脚本化
模型，经真实 Agents SDK runner 跑完场景集，确认调用链路、参数校验、结果回传与跳转。
`tests/gizclaw-e2e/go/openai` 中的 `TestAssistantScenariosWithLiveModel` 用同一场景集和 Docker
栈上 RuntimeProfile 的 `llm`（Volc Ark `doubao-mini-chat`）运行，只断言工具调用、最终路由和回复
中的关键事实，每个场景最多尝试三次，以区分小模型的波动和助手做不到的行为。

### 控制台聊天入口

Monitor console 右下角的聊天按钮打开诊断助手面板；面板代码（agent、模型 client 与
`@assistant-ui/react`）在第一次打开时按需加载，不进入首屏 bundle。

助手使用一台已有设备的 API Key。在 console 配置中加入：

```json
"assistant": {
  "apiKey": "gizclaw_sk_v1_...",
  "model": "llm",
  "endpoint": "https://edge.example.com"
}
```

`apiKey` 必须是 `gizclaw_sk_v1_` 开头的设备 API Key，助手用它经 `/openai/v1` 调用该设备
RuntimeProfile 中的模型；`model` 是 RuntimeProfile 的模型别名，默认 `llm`；`endpoint` 可选，
缺省时与设备 API 相同，为 `deviceEndpoint` 或 console 所在 origin。这把 Key 同时能读取和
控制它所属的设备，因此与 Monitor Token 一样随配置加密保存在浏览器、登出时清除，并随配置导出。
没有 `assistant` 时面板只说明如何配置，不发起任何请求。

每轮对话用该 Key 运行 `@gizclaw/assistant`。console 以现有的 hash 路由、`useFleet`、关注
列表和 `lib/peers` 实现 `AssistantRuntime`：跳转直接改变 `location.hash`，打开外部链接先经
`window.confirm`，跳到设备详情或设备日志时把尚未关注的设备加入关注列表；每个页面用
`usePageView` 发布结构化快照供助手读取，不读取 DOM。控制端错误映射为错误码交给助手，例如
`DEBUG_ACCESS_FORBIDDEN` 与网络错误 `NETWORK_ERROR`。面板逐轮显示回复与每个工具动作，运行中
可以停止；不提供编辑与重新生成。对话历史只保存在面板和助手会话的内存中，不写入浏览器存储，`/openai/v1`
的 Chat Completions 也不在服务端保存对话；"清空对话"会停止当前一轮并开始新会话，刷新页面同样清空。日志页的初始查询带 `peer_public_key:` 时，以该设备为数据源。
