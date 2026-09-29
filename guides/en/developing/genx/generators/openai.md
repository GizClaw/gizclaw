# OpenAI Adapter

OpenAI Adapter is implemented by `OpenAIGenerator` in the root package and adapts the OpenAI-compatible Chat Completions API to `genx.Generator`.

## Convert boundaries

- Convert prompts, messages, tools and model parameters of `ModelContext` to OpenAI request. Model text followed by tool calls becomes one assistant message carrying every call, and a tool's declared `Parameters` and `Strict` are sent unchanged.
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

## Doubao audio input

`pkgs/genx/generators/doubaochat` wraps the `OpenAIGenerator` of a Volc Ark Doubao chat completions Model. Doubao Seed chat Models accept audio input, but the API returns no transcript of it. The adapter merges the audio Blobs of each user message into one Blob the API accepts: raw Opus packets, Ogg/Opus, and signed 16-bit PCM are decoded to 16 kHz mono WAV, and MP3 and a single WAV pass through. When the latest user message carries audio, it appends one system instruction: write one complete sentence of the reply first, then one separate line `<asr>verbatim transcript</asr>`, then continue the reply, so the first reply text does not wait for the transcript. The adapter cuts the first `<asr>` segment out of the streamed text wherever it appears, holding back only trailing characters that could start the opening tag, and emits one `RoleUser` chunk labelled `genx.InputTranscriptLabel` (built by `genx.NewInputTranscriptChunk`) when the segment closes. The transcript ends at the first closing tag or line break, tolerating misspellings such as `</asr]`, and a segment still open at the end of the stream is reported as the transcript. A reply without the segment is passed through with a warning log and no transcript. Text-only requests pass through unchanged, and `Invoke` only converts audio. peergenx installs the adapter for Volc `chat_completions` Models whose `support_text_only` is false.
