# Gemini Adapter

Gemini Adapter 由根包的 `GeminiGenerator` 实现，把 Google Gemini GenerateContent API 适配为 `genx.Generator`。

## 转换边界

- 将 `ModelContext` 转为 Gemini contents、system instructions、tools 和 generation config。
- 将 Gemini streaming candidate 的 text、inline data 和 function call 转为 `MessageChunk`。
- 将 stop、max tokens 和 safety blocking 转为统一的 GenX terminal state。
- `Invoke` 使用 response schema 生成 typed FuncCall arguments。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`GeminiGenerator`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#GeminiGenerator) | 保存 Gemini client、model 与调用参数，并实现 Generator。 |
| `GeminiGenerator.GenerateStream` | 消费 Gemini streaming candidates 并输出 GenX Stream。 |
| `GeminiGenerator.Invoke` | 使用 Gemini response schema 生成 FuncCall arguments。 |

Gemini-specific content、finish reason 和 usage 只存在于 Adapter 内部，不能扩散到 Agent 或 GizClaw service contract。
