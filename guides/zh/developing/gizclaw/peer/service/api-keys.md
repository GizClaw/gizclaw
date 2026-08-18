# Peer HTTP · API Key

`peer_service_serve_peer_http_api_key.go` 把 `services/system/apikey` 适配到生成的 Peer HTTP 契约。Bearer 鉴权在 strict handler 前完成，认证 Key 的 owner 作为 GizClaw API 和 OpenAI 兼容服务使用的 Peer 身份。

服务分别保存公开 metadata 和 SHA-256 digest 索引，明文 secret 只在创建时返回。`manage_api_keys` 允许管理同一 owner 的 Key；任何 Key 都能查看并撤销自己。Peer 退役会先调用 `CleanupPeer`，原子删除所有 Key 并写入 owner retirement marker，再删除 RuntimeProfile owner binding；该 marker 防止已完成清理的 owner 重新创建 Key。
