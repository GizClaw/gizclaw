# OpenAI Adapter

OpenAI Adapter 由根包的 `OpenAIGenerator` 实现，把 OpenAI-compatible Chat Completions API 适配为 `genx.Generator`。

## 转换边界

- 将 `ModelContext` 的 prompts、messages、tools 和 model parameters 转为 OpenAI request。
- 将 streaming text、binary content、tool call 和 finish reason 转为 `MessageChunk` 与 `State`。
- `Invoke` 优先使用 JSON Schema structured output，也可使用 function tool call。
- 将 token usage 转为统一的 `genx.Usage`。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`OpenAIGenerator`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#OpenAIGenerator) | 保存 OpenAI client、model、生成参数和 capability flags，并实现 Generator。 |
| `OpenAIGenerator.GenerateStream` | 发起 streaming chat completion，并持续写入 GenX Stream。 |
| `OpenAIGenerator.Invoke` | 通过 structured output 或 tool call 生成 typed FuncCall arguments。 |
| [`FormatOpenAISchema`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#FormatOpenAISchema) | 将通用 JSON Schema 规范化为 OpenAI structured-output schema。 |

OpenAI-compatible 只表示 provider protocol compatibility；credential、endpoint 和产品 model selection 由调用方提供。
