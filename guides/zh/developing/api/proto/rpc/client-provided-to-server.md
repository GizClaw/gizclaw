# Client Provided to Server

这一组能力由 Client/Device 实现，由 Server 在 Peer connection 上调用。Server 使用它读取设备自身信息或请求设备执行本地能力。

## Methods

| Method | 作用 |
| --- | --- |
| `client.info.get` | 读取 Client 当前 device information |
| `client.identifiers.get` | 读取 Client hardware/device identifiers |
| `client.tool.invoke` | 请求 Client 执行其本地提供的 Tool |

## 调用关系

```mermaid
sequenceDiagram
    participant Server
    participant Client
    Server->>Client: client.* request
    Client->>Client: Read device state or invoke local tool
    Client-->>Server: typed response / RPC error
```

Client provider 只能返回该 Client 拥有或可执行的数据。Server resource access decision、跨 Peer lookup 和持久化管理不能实现为 `client.*`。

Go Client 的 provider dispatch 位于 `sdk/go/gizcli` 的 RPC Client implementation；Server 侧通过在线 Peer connection 调用这些 methods。
