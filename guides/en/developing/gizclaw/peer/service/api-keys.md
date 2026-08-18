# Peer HTTP · API keys

`peer_service_serve_peer_http_api_key.go` adapts `services/system/apikey` to the generated Peer HTTP contract. Bearer authentication is performed before the strict handlers run, and the authenticated key's owner becomes the Peer identity used by GizClaw and OpenAI-compatible services.

The service stores public metadata separately from a SHA-256 digest index. Plaintext secrets are returned only by creation. `manage_api_keys` grants same-owner management; every key can inspect and revoke itself. Peer retirement calls `CleanupPeer` to atomically remove every key and write an owner retirement marker before deleting the RuntimeProfile owner binding; the marker prevents a cleaned owner from creating another key.

The authenticated Peer RPC connection remains the device owner's root management authority through `server.api_key.create`, `server.api_key.list`, and `server.api_key.revoke`. The `manage_api_keys` flag delegates management to an issued API key; it does not gate or replace the Peer RPC root methods.
