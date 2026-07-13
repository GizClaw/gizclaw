# pkgs/audio/codecconv

连接 PCM、Ogg 与 Opus packages，提供 Ogg Opus encode/decode 和 packet/header conversion。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codecconv)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `PCMToOggOpusEncoder` / `NewPCMToOggOpusEncoder` | 增量编码 PCM 为 Ogg Opus。 |
| `OggToPCM` | 解码 Ogg Opus 到指定 sample rate 的 PCM。 |
| `OpusPacketsToOgg` / `OggOpusPackets` | 在 raw Opus packets 与 Ogg stream 之间转换。 |
| `OpusPacketRTPTicks` | 计算 packet duration 对应的 RTP ticks。 |
| `OpusHeadPacket` / `ParseOpusHeadPacket` | 构造或解析 OpusHead。 |
| `OpusTagsPacket` | 构造 OpusTags metadata packet。 |

该 package 只做格式转换，不决定媒体保存、网络发送或播放策略。
