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

Streaming `finish_reason: content_filter` maps to `StatusBlocked`, separately
from `length`/`StatusTruncated` and network or decoding errors. When the provider
supplies no refusal text, the error identifies
`provider finish_reason=content_filter; refusal detail was not supplied`.
An absent explanation is neither a normal completion nor evidence of a specific
filtering rule.

## Doubao audio input

`pkgs/genx/generators/doubaochat` wraps the `OpenAIGenerator` of a Volc Ark Doubao chat completions Model. Doubao Seed chat Models accept audio input, but the API returns no separate input-transcript field. The adapter merges each user message's audio Blobs: raw Opus packets, Ogg/Opus, and signed 16-bit PCM are decoded to 16 kHz mono WAV; MP3 and a single WAV pass through.

When the latest user message carries audio, the adapter starts two independent requests to the same Model in parallel. The reply request keeps the caller's prompts, history, Tools, CoT and parameters, replacing only the converted audio; it adds no ASR instruction. The transcription request contains only the current audio and the adapter's transcription-only prompt, without application prompts, history, Tools or CoT. It requests a JSON object with exactly one string field, `{"transcript":"verbatim spoken words"}`; an empty string means no speech.

Callers still receive one `genx.Stream`. Reply chunks pass through unchanged; the adapter never parses, inserts or removes `<asr>` text in the business reply. The transcript is published as a `RoleUser` sideband labelled `genx.InputTranscriptLabel`, built by `genx.NewInputTranscriptChunk`. The requests start and publish independently: first reply text does not wait for ASR, and ASR does not wait for the reply to finish. Normal outer-stream completion waits for both requests. Callers need no transcription prompt and need not change their visible reply format. Both upstream requests record their own usage; the successful terminal `State.Usage()` returns their sum.

Transcription has a 30-second timeout, requests 2048 completion tokens, and reads at most 64 KiB of text. Missing, duplicate or extra fields, non-string values, invalid JSON and upstream errors explicitly fail the outer stream; reply text is never substituted for a transcript. Failure of either request, caller cancellation or closing the stream cancels its sibling and releases both upstream streams. The adapter makes no extra recovery or retry request.

`Generator.TranscribeInput` reuses the same audio conversion, isolated transcription request and cancellation boundaries, returning only the transcript and usage without starting a reply request. Graph input integrations use it when current user text must be available before executing the original business prompt.

Text-only requests pass through unchanged, and `Invoke` only converts audio. peergenx installs the adapter for Volc `chat_completions` Models whose `support_text_only` is not true, and `Service.AcceptsAudioInput` answers "does this Model accept audio input" with the same condition; the Eino factory uses it to decide the [audio input path](/en/developing/gizclaw/services/ai#eino-audio-input-path).

The standard Docker E2E document `eino-lite-native-audio.asr-roundtrip.giztest.yaml` uses `doubao-lite-audio-chat` (`doubao-seed-2-1-lite-260915`). Its Workflow declares no application prompt, `asr_model` or reply Voice. Giztest synthesizes a fixed input recording, then asserts `AUDIO_INPUT_PATH_MODEL`, a correct independent transcript, a nonempty Lite reply, and no ASR tags in the reply. Input synthesis only prepares the recording; the Lite adapter's independent request performs transcription.
