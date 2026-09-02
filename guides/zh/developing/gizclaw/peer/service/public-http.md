# HTTP Service Entrypoints

`实现文件：peer_service_serve_peer_http.go`

提供普通 Peer Public HTTP 与 Edge Public HTTP，组装 API Key、CORS、OpenAI API、Edge signaling routes 以及 `/gizclaw/v1/device*`、`/gizclaw/v1/contacts*` 设备扩展，并执行 Edge client/signaling Peer 的准入判断。

该文件拥有 HTTP surface composition；API Key 状态属于 `services/system/apikey`，具体 API 行为属于对应领域 service。设备扩展 handler 分布在两个文件：`peer_service_serve_peer_http_device_api.go` 把 `/device`、`/device/runtime`、`/device/status`、`/device/telemetry*` 与 `/contacts*` 适配到 `peerresource.DeviceReads` 和 `services/social/contact`；`peer_service_serve_peer_http_device_control.go` 把 `PUT /device/volume`、`POST /device/actions/*` 与 `/device/wifi*` 经 `deviceController` 转发为 `client.device.*` / `client.wifi.*` RPC，并把设备回报的 `PeerStatus` 写回 `services/runtime/peertelemetry`。

## Owner binding 与 ingress

`/gizclaw/v1/*` 的 Bearer 鉴权在 strict handler 前完成：middleware 解析 API Key principal，校验 owner 是 active Client 且有 RuntimeProfile binding，再把 owner public key 放入 request context（`peerhttp.CallerPublicKey`）。Handler 只从 context 取 owner，任何 route 都不接受 Peer selector；manager Key 与普通 Key 对设备与 Contact route 能力相同。

Direct Server HTTP（`server.go` 的 mux，`serve-to-clients=true` 时开放）、Peer Public HTTP service 与 Edge HTTPS 使用同一 handler；Edge 只按 `/gizclaw/v1/` 前缀转发，不需要感知新 route。CORS 预检允许 `GET`、`POST`、`PUT`、`DELETE` 与 `OPTIONS`。

## Server→设备 RPC 转发

`deviceController` 在 owner 命令锁内查找 owner 的活动连接一次，在同一连接上打开独立 RPC stream，超时 5 秒；同一 owner 的命令按到达顺序串行转发。无连接返回 `409 DEVICE_OFFLINE`，超时返回 `504 DEVICE_TIMEOUT`，设备 `INVALID_PARAMS` 返回 `400 DEVICE_REJECTED`，`METHOD_NOT_FOUND` 返回 `501 DEVICE_UNSUPPORTED`，其余 RPC 错误返回脱敏的 `502 DEVICE_ERROR`。设备确认 `reboot` 后，controller 在释放命令锁前把真正回复的那条连接记为 rebooting；在该连接被替换前，后续命令直接返回 `409 DEVICE_OFFLINE`，而确认期间重连的新连接不会被误判。`volume.set` 的响应在 owner 的 telemetry status lock 下经 `StatusSync.ApplyDeviceStatus` 写入，与 telemetry 上报保持按字段的时间顺序。

浏览器请求携带 `Origin` 时，Direct Server、Peer Public HTTP 与 Edge Public HTTP 都按该实际 origin 返回 CORS 响应，并把 `Origin` 追加到 `Vary` 以隔离缓存；非浏览器请求保持 `*` 兼容。受支持路径的 `OPTIONS` 预检直接返回 `204`，允许 Public HTTP 实际支持的 `GET`、`POST`、`DELETE` 与 `OPTIONS`，以及 API Key、signaling 和 request correlation 使用的 headers。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `servePublic` / `serveEdgePublic` | 在对应 Giznet service 上启动普通或 Edge Public HTTP。 |
| `publicHTTPHandlerWithOptions` | 组装 API Key 管理、设备扩展与 signaling routes，并完成 owner binding。 |
| `deviceController` | 串行转发设备控制命令，映射离线/超时/设备错误，写回 `PeerStatus`。 |
| `deviceReadsForAPIKey` | 为 API Key owner 构造只读的 `peerresource.DeviceReads`。 |
| `allowEdgeClientPeer` | 判断 Peer 是否允许作为 Edge client。 |
| `allowEdgeSignalingPeer` | 判断 Peer 是否允许通过 Edge 发起 signaling。 |
| `setPeerHTTPCORSHeaders` | 设置 Peer HTTP surface 的 CORS headers。 |
