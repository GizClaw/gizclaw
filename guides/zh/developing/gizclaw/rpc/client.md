# Client

`实现文件：rpc_client.go`

定义无状态 `rpcClient`，并实现 Server 主动调用 Client Peer 的 RPC methods。当前能力包括读取设备信息和 identifiers；请求构造、RPC 调用与 typed response 解码都由这个文件负责。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `rpcClient` | Client-side RPC calls 的共享 receiver；自身不持有 connection 状态。 |
| `rpcClient.GetClientInfo` | 请求并解码 Client device info。 |
| `rpcClient.GetClientIdentifiers` | 请求并解码 Client identifiers。 |
