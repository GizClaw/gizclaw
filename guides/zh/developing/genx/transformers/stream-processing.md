# Stream Processing

Stream Processing 保存 provider-neutral 的 Transformer 组合与生命周期行为。`Mux` 按 pattern 选择 Adapter；选中的 Adapter 直接消费并返回 `genx.Stream`。

## Ownership

| Owner | 职责 |
| --- | --- |
| `transformers.Mux` | 选择一个 `genx.Transformer`，不建立能力类别专用 registry。 |
| `transformers/internal/streamkit` | Per-Transform output queue、pull observation、StreamID/MIME route 终态、interrupt、cancel 和共享 TTS segmentation。 |
| `transformers/audiostream.Normalizer` | 根据音频 MIME 对 byte stream 做可拼接处理，输入输出保持相同 codec 和 MIME；当前 MP3 handler 跨 chunk 去除 ID3v1 和 ID3v2 metadata。 |

StreamKit 是 `transformers` subtree 的内部实现，不提供 public construction surface，也不依赖 provider、agent、model、Tool、Workspace、Workflow、RPC 或设备类型。

## Stream lifecycle

每个 `Transform` invocation 独占 context、provider session、input reader、output queue 和 response state。同一个已配置 Transformer 可以并发执行多个调用；取消其中一个 invocation 不能关闭其他 invocation。

Output queue 不依赖 downstream 及时调用 `Next()`。显式 byte limit 超限时返回 `streamkit.ErrOutputLimit`；pull observer 只在 `Next()` 成功返回 chunk 后执行。Interrupt 只删除匹配 response 尚未拉取的 suffix，保留已经拉取的 prefix，为仍打开的每个 MIME route 发出一次 `EOS(error="interrupted")`，并拒绝迟到事件。Model response 使用 invocation-local 的新 StreamID，不复用已结束的 user transcript route；replacement response 也使用新的 StreamID。

StreamKit 不提供 model role 或 `assistant` label。Producer 负责提供 route metadata，StreamKit 只在生成 terminal chunk 时保留这些值。

## TTS Stream Processing

内部 TTS pipeline 按输入 StreamID 分别维护 sentence segmenter，可以在输入 EOS 前提前合成完整句子；EOS 到达后 flush 剩余文本，在同一逻辑 route 输出 audio EOS，并保留 role、name 与 label。没有 StreamID 的输入在 producer boundary 获得新的非空 ID。

Provider package 负责 SDK request 和 audio synthesis。公开的 `transformers/audiostream` 只处理 Transformer audio byte stream；调用方始终按实际 MIME 构造 `Normalizer`，无需预先判断具体格式。无需特殊处理或尚未支持的 MIME 原样透传；当前只有 MP3 会去除 ID3v1 和 ID3v2 metadata。Normalizer 不转换 codec、sample rate 或 MIME type。StreamKit 负责调用生命周期和 route 终态，不解析音频 container bytes。
