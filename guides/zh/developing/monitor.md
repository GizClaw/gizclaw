# Monitor

Monitor 前端位于 `web/monitor/`，使用 React、TypeScript、Vite、shadcn 风格组件和 Recharts。设计约束见该目录的 `DESIGN.md`。页面不访问外部 CDN，构建结果由 Go embed 加载。

## 入口和权限

Server 与 Edge 在现有 HTTP/HTTPS listener 上提供 `/monitor/node` 和 `/monitor/peer`，不新增仅监控 listener。

节点数据为当前进程的快照，GET `/monitor/api/node` 使用独立的 `Authorization: Bearer gizclaw_mk_...`。在节点配置文件中设置：

```yaml
monitor:
  token: ${GIZCLAW_MONITOR_TOKEN}
```

Token 前缀为 `gizclaw_mk_`，其后至少 32 个字符。建议用 `openssl rand -hex 32` 生成随机部分。空配置关闭节点数据接口（503），非法 Token 返回 401，成功和失败响应均为 no-store。网页只在内存保存凭证；刷新或退出后重新输入。公网入口使用已有 TLS 配置。

设备页通过 Edge 的现有 Public API 查询 SN/IMEI，并使用 `gizclaw_pk_` Bearer 访问选定设备。Server 的普通业务 HTTP 入口仍返回 PRIVATE_INGRESS_DENIED，因此设备监控应从 Edge 打开。readonly 允许读取，fullcontrol 才显示可执行的音量和重启操作；Server 每次重新校验权限。

## 数据语义

节点连接数是本进程 WebRTC association 数，包括上游连接；service stream 数独立统计。RX/TX 是进程启动以来的 WebRTC service payload 字节，不含 ICE、DTLS 等协议开销。设备计数来自所属 Server 的 Runtime。曲线每次请求完成一秒后继续采样，保留 120 次采样；连接计数重置时不产生负速率。

节点日志只展示本进程最近 500 条结构化日志。`/gizclaw/v1/device/logs` 只返回其中 peer_public_key 精确匹配授权设备的记录，最多 500 条，单条消息最多 4096 字节。日志不持久化，不代表固件串口日志。界面提供级别/文本筛选、自动跟随和虚拟滚动。

设备配置面板显示设备上报的 info、runtime 和 status；Telemetry 面板显示服务器已收到的最新采样，不声称能读取任意固件配置。请求失败会清空可操作数据并显示错误。

## 构建和验证

```sh
npm ci
npm run build:monitor
npm test --workspace @gizclaw/monitor
go build ./cmd/gizclaw
```

`dist/` 中的生成资源不提交，保留 `.keep` 以便纯 Go 测试在未构建 UI 时仍可编译。未构建 UI 的二进制访问页面返回明确的 503。Linux Docker 构建及 macOS release 流程会先构建 UI，然后编译 Go；本地修改 UI 后也需要重新编译使用 embed 的进程。

开发可运行 `npm run dev --workspace @gizclaw/monitor`，默认将 API 转发至本地 9821 Edge。前端请求使用生成的 Peer HTTP client，外部 JSON 经过运行时校验；凭证不放入 URL 或浏览器持久存储。

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
