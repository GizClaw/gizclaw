# Stream Processing

Stream Processing 保存 provider-neutral 的 Transformer 组合与生命周期行为。`Mux` 按 pattern 选择 Adapter；选中的 Adapter 直接消费并返回 `genx.Stream`。

## Ownership

| Owner | 职责 |
| --- | --- |
| `transformers.Mux` | 选择一个 `genx.Transformer`，不建立能力类别专用 registry。 |
| `pkgs/genx/internal/streamkit` | Per-Transform output queue、pull observation、StreamID/MIME route 终态、interrupt、cancel，以及供具体 TTS Transformer 选择的原生文本流与客户端断句工具。 |
| `transformers/audiostream` | `Normalizer` 保持 codec/MIME 并处理可拼接 byte stream；`DurationMeter` 从完整音频帧测量新增可播时长，供共享 TTS pipeline 调度。 |

StreamKit 是 `genx` subtree 的内部实现，不提供 public construction surface，也不依赖 provider、agent、model、Tool、Workspace、Workflow、RPC 或设备类型。

## Stream lifecycle

每个 `Transform` invocation 独占 context、provider session、input reader、output queue 和 response state。同一个已配置 Transformer 可以并发执行多个调用；取消其中一个 invocation 不能关闭其他 invocation。

Output queue 不依赖 downstream 及时调用 `Next()`。显式 byte limit 超限时返回 `streamkit.ErrOutputLimit`；pull observer 只接受 `Next()` 返回的同一个 chunk 实例，不通过内容相等或 FIFO 猜测 delivery acknowledgement。相同 timestamp 的 realtime chunk 保持 producer insertion order；generic queue 不解析 terminal error 文本来重排生命周期。Interrupt 只删除匹配 response 尚未拉取的 suffix，保留已经拉取的 prefix，为仍打开的每个 MIME route 发出一次 `EOS(error="interrupted")`，并拒绝迟到事件。Model response 使用 invocation-local 的新 StreamID，不复用已结束的 user transcript route；replacement response 也使用新的 StreamID。

StreamKit 不提供 model role 或 `assistant` label。Producer 负责提供 route metadata，StreamKit 只在生成 terminal chunk 时保留这些值。

Producer 完成与 consumer 放弃 delivery 是不同的生命周期边界。正常 `Close`/`Fail` 保留缓冲及其终态；随后到达的取消或 error close 仍会释放未完成的 deferred observation 和 queued delivery callback，并阻止取消 terminal 再创建 acknowledgement。已经开始执行的 delivery observer 继续完成，`WaitForObservers` 等待这些 callback 返回，不把已交付 prefix 当作丢弃。Producer 完成后的 readable buffer 与原始终态保持不变。Invocation 的 parent cancellation watch 在 delivery 完成或被放弃后退出；正常完成后尚未确认的 delivery 仍受 parent 取消约束。

## TTS Stream Processing

具体 TTS Transformer 决定使用 provider 的原生文本流，还是调用客户端断句与攒批工具。Audio Dock 传递原始 text chunks、音色路由、中断和交付背压，不替 provider 选择切句策略。两条路径都按输入 StreamID 保留 role、name、label、MessageID，显式输出 audio MIME 的 BOS、data 与匹配 EOS；无 StreamID 的输入在 producer boundary 获得非空 ID。

### 原生文本流

MiniMax bidi 使用 `NewStreamingTTSStream`，增量清理控制标签、URL 并处理跨 chunk UTF-8，保留正文标点和换行，由服务端攒句。每条 route 独占 provider 会话，客户端不执行候选断句或音频预算攒批。消息结束先 flush 并等待旧音频交付，再切换 MessageID；输入结束则 finish 并读取剩余音频。Provider 的句结束与请求结束不会提前结束 GenX route。交付 callback 使用会话 context，取消 IO 与阻塞交付后才等待 worker 退出。

### 客户端断句与攒批

豆包 Seed V2、ICL V2，以及 MiniMax 的默认 HTTP/可选普通 WS 模式选择 `NewMeasuredTTSStream`，共用候选边界识别和音频预算调度。Provider Adapter 接收本次提交的完整文本 batch，负责 SDK 请求与音频规范化。断句和提交合成保持两个步骤。

断句使用 `github.com/clipperhouse/uax29/v2` 的 Unicode 17 句界、词界和字素边界。每个 chunk 都是未结束文本，只有消息结束或 EOS 才按 EOF 处理。它依次返回已确认的安全候选边界，暂存未确认的标点、单词和字素；候选 segment 不直接开启 provider 请求。中文逗号、分号、冒号和省略号等停顿位置补充为 phrase boundary，数字、IPv6、邮箱和连字符单词内部不会因此提前分段。英文缩写使用内嵌的 Unicode CLDR release 48 English suppressions、`Dr.` 补充和 Unicode 大写首字母规则；数据源与 Unicode-3.0 许可保存在 StreamKit 的 `data/` 中。

