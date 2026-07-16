# HTTP Service Entrypoints

`Implementation file: peer_service_serve_peer_http.go`

Provides ordinary Peer Public HTTP and Edge Public HTTP, assembles login, session, CORS, Peer API, OpenAI API and Edge signaling routes, and performs access judgment of Edge client/signaling Peer.

This file has HTTP surface composition; the login session belongs to `services/system/publiclogin`, and the specific API behavior belongs to the corresponding domain service.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `servePublic` / `serveEdgePublic` | Start normal or Edge Public HTTP on the corresponding Giznet service. |
| `publicHTTPHandlerWithOptions` | Assemble login, session, Peer API, OpenAI API and signaling routes. |
| `edgeLoginPeerHTTP` | Adapt login handler to Edge HTTP surface. |
| `allowEdgeClientPeer` | Determine whether the Peer is allowed to serve as an Edge client. |
| `allowEdgeSignalingPeer` | Determine whether the Peer is allowed to initiate signaling through the Edge. |
| `setPeerHTTPCORSHeaders` | Set the CORS headers of the Peer HTTP surface. |
