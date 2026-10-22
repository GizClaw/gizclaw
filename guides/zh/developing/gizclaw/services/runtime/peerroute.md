# Peer Route

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerroute)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` 在配置的共享 Peer Store 中保存每个 Client Peer 固定的 home Server assignment。同一拓扑中的所有 Server 都能读取这个控制面目录；它不提供自动 reassignment、Server failover、Workspace ownership 或 Workspace routing。

`Lookup` 与 `Resolve` 只读。`Assign` 只在 assignment 不存在时创建 version 1；同一个 Server owner 可以原子刷新 endpoint 或 role metadata 并递增 version，其他 Server 一律得到 conflict，提供 `expected_version` 也不能转移 ownership。

Client activation 会在 Manager 发布 connection 之前执行 claim 或校验。归属其他 Server 的 direct 或 Edge-tunneled connection 会在 RPC、HTTP、packet、audio、Agent 或 PeerRun 工作开始前被拒绝。Route assignment、在线 connection 与持久化 Peer 仍是三个独立状态；存在 assignment 不表示 owner Server 或 Peer 当前可达。

Assignment 与 Peer 准入/删除围栏使用 [KV 强读](../../../stores/kv)，即使 connector 配置了 Redis 副本或 TiKV 地区 Learner 的陈旧读。Route resolve/assign 在读取 Peer 状态前检查 pending-deletion 围栏；副本中的旧 active 状态不能重新授权已删除的 Peer。
