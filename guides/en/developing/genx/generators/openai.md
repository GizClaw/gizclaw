# OpenAI Adapter

OpenAI Adapter is implemented by `OpenAIGenerator` in the root package and adapts the OpenAI-compatible Chat Completions API to `genx.Generator`.

## Convert boundaries

- Convert prompts, messages, tools and model parameters of `ModelContext` to OpenAI request.
- Convert streaming text, binary content, tool call and finish reason to `MessageChunk` and `State`.
- `Invoke` It is preferred to use JSON Schema structured output, and function tool call can also be used.
- Convert token usage to unified `genx.Usage`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`OpenAIGenerator`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#OpenAIGenerator) | Stores OpenAI client, model, generation parameters and capability flags, and implement Generator. |
| `OpenAIGenerator.GenerateStream` | Initiate streaming chat completion and continue writing to GenX Stream. |
| `OpenAIGenerator.Invoke` | Generate typed FuncCall arguments through structured output or tool call. |
| [`FormatOpenAISchema`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#FormatOpenAISchema) | Normalize generic JSON Schema to OpenAI structured-output schema. |

OpenAI-compatible only means provider protocol compatibility; credentials, endpoint and product model selection are provided by the caller.
