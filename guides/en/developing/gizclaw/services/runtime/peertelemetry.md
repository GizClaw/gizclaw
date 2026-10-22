# Peer Telemetry

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peertelemetry)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry)

`peertelemetry` Decode the Peer telemetry packet, project the frame into metrics sample and fixed Peer status patch, and provide aggregation entry for Admin query.

## Data flow

```mermaid
flowchart LR
    Packet["Telemetry packet"] --> Decode["Decode"]
    Decode --> Map["MapFrame"]
    Map --> Metrics["metrics.Store"]
    Map --> Status["Peer StatusSync"]
    Metrics --> Admin["AdminService"]
```

Telemetry schema belongs to `api/proto/telemetry`, metrics persistence belongs to `pkgs/store/metrics`. This package only has decoding, mapping and synchronization strategies.

Network observation `imei` / `imsi` values pass pattern and cellular-route validation into the `StatusPatch`, and `StatusSync` merges them per field by observation time into `PeerStatus.network_imei` / `network_imsi`, never as metrics or logs; see [Telemetry API](/en/developing/api/proto/telemetry#network-reporting) for the rules.

Activity observation `activity` and its optional `detail` enter the `StatusPatch` together, and `StatusSync` merges them as one unit into `PeerStatus.activity` / `activity_detail`, again as status and never as metrics; `SystemObservation.firmware_version` follows the same per-field rule into `PeerStatus.firmware_version`. See [Telemetry API](/en/developing/api/proto/telemetry#activity-reporting) for the rules.

The observation time of each telemetry-sourced field lives in the typed
`PeerStatus.telemetry_observed_at`, whose members are named after the `PeerStatus` field each one
describes and hold RFC 3339 timestamps. Those timestamps are what per-field ordering is decided on: a
late or replayed report is rejected instead of overwriting a newer observation. A member is absent
until that field has been observed at least once.

`telemetry_observed_at` is Server-maintained and not device-writable: a `telemetry_observed_at` carried
on a control response is dropped, and the observation time comes only from what the Server recorded
when it handled the observation.

Validated OTA observations update queryable runtime OTA status through `StatusSync`, without payload logs or metrics; see [Telemetry API](/en/developing/api/proto/telemetry#ota-reporting) for fields and SDK usage.

KV observations enter StatusPatch.KV after complete frame validation. StatusSync calls PutReportedKV to
atomically update the independent device-reported record; it neither interprets values nor creates metrics
or payload logs. Storage quotas are checked before committing the complete KV batch. See
[KV telemetry](/en/developing/api/proto/telemetry#device-reported-kv-state) for query projections and ordering.
