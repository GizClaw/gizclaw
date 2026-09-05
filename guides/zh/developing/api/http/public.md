# Public API

Public API 是 Server 在 WebRTC connection 建立前后向 Public/Peer caller 暴露的 HTTP contract。它是入口边界，不代表 Peer 领域 service 的全部能力。

Source：`api/http/peer.json`
Go 生成输出：`pkgs/gizclaw/api/peerhttp`

准确的 endpoint、参数、request 和 response 见 [API Reference](/api/)。本页只说明 Public/Peer surface 的设计边界。

`/webrtc/v1/offer` 发生在 Peer connection 建立之前，必须保留 HTTP signaling。建立连接后的 Peer 能力可以使用 reliable HTTP-over-service-stream 或 Peer RPC；选择 transport 时应避免为相同能力维护两套 contract。

Offer 的身份认证由签名 signaling contract 自身完成，不依赖 API Key。Public API 可以复用 `ErrorResponse`、`DeviceInfo` 和 `Runtime` 等真正 shared 类型，但不引用 Admin Resources。

API Key 的鉴权和管理契约见 [Peer HTTP · API Key](../../gizclaw/peer/service/api-keys)。设备首次入网仍由本地 BLE 通道负责；设备在线后可经 API Key 和 `/gizclaw/v1/device*` 扫描或更换 Wi‑Fi。

## 设备与 Contact surface

`/gizclaw/v1/device*` 与 `/gizclaw/v1/contacts*` 支持 `Authorization: Bearer <api-key>`，也支持下述设备调试访问。Server 从 Key record 取得不可变的 owner Peer；Bearer API Key 的 owner 始终由 Key 决定，manager Key 与普通 Key 对这些 route 拥有相同的 owner-scoped 能力。Key 无效或已撤销返回 `401 INVALID_API_KEY`，owner 不是 active Client 或没有 RuntimeProfile binding 返回 `403 API_KEY_OWNER_UNAVAILABLE`，owner pending deletion 返回 `409 PEER_PENDING_DELETION`，validation 与分页错误返回 `400 INVALID_REQUEST`，store 或 service 故障统一返回脱敏的 `500 INTERNAL_ERROR`。

读路径直接投影 authoritative service，不向设备发送 RPC：

- `GET /device` 返回 `DeviceInfo`（name、emoji、`HardwareInfo`、`DeviceIdentifiers`），与 `server.info.get` 同源。
- `GET /device/runtime` 返回 `Runtime`（online、last seen、address、RX/TX），读取不刷新在线状态。
- `GET /device/status` 返回最近一次 authoritative `PeerStatus` snapshot；不提供 `fresh` 参数，`client.device.status.get` 只用于控制响应回写。
- `GET /device/telemetry/{field}/latest`、`/device/telemetry`、`/device/telemetry/aggregate` 保留 Admin telemetry 的字段枚举、采样时间、查询边界、排序与 aggregate 语义，只把 Peer 固定为 owner。
- `GET /device/firmware` 返回 owner 绑定的 Firmware 配置的全部 channel（`stable`、`beta`、`develop`），每个 channel 携带可选的 `description` 与 `package`（`url`、`sha256`、`size`），与 `server.firmware.get` 同源。Channel 选择归调用方：Server 不保存设备当前使用的 channel，本 route 一次返回全部 channel，由调用方自行选择。未绑定 `firmware_id` 或绑定的配置已不存在返回 `404 FIRMWARE_NOT_FOUND`；某个 channel 未配置包时该 slot 省略 `package`，不报错。
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
| `POST /device/actions/firmware-update` `{ channel?, sha256? }` | `client.firmware.update` | `204` |
| `GET /device/wifi` | `client.wifi.status.get` | `200 DeviceWifiStatus` |
| `GET /device/wifi/saved` | `client.wifi.saved.list` | `200 DeviceWifiSavedList` |
| `DELETE /device/wifi/saved/{ssid}` | `client.wifi.saved.forget` | `204`；未知 ssid → `404 WIFI_NETWORK_NOT_FOUND` |
| `POST /device/wifi/scan` `{ timeout_ms? }` | `client.wifi.scan` | `200 { networks }` |
| `PUT /device/wifi` `{ ssid, passphrase? }` | `client.wifi.connect` | `202` |

`firmware-update` 通知设备执行一次 OTA，设备应答后自行下载、校验、写入并重启。`channel` 取自 `GET /device/firmware` 返回的 channel，省略时设备沿用自身的 channel；`sha256` 是调用方看到的目标包摘要，Server 只校验它是 64 位小写 hex，是否与设备解析出的包一致由设备判断，不一致时设备返回 `INVALID_PARAMS`，映射为 `400 DEVICE_REJECTED`。设备当前运行的包由 `PeerStatus.firmware_sha256` 上报，调用方与目标 channel 的 `package.sha256` 比较即可判断是否需要升级。

