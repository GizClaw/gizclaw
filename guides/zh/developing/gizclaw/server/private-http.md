# Public Client API 开关

`CmdServer.ServeHTTP` 在 Bearer 鉴权前执行 `serve-to-clients`。关闭时，`/gizclaw/v1/*` 和 `/openai/v1/*` 返回 `403 PRIVATE_INGRESS_DENIED`；`/server-info` 与 WebRTC signaling 仍可用于建立连接，不存在 private session 绕过方式。
