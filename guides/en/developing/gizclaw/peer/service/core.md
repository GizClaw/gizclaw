# Core Service

`Implementation file: peer_service.go`

Define the `PeerService` general entry point, verify whether the dependencies are complete, confirm that the Peer has been registered, and start Giznet services such as Admin, Public HTTP, OpenAI, RPC, etc. according to the connection request.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`PeerService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerService) | Aggregate Manager, public login sessions, API handlers and domain services. |
| [`PeerService.ServeConn`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerService.ServeConn) | Initialize the Peer connection and start allowed Giznet services in parallel. |
| `ensureConnectedPeer` | Ensure that the Peer resource corresponding to the connection identity exists. |
| `validateServices` | Verify required service dependencies before starting the connection. |
| `isPeerServiceClosed` | Determine whether the service loop ends due to normal connection close. |
