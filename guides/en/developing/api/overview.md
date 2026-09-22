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

## MHS v0 hardware-state migration {#mhs-v0-migration}

The following legacy interfaces are deprecated. Their requests, responses, validation, errors and runtime behavior remain unchanged, so existing devices and apps keep working:

| Deprecated RPC | Deprecated Peer HTTP | MHS v0 replacement |
| --- | --- | --- |
| `client.device.volume.set` (101) | `PUT /gizclaw/v1/device/volume` | `client.mhs.v0.write` (134); `PATCH /gizclaw/v1/device/mhs/v0/states` |
| `client.device.settings.get` (128) | `GET /gizclaw/v1/device/settings` | `client.mhs.v0.read` (133); `POST /gizclaw/v1/device/mhs/v0/read` |
| `client.device.settings.set` (129) | `PATCH /gizclaw/v1/device/settings` | `client.mhs.v0.write` (134); `PATCH /gizclaw/v1/device/mhs/v0/states` |

Control apps first read `GET /gizclaw/v1/device/mhs/v0/manifest`, sourced from the bound RuntimeProfile's `spec.mhs.v0`. Products define device IDs and state names in that manifest. The table below gives **recommended conventions**, not protocol-mandated IDs, and adds no keys automatically. `device_id/state` denotes two separate request fields. Read or write only keys declared in the manifest and implemented by the device; writes also require `read_write` access.

| Legacy field | Recommended MHS v0 key | Type and recommended constraints |
| --- | --- | --- |
| volume.set `level` | `speaker.main/volume` | `int`, 0–100 |
| volume.set `muted` | `speaker.main/muted` | `bool` |
| settings `screen_brightness` | `display.main/brightness` | `int`, 0–100 |
| settings `screen_off_timeout_ms` | `display.main/off_timeout_ms` | `int`, milliseconds, ≥0; 0 keeps the screen on |
| settings `led_brightness` | `led.status/brightness` | `int`, 0–100 |
| settings `cellular_enabled` | `cellular.main/enabled` | `bool` |
| settings `nfc_enabled` | `nfc.main/enabled` | `bool` |
| settings `auto_sleep_timeout_ms` | `power.main/auto_sleep_timeout_ms` | `int`, milliseconds, ≥0; 0 disables automatic sleep |
| settings `locale` | `system.main/locale` | `string`, retaining the BCP 47 language-tag convention |
| settings `default_interaction_mode` | `system.main/default_interaction_mode` | `enum`: `push-to-talk`, `realtime` |
| settings `key_feedback` | `system.main/key_feedback` | `enum`: `none`, `sound`, `vibrate`, `sound_and_vibrate` |
| settings `alert_mode` | `system.main/alert_mode` | `enum`: `silent`, `vibrate`, `ring` |

MHS enums retain the legacy settings' semantic string values. RPC uses `MhsValue.string_value`, not the old Protobuf enum numbers or symbolic names. Reads explicitly name the required keys; writes contain only keys to change. MHS write returns the actual values for that batch, not a full `DeviceSettings` or `PeerStatus`. Unknown or unimplemented keys return errors instead of inheriting legacy settings' ignore-unsupported-member behavior. Read back after a timeout to confirm current values. See [Peer HTTP](./http/public) and the [device provider contract](./proto/rpc/client-provided-to-server) for full semantics.

`sound.play` and `find` are actions, and MHS v0 has no procedures. `reboot`, `factory_reset`, `firmware.update`, Wi-Fi, audioplayer, `run.workspace.set`, tools and `rpc.methods.get` are not deprecated. `client.device.status.get` and `GET /gizclaw/v1/device/status` remain available for device identity and telemetry snapshots beyond hardware state; MHS does not replace them.

Retirement plan: remove the legacy interfaces only after both device firmware and control apps have migrated. This change sets no removal date.
