# MiniMax Adapter

`minimaxtts` package 将 MiniMax 语音合成适配为 GenX Transformer。

```go
transformer, err := minimaxtts.New(minimaxtts.Config{
    Client:  client,
    Model:   "speech-2.6-turbo",
    VoiceID: "female-shaonv",
})
```

`Config` 保存不可变的 client、model、voice、speed、volume、pitch、emotion、format、sample rate 和 bitrate。`New` 要求显式的非空白 `Model`，并校验 client 与 voice，但不建立连接；它不会替换成 `speech-2.6-hd` 或其他 provider model。每次 `Transform` 独占 Stream lifecycle 和 provider request state，因此同一个已配置 Transformer 支持并发调用。

合成开始日志以结构化字段记录生效的 model 和 voice ID，不记录合成文本、credential、audio 或 provider 原始 payload。

GizClaw MiniMax Voice 资源必须提供 `provider_data.model`。Voice get/list 会保留该配置值；缺少它的已存储 Voice 仍可读取，但不能用于构建 Transformer。

MiniMax TTS 是非 agent 的 Stream-to-Stream Transformer，不提供 Toolkit 配置入口。
