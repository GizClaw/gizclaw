# API Key

GizClaw 使用长期有效、绑定设备的 API Key 访问公开的 GizClaw API 和 OpenAI 兼容 HTTP API。完成注册的设备通过已认证的 Peer RPC 连接调用 `server.api_key.create`、`server.api_key.list` 和 `server.api_key.revoke` 管理 Key；该连接是根权限入口。API Key 是可恢复的管理资源：create、list、get 和 self 响应都包含完整的 `gizclaw_sk_v1_...` credential。

访问 `/gizclaw/v1/*` 和 `/openai/v1/*` 时发送 `Authorization: Bearer <api-key>`，不再需要 public-key header 或 login 交换。

普通 Key 可以使用公开 API，通过 `GET /gizclaw/v1/api-keys/self` 查看自己，并通过 `DELETE /gizclaw/v1/api-keys/self` 撤销自己。带 `manage_api_keys: true` 的 Key 还可以通过 `/gizclaw/v1/api-keys` 创建、列举、查看和撤销同一设备的其他 Key。`manage_api_keys` 只控制 Key 管理；任何 Key 都能读取和控制它绑定的设备，能力完全相同。

## 设备读取与控制

Key 绑定的设备是所有 `/gizclaw/v1/device*`、`/gizclaw/v1/contacts*`、`/gizclaw/v1/friends*` 与 `/gizclaw/v1/friend-groups*` 请求的固定目标（route 列表见 [API](./api#设备-http-api)）。读取设备状态和写入硬件状态：

```sh
curl -sS "$GIZCLAW_URL/gizclaw/v1/device/status" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY"

curl -sS -X PATCH "$GIZCLAW_URL/gizclaw/v1/device/mhs/v0/states" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"states":[{"device_id":"speaker.main","state":"volume","value":35}]}'

curl -sS -X POST "$GIZCLAW_URL/gizclaw/v1/device/tool/v0/invoke" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"tool":"wifi.scan","args":{"timeout_ms":8000}}'

curl -sS -X POST "$GIZCLAW_URL/gizclaw/v1/device/tool/v0/invoke" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"tool":"wifi.connect","args":{"ssid":"Office","passphrase":"correct-horse"}}'

```

有效的 MHS 写入返回实际状态。`tool/v0` 调用返回类型化的 `result`；`wifi.scan` 返回附近网络，`wifi.connect` 在切换网络前确认接收。展示控制入口前读取 manifest 和设备已安装的工具列表。Server 不保存、记录或回显 Wi-Fi 密码。

设备不在线时控制 route 返回 `409 DEVICE_OFFLINE`，普通控制在 5 秒内无响应、扫描在其请求上界内无响应返回 `504 DEVICE_TIMEOUT`，两者都不会改变已存储的 status；`reboot` 得到确认后，设备重连前的控制请求同样返回 `409`。设备拒绝参数返回 `400 DEVICE_REJECTED`，设备固件未实现对应能力返回 `501 DEVICE_UNSUPPORTED`。Key 被撤销或设备 Peer 被删除后，所有设备与 Contact 请求立即失败。

## SDK

控制端 SDK 封装了同样的 route 与错误契约：Dart 的 `gizclaw_control`（[Flutter SDK](./sdk/flutter)）和 npm 的 `@gizclaw/gizclaw-control`（[TypeScript SDK](./sdk/typescript)）。读取设备状态并设置音量：

```dart
import 'package:gizclaw_control/gizclaw_control.dart';

final client = GizClawControlClient(
  baseUrl: Uri.parse('https://ap.gizclaw.com'),
  apiKey: apiKey,
);
final status = await client.getDeviceStatus();
final applied = await client.setDeviceVolume(level: 35, muted: false);
final nearby = await client.scanDeviceWifi();
await client.connectDeviceWifi(
  const DeviceWifiConnectRequest(ssid: 'Office', passphrase: 'correct-horse'),
);
```

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey,
});
const status = await control.device.getStatus();
const applied = await control.device.setVolume({ level: 35, muted: false });
const nearby = await control.device.scanWifi();
await control.device.connectWifi({ ssid: "Office", passphrase: "correct-horse" });
```

失败分别抛出 `GizClawControlException` 与 `GizClawControlError`，其 `kind` 按上文的 status 与 `DEVICE_*` code 分类，例如 `deviceOffline`、`deviceTimeout`、`unauthorized`。

API Key 不会自动过期；Key 丢失或不再使用时应主动撤销。删除 Peer 会撤销该 Peer 拥有的全部 API Key。
