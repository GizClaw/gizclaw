# Telemetry API

`api/proto/telemetry/peer_telemetry.proto` Defines the telemetry event wire format sent by Peer to Server. It is a high-frequency one-way event stream, not an RPC method, and not an Admin HTTP resource.

See the [Streams Reference](/references/streams#direct-packets) for the direct-packet protocol, reliability, and transport boundary.

## Data path

```mermaid
sequenceDiagram
    participant Peer
    participant Conn as Giznet Peer connection
    participant Decoder as Telemetry decoder
    participant Service as Peer Telemetry service
    participant Store as Metrics store
    participant Admin as Admin HTTP

    Peer->>Conn: telemetry protobuf packet
    Conn->>Decoder: protocol + payload
    Decoder->>Service: typed telemetry event
    Service->>Store: append/update metrics
    Admin->>Service: query latest/aggregate
    Service-->>Admin: telemetry view
```

Telemetry Protobuf owns the wire fields reported by the device. Metrics store has save and query semantics; Admin HTTP has a response contract for administrators. Do not directly use the storage model as a telemetry wire message for convenience, and do not let the device depend on the Admin response DTO.

## Design rules

- The high frequency field should remain compact, stable and backward compatible.
- New fields must have explicit units, time semantics, and default values; they cannot just rely on guesswork from Go annotations.
- Decoder treats malformed or out-of-limit input as untrustworthy boundaries.
- Aggregation, retention and query filtering belong to service/store and not to wire schema.
- Regenerate Go and JavaScript telemetry code after Schema changes, and verify the real packet decode and service ingestion.

## OTA reporting

`Observation.ota` (field 14) carries `OtaObservation` for one device-owned update attempt:

| State | Value | Meaning |
| --- | --- | --- |
| `OTA_STATE_STARTED` | 1 | The device started the update attempt. |
| `OTA_STATE_DOWNLOADING` | 2 | Downloading; `download_percent` is required. |
| `OTA_STATE_SUCCEEDED` | 3 | The device confirmed update success; a 100% download is not success. |
| `OTA_STATE_FAILED` | 4 | The attempt failed; optional `error_code` and `error_message` describe it. |

`update_id` is a required device-supplied attempt identifier of 1–128 UTF-8 bytes;
use a new identifier for a retry. Optional `target_version` is at most 128 UTF-8 bytes.
`download_percent` is finite and in [0, 100]; absent means unreported, while explicit
zero means zero download progress. Error fields are allowed only for failure and
are limited to 128 (`error_code`) and 512 (`error_message`) UTF-8 bytes. Devices must
supply safe diagnostics without credentials, signed URLs, or secrets. Observation
time is the frame timestamp plus its observation delta in milliseconds.

Go exposes `Client.SendOTATelemetry(*telemetrypb.OtaObservation)`; JavaScript exposes
`otaTelemetry` and `OtaState` with the existing send APIs. C exposes
`gzc_telemetry_ota_frame_t`, `gzc_telemetry_encode_ota_frame`, and
`gzc_client_send_ota_telemetry`. Each C OTA frame carries one observation and borrows
strings during the call, preserving existing frame/observation layouts. Go and
JavaScript can mix observation types in one frame. Flutter currently has no
telemetry sending surface; this contract does not add a Flutter transport.

SDKs encode and send the wire message; the server validates the semantics above,
rejecting unspecified or unsupported states. After validating the entire frame,
the server updates runtime status. OTA payloads, including diagnostic strings, are
never logged and do not write numerical metrics or use telemetry latest/aggregate
queries. Bounded diagnostic strings are retained as supplied, without heuristic
secret detection, and returned only through the existing authenticated device
status access. Devices must avoid including secrets in diagnostics.

The latest OTA snapshot is persisted in the device runtime KV Store and exposed
as `PeerStatus.ota`. LiteLink and other API-key clients read
`GET /gizclaw/v1/device/status` for `state` (`started`, `downloading`, `succeeded`,
`failed`), `update_id`, `observed_at`, and optional version, download percentage,
and error fields. Peer RPC `server.status.get` returns the same snapshot. Status
SDKs preserve unknown future state strings. The last snapshot remains readable
while the device is disconnected; connection statistics at `/device/runtime` do
not carry OTA state.

The runtime atomically compares a dedicated per-peer OTA record. Older timestamps
cannot overwrite newer snapshots. Terminal states and download percentages cannot
regress within one attempt; equal timestamps may advance only the same attempt.
A new attempt requires a different `update_id` and later time and replaces the
whole snapshot, clearing previous errors and progress. Omitted version/progress
retain their latest values within an attempt. Control/status writes such as volume
changes cannot overwrite OTA.

Delivery retains direct-packet semantics without application acknowledgement,
retries, or exactly-once delivery. The server maintains the latest snapshot with the ordering rules above. Devices
should limit the frequency of progress reports.
