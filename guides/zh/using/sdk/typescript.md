# TypeScript SDK <Badge type="warning" text="WIP" />

GizClaw 提供两个 npm package，按角色划分，都作为 `v*` GitHub Release 资产分发：

| Package | 目录 | 角色 | 传输 |
| --- | --- | --- | --- |
| `@gizclaw/gizclaw` | `sdk/js/gizclaw` | 设备端：让 Browser/Node 作为 GizClaw 设备/Peer 接入，含 Admin HTTP、RPC、signaling 与 Telemetry | encrypted `/webrtc/v1/offer` signaling 与 WebRTC DataChannel |
| `@gizclaw/gizclaw-control` | `sdk/js/gizclaw-control` | 控制端：用 [API Key](../api-keys) 读取并控制绑定的设备 | HTTPS `/gizclaw/v1/*` |

`@gizclaw/gizclaw` 与 C SDK 的 `sdk/c/gizclaw` 对应同一侧能力。设备端 client 初始化、运行时要求与 RPC 调用的说明仍待补充；本页覆盖 `@gizclaw/gizclaw-control` 与设备端握手准入凭证。

## 安装 `@gizclaw/gizclaw-control`

从选定的 [GitHub Release](https://github.com/GizClaw/gizclaw/releases) 下载同一版本的
`npm-gizclaw-<version>.tgz`、`npm-gizclaw-control-<version>.tgz`、
`release-manifest.json` 与 `SHA256SUMS`。按 manifest 核对包名、版本、字节数、SHA-256
和 source commit，并核对 `SHA256SUMS` 中对应文件的摘要后，在使用方项目中安装两个本地包：

```sh
# VERSION 设为已下载的 Release 版本，去掉 tag 开头的 v。
npm install "./npm-gizclaw-${VERSION}.tgz" "./npm-gizclaw-control-${VERSION}.tgz"
```

两个包的版本都等于 Release tag 去掉 `v` 后的版本。control 包通过
`@gizclaw/gizclaw/peerhttp` 复用生成的 Public HTTP client，并精确依赖同一版本的 gizclaw；
同时传入两个 tarball，npm 即可从本地满足这项依赖。只使用设备端 SDK 时安装
`./npm-gizclaw-${VERSION}.tgz` 即可。运行时需要 `fetch`、`Request`、`Response` 与 `URL`：Node `^22.13.0 || >=23.5.0` 或现代浏览器。

## 初始化与调用

```ts
import { createGizClawControlClient } from "@gizclaw/gizclaw-control";

const control = createGizClawControlClient({
  baseUrl: "https://ap.gizclaw.com",
  apiKey, // gizclaw_sk_v1_...
});

const status = await control.device.getStatus();
const tools = await control.device.listTools();
console.log(status.volume, tools.tools);
```

每个请求都携带 API Key，因此 `baseUrl` 必须是 `https`。只有本地测试部署才用
`allowInsecureTransport: true` 连接明文 `http` 服务端，那会把凭据以明文发送。

client 按 route group 组织，方法名与 [Flutter SDK](./flutter) 的 `gizclaw_control` 一一对应：

- `apiKeys`：`create`、`list`、`getSelf`、`revokeSelf`、`get`、`revoke`。
- `device`：`get`、`getRuntime`、`getStatus`、`getTelemetryLatest`、`queryTelemetry`、`aggregateTelemetry`、`setVolume`、`playSound`、`find`、`reboot`、`getWifi`、`scanWifi`、`connectWifi`、`listSavedWifi`、`forgetSavedWifi`、`getSettings`、`updateSettings`、`factoryReset`、`listRpcMethods`、`setRunWorkspace`、`listTools`、`invokeTool`。
- `contacts`：`list`、`create`、`get`、`put`、`delete`。
- `friends`：`getInviteToken`、`createInviteToken`（可选 `{ ttl_seconds }`）、`clearInviteToken`、`add`、`list`、`get`、`delete`。
- `friendGroups`：`list`、`create`、`join`、`get`、`put`、`delete`（解散）、`leave`、`getInviteToken`、`createInviteToken`、`clearInviteToken`、`listMembers`、`addMember`、`putMember`、`deleteMember`。

`listMembers` 的每项带可选的 `online`（Server 本地连接状态）、`last_seen_at`（RFC 3339 UTC；未知或读取失败时省略）与 `in_room`（设备是否正在本 Server 上运行这个群的 Workspace，即是否在群的 SFU Room 里；为 true 时必然在线），add、put、join 返回的成员不带这三个字段。

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

## 设备握手准入

`connectGiznetWebRTCFromEndpoint({ ..., credential })` 与低层
`prepareEncryptedGiznetWebRTCOffer(identity, offerSDP, credential)` 接收
`AdmissionCredential` 对象：`{ version, type, value }`。SDK 使用生成的 protobuf-es codec
编码后放入 AEAD 信封；giznet transport 不解释字段含义。省略 credential 保留裸 SDP。
字段与总编码长度上限见 [Giznet](../../developing/giznet#signaling-准入凭证)。

GizClaw package 导出 `registrationTokenCredential(registrationToken)`，生成
`{ version: 1, type: "gizclaw.com/registration_token", value: registrationToken }`；可直接作为
connection options 的 `credential`。握手通过后仍需调用 `server.register` 完成产品绑定。
超限或空编码结构在请求 server-info 或创建 offer 之前被拒绝，并关闭此次 PeerConnection。
运营方配置见 [Security Policy](../../developing/gizclaw/server/security-policy)。

内置 type 由导出常量 `REGISTRATION_TOKEN_CREDENTIAL_TYPE` 定义，helper 引用该常量。value 最多 512 个 UTF-8 字节；超限在 helper 构造时返回错误或抛出异常。自定义 policy 应使用自己的域名前缀，内置类型保留 `gizclaw.com/` 前缀。

## MHS v0 HWD

设备端在 `deviceControl` 安装 `readMhsHwd` / `writeMhsHwd`。外层 RPC 请求携带 `id`、HWD 和 protobuf `payload`；`CLIENT_HWD_IDS` 及 `encodeClientHwdWriteRequestPayload` / `decodeClientHwdReadResponsePayload` 等生成函数负责按 HWD 编解码。控制端先调用 `getMhsManifest()`，再调用 `readMhsHwd({id:"speaker.main",hwd:"speaker"})` 或 `writeMhsHwd({id:"speaker.main",hwd:"speaker",value:{volume_percent:35}})`。每次只访问一个实例，写入返回实际生效值。

manifest 只声明实例，不自定义字段结构。wifi、ble、modem、battery、mic 只读；display、led、speaker 可写。详见 [Public API](/zh/developing/api/http/public) 与 [provider contract](/zh/developing/api/proto/rpc/client-provided-to-server)。

## tool/v0 过程

设备端只为实际支持的预定义 `ClientTool` 安装 handler；`client.tool.v0.list` 仅返回已安装的子集。控制端的类型化调用统一使用 `client.tool.v0.invoke` RPC。HWD 实例由已绑定 RuntimeProfile 的 MHS v0 manifest 声明，并通过 MHS 读写接口访问。
