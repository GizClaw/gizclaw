# MiniMax Adapter

The `minimaxtts` package adapts MiniMax speech synthesis to the GenX Transformer contract.

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    VoiceID: "female-shaonv",
})
```

`Config` stores the immutable client, model, voice, speed, volume, pitch, emotion, format, sample rate, and bitrate settings. `New` validates the client and voice without opening a connection. Each `Transform` call owns its Stream lifecycle and provider request state, so one configured Transformer supports concurrent calls.

MiniMax TTS is a non-agent Stream-to-Stream Transformer and has no Toolkit configuration surface.
