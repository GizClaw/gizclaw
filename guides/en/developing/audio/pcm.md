# pkgs/audio/pcm

Defines the PCM format, chunk, track, writer and mixer abstraction used by the GizClaw audio pipeline.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/pcm)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Format` | Express sample encoding, sample rate and channels. |
| `Chunk` / `DataChunk` / `SilenceChunk` | Express formatted audio data or silence. |
| `Writer` / `WriteCloser` / `WriteFunc` | Define chunk output contract. |
| `Track` / `TrackCtrl` | Manage single-channel PCM input and volume/control status. |
| `Mixer` / `NewMixer` | Mix multiple tracks into a unified output format. |
| `IOWriter` / `ChunkWriter` / `Copy` | Adapt between `io` stream and PCM chunks. |

The PCM package is not responsible for codec, device selection, or network transport; these capabilities are combined through the codec, portaudio, and peer media layers.
