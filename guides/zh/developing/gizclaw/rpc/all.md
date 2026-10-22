# Common RPC

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw)

源码：[rpc_all.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_all.go)

实现所有 RPC connection 共用的 Ping 调用，将 request ID、Ping payload 和响应解码接到通用 RPC call path。
