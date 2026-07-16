# OpenAI Compatible API

The OpenAI Compatible API is intended for applications using the OpenAI-style client contract, exposing GizClaw Agent, Model, and Audio capabilities as an intentionally limited compatible surface. It is not an Admin API and does not directly expose the GizClaw Resource CRUD.

Source:`api/http/openai-compat/v1/service.json`
Go generated output: `pkgs/gizclaw/api/openaihttp`

## Endpoints

| Endpoint | Function |
| --- | --- |
| `GET /models` | List the models that are compatible with the surface and can be used |
| `POST /chat/completions` | Chat completion and streaming response |
| `POST /audio/speech` | Speech synthesis |
| `POST /audio/transcriptions` | Audio transcription |

Compatibility targets are a clear subset of the above endpoints and payloads, and do not mean that all OpenAI APIs are implemented. The new fields or endpoints must be supported by the actual capabilities of GizClaw. They cannot just extend the schema and leave the placeholder handler.

The wire models of this surface remain in `openai-compat/v1/service.json`, and the Admin Model Resource or Peer RPC payload is not reused because of similar names. Adapter is responsible for mapping compatible requests to GizClaw Agent/GenX services.
