# Stream Processing

Stream Processing holds provider-neutral Transformer composition and lifecycle behavior. `Mux` selects an Adapter by pattern; the selected Adapter directly consumes and returns `genx.Stream` values.

## Ownership

| Owner | Responsibility |
| --- | --- |
| `transformers.Mux` | Select one `genx.Transformer` without creating capability-specific registries. |
| `pkgs/genx/internal/streamkit` | Per-Transform output queue, pull observation, route completion, interruption, cancellation, and native text streaming or client segmentation selected by each TTS adapter. |
| `transformers/audiostream` | `Normalizer` makes container bytes concatenable without changing codec/MIME; `DurationMeter` measures complete audio frames for shared TTS scheduling. |

StreamKit is internal to the `genx` subtree. It does not expose a public construction surface and does not depend on providers, agents, models, tools, Workspace, Workflow, RPC, or devices.

## Stream lifecycle

Each `Transform` invocation owns its context, provider session, input reader, output queue, and response state. A configured Transformer can serve concurrent calls; cancelling one invocation cannot close another.

The output queue grows independently of downstream `Next()` calls. A positive byte limit turns overflow into `streamkit.ErrOutputLimit`. Pull observation accepts only the exact chunk instance returned by `Next()`; it does not infer delivery acknowledgement from equal content or FIFO position. Realtime chunks with the same timestamp retain producer insertion order; the generic queue does not parse terminal error text to reorder lifecycle events. Interrupt removes only the matching response's unpulled suffix, preserves its pulled prefix, emits one `EOS(error="interrupted")` for every still-open MIME route, and rejects late events. Model responses receive fresh invocation-local StreamIDs instead of reusing completed user transcript routes; replacement responses also receive new StreamIDs.

StreamKit never supplies a model role or `assistant` label. Producers provide route metadata, and StreamKit preserves it on generated terminal chunks.

Producer completion and consumer abandonment are separate lifecycle boundaries. Normal `Close`/`Fail` retains the buffer and terminal result; later cancellation or error close still releases unfinished deferred observations and queued delivery callbacks, and cancellation terminals cannot acquire new acknowledgements. Delivery observers that have already started continue to completion. `WaitForObservers` waits for those callbacks, preserving the delivered prefix. A completed producer's readable buffer and original terminal result remain intact. The invocation's parent cancellation watch exits once delivery has completed or been abandoned; pending delivery remains cancellable after producer completion.

## TTS stream processing

Each TTS Transformer chooses native provider text streaming or shared client segmentation and batching. Audio Dock forwards original text chunks, voice routing, interruptions, and delivery backpressure. Both paths preserve input StreamID, role, name, label, and MessageID and emit explicit audio BOS, data, and matching EOS. Inputs without StreamID receive a non-empty ID at the producer boundary.

### Native text streaming

Explicit MiniMax bidi uses `NewStreamingTTSStream`. It incrementally removes control markup and URLs, tracks partial UTF-8, and preserves punctuation and newlines while the provider buffers sentences. Each route owns one provider session. A message flush waits for earlier audio delivery before switching MessageID; input completion finishes and drains the session. Sentence/request completion events do not prematurely close the GenX route. Delivery callbacks use the session context so cancellation releases IO and blocked delivery before workers are joined.

### Client segmentation and batching

Doubao Seed V2 and ICL V2, MiniMax's default HTTP transport, and optional ordinary WS use `NewMeasuredTTSStream`. Boundary detection and synthesis submission are separate steps: the adapter receives one complete text batch and owns the SDK request and audio normalization.

Candidate boundaries use Unicode 17 sentence, word, and grapheme rules from `github.com/clipperhouse/uax29/v2`. A chunk is unfinished input; only message completion or EOS is EOF. Unconfirmed punctuation, words, and graphemes remain buffered. Chinese commas, semicolons, colons, and ellipses add phrase boundaries without splitting numbers, IPv6, email, or hyphenated words. English abbreviations use embedded Unicode CLDR release 48 suppressions, an additional `Dr.`, and Unicode uppercase initials. The source and Unicode-3.0 license are stored together in StreamKit's `data/` directory.

Quote direction follows context for Chinese/English curly quotes, ASCII double quotes, German `„…“`, French `« … »`, and Japanese quotation marks. Opening quotes remain with following text and closing quotes with preceding punctuation. A comma or colon introducing a quotation stays with it: `“你好。”她说，“明天见。”` yields candidates `“你好。”` and `她说，“明天见。”`. French closing quotes may follow ordinary or non-breaking space. Quote state persists across chunks and candidates and resets at message completion. Observed whitespace can confirm terminal punctuation, while abbreviations and unresolved closing quotes retain lookahead.

Lexical cleanup precedes boundary detection. It holds whitespace until visible text arrives and drops trailing message whitespace, preserving spaces between words and numeric groups. XML attributes, nested control blocks, URLs, and partial UTF-8 retain state across chunks. Tag prefixes are bounded and control bodies are discarded as they arrive. Parentheses enclosing only a URL are removed with it; ordinary parenthesized text streams normally. Message completion discards unfinished control markup and URLs and resets lexical state.

Each StreamID owns its pending batch, audio budget, and timer. The first confirmed readable segment is submitted immediately by default, without a minimum rune count or startup delay. Before measurable audio arrives, an idle second synthesis slot can prefetch the next confirmed phrase. When both slots are occupied, later candidates remain mergeable. Request limits, message boundaries, and EOS still force submission.

