# Peer HTTP · API Key

`peer_service_serve_peer_http_api_key.go` 把 `services/system/apikey` 适配到生成的 Peer HTTP 契约。Bearer 鉴权在 strict handler 前完成，认证 Key 的 owner 作为 GizClaw API 和 OpenAI 兼容服务使用的 Peer 身份。

服务在 record 和 credential index 中直接明文保存完整 API Key；有权限的 create、list、get 和 self 操作都返回完整 Key。`manage_api_keys` 允许管理同一 owner 的 Key；任何 Key 都能查看并撤销自己。Peer 退役会先调用 `CleanupPeer`，原子删除所有 Key 并写入 owner retirement marker，再删除 RuntimeProfile owner binding；该 marker 防止已完成清理的 owner 重新创建 Key。

这种可恢复性是明确的 credential-store 信任边界。GizClaw 不 hash 或应用层加密这些 Key，也不引入 KMS：Server 进程、datastore operator 和 backup reader 都处于 credential authority，部署必须保护数据库访问与静态存储。应用接口仍严格限制 owner scope；管理操作只使用现有有界 operation observability，不记录 Key 值；轮换通过创建 replacement 后撤销旧 Key 完成，Peer 退役会撤销它拥有的全部 Key。

已认证的 Peer RPC 连接始终是设备 owner 的根管理权限，通过 `server.api_key.create`、`server.api_key.list` 和 `server.api_key.revoke` 管理 Key。`manage_api_keys` 只把管理能力委派给已签发的 API Key，不会限制或取代 Peer RPC 根方法。

每个 Peer 最多拥有 10 个 API Key，普通 Key 与管理 Key 合并计数。上限由 `apikey.PeerAPIKeyLimit` 固定，容量检查与 record、credential index 和 owner Set 在同一原子 mutation 中提交，因此不同 Server 并发创建也不会超额。已满时 Peer HTTP 返回 `409 API_KEY_LIMIT_REACHED`，Peer RPC 返回 `RESOURCE_EXHAUSTED`（8），reason 为 `API_KEY_LIMIT_REACHED`，不产生新 Key 或索引。撤销 Key 会释放一个名额；已有超额数据保持可读、可撤销，但不能继续新增。

Create、list、revoke 与 Peer cleanup 按 owner 协调。持久化 retirement marker 仍阻止同 owner 的迟到 publication，无关 owner 可在 Store scan 期间继续。只有注入的非线程安全 random source 使用独立短 mutex；全局唯一性仍由原子 KV guard 保证。
