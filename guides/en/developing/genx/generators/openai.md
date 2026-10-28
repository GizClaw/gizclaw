# OpenAI Adapter

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx)

OpenAI Adapter is implemented by `OpenAIGenerator` in the root package and adapts the OpenAI-compatible Chat Completions API to `genx.Generator`.

## Convert boundaries

- Convert prompts, messages, tools and model parameters of `ModelContext` to OpenAI request. Model text followed by tool calls becomes one assistant message carrying every call, and a tool's declared `Parameters` and `Strict` are sent unchanged.
- Convert streaming text, binary content, tool call and finish reason to `MessageChunk` and `State`.
- `Invoke` It is preferred to use JSON Schema structured output, and function tool call can also be used.
- Convert token usage to unified `genx.Usage`.

OpenAI-compatible only means provider protocol compatibility; credentials, endpoint and product model selection are provided by the caller.

Call `ModelParams.SetTemperature(0)` to configure zero explicitly; `HasTemperature()` reports whether temperature is configured. A JSON `"temperature": 0` survives decoding, parameter merging, re-encoding, and request serialization. An absent or `null` field creates no explicit value. For compatibility, `ModelParams{Temperature: 0}` in existing Go struct literals remains unspecified. Explicit temperature overrides the Generator default while other unspecified parameters continue to inherit their defaults.

Streaming `finish_reason: content_filter` maps to `StatusBlocked`, separately
from `length`/`StatusTruncated` and network or decoding errors. When the provider
supplies no refusal text, the error identifies
`provider finish_reason=content_filter; refusal detail was not supplied`.
An absent explanation is neither a normal completion nor evidence of a specific
filtering rule.

`OpenAIGenerator.RequestHeaders` is an optional process-local HTTP metadata callback. Each GenerateStream or Invoke evaluates it once before sending, clones its result, and uses SDK per-request options. SDK retries and compatibility fallbacks reuse the snapshot. The callback does not mutate shared client or Generator headers or enter ModelContext. GenX does not interpret Peers, Runtime KV, Model resources, or credential stores; see [AI service wiring](../../gizclaw/services/ai#model-request-bindings). With the callback enabled, provider errors expose only safe summaries while retaining typed causes in process.

Safe summaries also cover streamed refusal and `content_filter` terminal errors so upstreams cannot echo protected headers publicly. The error chain retains the `State` blocked/truncated status and usage; normal completion and context cancellation keep their existing semantics. Without the callback, existing refusal diagnostics remain available.

## Audio input

`OpenAIGenerator` converts audio Blobs of one user message into API input: raw Opus, Ogg/Opus and signed 16-bit PCM become 16 kHz mono WAV; MP3 and fragments of one WAV file are reassembled in order; multiple complete WAV files are not concatenated. Equivalent MIME case, whitespace, and parameter order do not split a recording; semantically different parameters remain distinct. Text, application prompts, history and Tools retain their original meaning.

The Generator performs only the requested model generation. [AudioDock](../agentkit#audio-dock) runs transcription through an independent ASR Transformer; RuntimeProfile selects the conversation Model and ASR Model separately.

The standard Docker E2E document `eino-lite-native-audio.asr-roundtrip.giztest.yaml` sends the recording directly to the audio Model while Profile-selected SAUC produces its transcript. The Workflow provides no transcription prompt.
