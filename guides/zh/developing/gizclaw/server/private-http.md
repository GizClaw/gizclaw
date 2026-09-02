# 仅限 Edge 的 Public Client API

`CmdServer.ServeHTTP` 始终在 Bearer 鉴权前拒绝直接访问 `/gizclaw/v1/*` 和 `/openai/v1/*`，返回 `403 PRIVATE_INGRESS_DENIED`。`/server-info` 与 WebRTC signaling 仍可用于建立连接。Edge 通过 `ServiceEdgeHTTP` 到达业务 handler；不存在 direct HTTP 或 private session 绕过方式。
