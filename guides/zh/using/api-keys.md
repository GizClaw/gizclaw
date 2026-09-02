# API Key

GizClaw 使用长期有效、绑定设备的 API Key 访问公开的 GizClaw API 和 OpenAI 兼容 HTTP API。完成注册的设备通过已认证的 Peer RPC 连接调用 `server.api_key.create`、`server.api_key.list` 和 `server.api_key.revoke` 管理 Key；该连接是根权限入口。API Key 是可恢复的管理资源：create、list、get 和 self 响应都包含完整的 `gizclaw_sk_v1_...` credential。

访问 `/gizclaw/v1/*` 和 `/openai/v1/*` 时发送 `Authorization: Bearer <api-key>`，不再需要 public-key header 或 login 交换。

普通 Key 可以使用公开 API，通过 `GET /gizclaw/v1/api-keys/self` 查看自己，并通过 `DELETE /gizclaw/v1/api-keys/self` 撤销自己。带 `manage_api_keys: true` 的 Key 还可以通过 `/gizclaw/v1/api-keys` 创建、列举、查看和撤销同一设备的其他 Key。`manage_api_keys` 只控制 Key 管理；任何 Key 都能读取和控制它绑定的设备，能力完全相同。

## 设备读取与控制

Key 绑定的设备是所有 `/gizclaw/v1/device*` 与 `/gizclaw/v1/contacts*` 请求的固定目标（route 列表见 [API](./api#设备-http-api)）。读取设备状态和设置音量：

```sh
curl -sS "$GIZCLAW_URL/gizclaw/v1/device/status" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY"

curl -sS -X PUT "$GIZCLAW_URL/gizclaw/v1/device/volume" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"level":35,"muted":false}'
```

`PUT /device/volume` 成功返回 `200 { "status": PeerStatus }`，其中的 `volume`、`muted` 与随后 `GET /device/status` 读到的一致；`play-sound`、`reboot` 与 `DELETE /wifi/saved/{ssid}` 成功返回 `204`。设备不在线时控制 route 返回 `409 DEVICE_OFFLINE`，设备 5 秒内无响应返回 `504 DEVICE_TIMEOUT`，两者都不会改变已存储的 status；`reboot` 得到确认后，设备重连前的控制请求同样返回 `409`。设备拒绝参数返回 `400 DEVICE_REJECTED`，设备固件未实现对应能力返回 `501 DEVICE_UNSUPPORTED`。Key 被撤销或设备 Peer 被删除后，所有设备与 Contact 请求立即失败。

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
```

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey,
});
const status = await control.device.getStatus();
const applied = await control.device.setVolume({ level: 35, muted: false });
```

失败分别抛出 `GizClawControlException` 与 `GizClawControlError`，其 `kind` 按上文的 status 与 `DEVICE_*` code 分类，例如 `deviceOffline`、`deviceTimeout`、`unauthorized`。

API Key 不会自动过期；Key 丢失或不再使用时应主动撤销。删除 Peer 会撤销该 Peer 拥有的全部 API Key。
