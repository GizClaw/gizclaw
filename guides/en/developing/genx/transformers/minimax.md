# MiniMax Adapter

MiniMax Adapter adapts MiniMax streaming speech API to GenX TTS Transformer via `MinimaxTTS`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`MinimaxTTS`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#MinimaxTTS) | Stores client, model, voice and audio generation parameters. |
| [`NewMinimaxTTS`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#NewMinimaxTTS) | Create a MiniMax TTS Transformer with the specified voice. |
| `MinimaxTTS.Transform` | Consume text Stream and convert provider streaming audio into output Stream. |

MiniMax-specific model, emotion, pitch, speed, volume and audio settings are expressed by Adapter options and do not enter the general `genx.Transformer` interface.
