# Stream Processing

Stream Processing 保存不属于特定 provider 的 Transformer 组合能力。通用 `Mux` 负责按 pattern 选择 Adapter；选中的 Adapter 直接消费输入 `genx.Stream` 并返回输出 `genx.Stream`。

## 核心结构与主函数

| 结构或函数 | 作用 |
| --- | --- |
| [`TTSAudioNormalizer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#TTSAudioNormalizer) | 统一 TTS output stream 的 audio MIME type 与 chunk boundary。 |
| [`Mux`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Mux) | 按 pattern 选择一个 `genx.Transformer`，不建立第二套 ASR/TTS registry。 |
| `agentkit.Output` | 每个 Transform invocation 独占的 growable pull-output queue；支持显式 byte limit、丢弃未拉取 chunk 和 pull-visible observer。 |
| `agentkit.Response` | 跟踪一个 assistant response 的 StreamID、MIME route 和 terminal EOS。 |
| `agentkit.Invocation` | 组合 per-call context、output、active response、cancel 与 interrupt 生命周期。 |
| `runTTSTransform` | Package 内部的公共 TTS pipeline；消费 text Stream，按 StreamID 聚合和切分文本，调用 Adapter synthesize，并输出 audio Stream。 |

`ASR` 和 `TTS` 是能力类别，不需要额外导出 facade、session 或 segment 类型。所有 Adapter 统一注册到 Transformer registry，调用方使用 `genx.Stream` 的 BOS、data、EOS 和 StreamID 表达连续输入与分段。Provider connection/session 只作为 Adapter 内部实现存在。

## TTS Stream Processing

公共 TTS pipeline 消费 GenX text Stream，按 StreamID 分别维护 sentence segmenter。输入过程中可以将完整句子提前交给 Adapter 合成；收到该 StreamID 的 EOS 后，pipeline flush 剩余文本，并输出对应 audio EOS。

文本分段、audio normalization 和 debug wrapper 属于公共 pipeline。通用 StreamID、BOS、EOS 和 Stream close contract 定义在 [GenX 总览](../overview#streamid-与-eos)；ASR、Realtime 等 Adapter 如何映射 provider 事件，由各 Adapter 文档说明。

## AgentKit Stream 生命周期

一个 `Transform` invocation 独占 context、provider session、输入 reader、输出 queue 和 response 状态。同一个已配置 Transformer 可以并发启动多个 invocation，取消其中一个不会关闭其他 invocation。

每个 assistant response 使用新生成的非空 StreamID；同一 response 的 text/audio 共享该 ID，但分别使用各自 MIME EOS。正常完成、provider failure、cancel、interrupt 和 buffer overflow 都必须产生调用方可观察的终态。

`Output` 不依赖 downstream 及时调用 `Next()`，provider reader 可以持续把 chunk 放入 growable queue。`MaxBytes > 0` 时，超过限制会终止该 invocation 并从 `Next()` 返回 `agentkit.ErrOutputLimit`，不会静默丢弃或重排。observer 只在 chunk 已由 `Next()` 成功交给调用方后执行；生产或进入 queue 不代表调用方已经收到。

interrupt 只终止当前 assistant response：先删除该 response 尚未拉取的 buffered suffix，再为仍打开的每个 MIME route 输出 `EOS(error="interrupted")`，并拒绝该 response 的迟到 provider event。已经拉取的 prefix 保留为 delivered output；replacement response 使用新的 StreamID。cancel 则终止完整 invocation 并释放 provider 和 stream 资源。

AgentKit 不定义 Tool、Toolkit、ToolCall、ToolResult、HistoryStore、LogStore 或 MemoryStore，也不依赖 Workspace、Workflow、RPC、Peer 或设备类型。
