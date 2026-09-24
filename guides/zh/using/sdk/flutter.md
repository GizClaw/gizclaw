# Flutter SDK <Badge type="warning" text="WIP" />

GizClaw 提供两个 Dart package，按角色划分：

| Package | 目录 | 角色 | 传输 | 依赖 |
| --- | --- | --- | --- | --- |
| `gizclaw` | `sdk/flutter/gizclaw` | 设备端：让 Flutter App 作为 GizClaw 设备/Peer 接入 | encrypted `/webrtc/v1/offer` signaling 与 WebRTC DataChannel | Flutter、`flutter_webrtc`、`protobuf` |
| `gizclaw_control` | `sdk/flutter/gizclaw_control` | 控制端：用 [API Key](../api-keys) 读取并控制绑定的设备 | HTTPS `/gizclaw/v1/*` | 纯 Dart，仅 `http` |

`gizclaw` 与 C SDK 的 `sdk/c/gizclaw` 对应同一侧能力；`gizclaw_control` 面向 LiteLink 这类手机控制 App，每张设备卡片保存一个 API Key。设备端 client 初始化、平台权限与 RPC 调用的说明仍待补充；本页覆盖 `gizclaw_control` 与设备端握手准入凭证。

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

每个 `vX.Y.Z` GitHub Release 还附带 `flutter-gizclaw-X.Y.Z.tar.gz` 与 `flutter-gizclaw_control-X.Y.Z.tar.gz`。它们是 pub hosted archive，`pubspec.yaml` 的 `version` 等于 Release 版本，供 pub 仓库以普通 hosted 依赖分发；打包与校验规则见[仓库发布](/zh/developing/tooling#仓库发布)。

## 初始化与调用

```dart
import 'package:gizclaw_control/gizclaw_control.dart';

final client = GizClawControlClient(
  baseUrl: Uri.parse('https://ap.gizclaw.com'),
  apiKey: apiKey, // gizclaw_sk_v1_...
);

final status = await client.getDeviceStatus();
final tools = await client.listDeviceTools();
print('${status.volume} -> $tools');

client.close();
```

每个请求都携带 API Key，因此 `baseUrl` 必须是 `https`。只有本地测试部署才用
`allowInsecureTransport: true` 连接明文 `http` 服务端，那会把凭据以明文发送。

`GizClawControlClient` 覆盖 `/gizclaw/v1/*` 的全部 route：

- API Key：`createApiKey`、`listApiKeys`、`getSelfApiKey`、`revokeSelfApiKey`、`getApiKey`、`revokeApiKey`。
- 设备读取：`getDevice`、`getDeviceRuntime`、`getDeviceStatus`、`getDeviceFirmware`、`getDeviceRuntimeProfile`、`getDeviceTelemetryLatest`、`queryDeviceTelemetry`、`aggregateDeviceTelemetry`。
- Workspace：`listDeviceWorkspaces`（可选 `collection`、`workflowName` 过滤）、`deleteDeviceWorkspace`、`listDeviceWorkspaceHistory`、`downloadDeviceHistoryAudio`，以及供自行拉流的播放器使用的 `deviceHistoryAudioUri` 与 `authorizationHeaders`。
- 设备控制：`playDeviceSound`、`findDevice`、`rebootDevice`、`updateDeviceFirmware`、`scanDeviceWifi`、`connectDeviceWifi`、`listDeviceSavedWifi`、`forgetDeviceSavedWifi`、`factoryResetDevice`、`setDeviceRunWorkspace`、`listDeviceTools`、`getMhsManifest`、`readMhsStates`、`writeMhsStates`。
- Contact：`listContacts`、`createContact`、`getContact`、`putContact`、`deleteContact`。
- 好友：`getFriendInviteToken`、`createFriendInviteToken`（可选 `ttl`，1 分钟到 7 天）、`clearFriendInviteToken`、`addFriend`、`listFriends`、`getFriend`、`deleteFriend`。
- 群组：`listFriendGroups`、`createFriendGroup`、`joinFriendGroup`、`getFriendGroup`、`putFriendGroup`、`deleteFriendGroup`（解散）、`leaveFriendGroup`、`getFriendGroupInviteToken`、`createFriendGroupInviteToken`、`clearFriendGroupInviteToken`、`listFriendGroupMembers`、`addFriendGroupMember`、`putFriendGroupMember`、`deleteFriendGroupMember`。`Friend` 与 `FriendGroupMember` 的 `info`（`PeerProfileInfo`）给出对方设备的名字与 emoji；群组以设备自己的群名寻址，角色为 `FriendGroupRole`。

成员列表的每项带可选的 `online`（Server 本地连接状态）与 `last_seen_at`（RFC 3339 UTC；未知或读取失败时省略），add、put、join 返回的成员不带这两个字段。Dart 对应 `online`（`bool?`）与 `lastSeenAt`（`DateTime?`）。

每个方法发送 `Authorization: Bearer <apiKey>`，返回 contract 对应的不可变 model；`202`/`204` route 返回 `Future<void>`。model 忽略未知 JSON 字段；开放式 schema（`PeerStatus`、`DeviceInfo`）额外提供 `raw` 保存完整解码对象。路径参数（`ssid`、`contactName`、`workspaceId`、`historyId`、`friendName`、`friendGroupName`、`memberName`）由 SDK 做 URL 编码。可选参数 `httpClient` 用于注入或复用 `http.Client`，`timeout` 默认 30 秒。

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

## 设备握手准入

`prepareEncryptedGiznetWebRtcOffer(identity, offerSdp, credential: credential)` 接收可选
生成类型 `AdmissionCredential(version: ..., type: ..., value: ...)`。该类型与
`registrationTokenCredential(registrationToken)` helper 均从 `package:gizclaw/gizclaw.dart`
导出。helper 设置 `version: 1`、`type: 'gizclaw.com/registration_token'` 和原始 token value；
具体业务类型留在 GizClaw 层，transport 只执行 protobuf 编码并在 AEAD 内密封。

省略或传 null 保留裸 SDP。在 `connectFlutterGiznetWebRtc` 的 `prepareOffer` callback
中传入结构即可，连接后仍需 `client.register(registrationToken)`。超限或空编码结构
抛出 `ArgumentError`；Dart 显式默认字段在副本中清除，使编码与 Go/JS/C 一致，原消息不变。
字段与编码上限见 [Giznet](../../developing/giznet#signaling-准入凭证)，运营方开关见
[Security Policy](../../developing/gizclaw/server/security-policy)。

内置 type 由导出常量 `registrationTokenCredentialType` 定义，helper 引用该常量。value 最多 512 个 UTF-8 字节；超限在 helper 构造时返回错误或抛出异常。自定义 policy 应使用自己的域名前缀，内置类型保留 `gizclaw.com/` 前缀。

## 设备连接清理

设备退出或重新拨号前使用 `await closeFlutterGiznetWebRtc(peerConnection)`。
它与必需通道关闭后的自动清理共用一次关闭操作，Server block 后也可安全调用。
通过 `peerEventSessionForFlutterGiznetWebRtc(peerConnection)!.events` 的完成通知
观察 event session 终止；重连创建新的 Peer 与 session。旧 Peer 开始关闭后会拒绝
创建新的 RPC 通道。

## MHS v0 硬件状态

设备端在 `GizClawDeviceControlHandlers` 安装 `readMhsStates`/`writeMhsStates`，使用生成的 `ClientMhsV0*` 与 `MhsValue`。控制端调用 `getMhsManifest()`、`readMhsStates(List<MhsStateRef>)`、`writeMhsStates(List<MhsStateValue>)`；`MhsStateValue.value` 是普通 bool/int/double/String，返回写入后的实际值。`MhsState` 的 type/access/min/max/step/enumValues/unit 用于渲染控件。

这是 GizClaw 自有的 MHS-inspired 预标准 v0，不声称官方兼容。manifest 离线可读，每次读写最多 32 个唯一 key；写入必须整批验证且驱动执行安全限制。完整错误与边界见 [Public API](/zh/developing/api/http/public) 和 [provider contract](/zh/developing/api/proto/rpc/client-provided-to-server)。

## tool/v0 过程

设备端只为实际支持的预定义 `ClientTool` 安装 handler；`client.tool.v0.list` 仅返回已安装的子集。控制端的类型化调用统一使用 `client.tool.v0.invoke` RPC。硬件状态由已绑定 RuntimeProfile 的 MHS v0 manifest 声明，并通过 MHS 读写接口访问。
