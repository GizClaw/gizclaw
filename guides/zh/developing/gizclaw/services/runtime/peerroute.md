# Peer Route

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerroute)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute)

`peerroute` 在配置的共享 Peer Store 中保存每个 Client Peer 固定的 home Server assignment。同一拓扑中的所有 Server 都能读取这个控制面目录；它不提供自动 reassignment、Server failover、Workspace ownership 或 Workspace routing。

`Lookup` 与 `Resolve` 只读。`Assign` 只在 assignment 不存在时创建 version 1；同一个 Server owner 可以原子刷新 endpoint 或 role metadata 并递增 version，其他 Server 一律得到 conflict，提供 `expected_version` 也不能转移 ownership。

Client activation performs the fixed-home claim or verification before Manager publishes the connection. Edge-tunneled clients also perform this admission before the tunnel acceptance result; a foreign owner is returned to the Edge for bounded redirection to a configured Server. The observed assignment accompanies the ownership conflict, avoiding a second potentially missing reader lookup. Direct foreign-home connections remain rejected. RPC, HTTP, packet, audio, Agent, and PeerRun work start only after normal connection activation.

Route assignment、在线 connection 与持久化 Peer 仍是三个独立状态；存在 assignment 不表示 owner Server 或 Peer 当前可达。

Assignments and Peer admission/deletion fences follow the [configured KV read policy](../../../stores/kv). Ordinary queries accept stale routes, active status and missing results without a primary refresh. With no explicit version, `Assign` reuses a matching existing home without a writer transaction. Missing assignments are claimed atomically; metadata changes and explicit-version no-ops compare writer conditions. A rejected condition permits bounded refresh. When activation has just confirmed a Peer but route admission cannot see it, Manager refreshes only that bootstrap update. These paths never transfer ownership or relax an explicit version.
