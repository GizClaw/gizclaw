# HTTP Service Entrypoints

源码：[peer_service_serve_peer_http.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_peer_http.go)

提供普通 Peer Public HTTP 与 Edge Public HTTP，组装 API Key、CORS、OpenAI API、Edge signaling routes 以及 `/gizclaw/v1/device*`、`/gizclaw/v1/contacts*`、`/gizclaw/v1/friends*`、`/gizclaw/v1/friend-groups*` 设备扩展，并执行 Edge client/signaling Peer 的准入判断。

Peer HTTP 组合 owner 范围内的资源和设备控制：`peer_service_serve_peer_http_device_api.go` 提供设备、Runtime、状态和联系人的存储读取；`peer_service_serve_peer_http_tool.go` 处理预定义 `tool/v0` 的 list/invoke；`peer_service_serve_peer_http_mhs.go` 处理由 manifest 约束的 `mhs/v0` HWD 读写。`peer_service_serve_peer_http_device_control.go` 集中处理在线检查、owner 串行化及设备错误映射。Workspace、遥测、社交和监控路由调用各自领域 service。Fiber app 启用 `Immutable`，保存的路径参数不会引用被复用的请求缓冲区。

## Owner binding 与 ingress

`peer.json` 中 owner-scoped 路由的 Bearer 鉴权在 strict handler 前完成：middleware 解析 API Key principal，校验 owner 是 active Client 且有 RuntimeProfile binding，再把 owner public key 放入 request context（`peerhttp.CallerPublicKey`）。Handler 只从 context 取 owner，任何 route 都不接受 Peer selector；manager Key 与普通 Key 对设备与 Contact route 能力相同。

`Server` 的普通 HTTP mux、Peer Public HTTP service 与 Edge HTTPS 在设备路由上复用 owner handler；CLI 宿主在直接 listener 前拒绝业务 API。设备路由的 CORS 预检允许 `GET`、`POST`、`PUT`、`DELETE` 与 `OPTIONS`。

Server 普通 HTTP 与可信 Edge HTTP 另行挂载公开只读的精确路径 `/gizclaw/v1/infra/flows`，在设备 API-key middleware 之前分流，不查询 owner。未配置 feed handler 时仍挂载关闭的接口，GET 返回 `503 FLOW_FEED_DISABLED`，OPTIONS 返回 204；不会落入设备鉴权。其 CORS 只允许 GET/OPTIONS，配置与资源边界见 [Infra Flow](../../../infra-flow)。

## Server→设备 RPC 转发

`deviceController` 在 owner 命令锁内查找 owner 的活动连接一次，在同一连接上打开独立 RPC stream，超时 5 秒；同一 owner 的命令按到达顺序串行转发。无连接返回 `409 DEVICE_OFFLINE`，超时返回 `504 DEVICE_TIMEOUT`，设备 `INVALID_PARAMS` 返回 `400 DEVICE_REJECTED`，`METHOD_NOT_FOUND` 返回 `501 DEVICE_UNSUPPORTED`，其余 RPC 错误返回脱敏的 `502 DEVICE_ERROR`。设备确认 `reboot` 后，controller 在释放命令锁前把真正回复的那条连接记为 rebooting；在该连接被替换前，后续命令直接返回 `409 DEVICE_OFFLINE`，而确认期间重连的新连接不会被误判。`volume.set` 的响应在 owner 的 telemetry status lock 下经 `StatusSync.ApplyDeviceStatus` 写入，与 telemetry 上报保持按字段的时间顺序。

浏览器请求携带 `Origin` 时，Direct Server、Peer Public HTTP 与 Edge Public HTTP 都按该实际 origin 返回 CORS 响应，并把 `Origin` 追加到 `Vary` 以隔离缓存；非浏览器请求保持 `*` 兼容。受支持路径的 `OPTIONS` 预检直接返回 `204`，允许 设备 Public HTTP 实际支持的 `GET`、`POST`、`PUT`、`DELETE` 与 `OPTIONS`，以及 API Key、signaling 和 request correlation 使用的 headers。

## MHS v0

`peer_service_serve_peer_http_mhs.go` 提供 manifest/read/write 路由；`peerresource.DeviceReads.MhsManifest` 离线读取当前 owner 绑定，`services/device/mhs` 校验请求与响应，`rpcClient.ReadMhsHwd/WriteMhsHwd` 复用 controller。完整 contract 见 [Public API](/zh/developing/api/http/public#mhs-v0-hwd)。

设备控制接口使用 `mhs/v0` 处理 manifest 声明的 HWD 实例，使用 `tool/v0` 处理预定义过程。两者都在联系设备前验证请求；见 [Public API](/zh/developing/api/http/public#设备控制流程)。
