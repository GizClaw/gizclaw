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
    │   ├── gizclaw_rpc.proto           # request, response, error, stream, and method registry
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
| Mem0 API | Self-hosted Mem0 service | HTTP / OpenAPI | [Memory Store](../stores/memory#self-hosted-mem0-service) |
| Peer RPC | Client, Server, Edge-node | Protobuf RPC over Giznet service stream | [Design](./proto/rpc/overview) · [Methods](/references/rpc) · [Streams](/references/streams#rpc-streams) |
| Peer Events | Client, Server | Protobuf over Peer Event Stream | [Events](/references/events) · [Streams](/references/streams) |
| Peer Telemetry | Client / Peer | Protobuf direct packet | [Design](./proto/telemetry) · [Transport](/references/streams#direct-packets) |

## Subdocument

- [HTTP API](./http/): OpenAPI surfaces, Shared, Resources and type ownership.
- [Proto API](./proto/): Peer RPC and Telemetry Protobuf contract.
- [Generation and Change](./generation): Go, JavaScript and C generation links and validation requirements.

Node Monitor API: Server and Edge provide the process-local HTTP contract in `api/http/monitor.json`; see [Monitor](../monitor) for authentication and generation ownership.

## HWDs and device procedures {#mhs-v0-hwd}

GizClaw defines nine HWDs in `api/proto/rpc/payload/mhs_v0.proto`. Each HWD has a typed read response; display, led, speaker, modem and nfc also have typed write request and applied-value response messages. RuntimeProfile `spec.mhs.v0.devices` lists only the `id` and `hwd` of each instance. Two LED entries with different IDs are two instances of the same HWD. The manifest does not define fields or access modes.

| HWD | Read | Write |
| --- | --- | --- |
| wifi, ble, battery, mic | Yes | No |
| display, led, speaker, modem, nfc | Yes | Yes |

Control apps discover instances with `GET /gizclaw/v1/device/mhs/v0/manifest`, then read one through `POST /device/mhs/v0/read` or write one through `POST /device/mhs/v0/write`. Requests carry `id` and `hwd`; a write also carries an HWD-specific `value` object. Responses identify the instance and return its typed `value`. Procedures such as Wi-Fi scan and connect remain in tool/v0. Read back after a write timeout to confirm the actual state.

`tool/v0` carries procedures through `client.tool.v0.invoke` (135). Each request selects one of the 38 predefined `ClientTool` values and carries that tool's encoded request message. Devices advertise only installed tools through `client.tool.v0.list` (136). `client.rpc.methods.list` (137) returns numeric `RpcMethod` values to identify supported protocol families and versions. Control apps use `GET /gizclaw/v1/device/tool/v0/tools` and `POST /gizclaw/v1/device/tool/v0/invoke`; the Server validates typed arguments before contacting the device. See [Peer HTTP](./http/public), [device providers](./proto/rpc/client-provided-to-server), and [RPC reference](/references/rpc).

`GET /gizclaw/v1/device/status` reads the Server's stored identity and telemetry snapshot; a live `device.status.get` tool call can refresh that snapshot.

Infrastructure Flow Feed is a read-only AP surface owned by `api/http/infra.json` and `pkgs/infraflow`; see [aggregate frames and resource limits](../infra-flow).

## Modem and NFC instance state

Modem enabled is the user-configured switch; powered is an actual observation. Both are optional; never
infer power from registration or RF signal. SIM/registration/RAT/signal fields remain compatible. Modem
writes require enabled and return applied state containing actual enabled, plus any observed power state.

NFC uses the same `{id,hwd}` identity: multiple instances have independent enabled, card_present and last_read
state. last_read caches the most recent attempt, including failure; removal does not clear it and reads do
not initiate hardware access. It contains positive int64 read_at_unix_ms (a decimal string in HTTP protobuf
JSON), optional UID/technology/data_format/raw_data, result and error_code. UID is at most 32 bytes and raw
data 1024 bytes, base64 in JSON; technology/data_format are at most 32 UTF-8 bytes and error_code 64.
Result is success or failed; failure requires an error code and success omits it. Missing last_read means no
attempt yet. NFC writes only enabled and must return actual enabled; omission never means false.

GizOS implements real power observations, switch effects and independent NFC caches. Products declare
instances in their manifests. GizClaw carries generic hardware data; Forge consumes the existing MHS and
Runtime/status reported_kv APIs. Protocol and SDK tests do not establish physical-device acceptance.
