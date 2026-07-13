# pkgs/audio/pcm

定义 GizClaw audio pipeline 使用的 PCM format、chunk、track、writer 和 mixer abstraction。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/pcm)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Format` | 表达 sample encoding、sample rate 与 channels。 |
| `Chunk` / `DataChunk` / `SilenceChunk` | 表达带格式的 audio data 或 silence。 |
| `Writer` / `WriteCloser` / `WriteFunc` | 定义 chunk output contract。 |
| `Track` / `TrackCtrl` | 管理单路 PCM input 与音量/控制状态。 |
| `Mixer` / `NewMixer` | 将多个 tracks 混合为统一 output format。 |
| `IOWriter` / `ChunkWriter` / `Copy` | 在 `io` stream 与 PCM chunks 之间适配。 |

PCM package 不负责 codec、设备选择或网络 transport；这些能力通过 codec、portaudio 和 Peer media 层组合。
