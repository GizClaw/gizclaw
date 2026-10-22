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
    │   ├── gizclaw_rpc.proto           # request、response、error、stream 与 method registry
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
| Mem0 API | 自托管 Mem0 服务 | HTTP / OpenAPI | [Memory Store](../stores/memory#自托管-mem0-服务) |
| Peer RPC | Client、Server、Edge-node | Protobuf RPC over Giznet service stream | [设计](./proto/rpc/overview) · [Methods](/references/rpc) · [Streams](/references/streams#rpc-streams) |
| Peer Events | Client、Server | Protobuf over Peer Event Stream | [Events](/references/events) · [Streams](/references/streams) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [设计](./proto/telemetry) · [Transport](/references/streams#direct-packets) |

## 子文档

- [HTTP API](./http/)：OpenAPI surfaces、Shared、Resources 与类型所有权。
- [Proto API](./proto/)：Peer RPC 与 Telemetry Protobuf contract。
- [生成与变更](./generation)：Go、JavaScript 与 C 生成链路及验证要求。

Node Monitor API：Server 和 Edge 提供 `api/http/monitor.json` 定义的本进程 HTTP 契约；认证与生成 ownership 见 [Monitor](../monitor)。

## HWD 与设备操作 {#mhs-v0-hwd}

MHS v0 由 GizClaw 定义九种 HWD。`api/proto/rpc/payload/mhs_v0.proto` 规定每种 HWD 的 read 响应消息；display、led、speaker、modem、nfc 还定义 write 请求及实际生效结果。RuntimeProfile 的 `spec.mhs.v0.devices` 只列出实例的 `id` 和 `hwd`，例如两条 LED 用两个 ID 引用同一 HWD。实例数由列表条目数决定，清单不定义字段类型或读写权限。

| HWD | read | write |
| --- | --- | --- |
| wifi、ble、battery、mic | 支持 | 不支持 |
| display、led、speaker、modem、nfc | 支持 | 支持 |

控制 App 从 `GET /gizclaw/v1/device/mhs/v0/manifest` 发现实例，然后通过 `POST /device/mhs/v0/read` 读取一个实例，或通过 `POST /device/mhs/v0/write` 写入一个可写实例。请求包含 `id`、`hwd`；写入另含 HWD 专属 `value` 对象。响应带同一实例及其类型化 `value`。设备的 Wi-Fi 扫描、连接等过程仍使用 tool/v0。写入超时后重新读取确认实际状态。

`tool/v0` 通过 `client.tool.v0.invoke`（135）承载操作。每次调用选择 38 个预定义 `ClientTool` 之一，并携带对应的 Protobuf 请求消息。设备通过 `client.tool.v0.list`（136）只公布实际安装的操作。`client.rpc.methods.list`（137）返回 `RpcMethod` 数字，用于识别协议 family 和版本。控制 App 使用 `GET /gizclaw/v1/device/tool/v0/tools` 与 `POST /gizclaw/v1/device/tool/v0/invoke`；Server 在接触设备前验证有类型的参数。详见 [Peer HTTP](./http/public)、[设备 provider](./proto/rpc/client-provided-to-server) 与 [RPC Reference](/references/rpc)。

`GET /gizclaw/v1/device/status` 读取 Server 保存的标识与遥测快照；实时调用 `device.status.get` 工具可以刷新该快照。

Infrastructure Flow Feed 使用 `api/http/infra.json`，由 `pkgs/infraflow` 提供只读 AP 契约；见 [聚合帧与资源上限](../infra-flow)。

## Modem 与 NFC 实例状态

Modem 的 `enabled` 是用户配置开关，`powered` 是真实供电观测；两者均为 optional。
无法观测电源时不设置 powered，不能从注册或信号推断。原 SIM、注册、制式和信号字段保持兼容。
Modem write 请求携带 enabled，返回 applied 必须含实际 enabled，可同时返回已知的供电状态。

NFC HWD 使用同一 `{id,hwd}` 机制；清单可以声明 `nfc.left`、`nfc.right` 等多个实例。
每实例独立返回 enabled、card_present 和可选 last_read。last_read 是最近一次尝试的缓存，含
`read_at_unix_ms`、可选 UID、technology、data_format、raw_data、result 和 error_code；
失败尝试也更新快照，移开卡不清除它，read 不触发硬件读取。读取时间为正 int64 毫秒，HTTP 的
Protobuf JSON 映射使用十进制字符串。UID 最多 32 bytes，raw_data 最多 1024 bytes，二者 JSON
使用 base64；technology/data_format 最多 32 UTF-8 bytes，error_code 最多 64。
result 为 success 或 failed，failed 必须有错误码，success 不带错误码。last_read 缺失表示从未读卡。
NFC write 只修改 enabled，applied 必须含实际 enabled；不能把缺失读作 false。

GizOS 驱动负责实际 Modem 供电观测、开关生效以及每 NFC 实例的缓存；产品仓库声明 manifest
中的实例并接入驱动。GizClaw 不解析 NFC 业务数据。Forge 通过原 MHS manifest/read/write 接口
发现和访问实例，通过 Runtime/status 的 reported_kv 读取设备自报快照。本仓库交付不修改下游
固件或应用，不代表已通过真实硬件的供电、NFC 或使用限制验收。
