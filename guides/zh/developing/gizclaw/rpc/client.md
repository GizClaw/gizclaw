# Client

源码：[rpc_client.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_client.go)

定义无状态 `rpcClient`，并实现 Server 主动调用 Client Peer 的 RPC methods。当前能力包括读取设备信息和 identifiers；请求构造、RPC 调用与 typed response 解码都由这个文件负责。
