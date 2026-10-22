# Core Service

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw)

Source：[peer_service.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service.go)

Define the `PeerService` general entry point, verify whether the dependencies are complete, confirm that the Peer has been registered, and start Giznet services such as Admin, Public HTTP, OpenAI, RPC, etc. according to the connection request.
