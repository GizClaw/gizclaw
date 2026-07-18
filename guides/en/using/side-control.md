# Side Control

Side Control lets an app or mini program read device state and telemetry and manage device contacts after explicit authorization. The controller uses its own keypair; it neither copies the device private key nor establishes another primary connection.

## Enrollment flow

1. The device calls `POST /me/side-control/device-tokens` with its primary session.
2. The device encodes the Public API endpoint, Server public key, returned token, and expiry in a client-defined QR code. GizClaw does not prescribe the QR presentation or envelope format.
3. After scanning, the controller creates or loads its own keypair and creates a login assertion for the Server public key.
4. The controller calls `POST /login` with `grant_type: side_control` and `device_token`, plus its own `X-Public-Key` and assertion.
5. The controller uses the returned bearer session for `/side-control/*`. A production mini program calls Edge over HTTPS, and Edge forwards through `ServiceEdgeHTTP` to the authoritative Server.
6. The device lists grants with `GET /me/side-control/sessions` and revokes one with `DELETE /me/side-control/sessions/{sessionId}`. Revocation immediately invalidates the controller bearer.

A device token is single-use and expires after five minutes. A Side Control session lasts at most 24 hours. Apps must not log device tokens, assertions, bearer tokens, or private keys.

## Available capabilities

- `/side-control/info`, `runtime`, and `status`: device information, presence, battery, GNSS, volume, and mute state.
- `/side-control/telemetry/*`: battery, GNSS, network RSSI/signal/connected, and system metrics.
- `/side-control/contacts`: CRUD for contacts owned by the target device.

Wi-Fi, device passwords, and playing sounds remain LiteLink direct-device capabilities. Side Control Public API does not define these routes or return SSIDs, saved networks, or Wi-Fi credentials.
