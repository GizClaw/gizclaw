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

`GetDebugMode` / `SetDebugMode` 持久化设备自设的调试权限，缺失时为 `off`。
设置跨断线重连保留，HTTP 鉴权每次读取并在存储失败时拒绝访问。

## OTA 状态

`Server.PutOTAStatus` 使用 SQL 条件更新保存 `peer_runs.ota_json`，防止并发或乱序进度覆盖终态。`GetStatus` 在一次查询中读取状态与 OTA；`PutStatus` 只更新 `status_json` 列，不覆盖 OTA。字段和排序规则见 [Telemetry API](/zh/developing/api/proto/telemetry#ota-上报)。

## SQL 与本机目录

`Server.DB` 借用统一管理的 SQLite 或 PostgreSQL 连接池，启动时调用 `Initialize` 建表和索引，请求路径不执行 DDL。每台 Server 使用自己的运行数据库。

`peer_runs` 按 `public_key` 保存状态；`pending_workspace`、`active_workspace`、`debug_mode` 和 `registered_at` 为独立列。设置 Pending 不覆盖 Active；激活通过单条条件 UPDATE 验证当前选择，避免旧激活覆盖新选择。

`RememberPeer` 记录本机注册或连接过的 Peer。Admin `ListPeers` 通过 `(registered_at, public_key)` 索引在本机分页，再按页内公钥读取共享注册信息。它不列举中心 Redis，也不代表全站设备列表。只有状态而没有本机注册记录的行不进入目录。
