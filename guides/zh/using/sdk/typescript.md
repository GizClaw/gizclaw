# TypeScript SDK <Badge type="warning" text="WIP" />

GizClaw 提供两个 npm package，按角色划分，都发布到 GitHub Packages：

| Package | 目录 | 角色 | 传输 |
| --- | --- | --- | --- |
| `@gizclaw/gizclaw` | `sdk/js/gizclaw` | 设备端：让 Browser/Node 作为 GizClaw 设备/Peer 接入，含 Admin HTTP、RPC、signaling 与 Telemetry | encrypted `/webrtc/v1/offer` signaling 与 WebRTC DataChannel |
| `@gizclaw/gizclaw-control` | `sdk/js/gizclaw-control` | 控制端：用 [API Key](../api-keys) 读取并控制绑定的设备 | HTTPS `/gizclaw/v1/*` |

`@gizclaw/gizclaw` 与 C SDK 的 `sdk/c/gizclaw` 对应同一侧能力。设备端 client 初始化、运行时要求与 RPC 调用的说明仍待补充；本页当前只覆盖 `@gizclaw/gizclaw-control`。

## 安装 `@gizclaw/gizclaw-control`

在项目 `.npmrc` 中把 `@gizclaw` scope 指向 GitHub Packages，然后安装：

```ini
@gizclaw:registry=https://npm.pkg.github.com
```

```sh
npm install @gizclaw/gizclaw-control
```

package 依赖 `@gizclaw/gizclaw` 的 `peerhttp` 入口复用生成的 Public HTTP client，会一并安装。运行时需要 `fetch`、`Request`、`Response` 与 `URL`：Node `^22.13.0 || >=23.5.0` 或现代浏览器。

## 初始化与调用

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey, // gizclaw_sk_v1_...
});

const status = await control.device.getStatus();
const applied = await control.device.setVolume({ level: 35, muted: false });
console.log(status.volume, "->", applied.status.volume);
```

每个请求都携带 API Key，因此 `baseUrl` 必须是 `https`。只有本地测试部署才用
`allowInsecureTransport: true` 连接明文 `http` 服务端，那会把凭据以明文发送。

client 按 route group 组织，方法名与 [Flutter SDK](./flutter) 的 `gizclaw_control` 一一对应：

- `apiKeys`：`create`、`list`、`getSelf`、`revokeSelf`、`get`、`revoke`。
- `device`：`get`、`getRuntime`、`getStatus`、`getTelemetryLatest`、`queryTelemetry`、`aggregateTelemetry`、`setVolume`、`playSound`、`reboot`、`getWifi`、`scanWifi`、`connectWifi`、`listSavedWifi`、`forgetSavedWifi`。
- `contacts`：`list`、`create`、`get`、`put`、`delete`。

request/response 类型直接来自 `@gizclaw/gizclaw/peerhttp` 的生成类型（`PeerStatus`、`DeviceControlStatus`、`Contact` 等），字段名与 wire format 相同。`204` route resolve 为 `void`。`control.client` 暴露已配置 bearer 与 `baseUrl` 的生成 client，可直接传给 `@gizclaw/gizclaw/peerhttp` 的其他函数。可选 `fetch` 参数用于注入自定义或测试用 fetch。

## 错误处理

所有失败都 reject 为 `GizClawControlError`，`kind` 与 [API Key](../api-keys#设备读取与控制) 描述的错误契约一一对应：

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
| `network` | fetch 抛错，没有 HTTP 响应 |

`DEVICE_*` 按响应 body 的 `error.code` 匹配，其余按 HTTP status；`classifyGizClawControlError(status, code)` 单独导出。错误同时携带 `status`、`code`、`details`、`requestId`（`X-Request-ID` 响应头）与 `cause`。

```ts
import { GizClawControlError } from "@gizclaw/gizclaw-control";

try {
  await control.device.playSound({ sound: "chime" });
} catch (error) {
  if (error instanceof GizClawControlError && error.kind === "deviceOffline") {
    // 提示设备离线，稍后重试。
  } else {
    throw error;
  }
}
```
