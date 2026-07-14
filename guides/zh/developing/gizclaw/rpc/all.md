# Common RPC

`实现文件：rpc_all.go`

实现所有 RPC connection 共用的 Ping 调用，将 request ID、Ping payload 和响应解码接到通用 RPC call path。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcServer.Ping` | 构造 Ping request，通过指定 RPC connection 调用并解码 response。 |
| [`PeerConn.Ping`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn.Ping) | 为 Peer connection 打开一次 RPC stream、执行 Ping 并关闭 stream。 |