Later collection spends only audio already generated. Each request has a separate duration meter. The pipeline accumulates available ordered audio, subtracts elapsed time after output starts, and reserves the larger of observed first-audio latency and `SynthesisLead`, plus `SafetyMargin` (50 ms by default). It reserves streaming first-audio latency rather than waiting for the entire next synthesis to finish.

New streamed audio can extend the deadline; new text cannot. When the budget expires, confirmed text submits promptly. A timer still fires if model text pauses and does not force a cut inside an unresolved number, word, or closing quote. After a stall, newly arriving audio restarts available time at arrival instead of retaining an old deficit. `MessageEnd` retains the route's budget and first-request state; interrupted replacement streams start fresh.

The audio clock starts at ordered output before final delivery acknowledgement. Time spent receiving or playing a large chunk consumes budget, and acknowledgement does not restart that chunk's clock. This is an estimate from generated media and server delivery, not device playback feedback; model starvation, provider stalls, or sudden latency increases can still cause underruns.

Requests default to at most 1,000 runes and can combine many candidates. Candidates have an 80-rune limit, reduced when the request limit is smaller. A hard limit first backs up to a confirmed word, then a complete grapheme; a pathological grapheme exceeding the limit advances by complete UTF-8 code points. Boundary search uses a bounded prefix with 32 runes of lookahead and, when available, one withheld whitespace rune.

Configure one invocation through GenX context, whether calling TTS directly or through Audio Dock:

```go
batching := genx.DefaultTTSBatchConfig()
batching.MaxRunes = 1000
batching.FirstMinRunes = 0
batching.FirstWait = 0
batching.MaxWait = 0 // No extra cap on collection within the audio budget.
batching.SynthesisLead = 500 * time.Millisecond
batching.SafetyMargin = 50 * time.Millisecond
ctx, err := genx.WithTTSBatching(ctx, batching)
if err != nil {
    return err
}
output, err := transformer.Transform(ctx, input)
```

`FirstWait` and `FirstMinRunes` adjust only startup collection and default to zero. A complete closed first sentence submits immediately, even a two-rune sentence such as `好。`, without waiting for the minimum size or startup delay. `MaxWait` optionally caps later collection; zero leaves it to the audio budget. All durations must be non-negative. `MaxRunes=0` selects 1,000; explicit limits range from 1 to 1,048,576. Optional `WaitBudget` consumer feedback can shorten the automatic budget and must already account for synthesis latency and margin. Zero or negative values make submission urgent. It runs outside locks and must return promptly and support concurrent routes. Supported formats provide automatic budgets without this callback.

`DurationMeter` measures complete PCM16LE, MP3 Layer III, and Ogg/Opus frames across chunks. Headers and incomplete frames add no budget. Ogg uses the 48 kHz granule clock, pre-skip, and final trimming and bounds untrusted granules by parsed packet duration. Raw PCM uses the request's rate and channel count. The meter neither decodes nor transcodes and adds no native codec dependency. Unsupported formats fall back to collecting until a synthesis slot is released; `WaitBudget` can also provide their budget.

A request limit, `MessageEnd`, EOS, or speech identity change submits remaining readable text. Text from different StreamIDs, MessageIDs, roles, names, or labels is never merged into one request. Interrupt, cancellation, and input/provider failure discard pending text and release timers and in-flight work. Empty text never starts synthesis.

Bigger batches change the submitted text size while audio remains incremental. Text leaves the pending batch only after a synthesis slot is reserved. Each route has at most two concurrent requests; prefetched audio uses a queue with a 1 MiB budget per request and is emitted in submission order without waiting for later text.

Regression corpora cover Chinese, Japanese, Korean, European languages, Arabic, Persian, Hebrew, Urdu, Hindi, Bengali, Marathi, Tamil, Thai, Indonesian, and Swahili. Whole-text, rune, byte, every rune split point, and seeded mixed chunks check text conservation, protected spans, and bounds. Virtual-clock tests cover immediate two-rune startup, audio-budget extension, first-audio reserve, margin, quote handling, route isolation, delivery backpressure, and recovery. They also retain underrun examples for short initial audio, model starvation, and provider stalls.

Giztest uses a local MiniMax HTTP/SSE fixture, the actual adapter/SDK, and Ogg/Opus encoding to check packet duration, maximum playback gaps, and total underrun time with automatic budgets and optional caps. These are local protocol and simulated-consumer tests. The mechanism does not infer semantic sentences or prosody from unpunctuated text, and the checks do not establish cloud-provider or physical-device acceptance.

Continuous narration enables TTS delivery backpressure through invocation context. The emitter waits outside locks for final audio delivery; cancellation or discard releases the wait. Each job retains its `TTSMeta.MessageID`. `MessageEnd` flushes the generation and publishes its audio boundary while the playback channel stays open.

Synthesis failure normally ends the shared emitter. A provider can return internal `TTSSegmentError` for an already logged failure confined to one submitted batch. Delivered audio is retained and later batches continue. If the whole route emits no audio, it ends with the first such failure. Cancellation and output failures remain terminal. `SegmentIndex` counts actual readable synthesis requests from one within each route. See [Seed V2 failures](./doubao#seed-v2-segment-failures) for provider logs and client completion.

If interruption or cancellation discards an explicit TTS BOS before downstream pulls it, StreamKit retains that producer declaration as an empty BOS followed by error EOS. It restores no discarded audio and creates no undeclared BOS. Provider packages own requests, synthesis, and meter construction. `audiostream.Normalizer` preserves codec, sample rate, and MIME while removing MP3 ID3 metadata across chunks; StreamKit consumes duration observations without parsing container bytes.
