# pkgs/audio/codec/ogg

实现 Ogg container 的 page、packet 和 stream framing，不解释 packet 内部使用 Opus 或其他 codec。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codec/ogg)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `Page` / `ParsePage` / `ParsePages` | 解析 Ogg pages。 |
| `Packet` / `ExtractPackets` | 从 pages 重组 packets。 |
| `BuildPacketPages` / `MarshalPages` | 将 packet 分页并编码。 |
| `StreamReader` / `NewStreamReader` | 增量读取 Ogg stream。 |
| `StreamWriter` / `NewStreamWriter` | 管理 serial、sequence 和 page output。 |
| `Packets` | 以 iterator 读取 packets。 |

Ogg package 拥有 container framing、checksum 和 page sequencing；Opus header 和 PCM conversion 属于 `codecconv` 与 `codec/opus`。
