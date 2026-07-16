# Peer HTTP · /me

`Implementation file: peer_service_serve_peer_http_self.go`

These are the endpoints in `ServicePeerHTTP` with `/me` as the root path. They are responsible for reading or updating the caller's own Peer resource, status and runtime, and verifying that the caller can only operate its own Peer identity.

Peer persistent resources and runtime status are owned by `services/runtime/peer`, `peerrun` and related runtime services respectively.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerHTTP.GetMe` | Returns the caller's own Peer resource. |
| `peerHTTP.GetMeStatus` | Returns the caller's own online status. |
| `peerHTTP.PutMeStatus` | Update the caller's own status. |
| `peerHTTP.GetMeRuntime` | Returns the caller's own runtime view. |
| `ensurePeerHTTPCaller` | Verify that the Peer in the URL/request is consistent with the session caller identity. |
