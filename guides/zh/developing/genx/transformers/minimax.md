# MiniMax Adapter

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts)

`minimaxtts` package 将 MiniMax 语音合成适配为 GenX Transformer。

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    Model:   "speech-2.6-turbo",
    VoiceID: "female-shaonv",
})
```

`Config` 保存不可变的 client、model、voice、speed、volume、pitch、emotion、format、sample rate、bitrate 和 `LanguageBoost`。`New` 要求显式的非空白 `Model`，并校验 client 与 voice，但不建立连接；它不会替换成 `speech-2.6-hd` 或其他 provider model。每次 `Transform` 独占 Stream lifecycle 和 provider request state，因此同一个已配置 Transformer 支持并发调用。

`Config.Transport` 默认使用 `minimaxtts.TransportHTTP`，通过 `/v1/t2a_v2` 接收 HTTP/SSE 增量音频，复用 HTTP transport 连接池。默认路径使用共享客户端断句与音频预算攒批：首个 segment 尽快提交，后续根据前一批音频可播放时长尽量合并完整 segment，参数见[共享 TTS 工具](./stream-processing#客户端断句与攒批)。

显式设置 `minimaxtts.TransportBidi` 时，通过 `/ws/v1/t2a_v2_bidi` 发送增量文本，由 MiniMax 服务端攒句。Transformer 选择原生文本流路径；共享层维护输入清理、UTF-8 状态、route、消息边界、背压和终态，不创建客户端 sentence segmenter 或 synthesis batch。换行保留为句界，空白 chunk 与相邻正文一起发送，避免服务端丢弃纯空白事件。单条上行超过 10,000 字符时仅按 wire 长度拆包，不按句切分。

同一输入 route 在 `MessageEnd` 时发送 `task_flush`，收到 `task_flushed` 且此前音频已交付后才发布消息边界；后续消息继续使用该会话。MessageID/role/name/label 改变前也先 flush，避免旧音频归到下一条消息。只有文本 EOS 或正常输入 EOF 才发送 `task_finish`，继续读取残留音频和 `task_finished`。`is_final`、`sentence_end` 不结束 GenX route。每次 Transform 的会话独立，取消和失败先释放 IO 与交付等待，再等待 worker 退出；会话每 40 秒发送 ping，避免静默期间被 provider 断开。

输出仍由相同的 PCM-to-Ogg 编码实现处理。一次消息内的服务端句子共用 Ogg 逻辑流，flush 后封尾，下一条消息使用新的 serial。每次 provider 结束帧报告的 `usage_characters` 按实际值记录，不根据输入估计。

`minimaxtts.TransportWebSocket` 显式选择普通 WS，也使用客户端断句与音频预算攒批。`genx.WithTTSBatching` 控制 HTTP/普通 WS 两种模式，不改变 bidi 的服务端切句策略；未知 Transport 在构造时拒绝。GizClaw builder 将配置的 HTTP client 交给 SDK，WS upgrade 同样使用该 client 的 transport 和代理。

HTTP 请求启用 `stream_options.exclude_aggregated_audio`，只下发增量音频；SDK 将终帧整段副本与增量分开。终帧后的 `Close` 最多等待 100ms、读取 64 KiB 的 HTTP 尾部，使正常 chunked 响应回到连接池；中断直接关闭。在确认终帧前遇到 EOF 会报错，已输出 prefix 不重试或重放。默认流式 HTTP 时长由 context 管理，普通 JSON 请求有 30 秒超时；显式 `HTTPClient.Timeout` 继续生效。

启用 `GIZCLAW_TTS_DEBUG` 后，首输出记录 `minimax tts: first audio timing`：连接复用、DNS/TCP/TLS、HTTP 响应头、请求发出至首块 provider 音频，以及本地编码/入队时间。HTTP/普通 WS 按合成请求记录，bidi 按会话记录。`-1` 表示没有观测到对应事件；WS 的请求至首音频还包含 task 启动，bidi 还可能包含服务端攒句等待。编码输出可能是头部或静音，不能当作设备首字时间。日志不包含正文、认证头或音频。

`LanguageBoost` 原样作为 MiniMax `language_boost` 发送，例如 `Japanese`、`French`；为空时省略该字段，由 MiniMax 自行判断文本语种。`minimaxtts.LanguageBoost(code)` 把 `ja`、`fr`、`es-MX` 这类 ISO 639-1 语种代码（兼容旧代码 `jp`）映射为 MiniMax 取值，未知或空代码返回空字符串。GizClaw 解析 MiniMax Voice 时，把 Voice pattern 的 `language` 参数（如 `voice/<alias>?language=ja`）经该映射写入 `LanguageBoost`；AST Translate 使用外部 Voice 时会把翻译目标语种作为该参数传入，`auto` 语种对不传。

合成开始日志以结构化字段记录生效的 model、voice ID 和 language_boost，不记录合成文本、credential、audio 或 provider 原始 payload。

GizClaw MiniMax Voice 资源必须提供 `provider_data.model`。Voice get/list 会保留该配置值；缺少它的已存储 Voice 仍可读取，但不能用于构建 Transformer。

MiniMax TTS 是非 agent 的 Stream-to-Stream Transformer，不提供 Toolkit 配置入口。
