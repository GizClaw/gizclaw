# Peer HTTP · API Key

`peer_service_serve_peer_http_api_key.go` 把 `services/system/apikey` 适配到生成的 Peer HTTP 契约。Bearer 鉴权在 strict handler 前完成，认证 Key 的 owner 作为 GizClaw API 和 OpenAI 兼容服务使用的 Peer 身份。

服务在 record 和 credential index 中直接明文保存完整 API Key；有权限的 create、list、get 和 self 操作都返回完整 Key。`manage_api_keys` 允许管理同一 owner 的 Key；任何 Key 都能查看并撤销自己。Peer 退役会先调用 `CleanupPeer`，原子删除所有 Key 并写入 owner retirement marker，再删除 RuntimeProfile owner binding；该 marker 防止已完成清理的 owner 重新创建 Key。

已认证的 Peer RPC 连接始终是设备 owner 的根管理权限，通过 `server.api_key.create`、`server.api_key.list` 和 `server.api_key.revoke` 管理 Key。`manage_api_keys` 只把管理能力委派给已签发的 API Key，不会限制或取代 Peer RPC 根方法。
