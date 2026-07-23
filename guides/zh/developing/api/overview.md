# api 总览

根目录 `api/` 是 GizClaw 对外协议与共享数据 contract 的 source of truth。这里定义“双方交换什么”，不实现 authorization、storage、service lifecycle 或领域业务。

`pkgs/gizclaw/api/`、JavaScript SDK 和 C SDK 是这些 contract 的生成结果或紧贴 wire format 的 adapter，不是另一份定义来源。

## 目录结构

```text
api/
├── http/
│   ├── admin.json              # Admin HTTP surface
│   ├── peer.json               # Public/Peer HTTP 与 WebRTC signaling surface
│   ├── openai-compat/v1/       # OpenAI-compatible HTTP subset
│   ├── shared.json             # 真正共享的 OpenAPI schema 聚合入口
│   ├── shared/                 # 跨 surface 或跨领域 DTO
│   └── resources/              # Resource、专属 Spec 与 Resource 聚合定义
└── proto/
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
| OpenAI Compatible API | Server | HTTP / OpenAPI | [设计](./http/openai-compatible) · [API Reference](/api/) |
| Peer RPC | Client、Server、Edge-node | Protobuf RPC over Giznet service stream | [设计](./proto/rpc/overview) · [Methods](/references/rpc) · [Streams](/references/streams#rpc-streams) |
| Peer Events | Client、Server | JSON over Agent Event Stream | [Events](/references/events) · [Streams](/references/streams) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [设计](./proto/telemetry) · [Transport](/references/streams#direct-packets) |

## 子文档

- [HTTP API](./http/)：OpenAPI surfaces、Shared、Resources 与类型所有权。
- [Proto API](./proto/)：Peer RPC 与 Telemetry Protobuf contract。
- [生成与变更](./generation)：Go、JavaScript 与 C 生成链路及验证要求。
