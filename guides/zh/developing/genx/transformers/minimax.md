# MiniMax Adapter

`minimaxtts` package 将 MiniMax 语音合成适配为 GenX Transformer。

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    Model:   "speech-2.6-turbo",
    VoiceID: "female-shaonv",
})
```

`Config` 保存不可变的 client、model、voice、speed、volume、pitch、emotion、format、sample rate、bitrate 和 `LanguageBoost`。`New` 要求显式的非空白 `Model`，并校验 client 与 voice，但不建立连接；它不会替换成 `speech-2.6-hd` 或其他 provider model。每次 `Transform` 独占 Stream lifecycle 和 provider request state，因此同一个已配置 Transformer 支持并发调用。

`LanguageBoost` 原样作为 MiniMax `language_boost` 发送，例如 `Japanese`、`French`；为空时省略该字段，由 MiniMax 自行判断文本语种。`minimaxtts.LanguageBoost(code)` 把 `ja`、`fr`、`es-MX` 这类 ISO 639-1 语种代码（兼容旧代码 `jp`）映射为 MiniMax 取值，未知或空代码返回空字符串。GizClaw 解析 MiniMax Voice 时，把 Voice pattern 的 `language` 参数（如 `voice/<alias>?language=ja`）经该映射写入 `LanguageBoost`；AST Translate 使用外部 Voice 时会把翻译目标语种作为该参数传入，`auto` 语种对不传。

合成开始日志以结构化字段记录生效的 model、voice ID 和 language_boost，不记录合成文本、credential、audio 或 provider 原始 payload。

GizClaw MiniMax Voice 资源必须提供 `provider_data.model`。Voice get/list 会保留该配置值；缺少它的已存储 Voice 仍可读取，但不能用于构建 Transformer。

MiniMax TTS 是非 agent 的 Stream-to-Stream Transformer，不提供 Toolkit 配置入口。
