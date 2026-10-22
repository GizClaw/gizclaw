# Gemini Adapter

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx)

Gemini Adapter 由根包的 `GeminiGenerator` 实现，把 Google Gemini GenerateContent API 适配为 `genx.Generator`。

## 转换边界

- 将 `ModelContext` 转为 Gemini contents、system instructions、tools 和 generation config。
- 将 Gemini streaming candidate 的 text、inline data 和 function call 转为 `MessageChunk`。
- 将 stop、max tokens 和 safety blocking 转为统一的 GenX terminal state。
- `Invoke` 使用 response schema 生成 typed FuncCall arguments。

Gemini-specific content、finish reason 和 usage 只存在于 Adapter 内部，不能扩散到 Agent 或 GizClaw service contract。
