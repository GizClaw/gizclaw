# OpenAI Compatible API

OpenAI Compatible API 面向使用 OpenAI-style client contract 的应用，将 GizClaw Agent、Model 与 Audio 能力暴露为一个有意受限的兼容 surface。它不是 Admin API，也不直接暴露 GizClaw Resource CRUD。

Source：`api/http/openai-compat/v1/service.json`
Go 生成输出：`pkgs/gizclaw/api/openaihttp`

## Endpoints

| Endpoint | 作用 |
| --- | --- |
| `GET /models` | 列出兼容 surface 可使用的 models |
| `POST /chat/completions` | Chat completion 与 streaming response |
| `POST /audio/speech` | Speech synthesis |
| `POST /audio/transcriptions` | Audio transcription |

兼容目标是上述 endpoint 和 payload 的明确 subset，不表示实现全部 OpenAI API。新增字段或 endpoint 必须由 GizClaw 实际能力支持，不能只扩展 schema 而留下 placeholder handler。

该 surface 的 wire models 留在 `openai-compat/v1/service.json`，不因为名称相似就复用 Admin Model Resource 或 Peer RPC payload。Adapter 负责把兼容 request 映射到 GizClaw Agent/GenX services。
