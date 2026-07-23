# OpenAI Compatible API

The OpenAI Compatible API is intended for applications using the OpenAI-style client contract, exposing GizClaw Agent, Model, and Audio capabilities as an intentionally limited compatible surface. It is not an Admin API and does not directly expose the GizClaw Resource CRUD.

Source:`api/http/openai-compat/v1/service.json`
Go generated output: `pkgs/gizclaw/api/openaihttp`

See the [API Reference](/api/) for exact endpoints, parameters, requests, and responses. Compatibility is limited to the endpoints and payloads listed in the Reference; it does not mean that all OpenAI APIs are implemented. New fields or endpoints must be supported by actual GizClaw capabilities and cannot leave placeholder handlers behind.

The wire models of this surface remain in `openai-compat/v1/service.json`, and the Admin Model Resource or Peer RPC payload is not reused because of similar names. Adapter is responsible for mapping compatible requests to GizClaw Agent/GenX services.