引号按当前引用上下文判断左右方向，覆盖 `“…”`、ASCII 双引号、德语 `„…“`、法语 `« … »` 以及日语引号。左引号保留给后面的正文，右引号附在前面的句末；逗号或冒号紧邻左引号时，引导语与引用保留在一起。因此 `“你好。”她说，“明天见。”` 的候选分块为 `“你好。”` 与 `她说，“明天见。”`。法语右引号允许隔着普通或不换行空格；引用上下文跨 chunk 和 segment 保留，在消息结束时清空。句末标点或右引号后已收到普通空白时，可确认停顿；缩写和仍待闭合的引用继续等待可见上下文。

文本清理在识别候选边界之前增量执行。暂存空白在后续可见字符到达时输出，消息末尾丢弃，避免跨 chunk 连接英文单词或数字。XML 标签的引号、嵌套控制块、URL 和 UTF-8 尾部均跨 chunk 保留状态。标签属性只保留有界前缀，控制块不缓存正文。直接包住 URL 的括号随 URL 删除，普通括号正文可以立即流式处理；括号内的说明文字保留，其中的 URL 单独删除。消息结束时丢弃未完成的 XML 控制内容和 URL，随后清空词法状态。

每条输入 StreamID 独立维护待提交 batch、音频预算与定时器。第一个可朗读的安全 segment 一确认就立即提交，默认没有最小字数门槛或额外启动等待。提供时长 meter 的 route 在首批可播音频返回前没有可花费的播放预算，使用空闲的第二个合成名额预取下一段，避免极短首段播完后还要等待下一次请求启动。两个名额都占用时，后续 segment 继续合并；音频返回后按实际预算攒批。请求长度上限、消息边界与 EOS 仍会要求及时提交。

后续攒批由前批已经生成的音频提供时间预算。Provider Adapter 为每个请求提供独立的音频时长 meter；共享 pipeline 累计已产生、可供有序输出的音频时长，扣除输出开始后已经流逝的时间，并预留下一批首包耗时与安全余量，在预算耗尽前提交当前 batch。首包预留采用当前 route 已观测到的最大首音频延迟与 `SynthesisLead` 中较大者，默认另留 50 ms `SafetyMargin`。预留的是流式首包延迟，后续批次不需要全部合成完才开始输出。

音频继续流式返回时，新增时长可以延后下一批的提交截止时间，让更多候选 segment 合在一个请求中；新文字本身不会创造额外预算。预算枯竭时立即提交已确认正文；模型暂时停止生成时，定时器仍会在最晚提交点触发，不会为了到期而强切尚未确认的数字、单词或右引号。音频耗尽后重新收到音频时，从新到达时间继续计入可播时长，不永久保留已经发生的停顿亏空。预算与首段状态属于整个 StreamID，`MessageEnd` 不重新冷启动；中断后的替代流使用新的预算。

音频时钟在有序输出开始时启动，早于可能阻塞的最终交付确认。消费者接收或播放大块音频时花费的时间也会消耗预算，交付确认不会把这块音频的时钟重新启动。该预算是基于已生成音频与服务端输出时序的估计，不是设备扬声器的实时播放回执。首包延迟突增、模型停顿或 provider 中途停止供给仍可能产生欠载。
默认单次 synthesis request 最多 1,000 个 rune，可合并多个候选 segment。候选 segment 最多 80 个 rune，配置更小的请求上限时同步缩小；达到上限先回退到已确认词界，单词超过预算时按完整字素切分。若单个异常组合字素本身超过预算，则仍按 rune 上限推进并保持 UTF-8。边界查找只扫描候选上限加 32 个字符的前缀，必要时补入已收到的一个空白作上下文。

调用方通过 GenX context 为单次 invocation 配置攒批，直接调用 TTS 或经 Audio Dock 组合都使用同一配置：

```go
batching := genx.DefaultTTSBatchConfig()
batching.MaxRunes = 1000
batching.FirstMinRunes = 0
batching.FirstWait = 0
batching.MaxWait = 0 // 不额外限制音频预算内的收集时间
batching.SynthesisLead = 500 * time.Millisecond // 下一批首包延迟的保守下限
batching.SafetyMargin = 50 * time.Millisecond
ctx, err := genx.WithTTSBatching(ctx, batching)
if err != nil {
    return err
}
output, err := transformer.Transform(ctx, input)
```

`FirstWait` 和 `FirstMinRunes` 仅用于显式调整首次请求的收集行为，默认均为零。闭合的完整首句立即提交，即使只有“好。”两个字，也不等待最小字数或额外启动时间。`MaxWait` 是可选的后续收集上限，零表示完全由音频预算决定；非零值限制最早待提交正文的等待时间，不改变音频预算优先到期的行为。`SynthesisLead` 和 `SafetyMargin` 都可调整；所有时长参数必须非负。`MaxRunes=0` 使用 1,000，其他值限制在 1 到 1,048,576。

可选 `WaitBudget` callback 可以用消费方反馈进一步缩短自动预算，返回值已经扣除合成延迟与余量；零或负值要求立即提交。它在输入、音频进度或定时事件时于锁外调用，必须及时返回并支持并发 route。支持的音频格式默认自动提供预算，不要求调用方设置此 callback。

