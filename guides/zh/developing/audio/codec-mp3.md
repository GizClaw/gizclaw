# pkgs/audio/codec/mp3

提供 MP3 stream decode，以及在受支持平台上的 PCM-to-MP3 encode。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codec/mp3)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Decoder` / `NewDecoder` | 流式解码 MP3 为 PCM。 |
| `DecodeFull` | 一次性返回 PCM、sample rate 与 channels。 |
| `Encoder` / `NewEncoder` | 将 PCM 写入 MP3 stream。 |
| `WithQuality` / `WithBitrate` | 配置 encoder quality 或 bitrate。 |
| `EncodePCMStream` | 转换完整 PCM input stream。 |

Encoder availability 受 build target 与 native dependency 约束；不支持的平台返回明确错误，不能静默生成伪输出。
