# Peer Route

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` 维护当前 Server 所知的 Peer assignment，并为 Edge/Server routing 提供查询和更新能力。它描述的是本 Server 的控制面状态，不代表 mesh-wide directory 或跨 Server 自动同步已经存在。

## 核心结构与主函数

| 结构或函数 | 作用 |
| --- | --- |
| `Server` | 提供 assignment 的读取、写入与 RPC handler。 |
| `PeerStore` | 读取 assignment 所关联的 Peer 资源。 |
| `ParsePublicKey` | 校验 wire/string public key。 |
| `ToRPC` | 将内部 `PeerAssignment` 转换为 RPC message。 |

Route assignment、Peer 在线 connection 和持久化 Peer 是三个不同状态。代码不能因为存在 assignment 就推断目标当前在线。
