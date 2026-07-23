# api overview

The root directory `api/` is the source of truth for GizClaw’s external agreement and shared data contract. This defines "what the two parties exchange" and does not implement authorization, storage, service lifecycle or domain business.

`pkgs/gizclaw/api/`, JavaScript SDK and C SDK are the generated results of these contracts or adapters that adhere to the wire format, not another source of definition.

## Directory structure

```text
api/
├── http/
│   ├── admin.json              # Admin HTTP surface
│   ├── peer.json               # Public/Peer HTTP and WebRTC signaling surface
│   ├── openai-compat/v1/       # OpenAI-compatible HTTP subset
│   ├── shared.json             # aggregation entry point for truly shared OpenAPI schemas
│   ├── shared/                 # cross-surface or cross-domain DTOs
│   └── resources/              # Resource, owned Spec, and Resource aggregation definitions
└── proto/
    ├── rpc/
    │   ├── rpc.proto           # request, response, error, stream, and method registry
    │   ├── nanopb.options      # C/nanopb generation configuration
    │   └── payload/            # method payloads organized by domain
    └── telemetry/
        └── peer_telemetry.proto # Peer telemetry event wire format
```

## API list

| Name | Provider | Protocol | Design / Reference |
| --- | --- | --- | --- |
| Admin API | Server | HTTP / OpenAPI | [Design](./http/admin) · [API Reference](/api/) |
| Public API | Server | HTTP / OpenAPI | [Design](./http/public) · [API Reference](/api/) |
| OpenAI Compatible API | Server | HTTP / OpenAPI | [Design](./http/openai-compatible) · [API Reference](/api/) |
| Peer RPC | Client, Server, Edge-node | Protobuf RPC over Giznet service stream | [Design](./proto/rpc/overview) · [Methods](/references/rpc) · [Streams](/references/streams#rpc-streams) |
| Peer Events | Client, Server | Protobuf over Peer Event Stream | [Events](/references/events) · [Streams](/references/streams) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [Design](./proto/telemetry) · [Transport](/references/streams#direct-packets) |

## Subdocument

- [HTTP API](./http/): OpenAPI surfaces, Shared, Resources and type ownership.
- [Proto API](./proto/): Peer RPC and Telemetry Protobuf contract.
- [Generation and Change](./generation): Go, JavaScript and C generation links and validation requirements.
