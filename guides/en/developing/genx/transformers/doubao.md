# Doubao Speech Adapter

Doubao Speech Adapter adapts Doubao speech protocol to `genx.Transformer`, covering one-way recognition, speech generation, real-time dialogue, duplex real-time dialogue and voice translation.

Each adapter uses a package-owned typed constructor:

```go
doubaoasr.New(doubaoasr.Config{Client: client})
doubaotts.NewSeedV2(doubaotts.SeedV2Config{Client: client, Speaker: speaker})
doubaotts.NewICLV2(doubaotts.ICLV2Config{Client: client, Speaker: speaker})
doubaoast.New(doubaoast.Config{Client: client})
doubaorealtime.New(doubaorealtime.Config{Client: client})
doubaorealtimeduplex.New(doubaorealtimeduplex.Config{Client: client})
```

Constructors do not open provider sessions; each concurrent `Transform` call owns its session and runtime state. ASR, TTS, AST, and Realtime Dialogue do not accept Toolkit configuration. Realtime Duplex is agent-capable because its provider protocol supports function-call output continuation.

## Abilities

| Transformer | Input and Output |
| --- | --- |
| `doubaoasr.Transformer` | Audio Stream → transcription Stream. |
| `doubaotts.SeedV2` | Text Stream → generated audio Stream. |
| `doubaotts.ICLV2` | Text Stream → ICL voice audio Stream. |
| `doubaorealtime.Transformer` | Adapts the Doubao Realtime Dialogue API (`volc.speech.dialog`), explicitly handles ASR, Chat, and TTS events, and supports Push-to-Talk, continuous voice, and text input. |
| `doubaorealtimeduplex.Transformer` | Adapts the independent Realtime Duplex API, which handles continuous duplex audio and its transcription, response text/audio, function call, and response cancellation events. |
| `doubaoast.Transformer` | Speech input → translated text/audio Stream. |

Each Transformer's typed Config defines stable configuration, while the context passed to `Transform` controls one request's lifecycle. The Adapter must internally convert provider events, audio formats, usage, terminal states, and errors to GenX Stream.

Every output route or MIME channel created by a Doubao Transformer has an explicit lifecycle owned by that Transformer. ASR transcripts and history audio, TTS audio, AST transcript/translation/history/audio, and Realtime transcript/assistant text/audio each emit BOS before or with their first data and one matching EOS, including empty and error completions. Input or unrelated routes that the Transformer does not create remain pass-through boundaries and are not repaired or closed on another component's behalf.

### ASR empty recognition

When a Doubao ASR provider session completes normally without non-whitespace final result text or definite utterance text, `doubaoasr.Transformer` completes that recognition successfully without emitting recognized transcript text. A zero-content terminal chunk required by the existing Stream route remains a successful internal boundary rather than recognized user text.

After an explicit audio EOS, `doubaoasr.Transformer` waits at most one minute for provider finalization. If the provider remains silent, the transformer closes that provider session and terminates the output stream with a `doubao asr: finalization timeout` error instead of inheriting the caller's longer lifecycle deadline.

An interim transcript route that never receives a definite result remains an error. Provider, protocol, cancellation, interrupted-input, malformed-audio, unsupported-format failures, and timeouts other than the exception below retain their normal error propagation.

Continuous ASR with `EmitInterim` sends every audio frame immediately as a non-final packet to the current healthy SAUC session. An explicit audio EOS sends the terminal marker, finishes that provider session, and leaves the outer Transformer stream open; the next audio route creates a replacement session and binds its transcript independently. A provider packet-wait timeout while finalizing is the expected recoverable post-route boundary. Other provider errors and the one-minute local finalization timeout terminate the outer stream.

### Seed V2 empty audio

`doubaotts.SeedV2` treats a successful provider terminal as successful synthesis only after a non-empty readable segment has emitted at least one normalized audio byte. A successful final-only provider stream, or any otherwise successful stream that emits zero normalized audio bytes, terminates the affected audio route with `doubaotts: seed v2 completed without audio` instead of a successful audio EOS.

Provider, protocol, context-cancellation, normalizer, and downstream emission errors keep their original error identity. The adapter does not retry the request, switch the configured Voice, or synthesize replacement content. Shared TTS and Audio Dock propagate the resulting terminal error through their existing route lifecycle.

## AST Translate input modes

`doubaoast.Transformer` supports realtime and Push-to-Talk audio input while keeping provider upload and event reception concurrent:

| Mode | Output boundary |
| --- | --- |
| Realtime | Normalized transcript, translation, and TTS chunks are forwarded as provider events arrive. |
| Push-to-Talk | Provider events are drained while input is active, but normalized transcript, translation, history, and TTS chunks remain unpublished until the matching input audio EOS. |

For Push-to-Talk, input audio EOS commits the unpublished chunks once in their original order. A provider failure recorded before that commit discards the entire unpublished turn and returns the provider error without exposing retained data or control chunks. The commit gate is scoped to the input StreamID and provider session epoch so late events from an interrupted session cannot affect a reused StreamID.

