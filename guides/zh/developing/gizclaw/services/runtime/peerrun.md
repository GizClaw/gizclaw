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

## OTA 状态

`Server.PutOTAStatus` 将设备 telemetry 投影保存到独立的 per-peer OTA KV record，使用原子 compare-and-mutate 防止并发或乱序进度覆盖终态。`GetStatus` 将其合并为 `PeerStatus.ota`；普通 `PutStatus` 不写 OTA record，控制响应不能覆盖升级进度。Store 必须支持原子 create-if-absent 与 compare-and-mutate，不支持时返回错误。字段和排序规则见 [Telemetry API](/zh/developing/api/proto/telemetry#ota-上报)。
