# pkgs/audio/codec/mp3

Provides MP3 stream decode, and PCM-to-MP3 encode on supported platforms.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codec/mp3)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Decoder` / `NewDecoder` | Streaming decoded MP3 to PCM. |
| `DecodeFull` | Returns PCM, sample rate and channels at once. |
| `Encoder` / `NewEncoder` | Write PCM to MP3 stream. |
| `WithQuality` / `WithBitrate` | Configure encoder quality or bitrate. |
| `EncodePCMStream` | Convert the complete PCM input stream. |

Encoder availability is subject to build target and native dependency; unsupported platforms return explicit errors and cannot silently generate spurious output.
