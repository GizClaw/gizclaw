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

## MHS v0 硬件状态迁移 {#mhs-v0-migration}

以下旧接口已弃用，但仍保持原有请求、响应、校验、错误和运行时行为，旧设备与 App 可继续使用：

| 已弃用 RPC | 已弃用 Peer HTTP | MHS v0 替代入口 |
| --- | --- | --- |
| `client.device.volume.set`（101） | `PUT /gizclaw/v1/device/volume` | `client.mhs.v0.write`（134）；`PATCH /gizclaw/v1/device/mhs/v0/states` |
| `client.device.settings.get`（128） | `GET /gizclaw/v1/device/settings` | `client.mhs.v0.read`（133）；`POST /gizclaw/v1/device/mhs/v0/read` |
| `client.device.settings.set`（129） | `PATCH /gizclaw/v1/device/settings` | `client.mhs.v0.write`（134）；`PATCH /gizclaw/v1/device/mhs/v0/states` |

控制 App 先通过 `GET /gizclaw/v1/device/mhs/v0/manifest` 读取设备绑定的 RuntimeProfile `spec.mhs.v0`。`device_id` 与 state 名称由产品在 manifest 中定义；下表是**推荐命名约定**，不是协议固定的 ID，也不会自动为产品添加这些 key。`device_id/state` 表示请求中的两个独立字段。仅对 manifest 声明且设备实现的 key 发起读写；写入还要求 `read_write`。

| 旧字段 | 推荐 MHS v0 key | 类型与推荐约束 |
| --- | --- | --- |
| volume.set `level` | `speaker.main/volume` | `int`，0–100 |
| volume.set `muted` | `speaker.main/muted` | `bool` |
| settings `screen_brightness` | `display.main/brightness` | `int`，0–100 |
| settings `screen_off_timeout_ms` | `display.main/off_timeout_ms` | `int`，毫秒，≥0；0 表示常亮 |
| settings `led_brightness` | `led.status/brightness` | `int`，0–100 |
| settings `cellular_enabled` | `cellular.main/enabled` | `bool` |
| settings `nfc_enabled` | `nfc.main/enabled` | `bool` |
| settings `auto_sleep_timeout_ms` | `power.main/auto_sleep_timeout_ms` | `int`，毫秒，≥0；0 表示不自动休眠 |
| settings `locale` | `system.main/locale` | `string`，保留 BCP 47 语言标签约定 |
| settings `default_interaction_mode` | `system.main/default_interaction_mode` | `enum`：`push-to-talk`、`realtime` |
| settings `key_feedback` | `system.main/key_feedback` | `enum`：`none`、`sound`、`vibrate`、`sound_and_vibrate` |
| settings `alert_mode` | `system.main/alert_mode` | `enum`：`silent`、`vibrate`、`ring` |

MHS enum 沿用旧设置的语义字符串值；RPC 中使用 `MhsValue.string_value`，不传旧 Protobuf enum 的整数或符号名。读取显式列出所需 key，写入只包含需要修改的 key；MHS write 返回本批次实际生效的值，不是完整 `DeviceSettings` 或 `PeerStatus`。未知或设备未实现的 key 返回错误，不沿用旧 settings 对不支持成员的忽略行为。超时后通过 read 确认当前值；完整规则见 [Peer HTTP](./http/public) 和 [设备 provider](./proto/rpc/client-provided-to-server)。

`sound.play`、`find` 是动作，MHS v0 没有 procedures；`reboot`、`factory_reset`、`firmware.update`、Wi-Fi、audioplayer、`run.workspace.set`、tools 和 `rpc.methods.get` 均不弃用。`client.device.status.get` 与 `GET /gizclaw/v1/device/status` 继续提供超出硬件状态范围的设备标识与遥测快照，不由 MHS 替代。

退役计划：只有在设备固件和控制 App 均完成迁移后才移除旧接口；本次变更不设移除日期。
