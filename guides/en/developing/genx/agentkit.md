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

Text input enters the Agent directly. Audio input is streamed incrementally to ASR with its original StreamID; the completed transcript becomes one text turn for the Agent. Agent text is immediately pullable while delivered text is copied to TTS. Synthesized audio and text share the response StreamID, with independent EOS markers for each MIME channel.

`ResolveVoice` receives the response StreamID, output node/name, and chunk metadata, and returns the pattern passed to the TTS mux. It is resolved independently for each named publisher inside a response, so parallel Flowcraft publishers may use different voices while sharing the response StreamID. An empty pattern keeps that publisher's text without synthesizing it. RuntimeProfile alias resolution belongs to the product factory, not Audio Dock.

One Dock supports concurrent `Transform` calls. ASR sessions, Agent runs, voices, TTS sessions, buffers, cancellation, and errors are scoped to one call and StreamID; one failing route does not terminate other calls. Output uses a growable internal queue, so producers do not depend on consumers pulling promptly before provider streams can be drained.

Closing output cancels the corresponding ASR, Agent, and TTS work. An interrupted route drops its unpulled suffix and emits error-bearing EOS markers for announced MIME channels. Audio Dock does not execute ToolCall or own provider protocols.
