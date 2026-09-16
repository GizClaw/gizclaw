# AgentKit

`pkgs/genx/agentkit` contains reusable Agent stream composition. It depends only on GenX interfaces and does not read Workspace, Workflow, RuntimeProfile, or provider credentials.

## Audio Dock

[`audiodock`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock) composes a text `genx.Transformer` with optional ASR and TTS into another `genx.Transformer`:

```go
dock, err := audiodock.New(audiodock.Config{
    Agent: textAgent,
    ASR:   asrTransformer,
    TTS:   ttsMux,
    ResolveVoice: func(ctx context.Context, request audiodock.VoiceRequest) (string, error) {
        return voicePattern(request.Name), nil
    },
})
```

Text input enters the Agent directly. Audio input is streamed incrementally to ASR with its original StreamID; the completed transcript becomes one text turn for the Agent. Agent text is immediately pullable and is also queued for TTS in its original order. Synthesized audio and text share the response StreamID, with independent EOS markers for each MIME channel.

Text content is independent of audio timing: voice resolution, TTS session startup, and synthesis do not block subsequent model text. A text EOS carrying content is split into immediately readable content and a deferred empty text EOS; the response epoch ends only after sibling routes finish or are cancelled. Each publisher queues TTS input in order. A new input BOS cancels old TTS work even during voice resolution or startup, and a late provider stream is closed without emitting old audio into the next turn. This boundary also applies to realtime/duplex input: the ASR definite transcript determines when the model starts, and Audio Dock adds no wait for the outer audio EOS or playback cadence.

Every child Transformer remains responsible for the StreamID or MIME channels it creates. Audio Dock preserves unrelated pass-through routes and validates each child TTS lifecycle by the child's original `(StreamID, canonical MIME)` key. Data before BOS, duplicate BOS, output after EOS, a missing EOS, or a child stream with no MIME lifecycle is a route error. When multiple publishers synthesize the same final MIME channel, Audio Dock merges their validated child boundaries into one final BOS and one final EOS; it owns only that remapped final route and does not repair an invalid child lifecycle.

`ResolveVoice` receives the response StreamID, output node/name, and chunk metadata, and returns the pattern passed to the TTS mux. It is resolved independently for each named publisher inside a response, so parallel Flowcraft publishers may use different voices while sharing the response StreamID. An empty pattern keeps that publisher's text without synthesizing it. RuntimeProfile alias resolution belongs to the product factory, not Audio Dock.

One Dock supports concurrent `Transform` calls. ASR sessions, Agent runs, voices, TTS sessions, buffers, cancellation, and errors are scoped to one call and StreamID; one failing route does not terminate other calls. Output uses a growable internal queue, so producers do not depend on consumers pulling promptly before provider streams can be drained.

Closing output cancels the corresponding ASR, Agent, and TTS work. An interrupted route drops its unpulled suffix and emits error-bearing EOS markers for announced MIME channels before the next input transcript becomes visible. If TTS is pending but has not announced an audio MIME channel yet, Audio Dock emits a response-level interrupted EOS without fabricating an audio MIME lifecycle. After Agent text EOS, TTS completion is bounded to one minute. Audio Dock does not execute ToolCall or own provider protocols.

`Config.SpeakerVoices` maps speaker names to TTS mux patterns. A response-local incremental parser removes configured `【name】` markers. At most two provider sessions start concurrently for the current and next segment; later segments wait for earlier completion. Output remains ordered and shares the final MIME lifecycle. Every segment must therefore produce the same audio MIME type: the first segment's audio MIME type is fixed for the response, and a later segment with a different one ends the response's audio and text with an error rather than opening a second audio MIME channel. Callers select voices whose TTS patterns share one output format. Without this mapping, `ResolveVoice` continues to select each publisher independently.
