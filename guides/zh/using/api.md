# API

GizClaw 为管理端和 Peer 提供两套主要接口：Admin API 用于管理整个 Server，Peer RPC 用于已连接 Peer 调用产品能力。两者都复用经过身份验证的 Giznet Peer connection，不应把 Admin API 当成无鉴权的公网 HTTP 接口。持有设备 API Key 的手机 App 或脚本另外可以通过 Public HTTP 读取和控制绑定设备，见下文 [设备 HTTP API](#设备-http-api)。

## 如何选择

| 接口 | 适用调用方 | Contract | Giznet service | 典型用途 |
| --- | --- | --- | --- | --- |
| Admin API | operator、CLI、管理 UI | OpenAPI 3.0 / HTTP | `0x10`（Admin HTTP） | Peer 管理、声明式资源、Provider 配置、Firmware、Telemetry 与 Server 日志 |
| Peer RPC | 设备、App、SDK | Protobuf RPC | `0x00`（Peer RPC） | 运行状态、Workspace、Workflow、Firmware、社交、玩法和设备能力 |

需要管理跨 Peer 的 Server 资源时使用 Admin API；需要以当前 Peer 的身份读取或操作产品数据时使用 Peer RPC。Edge node 的路由控制使用独立的 Edge RPC service `0x31`，不属于普通 Peer RPC client。

调用前需要持久化调用方自己的 keypair，并知道 Server 接入点与 Server public key。接入点是 `http://` 或 `https://` 基础 URL，例如 `https://ap.gizclaw.com`；裸 `host:port` 仍然可用，并按 `http` 解析。以下示例假定 SDK 已经建立连接：Go 中是已完成 `Dial` 的 `*gizcli.Client`，TypeScript 中是已由 `connectGiznetWebRTCFromEndpoint` 建立的 `RTCPeerConnection`。WebRTC 媒体不会复用接入点的 authority，因为 TLS 接入点所在端口可能不承载 ICE：SDK 从 `/server-info` 的 `endpoint` 字段获取 ICE UDP 地址。私钥、登录 assertion 和会话凭据不得写入日志或提交到仓库。

TypeScript SDK 以 `@gizclaw/gizclaw` 发布到 GitHub Packages。在使用方项目的
`.npmrc` 中加入以下内容，用 `GITHUB_PACKAGES_TOKEN` 提供具有
`read:packages` 权限的 GitHub token，然后运行
`npm install @gizclaw/gizclaw@0.7.2` 安装明确版本：

```ini
@gizclaw:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_PACKAGES_TOKEN}
```

`main` 上只有同时修改可发布 SDK 内容并递增
`sdk/js/gizclaw/package.json` 中稳定 SemVer 的提交才会发布；同一版本不可覆盖。

## Admin API

Admin API 保留 HTTP method、path、header、JSON/YAML body、HTTP status 和 SSE 语义，但请求通过 Admin HTTP service 传输。SDK 使用虚拟 base URL `http://gizclaw` 组装请求；它不是需要 DNS 解析或直接暴露的 Server 地址。

Server 只允许以下身份打开 Admin HTTP service：

- 配置项 `admin-public-key` 指定的 bootstrap admin key；
- 已注册、状态为 active 且角色为 `admin` 的 Peer。

普通设备和 App 不应持有 admin key。日常 operator 操作优先使用 `gizclaw admin` CLI；需要集成管理 UI 或自动化时再使用生成的 Admin client。

### 能力分组

- 声明式资源：`POST /@apply` 与 `/resources/{kind}/{name}`。
- Peer：查询、批准、阻止、刷新、设备信息与 runtime。
- AI 与 Runtime：Credential、Provider Tenant、Model、Voice、Workflow、Workspace、RuntimeProfile 与 RegistrationToken。
- Firmware 与玩法：Firmware channel package 配置、GameDef、PetDef、BadgeDef 和 Peer 玩法数据。
- 运维：Peer telemetry 查询、Server log SSE stream 与 active pending-deletion 查看/重试。

完整 path、参数和 response 以 [`api/http/admin.json`](https://github.com/GizClaw/gizclaw/blob/main/api/http/admin.json) 为准。

### TypeScript

```ts
import {
  createAdminAPIClient,
  listPeers,
} from "@gizclaw/gizclaw/admin";

const admin = createAdminAPIClient(pc);
const peers = await listPeers({
  client: admin,
  responseStyle: "data",
  throwOnError: true,
});
```

`createAdminAPIClient` 把生成的 OpenAPI client 绑定到现有 Peer connection。生成的 operation 函数提供 path、query、body 和 response 类型；对非成功响应，可使用 `throwOnError: true`，或显式检查返回的 `data`、`error` 与底层 `Response`。

### Pending-deletion 运维

先用 `listPendingDeletions` 查看 active task，再把返回的 `source` 与 `deletion_id` 一起传给 `getPendingDeletion` 或 `retryPendingDeletion`。只有 `failed` work 可以 retry；`queued`、`running` 与 `retry_wait` 返回 `409`。物理删除成功后 task 会立即移除，get/retry 返回 `404`，不存在可查询的 completion receipt。Operator response 只包含 `resource_id`、有界 error code/message、status 与时间；owner identity、descriptor、payload、lease token、fingerprint 和原始 backend error 会被刻意省略。

### Go

```go
admin, err := client.ServerAdminClient()
if err != nil {
	return err
}

response, err := admin.ListPeersWithResponse(
	ctx,
	&adminhttp.ListPeersParams{},
)
if err != nil {
	return err
}
if response.JSON200 == nil {
	return fmt.Errorf("list peers: status %d", response.StatusCode())
}
peers := response.JSON200.Items
```

`ServerAdminClient` 返回由 OpenAPI 生成的 `adminhttp.ClientWithResponses`。调用方必须同时处理 transport error 和非预期 HTTP status；只有对应的 typed response 字段非 `nil` 时，才能读取成功 body。

## Peer RPC

Peer RPC 在一条 Peer connection 上发送 Protobuf request、response、error 与 stream frame。每次调用都有 request ID、稳定的 method name 和 method-specific payload。调用方应使用生成的 typed method map 或 Go SDK 方法，不要手写 method number 或自行编解码 payload。

Method payload 内可寻址对象的 identity 统一叫 `name`，引用使用 `<kind>_name`。对象没有独立 Peer alias 时，Server 会把 canonical 或内部 record ID 原样放进 `name`，调用方仍只读取 `name` 字段。`display_name` 是展示文本，`actor_name` 是行为主体；RPC envelope 中的 `id` 仅用于 frame correlation，不是业务对象 identity。

Method name 的前缀表示能力提供方：

- `all.*`：Client 与 Server 都提供的通用能力，例如 `all.ping`。
- `server.*`：Server 提供、Client 调用的产品能力。
- `client.*`：Client 提供、Server 调用的设备能力。
- `runtime.*`：由 runtime contract 拥有的调用。

每个可用 method 的作用见 [RPC API Reference](/references/rpc)。Wire contract、method ID 和 error code 以 [`api/proto/rpc/rpc.proto`](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/rpc.proto) 为准；每个 method 的 request/response message 定义在 `api/proto/rpc/payload/`。

### TypeScript

```ts
import {
  createPeerRPCClient,
  RPC_METHODS,
} from "@gizclaw/gizclaw/rpc";

const rpc = createPeerRPCClient(pc);
const status = await rpc.call(
  RPC_METHODS["server.run.status"],
  {},
  { timeoutMs: 10_000 },
);
```

`createPeerRPCClient` 只接受 Peer RPC method，request 和 response 类型由 `RPC_METHODS` 对应的生成映射约束。固件、图标、音频等同时返回 metadata 与 bytes 的方法使用 `callBinary`。调用可传入 `AbortSignal` 或单次 `timeoutMs`；默认 request timeout 为 30 秒。

### Go

```go
status, err := client.GetServerStatus(ctx, "status-request-1")
if err != nil {
	return err
}
```

Go SDK 把常用 RPC 暴露为 `gizcli.Client` 的 typed 方法。传入的 request ID 应在当前连接的并发调用中保持唯一；`context` 负责取消和截止时间。

## 设备 HTTP API

绑定单一设备的 API Key（见 [API Key](./api-keys)）可以不建立 Peer connection，直接经 Direct Server HTTP（`serve-to-clients=true`）或 Edge HTTPS 访问 `/gizclaw/v1/device*` 与 `/gizclaw/v1/contacts*`。所有请求发送 `Authorization: Bearer <api-key>`；资源与命令始终作用于 Key 绑定的设备，不能指定其他 Peer。

| Route | 作用 |
| --- | --- |
| `GET /gizclaw/v1/device` | 设备 name、emoji、硬件信息与标识 |
| `GET /gizclaw/v1/device/runtime` | 在线状态、最后在线时间与流量 |
| `GET /gizclaw/v1/device/status` | 最近一次上报的电量、充电、音量、静音与 GNSS |
| `GET /gizclaw/v1/device/telemetry/{field}/latest`、`/telemetry`、`/telemetry/aggregate` | 与 Admin telemetry 相同语义的采样查询 |
| `PUT /gizclaw/v1/device/volume` | 设置音量与静音，返回设备实时回报的 status |
| `POST /gizclaw/v1/device/actions/play-sound` | 播放设备自定义提示音 |
| `POST /gizclaw/v1/device/actions/reboot` | 重启设备 |
| `GET /gizclaw/v1/device/firmware` | 设备绑定的 Firmware 配置的全部 channel 与各自的包 |
| `POST /gizclaw/v1/device/actions/firmware-update` | 通知设备执行一次 OTA |
| `GET /gizclaw/v1/device/wifi`、`/wifi/saved`，`DELETE /wifi/saved/{ssid}` | 查询 Wi‑Fi 状态、列出与清理已保存网络 |
| `/gizclaw/v1/contacts`、`/contacts/{contactName}` | 设备联系人的 list/create/get/put/delete |

读取 route 只投影 Server 已有数据，不会唤醒设备；控制 route 经 Server→设备 RPC 实时执行，设备离线返回 `409 DEVICE_OFFLINE`，5 秒无响应返回 `504 DEVICE_TIMEOUT`，设备未实现返回 `501 DEVICE_UNSUPPORTED`。状态变化通过轮询 `GET /device/status` 获取。Wi‑Fi 配网仍由设备本地 BLE 完成。

`GET /device/firmware` 一次返回 `stable`、`beta`、`develop` 三个 channel 及各自的 `package`（`url`、`sha256`、`size`）；Server 不保存设备当前使用的 channel，选哪个由调用方决定，`POST /device/actions/firmware-update` 用 `channel` 指定，省略时设备沿用自身的 channel。要判断是否需要升级，把 `GET /device/status` 的 `firmware_sha256`（设备上报的当前运行包）与目标 channel 的 `package.sha256` 比较；请求里带上同一个 `sha256`，设备解析出不同的包时会拒绝，避免升到与界面显示不同的版本。设备固件太旧、未实现该 RPC 时返回 `501 DEVICE_UNSUPPORTED`，应据此隐藏升级入口，而不是提示升级失败。

```sh
curl -sS -X PUT "$GIZCLAW_URL/gizclaw/v1/device/volume" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"level":35,"muted":false}'
```

TypeScript 使用 `@gizclaw/gizclaw/peerhttp` 生成的 `setDeviceVolume`、`getDeviceStatus`、`listContacts` 等 operation；Go 使用 `peerhttp.ClientWithResponses`（`gizcli.Client.PeerHTTPClient()` 返回同一类型）。完整 path、参数和 response 以 [`api/http/peer.json`](https://github.com/GizClaw/gizclaw/blob/main/api/http/peer.json) 为准。

## 错误处理与连接生命周期

- Admin API 的 HTTP error body 使用稳定的 error code；不要依赖 message 文本做程序分支。
- RPC failure 使用 `RpcStatus`：一个 canonical gRPC `StatusCode`（`google.rpc.Code`，`0`-`16`）、一条 message，以及可选的 `ErrorInfo`，其 `reason` 说明该 code 背后的具体原因。`NOT_FOUND` 是分类，`WORKSPACE_PENDING_DELETION` 是原因。
- transport error、timeout 或 connection close 不代表写操作一定没有生效。重试变更操作前，应先查询 Server 的最终状态。
- TypeScript 会话结束时关闭 `RTCPeerConnection`；Go 会话结束时关闭 `gizcli.Client`。连接断开后应重新连接，不要继续复用旧 client。

接口的设计与生成规则见 [Admin API 开发说明](../developing/api/http/admin) 和 [Peer RPC 开发说明](../developing/api/proto/rpc/overview)。
