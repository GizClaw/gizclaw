# Peer HTTP · Side Control

Side Control 允许未注册为 Peer 的 App 或小程序在目标设备明确授权后，通过独立 session 读取设备数据并管理目标设备拥有的联系人。它不会创建第二个 Primary connection，也不会改变目标 Peer 的 runtime identity。

## 授权模型

1. 目标设备的 Primary session 通过 `POST /me/side-control/device-tokens` 创建 5 分钟有效、单次使用的 device token。
2. Controller 使用自己的 `X-Public-Key` 和登录 assertion，通过 `POST /login` 提交 `grant_type=side_control` 与 device token。
3. Server 原子消费 token，并签发绑定 `controller_public_key`、`target_public_key` 和 session ID 的 24 小时 Side Control bearer session。
4. Primary session 可以列出或撤销目标设备的 Side Control sessions。撤销后 bearer 立即失效。

Primary session 只能访问 `/me/*`；Side Control session 只能访问 `/side-control/*`。Side controller 不需要注册为 Peer，Edge 只对显式 Side Control grant 跳过 active Client Peer 准入检查。

## Routes

| Route | 作用 |
| --- | --- |
| `POST /me/side-control/device-tokens` | 创建 device token |
| `DELETE /me/side-control/device-tokens/{tokenId}` | 撤销未消费 token |
| `GET /me/side-control/sessions` | 列出 active Side Control sessions |
| `DELETE /me/side-control/sessions/{sessionId}` | 撤销 session |
| `GET /side-control/info` | 目标设备信息 |
| `GET /side-control/runtime` | 目标 runtime |
| `GET /side-control/status` | 目标当前 status，包括电池与 GNSS |
| `GET /side-control/telemetry/latest` | 最新 Telemetry |
| `GET /side-control/telemetry` | 时间范围 Telemetry |
| `GET /side-control/telemetry/aggregate` | 聚合 Telemetry |
| `/side-control/contacts` | 目标设备拥有的联系人 CRUD |

Telemetry 字段由已有 Peer Telemetry contract 定义，包括 battery、GNSS、network RSSI/signal/connected 与 system runtime 指标。Server 不存储或暴露 SSID、Wi-Fi 扫描、saved network 或配网操作。

## Transport

小程序通过 Edge HTTPS 访问 route。Edge 在内部通过 `ServiceEdgeHTTP` service stream 转发到权威 Server。Server 也在 direct TCP HTTP surface 提供相同 contract；是否允许 client 直接使用 TCP 入口只由 `serve-to-clients` 控制。

设备密码、Wi-Fi、device-token 本地读取和播放声音等 LiteLink 能力继续由设备直连协议拥有，不在 Server HTTP 中重复定义。
