# Admin HTTP · Telemetry

`Implementation file: peer_service_serve_admin_telemetry.go`

Implement the latest telemetry, historical query and aggregation endpoints, parse Peer public key and field filter conditions, and map telemetry service errors.

Telemetry decoding, status and metric aggregation belong to `services/runtime/peertelemetry`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `GetPeerTelemetryLatest` | Returns the latest telemetry sample of the specified Peer. |
| `QueryPeerTelemetry` | Query telemetry samples by time and field. |
| `AggregatePeerTelemetry` | Aggregate telemetry in the specified window. |
| `parseAdminTelemetryPublicKey` | Parse and verify the Peer public key in the request. |
| `parsePeerTelemetryFields` | Parse field filter conditions. |
| `peerTelemetryAdminError` | Map telemetry service error to Admin API error. |