`sound` 是设备自定义字符串，Server 只检查非空且不超过 32 UTF‑8 bytes，由设备 provider 校验取值；`ssid` 同样限制 32 bytes。扫描 `timeout_ms` 缺省为 8000，并夹取到 1000–15000；它不复用其他控制 route 的 5 秒超时。加入开放网络时省略 `passphrase`，PSK 长度为 8–63 bytes。`202` 只表示设备接受凭据：设备先应答 RPC 再切网，随后必然掉线；掉线期间控制 route 返回 `409 DEVICE_OFFLINE`，客户端在设备重连后轮询 `GET /device/wifi`，以 `ssid` 是否变为目标网络判断成功或回退。密码只经过转发路径，不持久化、不记录日志、不回显。扫描结果由设备提供，Server 在返回前重新校验：最多 32 条，`ssid` 非空且不超过 32 bytes，`bssid` 不超过 17 bytes，`security` 不超过 5 bytes，越界的应答整体按 `502 DEVICE_ERROR` 拒绝而不回显越界值。

设备返回 `INVALID_PARAMS` 映射 `400 DEVICE_REJECTED`，`METHOD_NOT_FOUND`（设备未实现 provider）映射 `501 DEVICE_UNSUPPORTED`，其余 RPC 错误映射脱敏的 `502 DEVICE_ERROR`；响应体只携带稳定 `code` 与脱敏 `message`。同一 owner 的并发控制命令按到达顺序串行转发，不合并、不重放；`reboot`、`firmware.update` 或 `wifi.connect` 得到设备确认后，同一连接上的后续控制命令返回 `409 DEVICE_OFFLINE`，直到设备以新连接重连。控制命令不改变 PeerRun、Workspace 或 Agent 状态。

`/server-info` 在连接前返回 authoritative Server 的 `public_key`、软件 `version`、`build_commit` 与 transport 能力。Server identity 仍只由密码学 `public_key` 表达。经过 Edge 时这些构建字段保持 authoritative Server 的值，Edge transport 选择只由 `transport` 说明。

## 设备调试访问与匿名标识查询

设备通过自身已认证连接调用 `server.runtime.put`，设置 `{ "debug_mode": "readonly" }`。
允许 `off`（默认）、`readonly`、`fullcontrol`，其他值或缺失字段被拒绝。
该设置由设备所属 authoritative Server 持久化到 PeerRunStore 的 `runs` namespace 下的
`by-peer:<pubkey>:debug-mode`，通过 Runtime 的 `debug_mode` 字段读取；不属于 DeviceInfo，
也不通过 `server.info.put` 修改。断线重连保持设置，缺失记录按 off 处理。

设备和联系人 HTTP 接口使用 `Authorization: Bearer gizclaw_pk_<Base58公钥>` 选择调试设备。
公钥必须是 canonical Base58，裸公钥和 `public_key` query 不提供调试授权。
`gizclaw_sk_v1_` API Key 继续走原有鉴权，不会回退为公钥。
Edge 从公钥查询已有 Peer assignment 并代理到配置中的所属 Server；Edge 不读取 DeviceInfo 或调试模式。
所属 Server 每次从 PeerRunStore 读取当前权限：readonly 只允许 GET，fullcontrol 允许设备/联系人接口的读写和控制。
设备仍须为可用的 active Client 且具有 RuntimeProfile binding。
API key 管理、Admin 和 OpenAI 接口不接受公钥调试授权。
关闭模式后拒绝新请求，已开始的请求不被撤销；存储失败时拒绝访问，响应不暴露底层错误。
调试响应使用 `Cache-Control: no-store`。

以下 GET 接口无需任何 Authorization，返回 `{ "public_keys": [...] }`，包括调试关闭的全部匹配设备：

- `/gizclaw/v1/peers/@findBySn/{sn}`
- `/gizclaw/v1/peers/@findByImei/{tac}/{serial}`

无匹配返回空数组，只公开公钥。SN 和 IMEI 均为设备声明的非唯一标识。
IMEI 索引为 `by-imei:<tac>:<serial>:<pubkey>`，按前缀列举并回读设备记录核对，更新和删除只影响对应公钥。
Admin IMEI 查询为 `/peers/@findPubKeysByImei/{tac}/{serial}`，CLI `admin peers resolve-imei` 同样返回公钥列表。

## 音乐点播

以下路径均使用 `/gizclaw/v1` 前缀和现有设备访问授权。set/append 接收 `{ "items": [...] }`，play 接收 `{ "index": 0 }`，mode 接收 `{ "repeat": "all" }`。除 playlist.get 返回 `{ "items": [...], "playlist_revision": 1 }` 外，成功返回 `{ "status": ... }`，HTTP 200。错误沿用设备控制错误映射；append 不自动重试。点播单个 URL 时先设置单项列表，再播放索引 0。完整语义见 [播放器 provider](../proto/rpc/client-provided-to-server#音乐播放器)。

| HTTP | RPC |
| --- | --- |
| `GET /device/audioplayer` | `client.device.audioplayer.get` |
| `GET /device/audioplayer/playlist` | `client.device.audioplayer.playlist.get` |
| `PUT /device/audioplayer/playlist` | `client.device.audioplayer.playlist.set` |
| `POST /device/audioplayer/playlist/append` | `client.device.audioplayer.playlist.append` |
| `POST /device/audioplayer/actions/play` | `client.device.audioplayer.play` |
| `POST /device/audioplayer/actions/stop` | `client.device.audioplayer.stop` |
| `PUT /device/audioplayer/mode` | `client.device.audioplayer.mode.set` |