`DurationMeter` 在规范化后的 PCM16LE、MP3 Layer III 与 Ogg/Opus bytes 上跨 chunk 统计完整帧时长；header 和未完成帧不增加预算。Ogg 使用 48 kHz granule 时钟，计入 pre-skip 与末帧裁剪，并用已解析 packet 的时长限制不可信 granule。原始 PCM 使用 provider 请求的 sample rate 与 channels。Meter 不解码或转码，不引入 native codec 依赖。其他格式或无法识别的音频不提供可花费的音频预算，合成名额释放后提交已攒正文，也可由 `WaitBudget` 提供预算。
达到请求长度上限、`MessageEnd`、EOS 或 speech identity 变化时，立即提交剩余文本；消息结束按 EOF 确认并提交可朗读尾部，然后清空状态。不同 StreamID、MessageID、role、name 或 label 的正文不会合并为一个请求。中断、取消和输入/provider failure 会释放定时器及在途合成，待提交文本丢弃。没有空文本的 synthesis 请求。

较大的文本 batch 只改变一次请求提交多少正文：provider 返回首块音频后立即下发，后续音频继续流式输出，不等待整批合成完成。只有拿到合成名额，batch 才从待提交缓冲中移出；等待名额的正文仍可继续合并。每条流最多两个并发 provider 请求，提前合成的音频使用每个请求 1 MiB 预算的队列，仍按提交顺序输出。音频产生后立即下发，不等待后续文本；这使后续请求的首音频延迟与前一批音频交付并行。

质量语料覆盖中、日、韩、多种欧洲语言、阿拉伯语、波斯语、希伯来语、乌尔都语、印地语、孟加拉语、马拉地语、泰米尔语、泰语、印尼语和斯瓦希里语。整段、逐 rune、逐 UTF-8 字节、每个 rune 切点的两段分块和固定种子混合分块验证文本守恒、受保护片段及长度；虚拟时间测试另行验证首音频、预算到期、攒批、引用、背压与状态隔离。

虚拟时钟回归验证两字首段立即启动、不同前批音频时长下的合并数量、流式音频增加预算、首包预留与安全余量、跨 route 隔离、交付背压消耗预算和停顿后的恢复。相同的文本生成速率下，1、3、6 秒前批音频分别允许合并 3、8、15 个后续 segment。另有 20 ms PCM 帧与 500 ms 预缓冲的播放场景，保留短首段、模型饥饿、provider 中途停顿等欠载反例。

Giztest 使用本地 MiniMax HTTP/SSE fixture、真实 Adapter、SDK 与 Ogg/Opus 编码记录音频包到达时间，通过既有播放缓冲检查验证自动预算及可选等待上限；测试没有设置外部 `WaitBudget`。检查音频时长、最大空档和欠载总时长，单独的 p95 包间隔不能排除分段空档。这些结果属于本地协议与模拟消费方回归，不代表真实云 provider 或物理设备验收。

机制处理有标点文本的 Unicode 边界与输出节奏，不提供无标点文本的语义分句或韵律预测。

连续讲述由 Audio Dock 通过 invocation context 启用 TTS 背压；emitter 在锁外等待每个音频 chunk 的最终交付，取消或丢弃会释放等待。`TTSMeta.MessageID` 来自该正文的消息 identity，随每个 synthesis job 保留，避免下一段输入覆盖在途音频的归属。收到 `MessageEnd` 时 flush 该段，发出对应音频消息边界；播放 channel 继续保持打开。

共享 emitter 默认在 synthesis failure 时终止。Provider 可用内部 `TTSSegmentError` 明确标记
已记录的单段失败：emitter 保留该段已经发出的音频，继续后续段，并在所有段完成后结束 route。
如果整条 route 没有产生音频，则以第一条单段失败结束，而不是发送成功的空 EOS。该标记
不用于 cancellation 或输出失败；这两种情况仍取消在途 segment。分段 metadata 的
`SegmentIndex` 在每个输入 route 内从 1 递增，只计算实际提交的可朗读段。Seed V2 的具体
日志和客户端终态见 [Doubao Speech Adapter](./doubao#seed-v2-分段失败)。既有单段预取、
音频队列上限和增量发出时机保持不变。

Interrupt 或 cancel 删除了 downstream 尚未 pull 的显式 TTS BOS 时，StreamKit 会把这个由 producer 声明的边界保留为空 BOS，再发送 error EOS；它不恢复已丢弃的音频 data，也不会替从未发送 BOS 的 producer 创建边界。

Provider package 负责 SDK request、audio synthesis 和请求级时长 meter 的构造。`audiostream.Normalizer` 使 byte stream 可拼接；当前 MP3 handling 跨 chunk 去除 ID3v1 与 ID3v2 metadata。`DurationMeter` 测量新增可播时长。两者都保持原始 codec、sample rate 与 MIME type。StreamKit 只消费时长观测并负责攒批、生命周期和 route 终态，不解析音频 container bytes。
