# Monitor

Monitor 前端位于 `web/monitor/`，使用 React、TypeScript、Vite、shadcn 风格组件和 Recharts。设计约束见该目录的 `DESIGN.md`。应用代码不访问外部 CDN，构建结果由 Go embed 加载；定位地图使用外部 OpenStreetMap iframe。

## 入口和权限

Server 与 Edge 在现有 HTTP/HTTPS listener 上提供 `/monitor/node` 和 `/monitor/peer`，不新增仅监控 listener。

节点数据为当前进程的快照，GET `/monitor/api/node` 使用独立的 `Authorization: Bearer gizclaw_mk_...`。在节点配置文件中设置：

```yaml
monitor:
  token: ${GIZCLAW_MONITOR_TOKEN}
```

Token 前缀为 `gizclaw_mk_`，其后至少 32 个字符。建议用 `openssl rand -hex 32` 生成随机部分。空配置关闭节点数据接口（503），非法 Token 返回 401，成功和失败响应均为 no-store。网页将连接信息用不可导出的 Web Crypto 密钥加密后存入同源 IndexedDB，切页和刷新后恢复；“退出并清除”删除记录。此存储不是操作系统钥匙串，同源脚本仍能使用密钥；不持久化 Telemetry 或日志。保存需要 HTTPS 或 localhost，不支持时仍可临时连接并显示提示。公网入口使用已有 TLS 配置。

设备页通过 Edge 的现有 Public API 查询 SN/IMEI，并使用 `gizclaw_pk_` Bearer 访问选定设备。Server 的普通业务 HTTP 入口仍返回 PRIVATE_INGRESS_DENIED，因此设备监控应从 Edge 打开。readonly 允许读取，fullcontrol 才显示可执行的音量和重启操作；Server 每次重新校验权限。

## 数据语义

节点连接数是本进程 WebRTC association 数，包括上游连接；service stream 数独立统计。RX/TX 是进程启动以来的 WebRTC service payload 字节，不含 ICE、DTLS 等协议开销。设备计数来自所属 Server 的 Runtime。曲线每次请求完成一秒后继续采样，最多保留 1800 次采样，支持最近 2、10、30 分钟窗口；设备上行对应 Server RX，下行对应 Server TX。暂停后恢复会重新采样，连接计数重置时不产生负速率。

节点日志只展示本进程最近 500 条结构化日志。`/gizclaw/v1/device/logs` 只返回其中 peer_public_key 精确匹配授权设备的记录，最多 500 条，单条消息最多 4096 字节。日志不持久化，不代表固件串口日志。节点界面提供级别/文本筛选、自动跟随和虚拟滚动。设备运行日志使用下面的持久化查询，不依赖此缓冲区。

设备配置面板显示设备上报的 info、runtime 和 status；Telemetry 面板显示服务器已收到的最新采样，不声称能读取任意固件配置。请求失败会清空可操作数据并显示错误。

## 构建和验证

```sh
npm ci
npm run build:monitor
npm test --workspace @gizclaw/monitor
go build ./cmd/gizclaw
```

`dist/` 中的生成资源不提交，保留 `.keep` 以便纯 Go 测试在未构建 UI 时仍可编译。未构建 UI 的二进制访问页面返回明确的 503。Linux Docker 构建及 macOS release 流程会先构建 UI，然后编译 Go；本地修改 UI 后也需要重新编译使用 embed 的进程。

开发可运行 `npm run dev --workspace @gizclaw/monitor`，默认将 API 转发至本地 9821 Edge。前端请求使用生成的 Peer HTTP client，外部 JSON 经过运行时校验；凭证不放入 URL；设置 `MONITOR_PROXY` 可改变开发代理目标。

## HTTP 契约和验证

