# Server Provided to Edge-node

这一组能力由 Server 实现，只提供给具有 Edge-node role 的连接。Edge-node 使用它查询 Peer assignment 和解析上游路由，不向普通 Client 暴露控制面能力。

## Methods

| Method | 作用 |
| --- | --- |
| `server.peer.lookup` | 查询指定 Peer 当前 assignment |
| `server.peer.assign` | 为指定 Peer 创建或更新 assignment，并执行 version conflict 检查 |
| `server.route.resolve` | 为目标 Peer 解析可用 route/assignment |

## 调用关系

```mermaid
sequenceDiagram
    participant Edge as Edge-node
    participant Server as Server Edge RPC
    participant Route as Peer Route service
    Edge->>Server: lookup / assign / resolve
    Server->>Route: validate and query assignment
    Route-->>Server: assignment / route error
    Server-->>Edge: typed response / RPC error
```

Server 使用独立 Edge RPC dispatch，只接受上述三个 methods。普通 Client RPC surface 即使共享同一 `rpc.proto` registry，也不能因为 method 可解码就获得调用权限；role authorization 与 service exposure 必须同时限制 Edge control plane。
