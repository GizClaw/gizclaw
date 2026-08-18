# OpenAI Compatible API

GizClaw exposes an intentionally limited OpenAI-compatible surface through the current compatible `github.com/idy/ai-server-shell` module. AI Server Shell owns standard OpenAI routing, the 32 MiB body limit, OpenAPI request and response validation, request IDs, error envelopes, SSE framing, cancellation, and transport cleanup. GizClaw does not keep a second OpenAI OpenAPI document or generated wire package.

## Supported routes

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
- `GET /v1/voices`, a GizClaw extension outside the Shell handler

The ordinary authenticated Server and Edge entry mounts these paths below `/openai`; the reliable `ServicePeerOpenAI` service exposes them directly. An exact method/path gate runs before the Shell router. Every other route, including other routes known by the Shell, returns `404` after the existing ingress authentication boundary.

## Product boundary

The composition layer binds the verified Peer identity, RuntimeProfile-scoped model and voice resources, and the matching GenX Generator and Transformer to each request. Direct and Edge `/openai/v1/*` first authenticate a GizClaw API key and derive the binding from its owner; `ServicePeerOpenAI` continues to use reliable Peer connection identity. The Shell authenticator reads only this composition-layer binding and never forwards a bearer value or cookie to a model provider.

`services/ai/openaiapi` also maps one Conversation to one newly created caller-owned Workspace. Creation requires string metadata `collection` and `workflow_name`; the latter is the compatible Response model. Conversation items correlate to authoritative Workspace History, while lifecycle and immutable input snapshots are stored below the Workspace runtime prefix. Reads never start an Agent. A Response attaches the shared Workspace Agent only for that turn, supports foreground JSON/SSE or background cancellation, and leaves PeerRun selection unchanged.

Responses accept one non-empty text user turn only. Audio compatibility is composed through Audio Transcriptions, a text Response, and Audio Speech; raw Responses audio, images, files, tools, provider policy overrides, beta routes, unknown query keys, and list-Conversations are unsupported. `server.workspace.list` remains the Workspace/Conversation enumeration surface.

Unsupported options are rejected before mutation instead of being silently claimed. Chat additionally consumes the GizClaw `thinking.enabled` and `thinking.level` option. Speech streaming uses the official `stream_format: "sse"`; the former non-standard `stream` alias is rejected by schema validation.

JSON, binary, and ordered SSE responses are owned and framed by the Shell. GenX streams are closed on completion, failure, cancellation, or downstream disconnect. No Realtime or Responses WebSocket backend is registered.

The concrete Shell revision in `go.mod` and `go.sum` makes each build reproducible. Compatible updates advance through the normal weekly Go module Dependabot flow; GizClaw does not vendor, replace, or copy the upstream compatibility profile.
