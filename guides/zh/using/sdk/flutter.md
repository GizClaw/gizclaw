# Flutter SDK <Badge type="warning" text="WIP" />

GizClaw 提供两个 Dart package，按角色划分：

| Package | 目录 | 角色 | 传输 | 依赖 |
| --- | --- | --- | --- | --- |
| `gizclaw` | `sdk/flutter/gizclaw` | 设备端：让 Flutter App 作为 GizClaw 设备/Peer 接入 | encrypted `/webrtc/v1/offer` signaling 与 WebRTC DataChannel | Flutter、`flutter_webrtc`、`protobuf` |
| `gizclaw_control` | `sdk/flutter/gizclaw_control` | 控制端：用 [API Key](../api-keys) 读取并控制绑定的设备 | HTTPS `/gizclaw/v1/*` | 纯 Dart，仅 `http` |

`gizclaw` 与 C SDK 的 `sdk/c/gizclaw` 对应同一侧能力；`gizclaw_control` 面向 LiteLink 这类手机控制 App，每张设备卡片保存一个 API Key。设备端 client 初始化、平台权限与 RPC 调用的说明仍待补充；本页当前只覆盖 `gizclaw_control`。

## 安装 `gizclaw_control`

两个 package 都是 `publish_to: none`，通过 git 依赖引用仓库路径：

```yaml
dependencies:
  gizclaw_control:
    git:
      url: https://github.com/GizClaw/gizclaw.git
      ref: main
      path: sdk/flutter/gizclaw_control
```

`ref` 可以是分支或仓库 tag；发布 App 时应固定到包含该 package 的 tag。package 不依赖 Flutter，也可用于 Dart CLI 与 server 端。

## 初始化与调用

```dart
import 'package:gizclaw_control/gizclaw_control.dart';

final client = GizClawControlClient(
  baseUrl: Uri.parse('https://ap.gizclaw.com'),
  apiKey: apiKey, // gizclaw_sk_v1_...
);

final status = await client.getDeviceStatus();
final applied = await client.setDeviceVolume(level: 35, muted: false);
print('${status.volume} -> ${applied.status.volume}');

client.close();
```

每个请求都携带 API Key，因此 `baseUrl` 必须是 `https`。只有本地测试部署才用
`allowInsecureTransport: true` 连接明文 `http` 服务端，那会把凭据以明文发送。

`GizClawControlClient` 覆盖 `/gizclaw/v1/*` 的全部 route：

- API Key：`createApiKey`、`listApiKeys`、`getSelfApiKey`、`revokeSelfApiKey`、`getApiKey`、`revokeApiKey`。
- 设备读取：`getDevice`、`getDeviceRuntime`、`getDeviceStatus`、`getDeviceFirmware`、`getDeviceTelemetryLatest`、`queryDeviceTelemetry`、`aggregateDeviceTelemetry`。
- 设备控制：`setDeviceVolume`、`playDeviceSound`、`rebootDevice`、`updateDeviceFirmware`、`getDeviceWifi`、`scanDeviceWifi`、`connectDeviceWifi`、`listDeviceSavedWifi`、`forgetDeviceSavedWifi`。
- Contact：`listContacts`、`createContact`、`getContact`、`putContact`、`deleteContact`。

每个方法发送 `Authorization: Bearer <apiKey>`，返回 contract 对应的不可变 model；`204` route 返回 `Future<void>`。model 忽略未知 JSON 字段；开放式 schema（`PeerStatus`、`DeviceInfo`）额外提供 `raw` 保存完整解码对象。路径参数（`ssid`、`contactName`）由 SDK 做 URL 编码。可选参数 `httpClient` 用于注入或复用 `http.Client`，`timeout` 默认 30 秒。

## 错误处理

所有失败都抛出 `GizClawControlException`，`kind` 与 [API Key](../api-keys#设备读取与控制) 描述的错误契约一一对应：

| `kind` | 触发条件 |
| --- | --- |
| `unauthorized` / `forbidden` / `notFound` | `401` / `403` / `404` |
| `deviceOffline` | `409 DEVICE_OFFLINE` |
| `deviceTimeout` | `504 DEVICE_TIMEOUT` |
| `deviceRejected` | `400 DEVICE_REJECTED` |
| `deviceUnsupported` | `501 DEVICE_UNSUPPORTED` |
| `deviceError` | `502 DEVICE_ERROR` |
| `conflict` / `invalidRequest` / `server` | 其他 `409` / `400` / `5xx` |
| `unexpectedStatus` | 其他非 2xx |
| `malformedResponse` | 2xx 但 body 不符合 contract |
| `network` | DNS、socket、TLS 或超时，没有 HTTP 响应 |

`DEVICE_*` 按响应 body 的 `error.code` 匹配，其余按 HTTP status。异常同时携带 `statusCode`、`code`、`message`、`details` 与 `X-Request-ID` 响应头对应的 `requestId`。

```dart
try {
  await client.playDeviceSound(sound: 'chime');
} on GizClawControlException catch (e) {
  switch (e.kind) {
    case GizClawControlErrorKind.deviceOffline:
      // 提示设备离线，稍后重试。
    case GizClawControlErrorKind.unauthorized:
      // API Key 已撤销，要求用户重新绑定。
    default:
      // 展示 e.message。
  }
}
```
