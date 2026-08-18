# OpenAI Compatible API

GizClaw 通过当前兼容的 `github.com/idy/ai-server-shell` module 暴露有意受限的 OpenAI-compatible surface。AI Server Shell 拥有标准 OpenAI routing、32 MiB body limit、OpenAPI request/response validation、request ID、error envelope、SSE framing、cancellation 与 transport cleanup。GizClaw 不再维护第二份 OpenAI OpenAPI 文档或 generated wire package。

## 支持路由

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/audio/speech`
- `POST /v1/audio/transcriptions`
- `POST /v1/conversations`
- `GET /v1/conversations/{conversation_id}`
- `GET /v1/conversations/{conversation_id}/items`
- `GET /v1/conversations/{conversation_id}/items/{item_id}`
- `POST /v1/responses`
- `GET /v1/responses/{response_id}`
- `GET /v1/responses/{response_id}/input_items`
- `POST /v1/responses/{response_id}/cancel`
- `GET /v1/voices`，位于 Shell handler 外的 GizClaw 扩展

普通 authenticated Server/Edge 入口把这些路径挂到 `/openai` 下；可靠的 `ServicePeerOpenAI` service 直接暴露这些路径。Shell router 前有 exact method/path gate；其他任何路由，包括 Shell 已知但 GizClaw 未支持的路由，都会在既有 ingress authentication boundary 之后返回 `404`。

## 产品边界

组合层为每个请求绑定已验证的 Peer identity、RuntimeProfile-scoped model/voice resources，以及对应 GenX Generator/Transformer。Shell authenticator 只读取该 verified binding；传入 bearer、cookie 或 dummy OpenAI API key 都不会被信任或转发。

`services/ai/openaiapi` 还把一个 Conversation 映射到一个新建的 caller-owned Workspace。创建必须提供字符串 metadata `collection` 与 `workflow_name`，后者也是兼容 Response model。Conversation item 精确关联权威 Workspace History；生命周期与不可变 input snapshot 位于 Workspace runtime prefix 下。读取不会启动 Agent；Response 只在该 turn 内 attach 共享 Workspace Agent，支持前台 JSON/SSE 和后台取消，并且不改变 PeerRun selection。

Responses 只接受一个非空文本 user turn。音频兼容路径由 Audio Transcriptions、文本 Response 和 Audio Speech 组合；不支持 raw Responses audio、图片、文件、tool、provider policy override、beta route、未知 query key 与 list-Conversations。Workspace/Conversation 枚举仍使用 `server.workspace.list`。

不支持的 option 会在 mutation 前显式拒绝，不会被静默宣称已执行。Chat 额外消费 GizClaw 的 `thinking.enabled` 与 `thinking.level` option。Speech streaming 使用官方 `stream_format: "sse"`；原有非标准 `stream` alias 由 schema validation 拒绝。

JSON、binary 与 ordered SSE response 均由 Shell 拥有并 framing。GenX stream 会在完成、失败、取消或 downstream disconnect 时关闭。系统不注册 Realtime 或 Responses WebSocket backend。

`go.mod` 与 `go.sum` 中的具体 Shell revision 保证单次 build 可复现。兼容更新通过既有每周 Go module Dependabot flow 推进；GizClaw 不 vendor、replace 或复制上游 compatibility profile。
