# API keys

GizClaw uses long-lived, device-bound API keys for the public GizClaw and OpenAI-compatible HTTP APIs. A registered device manages its keys over the authenticated Peer RPC connection with `server.api_key.create`, `server.api_key.list`, and `server.api_key.revoke`. The Peer connection is the root authority. API keys are recoverable management resources: create, list, get, and self responses include the complete `gizclaw_sk_v1_...` credential.

Send the secret as `Authorization: Bearer <api-key>` to `/gizclaw/v1/*` and `/openai/v1/*`. No public-key header or login exchange is required.

An ordinary key can use public APIs, inspect itself with `GET /gizclaw/v1/api-keys/self`, and revoke itself with `DELETE /gizclaw/v1/api-keys/self`. A key created with `manage_api_keys: true` can also create, list, inspect, and revoke other keys belonging to the same device through `/gizclaw/v1/api-keys`. `manage_api_keys` only governs key management; every key can read and control its bound device with identical capability.

## Device reads and control

The key's bound device is the fixed target of every `/gizclaw/v1/device*` and `/gizclaw/v1/contacts*` request (see [API](./api#device-http-api) for the route list). Read the device status and set the volume:

```sh
curl -sS "$GIZCLAW_URL/gizclaw/v1/device/status" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY"

curl -sS -X PUT "$GIZCLAW_URL/gizclaw/v1/device/volume" \
  -H "Authorization: Bearer $GIZCLAW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"level":35,"muted":false}'
```

A successful `PUT /device/volume` answers `200 { "status": PeerStatus }` whose `volume` and `muted` match the next `GET /device/status`; `play-sound`, `reboot`, and `DELETE /wifi/saved/{ssid}` answer `204`. When the device is offline, control routes answer `409 DEVICE_OFFLINE`; when it does not answer within 5 seconds they answer `504 DEVICE_TIMEOUT`; neither changes the stored status. After a `reboot` is acknowledged, control requests answer `409` until the device reconnects. A device that rejects the parameters answers `400 DEVICE_REJECTED`, and firmware without the capability answers `501 DEVICE_UNSUPPORTED`. Once the key is revoked or the device Peer is deleted, every device and contact request fails immediately.

API keys do not expire automatically. Revoke keys that are lost or no longer needed. Deleting a Peer revokes all keys owned by that Peer.
