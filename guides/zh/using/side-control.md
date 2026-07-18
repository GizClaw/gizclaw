# Side Control

Side Control 用于让 App 或小程序在设备明确授权后读取该设备的状态、Telemetry，并管理设备联系人。Controller 使用自己的 keypair，不复制设备私钥，也不建立第二条 Primary connection。

## 接入流程

1. 设备通过 Primary session 调用 `POST /me/side-control/device-tokens`。
2. 设备将 Public API endpoint、Server public key、返回的 token 与 expiry 编码到本地定义的二维码中。GizClaw 不规定二维码 UI 或封装格式。
3. Controller 扫码后生成或读取自己的 keypair，并为 Server public key 创建登录 assertion。
4. Controller 调用 `POST /login`，提交 `grant_type: side_control` 与 `device_token`，同时发送自己的 `X-Public-Key` 和 assertion。
5. Controller 使用返回的 bearer session 访问 `/side-control/*`。生产小程序访问 Edge HTTPS；Edge 通过 `ServiceEdgeHTTP` 转发到权威 Server。
6. 设备通过 `GET /me/side-control/sessions` 查看授权，并用 `DELETE /me/side-control/sessions/{sessionId}` 撤销。撤销立即使 controller bearer 失效。

Device token 只能使用一次并在五分钟后过期。Side Control session 最长有效 24 小时。App 不应记录 device token、assertion、bearer 或私钥。

## 可用能力

- `/side-control/info`、`runtime` 与 `status`：设备信息、在线状态、电池、GNSS、音量与静音状态。
- `/side-control/telemetry/*`：battery、GNSS、network RSSI/signal/connected 和 system 指标。
- `/side-control/contacts`：目标设备拥有的联系人 CRUD。

Wi-Fi、设备密码和播放声音仍通过 LiteLink 的设备直连能力完成。Side Control Public API 不提供这些 route，也不返回 SSID、saved network 或 Wi-Fi credential。
