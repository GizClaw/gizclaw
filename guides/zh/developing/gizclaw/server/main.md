# Server

`实现文件：server.go`

定义可复用的 `Server` composition root：接收 identity、Peer listener、stores 与运行配置；初始化各领域 service；启动 HTTP 和 Peer listener；处理 Peer event；管理后台 cleanup、关闭顺序和 module store fallback。

它可以组合多个领域，但单一领域的 resource、validation、storage 和 lifecycle 应留在 `services/<domain>`。进程配置与启动属于 `cmd/internal/server`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Server`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server) | 可复用 GizClaw Server 的 composition root。 |
| [`PeerListenerOptions`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerListenerOptions) / [`PeerListenerFactory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerListenerFactory) | 描述并创建 Peer listener。 |
| [`Server.ServeHTTP`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.ServeHTTP) | 服务 Server HTTP surface。 |
| [`Server.Listen`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Listen) / [`Serve`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Serve) | 创建 listeners 并接受 Peer connections。 |
| [`Server.PublicKey`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.PublicKey) | 返回 Server identity public key。 |
| [`Server.PeerService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.PeerService) / [`Manager`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Manager) | 返回已组装的 Peer service 或在线 Peer Manager。 |
| `init` | 初始化 stores、领域 services、HTTP mux 和 Peer Runtime。 |
| `servePeerListener` | 接受单个 listener 上的 Peer connections。 |
| `startCleanup` | 启动后台资源清理。 |
| [`Server.Close`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#Server.Close) | 停止 listeners、后台任务并关闭 Server 资源。 |
