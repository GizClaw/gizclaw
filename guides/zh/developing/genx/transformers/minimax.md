# MiniMax Adapter

MiniMax Adapter 通过 `MinimaxTTS` 将 MiniMax streaming speech API 适配为 GenX TTS Transformer。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`MinimaxTTS`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#MinimaxTTS) | 保存 client、model、voice 和 audio generation parameters。 |
| [`NewMinimaxTTS`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#NewMinimaxTTS) | 创建指定 voice 的 MiniMax TTS Transformer。 |
| `MinimaxTTS.Transform` | 消费 text Stream，并把 provider streaming audio 转换为输出 Stream。 |

MiniMax-specific model、emotion、pitch、speed、volume 与 audio settings 由 Adapter options 表达，不进入通用 `genx.Transformer` interface。