Unpublished assistant TTS output is limited to two minutes of normalized Opus packet duration per turn. Exceeding the limit discards the unpublished turn and emits one error EOS for that StreamID without closing the shared transformer output; input and history audio do not count toward this limit.

## Two sets of Realtime API

| Boundary | Realtime Dialogue | Realtime Duplex |
| --- | --- | --- |
| Go Adapter | `doubaorealtime.Transformer` | `doubaorealtimeduplex.Transformer` |
| Provider session | `Client.Realtime.Connect` | `Client.RealtimeDuplex.OpenSession` |
| Input method | Push-to-Talk, continuous realtime, text | Continuous full-duplex audio |
| Provider events | ASR, Chat, TTS, Session | Transcription, Response text/audio, Function call, Session |
| Interrupt operation | `Interrupt` | `CancelResponse` |
| Tool result | Not provided by this session contract | `SendFunctionCallOutputs` |

The two Adapters can share GenX Stream, audio conversion, StreamID, and lifecycle infrastructure, but cannot merge provider session interface or event mapping. Push-to-Talk belongs only to the Realtime Dialogue API and should not be emulated by the Realtime Duplex Adapter.

## Realtime Duplex function-tool continuation

```go
transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
    Client:       client,
    ToolInvoker: runtimeTools,
    MaxToolCalls: 32,
})
```

When `ToolInvoker` is non-nil, every `Transform` resolves the current function names, descriptions, and JSON Schemas before opening its provider session. Realtime Duplex function calls execute in provider order through `InvokeTool(name, arguments)`. Each raw JSON result is sent immediately with the original provider call ID so the provider can continue the conversation. ToolCall and ToolResult control data remain internal and never enter the public GenX Stream.

The Transformer owns provider call IDs, ordering, duplicate-ID rejection, and the per-invocation `MaxToolCalls` budget. Zero uses 32 and negative values are rejected. Independent concurrent `Transform` calls have separate call-ID sets and budgets, even when they share one invoker. A nil invoker advertises no tools; a provider function call in that state is a transform error.

Resolution, invocation, invalid result JSON, result submission, cancellation, duplicate-ID, and budget errors terminate only the affected Transform. The injected invoker owns runtime resource lookup, authorization, argument validation, and executor dispatch. Realtime Dialogue remains unchanged because its session contract has no function-result continuation operation.

## Realtime Dialogue input mode

`doubaorealtime.Transformer` supports three input modes:

| Mode | Input Boundaries |
| --- | --- |
| Push-to-Talk | BOS starts a push-to-talk, the audio chunks belong to the current turn, EOS ends the input and triggers `EndASR`. |
| Realtime | Continuously sends audio, and the user utterance is divided by provider VAD; entering EOS only closes the local segment. |
| Text | Sends text chunks, does not accept audio input. |

`Config.Model` is required and is never inferred by the transformer. `Config.Instructions` is the semantic initial dialogue instruction. GizClaw passes it unchanged to `doubao-speech-go`; the SDK maps it to `dialog.system_role` for O20 or `dialog.character_manifest` for SC20 after model normalization. Exact `SystemRole`, `SpeakingStyle`, and `CharacterManifest` settings remain independent advanced fields and are validated by the SDK. The adapter does not copy semantic instructions into `prompt.system`, and it does not inject an O-only `BotName` into SC20 sessions.

One provider response owns one spoken-text route and one audio route. Non-empty sentence text from TTS-start event `350` is canonical and is emitted once per synthesized sentence; Chat event `550` text is buffered and used only when the response finishes without any TTS sentence text. The first TTS start or audio payload emits one audio BOS, later sentence starts reuse it, TTS finish emits one audio EOS, and the selected text source emits one text EOS. Failures and interruptions discard buffered, unspoken Chat text rather than publishing it as a successful response.

The transformer owns the long-lived lifecycle. It starts connecting when `Transform` starts and reuses one healthy Realtime Dialogue session across ordinary input turns and BOS/EOS boundaries. A Realtime-mode BOS that interrupts an active response closes that provider session locally and immediately opens a replacement with the same configured instructions, model, and `DialogID`; it does not send the Push-to-Talk-only `ClientInterrupt` event. Unread audio for the new route is consumed only by the replacement. Once a Realtime response starts, one minute without provider progress is treated as provider loss: open transcript or assistant routes receive an error EOS, the stalled session is closed, and reconnect begins. A provider terminal event, transport error, or session I/O error follows the same replacement path with capped exponential backoff. Attempts continue until the transform context or output stream ends.

Input already handed to a failed session is not replayed. Unread input remains behind the bounded stream backpressure and is consumed by the replacement session. In Push-to-Talk mode, provider loss invalidates the active turn: retained transcript and assistant output are discarded, the remaining chunks are consumed locally through that turn's audio EOS, and the next BOS starts a fresh turn.

