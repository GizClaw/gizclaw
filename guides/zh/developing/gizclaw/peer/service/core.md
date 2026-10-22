# Core Service

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw)

源码：[peer_service.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service.go)

定义 `PeerService` 总入口，验证依赖是否完整，确认 Peer 已登记，并根据连接请求启动 Admin、Public HTTP、OpenAI、RPC 等 Giznet services。
