# Peer HTTP · API keys

`peer_service_serve_peer_http_api_key.go` adapts `services/system/apikey` to the generated Peer HTTP contract. Bearer authentication is performed before the strict handlers run, and the authenticated key's owner becomes the Peer identity used by GizClaw and OpenAI-compatible services.

The service stores the complete API key in plaintext in its record and plaintext credential index. Authorized create, list, get, and self operations return that complete key. `manage_api_keys` grants same-owner management; every key can inspect and revoke itself. Peer retirement calls `CleanupPeer` to atomically remove every key and write an owner retirement marker before deleting the RuntimeProfile owner binding; the marker prevents a cleaned owner from creating another key.

This recoverability is an intentional credential-store boundary. GizClaw does not hash or application-encrypt these keys and does not introduce a KMS: the Server process and datastore operators or backup readers are trusted at credential authority. Deployments must protect datastore access and storage at rest. Application access remains owner-scoped, management operations use the existing bounded operation observability without recording key values, rotation means creating a replacement and revoking the old key, and Peer retirement revokes every owned key.

The authenticated Peer RPC connection remains the device owner's root management authority through `server.api_key.create`, `server.api_key.list`, and `server.api_key.revoke`. The `manage_api_keys` flag delegates management to an issued API key; it does not gate or replace the Peer RPC root methods.

Create, list, revoke, and Peer cleanup coordinate per owner. The durable retirement marker still prevents late same-owner publication, while unrelated owners continue during Store scans. Only access to an injected non-thread-safe random source uses a separate short mutex; global uniqueness remains enforced by atomic KV guards.
