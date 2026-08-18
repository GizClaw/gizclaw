# API Key

GizClaw 使用长期有效、绑定设备的 API Key 访问公开的 GizClaw API 和 OpenAI 兼容 HTTP API。完成注册的设备先通过已认证的 Peer RPC 连接调用 `server.api_key.create` 创建 Key。完整的 `gizclaw_sk_v1_...` secret 只返回一次，请安全保存。

访问 `/gizclaw/v1/*` 和 `/openai/v1/*` 时发送 `Authorization: Bearer <api-key>`，不再需要 public-key header 或 login 交换。

普通 Key 可以使用公开 API，通过 `GET /gizclaw/v1/api-keys/self` 查看自己，并通过 `DELETE /gizclaw/v1/api-keys/self` 撤销自己。带 `manage_api_keys: true` 的 Key 还可以通过 `/gizclaw/v1/api-keys` 创建、列举、查看和撤销同一设备的其他 Key。

API Key 不会自动过期；Key 丢失或不再使用时应主动撤销。删除 Peer 会撤销该 Peer 拥有的全部 API Key。
