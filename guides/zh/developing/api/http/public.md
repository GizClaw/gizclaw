# Public API

Public API 是 Server 在 WebRTC connection 建立前后向 Public/Peer caller 暴露的 HTTP contract。它是入口边界，不代表 Peer 领域 service 的全部能力。

Source：`api/http/peer.json`
Go 生成输出：`pkgs/gizclaw/api/peerhttp`

准确的 endpoint、参数、request 和 response 见 [API Reference](/api/)。本页只说明 Public/Peer surface 的设计边界。

`/webrtc/v1/offer` 发生在 Peer connection 建立之前，必须保留 HTTP signaling。建立连接后的 Peer 能力可以使用 reliable HTTP-over-service-stream 或 Peer RPC；选择 transport 时应避免为相同能力维护两套 contract。

Offer 的身份认证由签名 signaling contract 自身完成，不依赖 API Key。Public API 可以复用 `ErrorResponse`、`DeviceInfo` 和 `Runtime` 等真正 shared 类型，但不引用 Admin Resources。

API Key 的鉴权和管理契约见 [Peer HTTP · API Key](../../gizclaw/peer/service/api-keys)。Wi‑Fi 配网（扫描、写入凭据）和设备密码属于设备本地 BLE 通道；其余日常设备能力经 API Key 由 `/gizclaw/v1/device*` 暴露。

## 设备与 Contact surface

`/gizclaw/v1/device*` 与 `/gizclaw/v1/contacts*` 只接受 `Authorization: Bearer <api-key>`。Server 从 Key record 取得不可变的 owner Peer；path、query、header 与 body 都不接受 Peer selector，manager Key 与普通 Key 对这些 route 拥有相同的 owner-scoped 能力。Key 无效或已撤销返回 `401 INVALID_API_KEY`，owner 不是 active Client 或没有 RuntimeProfile binding 返回 `403 API_KEY_OWNER_UNAVAILABLE`，owner pending deletion 返回 `409 PEER_PENDING_DELETION`，validation 与分页错误返回 `400 INVALID_REQUEST`，store 或 service 故障统一返回脱敏的 `500 INTERNAL_ERROR`。

读路径直接投影 authoritative service，不向设备发送 RPC：

- `GET /device` 返回 `DeviceInfo`（name、emoji、`HardwareInfo`、`DeviceIdentifiers`），与 `server.info.get` 同源。
- `GET /device/runtime` 返回 `Runtime`（online、last seen、address、RX/TX），读取不刷新在线状态。
- `GET /device/status` 返回最近一次 authoritative `PeerStatus` snapshot；不提供 `fresh` 参数，`client.device.status.get` 只用于控制响应回写。
- `GET /device/telemetry/latest`、`/device/telemetry`、`/device/telemetry/aggregate` 保留 Admin telemetry 的字段枚举、采样时间、查询边界、排序与 aggregate 语义，只把 Peer 固定为 owner。
- `/contacts` 的 list/create/get/put/delete 使用 `services/social/contact` 的同一 owner-scoped 数据；`{contactName}` 是 owner 作用域内不可变的 `name`，跨 owner 与不存在统一返回 `404 CONTACT_NOT_FOUND`，name 或 phone 冲突返回 `409 CONTACT_ALREADY_EXISTS`。

## 设备控制流程

控制 route 由 Server 转发为 Server→设备 RPC（见 [Client Provided to Server](../proto/rpc/client-provided-to-server)）：

```text
PUT /gizclaw/v1/device/volume { level: 0..100, muted }
  → 解析 API Key owner
  → 查找 owner 的活动连接；无连接 → 409 DEVICE_OFFLINE
  → client.device.volume.set，超时 5s → 504 DEVICE_TIMEOUT
  → 设备返回 PeerStatus → 写入 owner 的 PeerStatus（reported_at 取设备回报时间）
  → 200 { status: PeerStatus }
```

| Route | RPC | 成功响应 |
| --- | --- | --- |
| `PUT /device/volume` | `client.device.volume.set` | `200 { status }` |
| `POST /device/actions/play-sound` `{ sound, duration_ms? }` | `client.device.sound.play` | `204` |
| `POST /device/actions/reboot` `{ delay_ms? }` | `client.device.reboot` | `204` |
| `GET /device/wifi` | `client.wifi.status.get` | `200 DeviceWifiStatus` |
| `GET /device/wifi/saved` | `client.wifi.saved.list` | `200 DeviceWifiSavedList` |
| `DELETE /device/wifi/saved/{ssid}` | `client.wifi.saved.forget` | `204`；未知 ssid → `404 WIFI_NETWORK_NOT_FOUND` |

`sound` 是设备自定义字符串，Server 只检查非空且不超过 32 UTF‑8 bytes，由设备 provider 校验取值；`ssid` 同样限制 32 bytes。设备返回 `INVALID_PARAMS` 映射 `400 DEVICE_REJECTED`，`METHOD_NOT_FOUND`（设备未实现 provider）映射 `501 DEVICE_UNSUPPORTED`，其余 RPC 错误映射脱敏的 `502 DEVICE_ERROR`；响应体只携带稳定 `code` 与脱敏 `message`。同一 owner 的并发控制命令按到达顺序串行转发，不合并、不重放；`reboot` 得到设备确认后，同一连接上的后续控制命令返回 `409 DEVICE_OFFLINE`，直到设备以新连接重连。控制命令不改变 PeerRun、Workspace 或 Agent 状态。

`/server-info` 在连接前返回 authoritative Server 的 `public_key`、软件 `version`、`build_commit` 与 transport 能力。Server identity 仍只由密码学 `public_key` 表达。经过 Edge 时这些构建字段保持 authoritative Server 的值，Edge transport 选择只由 `transport` 说明。
