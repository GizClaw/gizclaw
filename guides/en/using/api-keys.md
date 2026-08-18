# API keys

GizClaw uses long-lived, device-bound API keys for the public GizClaw and OpenAI-compatible HTTP APIs. A registered device creates its first key over the authenticated Peer RPC connection with `server.api_key.create`. The complete `gizclaw_sk_v1_...` secret is returned once; store it securely.

Send the secret as `Authorization: Bearer <api-key>` to `/gizclaw/v1/*` and `/openai/v1/*`. No public-key header or login exchange is required.

An ordinary key can use public APIs, inspect itself with `GET /gizclaw/v1/api-keys/self`, and revoke itself with `DELETE /gizclaw/v1/api-keys/self`. A key created with `manage_api_keys: true` can also create, list, inspect, and revoke other keys belonging to the same device through `/gizclaw/v1/api-keys`.

API keys do not expire automatically. Revoke keys that are lost or no longer needed. Deleting a Peer revokes all keys owned by that Peer.
