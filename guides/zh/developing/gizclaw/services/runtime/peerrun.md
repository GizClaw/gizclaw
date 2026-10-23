# Peer Run

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/gizclaw/services/runtime/peerrun)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun)

`peerrun` 保存 Peer 当前运行状态及其 Agent selection。它拥有 Peer 与运行选择之间的关联，不拥有 Agent definition、Workspace、Workflow 或 Agent instance lifecycle。

`peerrun` 只保存和解析 selection；真正启动、停止和替换 Agent runtime 由 `agenthost.Service` 完成。

`GetDebugMode` / `SetDebugMode` 持久化设备自设的调试权限，缺失时为 `off`。
设置跨断线重连保留，HTTP 鉴权每次读取并在存储失败时拒绝访问。

## 最近连接地址

Peer Run 保存最近一次接受连接的信令来源地址，供 Runtime 在设备离线或 Server
重启后继续读取。连接上线时只更新地址，断线时只更新最后活动时间；状态、OTA、
Workspace 选择和本机目录保持各自的写入边界。Manager 对同一 Peer 的地址写入与
连接发布进行串行协调，旧连接退出不会写回旧地址。来源与读取语义见
[Peer Management](../../peer/manager#连接来源地址)。

## OTA 状态

`Server.PutOTAStatus` 使用 SQL 条件更新保存 `peer_runs.ota_json`，防止并发或乱序进度覆盖终态。`GetStatus` 在一次查询中读取状态与 OTA；`PutStatus` 只更新 `status_json` 列，不覆盖 OTA。字段和排序规则见 [Telemetry API](/zh/developing/api/proto/telemetry#ota-上报)。

## SQL 与本机目录

`Server.DB` 借用统一管理的 SQLite 或 PostgreSQL 连接池，启动时调用 `Initialize` 建表和索引，请求路径不执行 DDL。每台 Server 使用自己的运行数据库。

`peer_runs` 按 `public_key` 保存状态；`pending_workspace`、`active_workspace`、`debug_mode` 和 `registered_at` 为独立列。设置 Pending 不覆盖 Active；激活通过单条条件 UPDATE 验证当前选择，避免旧激活覆盖新选择。

`RememberPeer` 记录本机注册或连接过的 Peer。Admin `ListPeers` 通过 `(registered_at, public_key)` 索引在本机分页，再按页内公钥读取共享注册信息。它不列举中心 Redis，也不代表全站设备列表。只有状态而没有本机注册记录的行不进入目录。

## KV 状态

`PutReportedKV` 使用独立 `peer_reported_kv` 表对每设备整份有界记录做 SQL 比较更新。
该帧所有 KV 条目一起接受或拒绝，包含 key、值、时间和删除墓碑的存储预算在提交前检查。
`GetReportedKV`、`GetStatus` 与 Runtime 读取同一记录；普通 `PutStatus` 忽略传入的 reported_kv，
设备控制响应不能替换自报状态。删除和乱序规则、SDK 字节预算见 [KV 自报状态](/zh/developing/api/proto/telemetry#kv-自报状态)。

[Model 请求参数绑定](../ai#model-request-bindings) 直接读取同一个持久化快照，不新增上报通道或 KV 副本。单次调用的多个参数共用一次读取；之后的调用可观察更新与删除。此读取不把设备自报状态升级为 Server 权威状态。
