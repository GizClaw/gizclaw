# OpenAI Compatible API

GizClaw exposes an intentionally limited OpenAI-compatible surface through the current compatible `github.com/idy/ai-server-shell` module. AI Server Shell owns standard OpenAI routing, the 32 MiB body limit, OpenAPI request and response validation, request IDs, error envelopes, SSE framing, cancellation, and transport cleanup. GizClaw does not keep a second OpenAI OpenAPI document or generated wire package.

## Supported routes

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/audio/speech`
- `POST /v1/audio/transcriptions`
- `GET /v1/voices`, a GizClaw extension outside the Shell handler

The ordinary authenticated Server and Edge entry mounts these paths below `/openai`; the reliable `ServicePeerOpenAI` service exposes them directly. An exact method/path gate runs before the Shell router. Every other route, including other routes known by the Shell, returns `404` after the existing ingress authentication boundary.

## Product boundary

The composition layer binds the already verified Peer identity, RuntimeProfile-scoped model and voice resources, and the matching GenX Generator and Transformer to each request. The Shell authenticator reads only this verified binding. It ignores and never forwards an incoming bearer value, cookie, or dummy OpenAI API key.

`services/ai/openaiapi` implements only `models/listModels`, `chat/createChatCompletion`, `audio/createSpeech`, and `audio/createTranscription` through `ai-server-shell/backend`. Unsupported options are rejected instead of being silently claimed. Chat additionally consumes the GizClaw `thinking.enabled` and `thinking.level` option. Speech streaming uses the official `stream_format: "sse"`; the former non-standard `stream` alias is rejected by schema validation.

JSON, binary, and ordered SSE responses are owned and framed by the Shell. GenX streams are closed on completion, failure, cancellation, or downstream disconnect. No Realtime or Responses WebSocket backend is registered.

The concrete Shell revision in `go.mod` and `go.sum` makes each build reproducible. Compatible updates advance through the normal weekly Go module Dependabot flow; GizClaw does not vendor, replace, or copy the upstream compatibility profile.