`api/http/monitor.json` 拥有独立的 Monitor OpenAPI surface。Server 和 Edge 在已有 HTTP listener 上挂载 `pkgs/monitor.Handler`，Token middleware 先执行认证，然后进入生成的标准库 router；`nodeServer` 实现生成的 strict interface。控制台调用生成的 JavaScript client。该接口只读取本进程，不使用 Peer assignment 或 Admin 认证。

| 状态码 | 响应 |
| --- | --- |
| 200 | 生成的 `NodeSnapshot`，包含本地计数和有界日志 |
| 401 | `{"error":"INVALID_MONITOR_TOKEN"}` |
| 503 | 未配置 Token 时返回 `{"error":"MONITOR_DISABLED"}` |
| 405 | 不支持的方法返回空 body 和 `Allow: GET` |

所有节点 API 响应都有 `Cache-Control: no-store`。执行 `go generate ./pkgs/monitor/api` 生成 JSON 成功/错误类型及 Go strict server/client；执行 `npm --prefix sdk/js run gen:sdk` 生成 JavaScript client。配置文件与已提交输出路径见 [API 生成](api/generation)。

`npm test --workspace @gizclaw/monitor -- polling.test.tsx` 使用确定性的 API stub 挂载实际 React 应用，验证正常采样、失败时清空图表和时间，以及恢复后的新采样。`go test ./pkgs/monitor -run TestGeneratedMonitorClientContract` 使用生成的客户端验证实际 HTTP handler 的 200/401/503 响应和 405 方法边界。

## 设备调试工作台

- Telemetry 按电源、网络、系统、定位分组，显示数值、单位、采样时间和当前页面采集的趋势；未知字段仍可见。未上报不同于零，原始数据可折叠查看。
- 定位使用最后上报的 GNSS 经纬度，不使用浏览器定位。非法或缺失坐标不加载地图；显示海拔、精度和采样时间。打开定位页且收到有效坐标时自动加载地图，不额外要求确认。iframe 仅允许 `https://www.openstreetmap.org`，CSP 的 `frame-src` 明确限定该来源，`frame-ancestors 'none'` 保持禁止其他页面嵌入控制台。地图服务会收到视窗与坐标。浏览器离线时不创建 iframe，仍显示最后上报数值；地图服务被阻断时，页面保留“加载失败时查看数值”的提示。
- `GET /gizclaw/v1/device/workspaces` 只列出该 Peer 明确拥有的 Workspace，包括系统 Workspace，按 Workflow 分组。共享和无 owner 的空间不返回。
- `GET /gizclaw/v1/device/workspaces/{workspaceId}/history` 从持久化 History 查询文本和游标分页；每页最多 200 条，页面使用 100 条。游标为返回的历史 entry ID 时间边界，格式非法返回 400 `INVALID_HISTORY_CURSOR`；边界本身不授予访问其他 Workspace 的权限。浏览历史不启动 Agent。音频通过同路径下 `/{historyId}/audio.ogg` 认证读取，支持已保存的 Ogg 资产。
- `GET /gizclaw/v1/device/logs/search` 查询配置的 `services.system_log.query_store`，支持正数 Unix 毫秒时间区间、最长 512 UTF-8 字节的文本、严格 `DEBUG|INFO|WARN|ERROR` 级别和游标；每页最多 500 条，页面使用 200 条。Server 强制绑定授权 Peer，续页不能改用其他 Peer 的游标。非法级别返回 400 `INVALID_REQUEST`，非法/跨设备日志游标分别返回 400 `INVALID_LOG_CURSOR` / `LOG_CURSOR_MISMATCH`，未配置查询 Store 返回 500 `LOG_QUERY_NOT_CONFIGURED`。单行日志显示时间、级别、模块、消息和错误，支持横向滚动。

上述接口走现有 Edge 路由与 Server runtime 权限校验，readonly 可读取；不把工作流 spec 或 provider credentials 暴露给浏览器。API 定义位于 `api/http/peer.json`，Go 和 JavaScript client 随 Schema 生成。
