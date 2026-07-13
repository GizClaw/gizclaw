# pkgs/audio/codec/opus

提供 Opus encoder/decoder 和受支持 sample-rate contract，用于语音、WebRTC media 与 Ogg Opus conversion。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codec/opus)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Application` | 区分 VoIP、audio 与 low-delay encoder mode。 |
| `OpusSampleRate` | 表达 Opus 支持的 sample rates。 |
| `Encoder` / `NewEncoder` | 将 PCM frames 编码为 Opus packets。 |
| `Decoder` / `NewDecoder` | 将 Opus packets 解码为 PCM。 |
| `Version` | 返回 native Opus runtime version。 |
| `IsRuntimeSupported` | 判断当前 build/runtime 是否支持 codec。 |

该 package 不拥有 Ogg container、RTP timestamp 或 WebRTC track lifecycle。
