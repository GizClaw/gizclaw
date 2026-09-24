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

## 设备状态与操作 {#mhs-v0-migration}

设备提供 MHS v0 硬件状态和预定义的 tool/v0 操作。绑定的 RuntimeProfile `spec.mhs.v0` manifest 声明产品自定的 `(device_id, state)` key。控制 App 先调用 `GET /gizclaw/v1/device/mhs/v0/manifest`，再通过 `POST /gizclaw/v1/device/mhs/v0/read` 或 `PATCH /gizclaw/v1/device/mhs/v0/states` 读写。下表只是命名建议，不会自动安装协议 ID；只能读取 manifest 声明且设备实现的 key，写入还需要 `read_write`。

| 状态示例 | 推荐 MHS v0 key | 类型与推荐约束 |
| --- | --- | --- |
| 扬声器音量 | `speaker.main/volume` | `int`，0–100 |
| 扬声器静音 | `speaker.main/muted` | `bool` |
| 设备 `screen_brightness` | `display.main/brightness` | `int`，0–100 |
| 设备 `screen_off_timeout_ms` | `display.main/off_timeout_ms` | `int`，毫秒，≥0；0 表示常亮 |
| 设备 `led_brightness` | `led.status/brightness` | `int`，0–100 |
| 设备 `cellular_enabled` | `cellular.main/enabled` | `bool` |
| 设备 `nfc_enabled` | `nfc.main/enabled` | `bool` |
| 设备 `auto_sleep_timeout_ms` | `power.main/auto_sleep_timeout_ms` | `int`，毫秒，≥0；0 表示不自动休眠 |
| 设备 `locale` | `system.main/locale` | `string`，保留 BCP 47 语言标签约定 |
| 设备 `default_interaction_mode` | `system.main/default_interaction_mode` | `enum`：`push-to-talk`、`realtime` |
| 设备 `key_feedback` | `system.main/key_feedback` | `enum`：`none`、`sound`、`vibrate`、`sound_and_vibrate` |
| 设备 `alert_mode` | `system.main/alert_mode` | `enum`：`silent`、`vibrate`、`ring` |

MHS enum 使用 `MhsValue.string_value` 中的语义字符串。读取显式列出所需 key；写入只包含要修改的 key 并返回实际生效的值。设备必须先整批校验再应用；未知或未实现的 key 返回错误。超时后应重新读取以确认状态。

`tool/v0` 通过 `client.tool.v0.invoke`（135）承载操作。每次调用选择 21 个预定义 `ClientTool` 之一，并携带对应的 Protobuf 请求消息。设备通过 `client.tool.v0.list`（136）只公布实际安装的操作。`client.rpc.methods.list`（137）返回 `RpcMethod` 数字，用于识别协议 family 和版本。控制 App 使用 `GET /gizclaw/v1/device/tool/v0/tools` 与 `POST /gizclaw/v1/device/tool/v0/invoke`；Server 在接触设备前验证有类型的参数。详见 [Peer HTTP](./http/public)、[设备 provider](./proto/rpc/client-provided-to-server) 与 [RPC Reference](/references/rpc)。

`GET /gizclaw/v1/device/status` 读取 Server 保存的标识与遥测快照；实时调用 `device.status.get` 工具可以刷新该快照。
