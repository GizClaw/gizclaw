# api 总览

根目录 `api/` 是 GizClaw 对外协议与共享数据 contract 的 source of truth。这里定义“双方交换什么”，不实现 authorization、storage、service lifecycle 或领域业务。

`pkgs/gizclaw/api/`、JavaScript SDK 和 C SDK 是这些 contract 的生成结果或紧贴 wire format 的 adapter，不是另一份定义来源。

## 目录结构

```text
api/
├── http/
│   ├── admin.json              # Admin HTTP surface
│   ├── public.json             # Public/Peer HTTP 与 WebRTC signaling surface
│   ├── openai-compat/v1/       # OpenAI-compatible HTTP subset
│   ├── shared.json             # 真正共享的 OpenAPI schema 聚合入口
│   ├── shared/                 # 跨 surface 或跨领域 DTO
│   ├── resources.json          # Admin Resource 聚合入口
│   └── resources/              # Resource 与其专属 Spec
└── proto/
    ├── rpc/
    │   ├── rpc.proto           # request、response、error、stream 与 method registry
    │   ├── nanopb.options      # C/nanopb 生成配置
    │   └── payload/            # 按领域划分的 method payload
    └── telemetry/
        └── peer_telemetry.proto # Peer telemetry event wire format
```

## API 列表

| Name | Provider | Protocol | Link |
| --- | --- | --- | --- |
| Admin API | Server | HTTP / OpenAPI | [GOTO](./http/admin) |
| Public API | Server | HTTP / OpenAPI | [GOTO](./http/public) |
| OpenAI Compatible API | Server | HTTP / OpenAPI | [GOTO](./http/openai-compatible) |
| Peer RPC | Client、Server、Edge-node | Protobuf RPC over Giznet service stream | [GOTO](./proto/rpc/overview) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [GOTO](./proto/telemetry) |

## 子文档

- [HTTP API](./http/)：OpenAPI surfaces、Shared、Resources 与类型所有权。
- [Proto API](./proto/)：Peer RPC 与 Telemetry Protobuf contract。
- [生成与变更](./generation)：Go、JavaScript 与 C 生成链路及验证要求。
