# api 总览

根目录 `api/` 是 GizClaw 对外协议与共享数据 contract 的 source of truth。这里定义“双方交换什么”，不实现 authorization、storage、service lifecycle 或领域业务。

`pkgs/gizclaw/api/`、JavaScript SDK 和 C SDK 是这些 contract 的生成结果或紧贴 wire format 的 adapter，不是另一份定义来源。

## 目录结构

```text
api/
├── embed.go                   # 为运行时 contract consumer 提供嵌入的 Giztest、HTTP 与 Protobuf source filesystem
├── giztest/
│   └── giztest.schema.json     # Giztest 场景文档的跨语言 schema
├── http/
│   ├── admin.json              # Admin HTTP surface
│   ├── peer.json               # Public/Peer HTTP 与 WebRTC signaling surface
│   ├── shared.json             # 真正共享的 OpenAPI schema 聚合入口
│   ├── shared/                 # 跨 surface 或跨领域 DTO
│   └── resources/              # Resource、专属 Spec 与 Resource 聚合定义
└── proto/
    ├── giznet/
    │   ├── admission.proto     # Giznet admission credential
    │   └── nanopb.options      # bounded C strings
    ├── rpc/
    │   ├── rpc.proto           # request、response、error、stream 与 method registry
    │   ├── nanopb.options      # C/nanopb 生成配置
    │   └── payload/            # 按领域划分的 method payload
    └── telemetry/
        └── peer_telemetry.proto # Peer telemetry event wire format
```

## API 列表

| Name | Provider | Protocol | Design / Reference |
| --- | --- | --- | --- |
| Admin API | Server | HTTP / OpenAPI | [设计](./http/admin) · [API Reference](/api/) |
| Public API | Server | HTTP / OpenAPI | [设计](./http/public) · [API Reference](/api/) |
| OpenAI Compatible API | Server | AI Server Shell over HTTP | [设计](./http/openai-compatible) |
| Peer RPC | Client、Server、Edge-node | Protobuf RPC over Giznet service stream | [设计](./proto/rpc/overview) · [Methods](/references/rpc) · [Streams](/references/streams#rpc-streams) |
| Peer Events | Client、Server | Protobuf over Peer Event Stream | [Events](/references/events) · [Streams](/references/streams) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [设计](./proto/telemetry) · [Transport](/references/streams#direct-packets) |

## 子文档

- [HTTP API](./http/)：OpenAPI surfaces、Shared、Resources 与类型所有权。
- [Proto API](./proto/)：Peer RPC 与 Telemetry Protobuf contract。
- [生成与变更](./generation)：Go、JavaScript 与 C 生成链路及验证要求。

Node Monitor API：Server 和 Edge 提供 `api/http/monitor.json` 定义的本进程 HTTP 契约；认证与生成 ownership 见 [Monitor](../monitor)。

## HWD 与设备操作 {#mhs-v0-hwd}

MHS v0 由 GizClaw 定义八种 HWD。`api/proto/rpc/payload/mhs_v0.proto` 规定每种 HWD 的 read 响应消息；display、led、speaker 还定义 write 请求及实际生效结果。RuntimeProfile 的 `spec.mhs.v0.devices` 只列出实例的 `id` 和 `hwd`，例如两条 LED 用两个 ID 引用同一 HWD。实例数由列表条目数决定，清单不定义字段类型或读写权限。

| HWD | read | write |
| --- | --- | --- |
| wifi、ble、modem、battery、mic | 支持 | 不支持 |
| display、led、speaker | 支持 | 支持 |

控制 App 从 `GET /gizclaw/v1/device/mhs/v0/manifest` 发现实例，然后通过 `POST /device/mhs/v0/read` 读取一个实例，或通过 `POST /device/mhs/v0/write` 写入一个可写实例。请求包含 `id`、`hwd`；写入另含 HWD 专属 `value` 对象。响应带同一实例及其类型化 `value`。设备的 Wi-Fi 扫描、连接等过程仍使用 tool/v0。写入超时后重新读取确认实际状态。

`tool/v0` 通过 `client.tool.v0.invoke`（135）承载操作。每次调用选择 21 个预定义 `ClientTool` 之一，并携带对应的 Protobuf 请求消息。设备通过 `client.tool.v0.list`（136）只公布实际安装的操作。`client.rpc.methods.list`（137）返回 `RpcMethod` 数字，用于识别协议 family 和版本。控制 App 使用 `GET /gizclaw/v1/device/tool/v0/tools` 与 `POST /gizclaw/v1/device/tool/v0/invoke`；Server 在接触设备前验证有类型的参数。详见 [Peer HTTP](./http/public)、[设备 provider](./proto/rpc/client-provided-to-server) 与 [RPC Reference](/references/rpc)。

`GET /gizclaw/v1/device/status` 读取 Server 保存的标识与遥测快照；实时调用 `device.status.get` 工具可以刷新该快照。
