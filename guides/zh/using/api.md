# API

GizClaw 为管理端和 Peer 提供两套主要接口：Admin API 用于管理整个 Server，Peer RPC 用于已连接 Peer 调用产品能力。两者都复用经过身份验证的 Giznet Peer connection，不应把 Admin API 当成无鉴权的公网 HTTP 接口。

## 如何选择

| 接口 | 适用调用方 | Contract | Giznet service | 典型用途 |
| --- | --- | --- | --- | --- |
| Admin API | operator、CLI、管理 UI | OpenAPI 3.0 / HTTP | `0x10`（Admin HTTP） | Peer 管理、声明式资源、Provider 配置、Firmware、Telemetry 与 Server 日志 |
| Peer RPC | 设备、App、SDK | Protobuf RPC | `0x00`（Peer RPC） | 运行状态、Workspace、Workflow、Firmware、社交、玩法和设备能力 |

需要管理跨 Peer 的 Server 资源时使用 Admin API；需要以当前 Peer 的身份读取或操作产品数据时使用 Peer RPC。Edge node 的路由控制使用独立的 Edge RPC service `0x31`，不属于普通 Peer RPC client。

调用前需要持久化调用方自己的 keypair，并知道 Server endpoint 与 Server public key。以下示例假定 SDK 已经建立连接：Go 中是已完成 `Dial` 的 `*gizcli.Client`，TypeScript 中是已由 `connectGiznetWebRTCFromEndpoint` 建立的 `RTCPeerConnection`。私钥、登录 assertion 和会话凭据不得写入日志或提交到仓库。

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
- Firmware 与玩法：Firmware、artifact、GameDef、PetDef、BadgeDef 和 Peer 玩法数据。
- 运维：Peer telemetry 查询与 Server log SSE stream。

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

## 错误处理与连接生命周期

- Admin API 的 HTTP error body 使用稳定的 error code；不要依赖 message 文本做程序分支。
- RPC failure 使用 `RpcErrorCode`。标准 code 包括 parse error、invalid request、method not found、invalid params、internal error，以及 `400`、`403`、`404`、`409`。
- transport error、timeout 或 connection close 不代表写操作一定没有生效。重试变更操作前，应先查询 Server 的最终状态。
- TypeScript 会话结束时关闭 `RTCPeerConnection`；Go 会话结束时关闭 `gizcli.Client`。连接断开后应重新连接，不要继续复用旧 client。

接口的设计与生成规则见 [Admin API 开发说明](../developing/api/http/admin) 和 [Peer RPC 开发说明](../developing/api/proto/rpc/overview)。
