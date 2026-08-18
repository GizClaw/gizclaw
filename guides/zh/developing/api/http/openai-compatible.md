# OpenAI Compatible API

GizClaw 通过当前兼容的 `github.com/idy/ai-server-shell` module 暴露有意受限的 OpenAI-compatible surface。AI Server Shell 拥有标准 OpenAI routing、32 MiB body limit、OpenAPI request/response validation、request ID、error envelope、SSE framing、cancellation 与 transport cleanup。GizClaw 不再维护第二份 OpenAI OpenAPI 文档或 generated wire package。

## 支持路由

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/audio/speech`
- `POST /v1/audio/transcriptions`
- `GET /v1/voices`，位于 Shell handler 外的 GizClaw 扩展

普通 authenticated Server/Edge 入口把这些路径挂到 `/openai` 下；可靠的 `ServicePeerOpenAI` service 直接暴露这些路径。Shell router 前有 exact method/path gate；其他任何路由，包括 Shell 已知但 GizClaw 未支持的路由，都会在既有 ingress authentication boundary 之后返回 `404`。

## 产品边界

组合层为每个请求绑定已验证的 Peer identity、RuntimeProfile-scoped model/voice resources，以及对应 GenX Generator/Transformer。Shell authenticator 只读取该 verified binding；传入 bearer、cookie 或 dummy OpenAI API key 都不会被信任或转发。

`services/ai/openaiapi` 仅通过 `ai-server-shell/backend` 实现 `models/listModels`、`chat/createChatCompletion`、`audio/createSpeech` 与 `audio/createTranscription`。不支持的 option 会被显式拒绝，不会被静默宣称已执行。Chat 额外消费 GizClaw 的 `thinking.enabled` 与 `thinking.level` option。Speech streaming 使用官方 `stream_format: "sse"`；原有非标准 `stream` alias 由 schema validation 拒绝。

JSON、binary 与 ordered SSE response 均由 Shell 拥有并 framing。GenX stream 会在完成、失败、取消或 downstream disconnect 时关闭。系统不注册 Realtime 或 Responses WebSocket backend。

`go.mod` 与 `go.sum` 中的具体 Shell revision 保证单次 build 可复现。兼容更新通过既有每周 Go module Dependabot flow 推进；GizClaw 不 vendor、replace 或复制上游 compatibility profile。
