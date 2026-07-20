# Stream Processing

Stream Processing holds provider-neutral Transformer composition and lifecycle behavior. `Mux` selects an Adapter by pattern; the selected Adapter directly consumes and returns `genx.Stream` values.

## Ownership

| Owner | Responsibility |
| --- | --- |
| `transformers.Mux` | Select one `genx.Transformer` without creating capability-specific registries. |
| `transformers/internal/streamkit` | Per-Transform output queue, pull observation, StreamID/MIME-route completion, interruption, cancellation, and shared TTS segmentation. |
| `audio/codecconv.TTSAudioNormalizer` | Remove repeated provider container headers from streaming TTS audio. |

StreamKit is internal to the `transformers` subtree. It does not expose a public construction surface and does not depend on providers, agents, models, tools, Workspace, Workflow, RPC, or devices.

## Stream lifecycle

Each `Transform` invocation owns its context, provider session, input reader, output queue, and response state. A configured Transformer can serve concurrent calls; cancelling one invocation cannot close another.

The output queue grows independently of downstream `Next()` calls. A positive byte limit turns overflow into `streamkit.ErrOutputLimit`. Pull observers run only after `Next()` returns a chunk. Interrupt removes only the matching response's unpulled suffix, preserves its pulled prefix, emits one `EOS(error="interrupted")` for every still-open MIME route, and rejects late events. A replacement response receives a new StreamID.

StreamKit never supplies a model role or `assistant` label. Producers provide route metadata, and StreamKit preserves it on generated terminal chunks.

## TTS stream processing

The internal TTS pipeline maintains one sentence segmenter per input StreamID. It can synthesize complete sentences before input EOS, flushes remaining text at EOS, preserves role/name/label metadata, and emits audio EOS on the same logical route. Inputs without a StreamID receive a fresh non-empty ID at the producer boundary.

Provider packages own SDK requests and audio synthesis. Container normalization remains in `pkgs/audio/codecconv` so callers outside Go's `internal` boundary can reuse it.
