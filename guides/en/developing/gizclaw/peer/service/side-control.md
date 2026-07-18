# Peer HTTP · Side Control

Side Control lets an app or mini program that is not registered as a Peer read device data and manage target-owned contacts after explicit device authorization. It does not create another primary connection or change the target Peer's runtime identity.

## Authorization model

1. The target device's primary session creates a single-use, five-minute device token with `POST /me/side-control/device-tokens`.
2. The controller submits its own `X-Public-Key` and login assertion to `POST /login` with `grant_type=side_control` and the device token.
3. The Server atomically consumes the token and issues a 24-hour Side Control bearer session bound to the controller public key, target public key, and session ID.
4. The primary session can list or revoke the target's Side Control sessions. Revocation invalidates the bearer immediately.

Primary sessions can access `/me/*`; Side Control sessions can access `/side-control/*`. A side controller does not need a Peer registration. Edge bypasses active Client Peer admission only for an explicit Side Control grant.

## Routes

| Route | Purpose |
| --- | --- |
| `POST /me/side-control/device-tokens` | Create a device token |
| `DELETE /me/side-control/device-tokens/{tokenId}` | Revoke an unconsumed token |
| `GET /me/side-control/sessions` | List active Side Control sessions |
| `DELETE /me/side-control/sessions/{sessionId}` | Revoke a session |
| `GET /side-control/info` | Target device information |
| `GET /side-control/runtime` | Target runtime |
| `GET /side-control/status` | Current target status, including battery and GNSS |
| `GET /side-control/telemetry/latest` | Latest telemetry |
| `GET /side-control/telemetry` | Time-range telemetry |
| `GET /side-control/telemetry/aggregate` | Aggregated telemetry |
| `/side-control/contacts` | CRUD for target-owned contacts |

Telemetry fields come from the existing Peer Telemetry contract and include battery, GNSS, network RSSI/signal/connected, and system runtime metrics. The Server does not store or expose SSIDs, Wi-Fi scans, saved networks, or provisioning operations.

## Transport

Mini programs call Edge over HTTPS. Edge forwards the request internally to the authoritative Server through the `ServiceEdgeHTTP` service stream. The Server serves the same contract on its direct TCP HTTP surface; `serve-to-clients` alone controls whether clients may use that TCP ingress.

LiteLink capabilities such as device passwords, Wi-Fi, local device-token retrieval, and playing sounds remain owned by the direct device protocol and are not duplicated in Server HTTP.
