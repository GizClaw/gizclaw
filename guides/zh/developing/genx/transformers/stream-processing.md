# Stream Processing

Stream Processing 保存不属于特定 provider 的 Transformer 组合能力。通用 `Mux` 负责按 pattern 选择 Adapter；选中的 Adapter 直接消费输入 `genx.Stream` 并返回输出 `genx.Stream`。

## 核心结构与主函数

| 结构或函数 | 作用 |
| --- | --- |
| [`TTSAudioNormalizer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#TTSAudioNormalizer) | 统一 TTS output stream 的 audio MIME type 与 chunk boundary。 |
| [`Mux`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Mux) | 按 pattern 选择一个 `genx.Transformer`，不建立第二套 ASR/TTS registry。 |
| `runTTSTransform` | Package 内部的公共 TTS pipeline；消费 text Stream，按 StreamID 聚合和切分文本，调用 Adapter synthesize，并输出 audio Stream。 |

`ASR` 和 `TTS` 是能力类别，不需要额外导出 facade、session 或 segment 类型。所有 Adapter 统一注册到 Transformer registry，调用方使用 `genx.Stream` 的 BOS、data、EOS 和 StreamID 表达连续输入与分段。Provider connection/session 只作为 Adapter 内部实现存在。

## TTS Stream Processing

公共 TTS pipeline 消费 GenX text Stream，按 StreamID 分别维护 sentence segmenter。输入过程中可以将完整句子提前交给 Adapter 合成；收到该 StreamID 的 EOS 后，pipeline flush 剩余文本，并输出对应 audio EOS。

文本分段、audio normalization 和 debug wrapper 属于公共 pipeline。通用 StreamID、BOS、EOS 和 Stream close contract 定义在 [GenX 总览](../overview#streamid-与-eos)；ASR、Realtime 等 Adapter 如何映射 provider 事件，由各 Adapter 文档说明。