Realtime mode treats ordinary BOS, MIME EOS, and route EOS as local stream boundaries. They do not call `EndASR`, inject silence, commit audio, or send `ClientInterrupt`. The only BOS-triggered session replacement is the local interruption handoff described above. Input EOF remains terminal for the transform: it stops reconnecting and closes the current session after draining the matching Chat/TTS response for a submitted finite Push-to-Talk or Text turn; it closes immediately when no response is pending and never triggers a rebuild. Provider `ASRInfo` performs the same local close-and-replace handoff when a response is pending; duplicate or stale events from the closed epoch cannot affect its replacement. Text mode never sends `EndASR` or `ClientInterrupt`. Push-to-Talk remains the only mode that uses those provider operations.

### doubaorealtime Push-to-Talk state machine

This section only describes `doubaorealtime.Transformer`'s adaptation to the Realtime Dialogue API's native Push-to-Talk mode. `doubaorealtimeduplex.Transformer` does not support Push-to-Talk and does not use this state machine.

```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> Capturing: BOS
    Capturing --> Capturing: audio chunk
    Capturing --> WaitingResponse: EOS / EndASR
    WaitingResponse --> Responding: assistant output starts
    WaitingResponse --> Capturing: next BOS
    Responding --> Idle: assistant output ends
    Responding --> Capturing: BOS / interrupt response
```

`doubaorealtime.Transformer`'s Push-to-Talk adaptation must explicitly track the current turn: the Idle state cannot receive audio or EOS; each turn in Capturing can only accept EOS once; after EOS, it cannot continue to send audio to the same turn. When the new BOS arrives, if the previous assistant is still outputting, `Interrupt` of the Realtime Dialogue session should be called, and then the input boundary for the new turn should be established.

Push-to-Talk retains the latest ASR hypothesis and all assistant output until both input audio EOS and provider `ASREnded` have occurred. It then publishes one final transcript plus transcript EOS before releasing assistant chunks in provider order. Retained assistant Opus is limited to two minutes of normalized packet duration; exceeding the limit discards the uncommitted turn, emits one assistant error EOS, and keeps the transformer available for later turns.

All `OpenSession`, `SendAudio`, `SendText`, `EndASR`, interrupt/cancel and function-call output operations must use the context received by `Transform`. Cancel Transform must be able to terminate provider I/O, event receiver and input reader, and cannot start `context.Background()` requests that are out of the calling life cycle.

## Public Realtime Pipeline

Realtime and Realtime Duplex can use different provider event adapters, but should share the following basic components:

- audio MIME normalization, PCM/MP3/Opus decode, Opus encode/transcode and frame preparation;
- per-stream audio input lifecycle;
- StreamID, segment and response ID management;
- assistant interruption epoch, BOS/EOS and growable output buffering;
- pending input, session restart, context cancellation and error shutdown.

Provider-specific event enum, session method and config conversion remain in their respective Adapters. Public media and stream lifecycle cannot be copied into two sets of realtime/duplex implementations.

## Change and regression constraints

Doubao Transformers handle provider session, concurrent event receiver, audio codec, StreamID and BOS/EOS at the same time. Any modification must first fix the behavior contract and then change the implementation.

### Bug fix process

1. First add a regression test that can stabilize failure at the minimum level to prove the input, status and error results of the bug.
2. If the problem exists in both Realtime and Duplex, first add the same test case to the public contract test; you cannot only repair one copy of the implementation.
3. Only modify the layer with this responsibility: provider event mapping, public media pipeline or GenX Stream lifecycle, and cannot be easily rewritten across layers.
4. Keep the mapping of provider event, GenX chunk, StreamID, role, label, BOS/EOS and error compatible; expected changes must update the contract document in the same change.
5. After fixing, run target tests, full package tests, and race tests, and do a new code review.

### Must-test behavior matrix

| Dimensions | Required boundaries |
| --- | --- |
| Input format | PCM, MP3, raw Opus; supported sample rates and channels; illegal MIME and corrupt frames. |
| Stream contract | BOS, data, EOS; duplicate/out-of-order marker; StreamID, role, label and terminal error. |
| Lifecycle | normal close, context cancel, provider EOF/error, blocked Send/Recv, session restart and repeated Close. |
| Realtime Dialogue | Push-to-Talk legal state transitions, single EndASR per turn, Realtime VAD, text mode and Interrupt. |
| Realtime Duplex | continuous input, transcription, text/audio response, function call output and CancelResponse. |
| Barge-in | pending response, text is being output, audio is being output; only one interrupted EOS is generated, and old epochs must not continue to be output. |
| Output buffering | Provider audio drains immediately into a growable buffer; a slow consumer must not backpressure the provider session. |

Realtime and Duplex's public media and Stream lifecycle must use the same set of table-driven contract tests. Provider-specific fake session only supplements the differences of respective events/session and cannot replicate the entire set of common tests.

### Required verification

```sh
go test ./pkgs/genx/transformers -count=1
go test -race ./pkgs/genx/transformers -count=1
go test ./pkgs/genx/... -count=1
```

Credential-protected integration tests must also be run when real provider contracts, SDK upgrades, or event schema changes are involved; unit test fakes cannot replace the real session's cancel, Close/Recv concurrency, and event ordering verification.
