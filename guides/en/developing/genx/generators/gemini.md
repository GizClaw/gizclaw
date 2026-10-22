# Gemini Adapter

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx)

Gemini Adapter is implemented by `GeminiGenerator` in the root package and adapts Google Gemini GenerateContent API to `genx.Generator`.

## Convert boundaries

- Convert `ModelContext` to Gemini contents, system instructions, tools and generation config.
- Convert the text, inline data and function call of Gemini streaming candidate to `MessageChunk`.
- Convert stop, max tokens and safety blocking to unified GenX terminal state.
- `Invoke` Use response schema to generate typed FuncCall arguments.

Gemini-specific content, finish reason and usage only exist inside the Adapter and cannot be spread to Agent or GizClaw service contract.
