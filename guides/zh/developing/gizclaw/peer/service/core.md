# Core Service

`实现文件：peer_service.go`

定义 `PeerService` 总入口，验证依赖是否完整，确认 Peer 已登记，并根据连接请求启动 Admin、Public HTTP、OpenAI、RPC 等 Giznet services。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`PeerService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerService) | 聚合 Manager、public login sessions、API handlers 与领域 services。 |
| [`PeerService.ServeConn`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerService.ServeConn) | 初始化 Peer connection，并并行启动允许的 Giznet services。 |
| `ensureConnectedPeer` | 确保连接 identity 对应的 Peer 资源存在。 |
| `validateServices` | 在启动 connection 前验证必需 service dependencies。 |
| `isPeerServiceClosed` | 判断 service loop 是否因正常 connection close 结束。 |
