# api overview

The root directory `api/` is the source of truth for GizClaw’s external agreement and shared data contract. This defines "what the two parties exchange" and does not implement authorization, storage, service lifecycle or domain business.

`pkgs/gizclaw/api/`, JavaScript SDK and C SDK are the generated results of these contracts or adapters that adhere to the wire format, not another source of definition.

## Directory structure

```text
api/
├── embed.go                   # embedded Giztest, HTTP, and protobuf source filesystem for runtime contract consumers
├── giztest/
│   └── giztest.schema.json     # Cross-language schema for Giztest scenario documents
├── http/
│   ├── admin.json              # Admin HTTP surface
│   ├── peer.json               # Public/Peer HTTP and WebRTC signaling surface
│   ├── shared.json             # aggregation entry point for truly shared OpenAPI schemas
│   ├── shared/                 # cross-surface or cross-domain DTOs
│   └── resources/              # Resource, owned Spec, and Resource aggregation definitions
└── proto/
    ├── giznet/
    │   ├── admission.proto     # Giznet admission credential
    │   └── nanopb.options      # bounded C strings
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
| OpenAI Compatible API | Server | AI Server Shell over HTTP | [Design](./http/openai-compatible) |
| Peer RPC | Client, Server, Edge-node | Protobuf RPC over Giznet service stream | [Design](./proto/rpc/overview) · [Methods](/references/rpc) · [Streams](/references/streams#rpc-streams) |
| Peer Events | Client, Server | Protobuf over Peer Event Stream | [Events](/references/events) · [Streams](/references/streams) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [Design](./proto/telemetry) · [Transport](/references/streams#direct-packets) |

## Subdocument

- [HTTP API](./http/): OpenAPI surfaces, Shared, Resources and type ownership.
- [Proto API](./proto/): Peer RPC and Telemetry Protobuf contract.
- [Generation and Change](./generation): Go, JavaScript and C generation links and validation requirements.

Node Monitor API: Server and Edge provide the process-local HTTP contract in `api/http/monitor.json`; see [Monitor](../monitor) for authentication and generation ownership.

## Device state and procedures {#mhs-v0-migration}

A device exposes MHS v0 hardware states and predefined tool/v0 procedures. The bound RuntimeProfile's `spec.mhs.v0` manifest declares product-defined `(device_id, state)` keys. Control apps read that manifest through `GET /gizclaw/v1/device/mhs/v0/manifest`, then call `POST /gizclaw/v1/device/mhs/v0/read` or `PATCH /gizclaw/v1/device/mhs/v0/states`. The following names are recommendations, not automatically installed protocol IDs; only manifest-declared, device-implemented keys may be read, and writes require `read_write`.

| Example state | Recommended MHS v0 key | Type and recommended constraints |
| --- | --- | --- |
| speaker volume | `speaker.main/volume` | `int`, 0–100 |
| speaker mute | `speaker.main/muted` | `bool` |
| device `screen_brightness` | `display.main/brightness` | `int`, 0–100 |
| device `screen_off_timeout_ms` | `display.main/off_timeout_ms` | `int`, milliseconds, ≥0; 0 keeps the screen on |
| device `led_brightness` | `led.status/brightness` | `int`, 0–100 |
| device `cellular_enabled` | `cellular.main/enabled` | `bool` |
| device `nfc_enabled` | `nfc.main/enabled` | `bool` |
| device `auto_sleep_timeout_ms` | `power.main/auto_sleep_timeout_ms` | `int`, milliseconds, ≥0; 0 disables automatic sleep |
| device `locale` | `system.main/locale` | `string`, retaining the BCP 47 language-tag convention |
| device `default_interaction_mode` | `system.main/default_interaction_mode` | `enum`: `push-to-talk`, `realtime` |
| device `key_feedback` | `system.main/key_feedback` | `enum`: `none`, `sound`, `vibrate`, `sound_and_vibrate` |
| device `alert_mode` | `system.main/alert_mode` | `enum`: `silent`, `vibrate`, `ring` |

MHS enums use semantic strings in `MhsValue.string_value`. Reads name each requested key; writes include only changed keys and return the values actually applied. The device validates an entire write before applying it. Unknown or unimplemented keys return an error. Read back after a timeout to confirm state.

`tool/v0` carries procedures through `client.tool.v0.invoke` (135). Each request selects one of the 21 predefined `ClientTool` values and carries that tool's encoded request message. Devices advertise only installed tools through `client.tool.v0.list` (136). `client.rpc.methods.list` (137) returns numeric `RpcMethod` values to identify supported protocol families and versions. Control apps use `GET /gizclaw/v1/device/tool/v0/tools` and `POST /gizclaw/v1/device/tool/v0/invoke`; the Server validates typed arguments before contacting the device. See [Peer HTTP](./http/public), [device providers](./proto/rpc/client-provided-to-server), and [RPC reference](/references/rpc).

`GET /gizclaw/v1/device/status` reads the Server's stored identity and telemetry snapshot; a live `device.status.get` tool call can refresh that snapshot.
