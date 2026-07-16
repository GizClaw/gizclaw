# Gemini Adapter

Gemini Adapter is implemented by `GeminiGenerator` in the root package and adapts Google Gemini GenerateContent API to `genx.Generator`.

## Convert boundaries

- Convert `ModelContext` to Gemini contents, system instructions, tools and generation config.
- Convert the text, inline data and function call of Gemini streaming candidate to `MessageChunk`.
- Convert stop, max tokens and safety blocking to unified GenX terminal state.
- `Invoke` Use response schema to generate typed FuncCall arguments.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`GeminiGenerator`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx#GeminiGenerator) | Stores Gemini client, model and calling parameters, and implement Generator. |
| `GeminiGenerator.GenerateStream` | Consumes Gemini streaming candidates and outputs GenX Stream. |
| `GeminiGenerator.Invoke` | Use Gemini response schema to generate FuncCall arguments. |

Gemini-specific content, finish reason and usage only exist inside the Adapter and cannot be spread to Agent or GizClaw service contract.
