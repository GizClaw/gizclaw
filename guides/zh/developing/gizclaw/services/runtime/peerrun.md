# Peer Run

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun)

`peerrun` 保存 Peer 当前运行状态及其 Agent selection。它拥有 Peer 与运行选择之间的关联，不拥有 Agent definition、Workspace、Workflow 或 Agent instance lifecycle。

## 核心结构与主函数

| 函数 | 作用 |
| --- | --- |
| `Server.GetStatus` / `PutStatus` | 读取或更新 Peer runtime status snapshot。 |
| `Server.GetRunAgent` | 读取 Peer 当前保存的 Agent selection。 |
| `Server.SetRunAgent` | 保存新的 Agent selection。 |
| `Server.ResolveRunAgent` | 解析 Peer 当前有效的运行选择。 |
| `Server.ActivateRunAgent` | 激活选择并返回更新后的运行状态。 |

`peerrun` 只保存和解析 selection；真正启动、停止和替换 Agent runtime 由 `agenthost.Service` 完成。
